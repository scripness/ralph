package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Provider signal markers
const (
	DoneMarker     = "<ralph>DONE</ralph>"
	VerifiedMarker = "<ralph>VERIFIED</ralph>"
)

var (
	LearningPattern = regexp.MustCompile(`<ralph>LEARNING:(.+?)</ralph>`)
	ResetPattern    = regexp.MustCompile(`<ralph>RESET:(.+?)</ralph>`)
	ReasonPattern   = regexp.MustCompile(`<ralph>REASON:(.+?)</ralph>`)
)

// ProviderResult contains the result of a provider iteration
type ProviderResult struct {
	Output    string
	Done      bool
	Learnings []string
	Resets    []string
	Reason    string
	Verified  bool
	ExitCode  int
	TimedOut  bool
}

// runLoop runs the main implementation loop for a feature
func runLoop(cfg *ResolvedConfig, featureDir *FeatureDir) error {
	prdPath := featureDir.PrdJsonPath()
	git := NewGitOps(cfg.ProjectRoot)

	// Load PRD first
	prd, err := LoadPRD(prdPath)
	if err != nil {
		return err
	}

	// Acquire lock
	lock := NewLockFile(cfg.ProjectRoot)
	if err := lock.Acquire(featureDir.Feature, prd.BranchName); err != nil {
		return err
	}
	defer lock.Release()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n\nInterrupted. Releasing lock and exiting...")
		lock.Release()
		os.Exit(130)
	}()

	// Ensure we're on the correct branch
	fmt.Printf("Ensuring branch: %s\n", prd.BranchName)
	if err := git.EnsureBranch(prd.BranchName); err != nil {
		return fmt.Errorf("failed to switch to branch %s: %w", prd.BranchName, err)
	}

	fmt.Println(strings.Repeat("=", 60))
	fmt.Println(" Ralph - Autonomous Agent Loop")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf(" Feature: %s\n", featureDir.Feature)
	fmt.Printf(" Branch: %s\n", prd.BranchName)
	fmt.Printf(" PRD: %s\n", prdPath)
	fmt.Printf(" Project root: %s\n", cfg.ProjectRoot)
	fmt.Println(strings.Repeat("=", 60))

	// Check for crash recovery - if currentStoryId is set, we were interrupted
	if prd.Run.CurrentStoryID != nil {
		fmt.Printf("\n⚠ Resuming from interrupted story: %s\n", *prd.Run.CurrentStoryID)
	}

	// Initialize service manager
	svcMgr := NewServiceManager(cfg.ProjectRoot, cfg.Config.Services)
	defer svcMgr.StopAll()

	// Start services if configured
	if svcMgr.HasServices() {
		fmt.Println("\nStarting services...")
		if err := svcMgr.EnsureRunning(); err != nil {
			return fmt.Errorf("failed to start services: %w", err)
		}
	}

	iteration := 0
	for {
		iteration++

		// Reload PRD at start of each iteration
		prd, err = LoadPRD(prdPath)
		if err != nil {
			return err
		}

		// Check if all stories complete
		if AllStoriesComplete(prd) {
			fmt.Println()
			fmt.Println(strings.Repeat("=", 60))
			fmt.Println(" All stories complete! Running final verification...")
			fmt.Println(strings.Repeat("=", 60))

			// Run final verification
			verified, err := runFinalVerification(cfg, featureDir, prd)
			if err != nil {
				return err
			}

			if verified {
				fmt.Println()
				fmt.Println(strings.Repeat("=", 60))
				fmt.Println(" ✓ Ralph completed and verified!")
				fmt.Println(strings.Repeat("=", 60))
				fmt.Println()
				fmt.Println("Ready to merge. Review changes with:")
				fmt.Printf("  git log --oneline %s..HEAD\n", prd.BranchName)
				return nil
			}

			// Stories were reset, continue loop
			fmt.Println("\nStories reset. Continuing implementation loop...")
			continue
		}

		// Check if all remaining stories are blocked
		if HasBlockedStories(prd) && GetNextStory(prd) == nil {
			fmt.Println()
			fmt.Println(strings.Repeat("=", 60))
			fmt.Println(" ⚠ All remaining stories are blocked")
			fmt.Println(strings.Repeat("=", 60))
			fmt.Println()
			fmt.Println("Blocked stories:")
			for _, s := range GetBlockedStories(prd) {
				fmt.Printf("  - %s: %s\n", s.ID, s.Title)
				if s.Notes != "" {
					fmt.Printf("    └─ %s\n", s.Notes)
				}
			}
			fmt.Println()
			fmt.Println("Manual intervention required.")
			return fmt.Errorf("all remaining stories blocked")
		}

		// Get next story
		story := GetNextStory(prd)
		if story == nil {
			// This shouldn't happen given checks above
			return fmt.Errorf("unexpected: no next story but not all complete")
		}

		fmt.Println()
		fmt.Println(strings.Repeat("=", 60))
		fmt.Printf(" Iteration %d: %s - %s\n", iteration, story.ID, story.Title)
		fmt.Println(strings.Repeat("=", 60))

		// Set current story
		prd.SetCurrentStory(story.ID)
		if err := SavePRD(prdPath, prd); err != nil {
			return fmt.Errorf("failed to save PRD: %w", err)
		}

		// Commit PRD state change
		if cfg.Config.Commits.PrdChanges {
			if err := commitPrdOnly(cfg.ProjectRoot, prdPath, fmt.Sprintf("ralph: start %s", story.ID)); err != nil {
				fmt.Printf("Warning: failed to commit PRD: %v\n", err)
			}
		}

		// Generate and send prompt
		prompt := generateRunPrompt(cfg, featureDir, prd, story)
		result, err := runProvider(cfg, prompt)
		if err != nil {
			return fmt.Errorf("provider error: %w", err)
		}

		// Process learnings
		for _, learning := range result.Learnings {
			prd.AddLearning(learning)
		}

		// Check for DONE marker
		if !result.Done {
			fmt.Println("\nProvider did not signal completion. Retrying...")
			prd.MarkStoryFailed(story.ID, "Provider did not signal completion", cfg.Config.MaxRetries)
			prd.ClearCurrentStory()
			if err := SavePRD(prdPath, prd); err != nil {
				return fmt.Errorf("failed to save PRD: %w", err)
			}
			continue
		}

		// Run verification
		fmt.Println("\nRunning verification...")
		verifyResult, err := runStoryVerification(cfg, featureDir, story, svcMgr)
		if err != nil {
			return fmt.Errorf("verification error: %w", err)
		}

		if !verifyResult.passed {
			fmt.Printf("\nVerification failed: %s\n", verifyResult.reason)
			prd.MarkStoryFailed(story.ID, verifyResult.reason, cfg.Config.MaxRetries)
			prd.ClearCurrentStory()
			if err := SavePRD(prdPath, prd); err != nil {
				return fmt.Errorf("failed to save PRD: %w", err)
			}

			if cfg.Config.Commits.PrdChanges {
				commitPrdOnly(cfg.ProjectRoot, prdPath, fmt.Sprintf("ralph: %s failed verification", story.ID))
			}
			continue
		}

		// Story passed!
		commit := getLastCommit(cfg.ProjectRoot)
		summary := getCommitMessage(cfg.ProjectRoot, commit)
		prd.MarkStoryPassed(story.ID, commit, summary)
		prd.ClearCurrentStory()

		if err := SavePRD(prdPath, prd); err != nil {
			return fmt.Errorf("failed to save PRD: %w", err)
		}

		if cfg.Config.Commits.PrdChanges {
			commitPrdOnly(cfg.ProjectRoot, prdPath, fmt.Sprintf("ralph: %s complete", story.ID))
		}

		fmt.Printf("\n✓ %s complete\n", story.ID)
	}
}

