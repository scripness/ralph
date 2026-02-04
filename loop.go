package main

import (
	"bufio"
	"bytes"
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
	StuckMarker    = "<ralph>STUCK</ralph>"
)

var (
	LearningPattern    = regexp.MustCompile(`<ralph>LEARNING:(.+?)</ralph>`)
	ResetPattern       = regexp.MustCompile(`<ralph>RESET:(.+?)</ralph>`)
	ReasonPattern      = regexp.MustCompile(`<ralph>REASON:(.+?)</ralph>`)
	BlockPattern       = regexp.MustCompile(`<ralph>BLOCK:(.+?)</ralph>`)
	SuggestNextPattern = regexp.MustCompile(`<ralph>SUGGEST_NEXT:(.+?)</ralph>`)
)

// ProviderResult contains the result of a provider iteration
type ProviderResult struct {
	Output      string
	Done        bool
	Stuck       bool
	Learnings   []string
	Resets      []string
	Blocks      []string
	SuggestNext string
	Reason      string
	Verified    bool
	ExitCode    int
	TimedOut    bool
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
			verified, err := runFinalVerification(cfg, featureDir, prd, svcMgr)
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
				fmt.Printf("  git log --oneline %s..HEAD\n", git.DefaultBranch())
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

		// Capture commit hash AFTER PRD commit, BEFORE provider runs
		// This avoids false positives from the PRD state commit above
		preRunCommit := git.GetLastCommit()

		// Generate and send prompt
		prompt := generateRunPrompt(cfg, featureDir, prd, story)
		result, err := runProvider(cfg, prompt)

		// Process learnings and blocks even on error (result is non-nil even on timeout)
		if result != nil {
			for _, learning := range result.Learnings {
				prd.AddLearning(learning)
			}
			for _, storyID := range result.Blocks {
				fmt.Printf("\n⚠ Provider blocked story: %s\n", storyID)
				prd.MarkStoryBlocked(storyID, result.Reason)
			}
		}

		if err != nil {
			prd.ClearCurrentStory()
			if saveErr := SavePRD(prdPath, prd); saveErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save PRD: %v\n", saveErr)
			}
			if cfg.Config.Commits.PrdChanges {
				commitPrdOnly(cfg.ProjectRoot, prdPath, fmt.Sprintf("ralph: %s provider error", story.ID))
			}
			return fmt.Errorf("provider error: %w", err)
		}

		// Check for STUCK marker (provider can't complete but doesn't know why)
		if result.Stuck {
			reason := result.Reason
			if reason == "" {
				reason = "Provider signaled STUCK"
			}
			fmt.Printf("\n⚠ Provider stuck on %s: %s\n", story.ID, reason)
			prd.MarkStoryFailed(story.ID, reason, cfg.Config.MaxRetries)
			prd.ClearCurrentStory()
			if err := SavePRD(prdPath, prd); err != nil {
				return fmt.Errorf("failed to save PRD: %w", err)
			}
			if cfg.Config.Commits.PrdChanges {
				commitPrdOnly(cfg.ProjectRoot, prdPath, fmt.Sprintf("ralph: %s stuck", story.ID))
			}
			continue
		}

		// Check for DONE marker
		if !result.Done {
			fmt.Println("\nProvider did not signal completion. Retrying...")
			prd.MarkStoryFailed(story.ID, "Provider did not signal completion", cfg.Config.MaxRetries)
			prd.ClearCurrentStory()
			if err := SavePRD(prdPath, prd); err != nil {
				return fmt.Errorf("failed to save PRD: %w", err)
			}
			if cfg.Config.Commits.PrdChanges {
				commitPrdOnly(cfg.ProjectRoot, prdPath, fmt.Sprintf("ralph: %s no completion signal", story.ID))
			}
			continue
		}

		// Check that provider actually committed something
		if !git.HasNewCommitSince(preRunCommit) {
			fmt.Println("\n⚠ Provider signaled DONE but made no new commit.")
			prd.MarkStoryFailed(story.ID, "No commit made — provider signaled DONE without committing code", cfg.Config.MaxRetries)
			prd.ClearCurrentStory()
			if err := SavePRD(prdPath, prd); err != nil {
				return fmt.Errorf("failed to save PRD: %w", err)
			}
			if cfg.Config.Commits.PrdChanges {
				commitPrdOnly(cfg.ProjectRoot, prdPath, fmt.Sprintf("ralph: %s no commit", story.ID))
			}
			continue
		}

		// Warn if working tree is dirty (provider left uncommitted files)
		if !git.IsWorkingTreeClean() {
			fmt.Println("\n⚠ Working tree has uncommitted changes after provider finished.")
			fmt.Println("  Provider may have left untracked or modified files.")
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

// buildProviderArgs builds the final argument list for a provider subprocess.
// It returns the args to pass and an optional temp file path (for file mode) that
// the caller must clean up.
func buildProviderArgs(baseArgs []string, promptMode, promptFlag, prompt string) (args []string, promptFile string, err error) {
	args = append([]string{}, baseArgs...)

	switch promptMode {
	case "arg":
		if promptFlag != "" {
			args = append(args, promptFlag)
		}
		args = append(args, prompt)
	case "file":
		f, ferr := os.CreateTemp("", "ralph-prompt-*.md")
		if ferr != nil {
			return nil, "", fmt.Errorf("failed to create temp prompt file: %w", ferr)
		}
		promptFile = f.Name()
		if _, ferr := f.WriteString(prompt); ferr != nil {
			f.Close()
			os.Remove(promptFile)
			return nil, "", fmt.Errorf("failed to write prompt file: %w", ferr)
		}
		f.Close()
		if promptFlag != "" {
			args = append(args, promptFlag)
		}
		args = append(args, promptFile)
	}
	// "stdin" mode doesn't modify args

	return args, promptFile, nil
}

// runProvider runs the provider with the given prompt
func runProvider(cfg *ResolvedConfig, prompt string) (*ProviderResult, error) {
	timeout := time.Duration(cfg.Config.Provider.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	p := cfg.Config.Provider
	args, promptFile, err := buildProviderArgs(p.Args, p.PromptMode, p.PromptFlag, prompt)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, cfg.Config.Provider.Command, args...)
	cmd.Dir = cfg.ProjectRoot
	cmd.Env = os.Environ()

	// Setup stdin pipe for stdin mode
	var stdinPipe io.WriteCloser
	if cfg.Config.Provider.PromptMode == "stdin" || cfg.Config.Provider.PromptMode == "" {
		var err error
		stdinPipe, err = cmd.StdinPipe()
		if err != nil {
			if promptFile != "" {
				os.Remove(promptFile)
			}
			return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
		}
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		if promptFile != "" {
			os.Remove(promptFile)
		}
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		if promptFile != "" {
			os.Remove(promptFile)
		}
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		if promptFile != "" {
			os.Remove(promptFile)
		}
		return nil, fmt.Errorf("failed to start provider: %w", err)
	}

	// Cleanup prompt file when done (for file mode)
	if promptFile != "" {
		defer os.Remove(promptFile)
	}

	// Write prompt to stdin (for stdin mode)
	if stdinPipe != nil {
		go func() {
			defer stdinPipe.Close()
			io.WriteString(stdinPipe, prompt)
		}()
	}

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
	if strings.Contains(line, StuckMarker) {
		result.Stuck = true
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

	// Extract blocks
	if matches := BlockPattern.FindStringSubmatch(line); len(matches) > 1 {
		ids := strings.Split(matches[1], ",")
		for _, id := range ids {
			result.Blocks = append(result.Blocks, strings.TrimSpace(id))
		}
	}

	// Extract suggest next
	if matches := SuggestNextPattern.FindStringSubmatch(line); len(matches) > 1 {
		result.SuggestNext = strings.TrimSpace(matches[1])
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
		output, err := runCommand(cfg.ProjectRoot, cmd, cfg.Config.Verify.Timeout)
		if err != nil {
			result.passed = false
			result.reason = fmt.Sprintf("%s failed: %v\n\n--- Output (last 50 lines) ---\n%s", cmd, err, output)
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

		// Run built-in browser verification
		if cfg.Config.Browser != nil && cfg.Config.Browser.Enabled {
			baseURL := GetBaseURL(cfg.Config.Services)
			if baseURL != "" {
				browser := NewBrowserRunner(cfg.ProjectRoot, cfg.Config.Browser)

				// Use interactive steps if defined, otherwise fall back to URL checks
				if len(story.BrowserSteps) > 0 {
					fmt.Println("  → Running browser verification steps...")
					browserResult, err := browser.RunSteps(story, baseURL)
					if err != nil {
						result.passed = false
						result.reason = fmt.Sprintf("browser initialization failed: %v", err)
						return result, nil
					} else if browserResult != nil {
						fmt.Print(FormatStepResult(browserResult))

						// Fail story if browser steps failed
						if browserResult.Error != nil {
							result.passed = false
							result.reason = fmt.Sprintf("browser verification failed: %v", browserResult.Error)
							return result, nil
						}

						// Fail on console errors
						if len(browserResult.ConsoleErrors) > 0 {
							fmt.Printf("  ✗ Console errors detected: %d\n", len(browserResult.ConsoleErrors))
							for _, ce := range browserResult.ConsoleErrors {
								fmt.Printf("    - %s\n", ce)
							}
							result.passed = false
							result.reason = fmt.Sprintf("browser console errors: %d error(s) detected", len(browserResult.ConsoleErrors))
							return result, nil
						}
					}
				} else {
					// Fallback: basic URL checks
					fmt.Println("  → Running browser checks...")
					browserResults, err := browser.RunChecks(story, baseURL)
					if err != nil {
						result.passed = false
						result.reason = fmt.Sprintf("browser initialization failed: %v", err)
						return result, nil
					} else if len(browserResults) > 0 {
						fmt.Print(FormatBrowserResults(browserResults))
						for _, r := range browserResults {
							if r.Error != nil {
								result.passed = false
								result.reason = fmt.Sprintf("page load failed: %v", r.Error)
								return result, nil
							}
							if len(r.ConsoleErrors) > 0 {
								fmt.Printf("  ✗ Console errors detected: %d\n", len(r.ConsoleErrors))
								for _, ce := range r.ConsoleErrors {
									fmt.Printf("    - %s\n", ce)
								}
								result.passed = false
								result.reason = fmt.Sprintf("browser console errors: %d error(s) detected", len(r.ConsoleErrors))
								return result, nil
							}
						}
					}
				}
			}
		}

		// Run UI verification commands
		for _, cmd := range cfg.Config.Verify.UI {
			fmt.Printf("  → %s\n", cmd)
			output, err := runCommand(cfg.ProjectRoot, cmd, cfg.Config.Verify.Timeout)
			if err != nil {
				result.passed = false
				result.reason = fmt.Sprintf("%s failed: %v\n\n--- Output (last 50 lines) ---\n%s", cmd, err, output)
				return result, nil
			}
		}
	}

	// Check service health after all verification
	if svcMgr != nil && svcMgr.HasServices() {
		if healthIssues := svcMgr.CheckServiceHealth(); len(healthIssues) > 0 {
			// Include recent service output for diagnostics
			reason := fmt.Sprintf("service health check failed: %s", strings.Join(healthIssues, "; "))
			for _, svc := range cfg.Config.Services {
				svcOutput := svcMgr.GetRecentOutput(svc.Name, 30)
				if svcOutput != "" {
					reason += fmt.Sprintf("\n\n--- %s output (last 30 lines) ---\n%s", svc.Name, svcOutput)
				}
			}
			result.passed = false
			result.reason = reason
			return result, nil
		}
	}

	return result, nil
}

// runFinalVerification runs final verification and handles story resets
func runFinalVerification(cfg *ResolvedConfig, featureDir *FeatureDir, prd *PRD, svcMgr *ServiceManager) (bool, error) {
	prdPath := featureDir.PrdJsonPath()

	// Run all verification commands first
	fmt.Println("\nRunning verification commands...")
	verifyFailed := false
	var summaryLines []string
	for _, cmd := range cfg.Config.Verify.Default {
		fmt.Printf("  → %s\n", cmd)
		output, err := runCommand(cfg.ProjectRoot, cmd, cfg.Config.Verify.Timeout)
		if err != nil {
			fmt.Printf("  ✗ %s failed\n", cmd)
			verifyFailed = true
			summaryLines = append(summaryLines, "FAIL: "+cmd+"\n"+output)
		} else {
			summaryLines = append(summaryLines, "PASS: "+cmd)
		}
	}
	for _, cmd := range cfg.Config.Verify.UI {
		fmt.Printf("  → %s\n", cmd)
		output, err := runCommand(cfg.ProjectRoot, cmd, cfg.Config.Verify.Timeout)
		if err != nil {
			fmt.Printf("  ✗ %s failed\n", cmd)
			verifyFailed = true
			summaryLines = append(summaryLines, "FAIL: "+cmd+"\n"+output)
		} else {
			summaryLines = append(summaryLines, "PASS: "+cmd)
		}
	}

	// Run browser verification for all UI stories
	if svcMgr != nil && cfg.Config.Browser != nil && cfg.Config.Browser.Enabled {
		baseURL := GetBaseURL(cfg.Config.Services)
		if baseURL != "" {
			for i := range prd.UserStories {
				story := &prd.UserStories[i]
				if story.Blocked || !IsUIStory(story) {
					continue
				}

				fmt.Printf("  → Browser: %s (%s)\n", story.ID, story.Title)

				if svcMgr.HasUIServices() {
					if err := svcMgr.RestartForVerify(); err != nil {
						fmt.Printf("    ⚠ Service restart failed: %v\n", err)
						verifyFailed = true
						summaryLines = append(summaryLines, fmt.Sprintf("FAIL: service restart for %s (%v)", story.ID, err))
						continue
					}
				}

				browser := NewBrowserRunner(cfg.ProjectRoot, cfg.Config.Browser)

				if len(story.BrowserSteps) > 0 {
					// Interactive browser steps
					browserResult, err := browser.RunSteps(story, baseURL)
					if err != nil {
						fmt.Printf("    ✗ Browser error: %v\n", err)
						verifyFailed = true
						summaryLines = append(summaryLines, fmt.Sprintf("FAIL: browser %s (init: %v)", story.ID, err))
					} else if browserResult != nil {
						fmt.Print(FormatStepResult(browserResult))
						if browserResult.Error != nil {
							fmt.Printf("    ✗ Failed: %v\n", browserResult.Error)
							verifyFailed = true
							summaryLines = append(summaryLines, fmt.Sprintf("FAIL: browser %s (error: %v)", story.ID, browserResult.Error))
						}
						if len(browserResult.ConsoleErrors) > 0 {
							fmt.Printf("    ✗ Console errors: %d\n", len(browserResult.ConsoleErrors))
							for _, ce := range browserResult.ConsoleErrors {
								fmt.Printf("      - %s\n", ce)
							}
							verifyFailed = true
							summaryLines = append(summaryLines, fmt.Sprintf("FAIL: browser %s (console errors: %d)", story.ID, len(browserResult.ConsoleErrors)))
						}
						if browserResult.Error == nil && len(browserResult.ConsoleErrors) == 0 {
							summaryLines = append(summaryLines, fmt.Sprintf("PASS: browser %s", story.ID))
						}
					}
				} else {
					// Fallback: basic URL checks for UI stories without explicit browserSteps
					fmt.Println("    → Running browser checks (fallback)...")
					browserResults, err := browser.RunChecks(story, baseURL)
					if err != nil {
						fmt.Printf("    ✗ Browser check error: %v\n", err)
						verifyFailed = true
						summaryLines = append(summaryLines, fmt.Sprintf("FAIL: browser %s (init: %v)", story.ID, err))
					} else if len(browserResults) > 0 {
						fmt.Print(FormatBrowserResults(browserResults))
						hasFailed := false
						for _, r := range browserResults {
							if r.Error != nil {
								verifyFailed = true
								hasFailed = true
								summaryLines = append(summaryLines, fmt.Sprintf("FAIL: browser %s (page load: %v)", story.ID, r.Error))
								break
							}
							if len(r.ConsoleErrors) > 0 {
								fmt.Printf("    ✗ Console errors: %d\n", len(r.ConsoleErrors))
								for _, ce := range r.ConsoleErrors {
									fmt.Printf("      - %s\n", ce)
								}
								verifyFailed = true
								hasFailed = true
								summaryLines = append(summaryLines, fmt.Sprintf("FAIL: browser %s (console errors: %d)", story.ID, len(r.ConsoleErrors)))
								break
							}
						}
						if !hasFailed {
							summaryLines = append(summaryLines, fmt.Sprintf("PASS: browser %s", story.ID))
						}
					}
				}
			}
		}
	}

	// Check service health after verification
	if svcMgr != nil && svcMgr.HasServices() {
		if healthIssues := svcMgr.CheckServiceHealth(); len(healthIssues) > 0 {
			for _, issue := range healthIssues {
				fmt.Printf("  ✗ %s\n", issue)
			}
			verifyFailed = true
			summaryLines = append(summaryLines, "FAIL: service health: "+strings.Join(healthIssues, "; "))
		} else {
			summaryLines = append(summaryLines, "PASS: service health")
		}
	}

	// Check knowledgeFile modification
	git := NewGitOps(cfg.ProjectRoot)
	knowledgeFile := cfg.Config.Provider.KnowledgeFile
	if knowledgeFile != "" {
		if git.HasFileChanged(knowledgeFile) {
			summaryLines = append(summaryLines, fmt.Sprintf("PASS: %s was updated", knowledgeFile))
		} else {
			summaryLines = append(summaryLines, fmt.Sprintf("WARN: %s was NOT modified on this branch — verify documentation is up to date", knowledgeFile))
		}
	}

	// Check if test files were modified
	if git.HasTestFileChanges() {
		summaryLines = append(summaryLines, "PASS: test files were modified")
	} else {
		summaryLines = append(summaryLines, "WARN: no test files were modified on this branch — verify test coverage exists")
	}

	verifySummary := strings.Join(summaryLines, "\n")

	// Send verification prompt to provider
	prompt := generateVerifyPrompt(cfg, featureDir, prd, verifySummary)
	result, err := runProvider(cfg, prompt)

	// Save learnings from verification
	if result != nil && len(result.Learnings) > 0 {
		for _, learning := range result.Learnings {
			prd.AddLearning(learning)
		}
		if saveErr := SavePRD(prdPath, prd); saveErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save verification learnings: %v\n", saveErr)
		}
		if cfg.Config.Commits.PrdChanges {
			commitPrdOnly(cfg.ProjectRoot, prdPath, "ralph: save verification learnings")
		}
	}

	if err != nil {
		return false, err
	}

	// Check for VERIFIED marker
	if result.Verified {
		if verifyFailed {
			fmt.Println("\nProvider signaled VERIFIED but verification failed.")
			fmt.Println("Overriding to not-verified — fix failing checks before verification can pass.")
			return false, nil
		}
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

	// Start services for browser verification
	svcMgr := NewServiceManager(cfg.ProjectRoot, cfg.Config.Services)
	defer svcMgr.StopAll()
	if svcMgr.HasServices() {
		fmt.Println("Starting services...")
		if err := svcMgr.EnsureRunning(); err != nil {
			return fmt.Errorf("failed to start services: %w", err)
		}
	}

	verified, err := runFinalVerification(cfg, featureDir, prd, svcMgr)
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

// runCommand runs a shell command with a per-command timeout, printing output and returning it with any error.
// The captured output (last maxOutputLines lines) is returned for diagnostic context.
func runCommand(dir, cmdStr string, timeoutSec int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &buf)
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return truncateOutput(buf.String(), 50), fmt.Errorf("timed out after %ds", timeoutSec)
	}
	return truncateOutput(buf.String(), 50), err
}

// truncateOutput keeps the last N lines of output for diagnostic context.
func truncateOutput(output string, maxLines int) string {
	lines := strings.Split(output, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}

// commitPrdOnly commits only the PRD file using GitOps
func commitPrdOnly(projectRoot, prdPath, message string) error {
	git := NewGitOps(projectRoot)
	return git.CommitFile(prdPath, message)
}

// getLastCommit returns the last commit hash (short, for display)
func getLastCommit(projectRoot string) string {
	git := NewGitOps(projectRoot)
	return git.GetLastCommitShort()
}

// getCommitMessage returns the commit message for a hash
func getCommitMessage(projectRoot, hash string) string {
	git := NewGitOps(projectRoot)
	return git.GetCommitMessage(hash)
}