// runProvider runs the provider with the given prompt
func runProvider(cfg *ResolvedConfig, prompt string) (*ProviderResult, error) {
	timeout := time.Duration(cfg.Config.Provider.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, cfg.Config.Provider.Command, cfg.Config.Provider.Args...)
	cmd.Dir = cfg.ProjectRoot
	cmd.Env = os.Environ()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start provider: %w", err)
	}

	// Write prompt to stdin
	go func() {
		defer stdin.Close()
		io.WriteString(stdin, prompt)
	}()

	// Collect output with marker detection
	var mu sync.Mutex
	var outputBuilder strings.Builder
	result := &ProviderResult{}

	// Use WaitGroup for stderr goroutine
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintln(os.Stderr, line)
			mu.Lock()
			outputBuilder.WriteString(line + "\n")
			processLine(line, result)
			mu.Unlock()
		}
	}()

	// Process stdout
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Println(line)
		mu.Lock()
		outputBuilder.WriteString(line + "\n")
		processLine(line, result)
		mu.Unlock()
	}

	// Wait for stderr to finish
	wg.Wait()

	// Check for timeout
	if ctx.Err() == context.DeadlineExceeded {
		cmd.Process.Kill()
		result.TimedOut = true
		return result, fmt.Errorf("provider timed out after %v", timeout)
	}

	// Wait for process
	err = cmd.Wait()
	result.Output = outputBuilder.String()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
		return result, nil // Non-zero exit is not an error, just a failed iteration
	}

	result.ExitCode = 0
	return result, nil
}

// processLine processes a line of output for markers
func processLine(line string, result *ProviderResult) {
	if strings.Contains(line, DoneMarker) {
		result.Done = true
	}
	if strings.Contains(line, VerifiedMarker) {
		result.Verified = true
	}

	// Extract learnings
	if matches := LearningPattern.FindStringSubmatch(line); len(matches) > 1 {
		result.Learnings = append(result.Learnings, strings.TrimSpace(matches[1]))
	}

	// Extract resets
	if matches := ResetPattern.FindStringSubmatch(line); len(matches) > 1 {
		ids := strings.Split(matches[1], ",")
		for _, id := range ids {
			result.Resets = append(result.Resets, strings.TrimSpace(id))
		}
	}

	// Extract reason
	if matches := ReasonPattern.FindStringSubmatch(line); len(matches) > 1 {
		result.Reason = strings.TrimSpace(matches[1])
	}
}

// StoryVerifyResult contains the result of story verification
type StoryVerifyResult struct {
	passed bool
	reason string
}

// runStoryVerification runs verification for a single story
func runStoryVerification(cfg *ResolvedConfig, featureDir *FeatureDir, story *UserStory, svcMgr *ServiceManager) (*StoryVerifyResult, error) {
	result := &StoryVerifyResult{passed: true}

	// Run default verification commands
	for _, cmd := range cfg.Config.Verify.Default {
		fmt.Printf("  → %s\n", cmd)
		if err := runCommand(cfg.ProjectRoot, cmd); err != nil {
			result.passed = false
			result.reason = fmt.Sprintf("%s failed: %v", cmd, err)
			return result, nil
		}
	}

	// Run UI verification if story has UI tag
	if IsUIStory(story) {
		// Restart services before UI verification (fresh state)
		if svcMgr != nil && svcMgr.HasUIServices() {
			fmt.Println("  → Restarting services for UI verification...")
			if err := svcMgr.RestartForVerify(); err != nil {
				result.passed = false
				result.reason = fmt.Sprintf("service restart failed: %v", err)
				return result, nil
			}
		}

		// Run built-in browser checks
		if cfg.Config.Browser != nil && cfg.Config.Browser.Enabled {
			fmt.Println("  → Running browser checks...")
			baseURL := GetBaseURL(cfg.Config.Services)
			if baseURL != "" {
				browser := NewBrowserRunner(cfg.ProjectRoot, cfg.Config.Browser)
				browserResults, err := browser.RunChecks(story, baseURL)
				if err != nil {
					fmt.Printf("  ⚠ Browser check error: %v\n", err)
					// Don't fail on browser errors, just warn
				} else if len(browserResults) > 0 {
					fmt.Print(FormatBrowserResults(browserResults))
					// Check for console errors (warn but don't fail)
					for _, r := range browserResults {
						if r.Error != nil {
							fmt.Printf("  ⚠ Page load error: %v\n", r.Error)
						}
					}
				}
			}
		}

		// Run UI verification commands
		for _, cmd := range cfg.Config.Verify.UI {
			fmt.Printf("  → %s\n", cmd)
			if err := runCommand(cfg.ProjectRoot, cmd); err != nil {
				result.passed = false
				result.reason = fmt.Sprintf("%s failed: %v", cmd, err)
				return result, nil
			}
		}
	}

	return result, nil
}

// runFinalVerification runs final verification and handles story resets
func runFinalVerification(cfg *ResolvedConfig, featureDir *FeatureDir, prd *PRD) (bool, error) {
	prdPath := featureDir.PrdJsonPath()

	// Run all verification commands first
	fmt.Println("\nRunning verification commands...")
	for _, cmd := range cfg.Config.Verify.Default {
		fmt.Printf("  → %s\n", cmd)
		if err := runCommand(cfg.ProjectRoot, cmd); err != nil {
			fmt.Printf("  ✗ %s failed\n", cmd)
			// Don't fail yet, let provider review
		}
	}
	for _, cmd := range cfg.Config.Verify.UI {
		fmt.Printf("  → %s\n", cmd)
		if err := runCommand(cfg.ProjectRoot, cmd); err != nil {
			fmt.Printf("  ✗ %s failed\n", cmd)
		}
	}

	// Send verification prompt to provider
	prompt := generateVerifyPrompt(cfg, featureDir, prd)
	result, err := runProvider(cfg, prompt)
	if err != nil {
		return false, err
	}

	// Check for VERIFIED marker
	if result.Verified {
		return true, nil
	}

	// Check for RESET markers
	if len(result.Resets) > 0 {
		fmt.Println("\nResetting stories:")
		for _, storyID := range result.Resets {
			fmt.Printf("  - %s\n", storyID)
			prd.ResetStory(storyID, result.Reason, cfg.Config.MaxRetries)
		}

		if err := SavePRD(prdPath, prd); err != nil {
			return false, fmt.Errorf("failed to save PRD: %w", err)
		}

		if cfg.Config.Commits.PrdChanges {
			commitPrdOnly(cfg.ProjectRoot, prdPath, "ralph: reset stories after verification")
		}

		return false, nil
	}

	// No markers found - treat as not verified
	fmt.Println("\nProvider did not output VERIFIED or RESET markers.")
	return false, nil
}

// runVerify runs verification only (no implementation)
func runVerify(cfg *ResolvedConfig, featureDir *FeatureDir) error {
	prd, err := LoadPRD(featureDir.PrdJsonPath())
	if err != nil {
		return err
	}

	verified, err := runFinalVerification(cfg, featureDir, prd)
	if err != nil {
		return err
	}

	if verified {
		fmt.Println("\n✓ All verification passed")
	} else {
		fmt.Println("\n✗ Verification found issues")
		fmt.Printf("\nRun 'ralph run %s' to continue implementation.\n", featureDir.Feature)
	}

	return nil
}

// runCommand runs a shell command and returns error if it fails
func runCommand(dir, cmdStr string) error {
	cmd := exec.Command("sh", "-c", cmdStr)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// commitPrdOnly commits only the PRD file using GitOps
func commitPrdOnly(projectRoot, prdPath, message string) error {
	git := NewGitOps(projectRoot)
	return git.CommitFile(prdPath, message)
}

// getLastCommit returns the last commit hash
func getLastCommit(projectRoot string) string {
	git := NewGitOps(projectRoot)
	return git.GetLastCommit()
}

// getCommitMessage returns the commit message for a hash
func getCommitMessage(projectRoot, hash string) string {
	git := NewGitOps(projectRoot)
	return git.GetCommitMessage(hash)
}
