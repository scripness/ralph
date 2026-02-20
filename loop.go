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

// Provider signal markers — only 3 remain
const (
	DoneMarker  = "<ralph>DONE</ralph>"
	StuckMarker = "<ralph>STUCK</ralph>"
)

var (
	LearningPattern    = regexp.MustCompile(`^<ralph>LEARNING:(.+?)</ralph>$`)
	StuckNotePattern   = regexp.MustCompile(`^<ralph>STUCK:(.+?)</ralph>$`)
	VerifyPassPattern  = regexp.MustCompile(`^<ralph>VERIFY_PASS</ralph>$`)
	VerifyFailPattern  = regexp.MustCompile(`^<ralph>VERIFY_FAIL:(.+?)</ralph>$`)
)

// ProviderResult contains the result of a provider iteration
type ProviderResult struct {
	Output    string
	Done      bool
	Stuck     bool
	StuckNote string
	Learnings []string
	ExitCode  int
	TimedOut  bool
}

// runLoop runs the main implementation loop for a feature
func runLoop(cfg *ResolvedConfig, featureDir *FeatureDir) error {
	prdPath := featureDir.PrdJsonPath()
	statePath := featureDir.RunStatePath()
	git := NewGitOps(cfg.ProjectRoot)

	// Load PRD definition + run state
	def, err := LoadPRDDefinition(prdPath)
	if err != nil {
		return err
	}
	state, err := LoadRunState(statePath)
	if err != nil {
		return err
	}

	// Create cleanup coordinator early for signal handling
	cleanup := NewCleanupCoordinator()

	// Initialize logger
	logger, err := NewRunLogger(featureDir.Path, cfg.Config.Logging)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	cleanup.SetLogger(logger)
	defer logger.Close()

	// Acquire lock
	lock := NewLockFile(cfg.ProjectRoot)
	if err := lock.Acquire(featureDir.Feature, def.BranchName); err != nil {
		return err
	}
	cleanup.SetLock(lock)
	defer lock.Release()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n\nInterrupted. Cleaning up and exiting...")
		cleanup.Cleanup()
		os.Exit(130)
	}()

	// Log run start
	logger.RunStart(featureDir.Feature, def.BranchName, len(def.UserStories))

	// Ensure we're on the correct branch
	logger.LogPrintln("Ensuring branch:", def.BranchName)
	if err := git.EnsureBranch(def.BranchName); err != nil {
		logger.Error("failed to switch branch", err)
		logger.RunEnd(false, "branch switch failed")
		return fmt.Errorf("failed to switch to branch %s: %w", def.BranchName, err)
	}

	fmt.Println(strings.Repeat("=", 60))
	fmt.Println(" Ralph - Autonomous Agent Loop")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf(" Feature: %s\n", featureDir.Feature)
	fmt.Printf(" Branch: %s\n", def.BranchName)
	fmt.Printf(" PRD: %s\n", prdPath)
	fmt.Printf(" Project root: %s\n", cfg.ProjectRoot)
	if logger.LogPath() != "" {
		fmt.Printf(" Run: #%d (logs: %s)\n", logger.RunNumber(), logger.LogPath())
	}
	fmt.Println(strings.Repeat("=", 60))

	// Initialize service manager
	svcMgr := NewServiceManager(cfg.ProjectRoot, cfg.Config.Services)
	cleanup.SetServiceManager(svcMgr)
	defer svcMgr.StopAll()

	// Start services if configured
	if svcMgr.HasServices() {
		logger.LogPrintln("\nStarting services...")
		for _, svc := range cfg.Config.Services {
			logger.ServiceStart(svc.Name, svc.Start)
		}
		startTime := time.Now()
		if err := svcMgr.EnsureRunning(); err != nil {
			logger.Error("failed to start services", err)
			logger.RunEnd(false, "service start failed")
			return fmt.Errorf("failed to start services: %w", err)
		}
		for _, svc := range cfg.Config.Services {
			logger.ServiceReady(svc.Name, svc.Ready, time.Since(startTime).Nanoseconds())
		}
	}

	// Discover codebase context once (used for resource sync + run prompt)
	codebaseCtx := DiscoverCodebase(cfg.ProjectRoot, &cfg.Config)
	codebaseStr := FormatCodebaseContext(codebaseCtx)

	// Sync source code resources for detected frameworks
	rm := ensureResourceSync(cfg, codebaseCtx)

	iteration := 0
	for {
		iteration++
		logger.SetIteration(iteration)

		// Reload state at start of each iteration (definition is immutable)
		state, err = LoadRunState(statePath)
		if err != nil {
			logger.Error("failed to load run state", err)
			logger.RunEnd(false, "state load failed")
			return err
		}

		// Check if all stories complete
		if AllComplete(def, state) {
			logger.LogPrintln()
			fmt.Println(strings.Repeat("=", 60))
			logger.LogPrintln(" All stories complete!")
			fmt.Println(strings.Repeat("=", 60))

			// Print captured learnings summary
			if len(state.Learnings) > 0 {
				logger.LogPrintln()
				logger.LogPrint("Learnings captured (%d):\n", len(state.Learnings))
				for i, l := range state.Learnings {
					logger.LogPrint("  %d. %s\n", i+1, l)
				}
			}

			// Check if knowledge file was updated
			knowledgeFile := cfg.Config.Provider.KnowledgeFile
			if knowledgeFile != "" && !git.HasFileChanged(knowledgeFile) {
				logger.LogPrintln()
				logger.LogPrint("! %s was not updated. Consider adding discovered patterns for future features.\n", knowledgeFile)
			}

			logger.LogPrintln()
			logger.LogPrintln("Run 'ralph verify " + featureDir.Feature + "' for comprehensive verification.")
			logger.RunEnd(true, "all stories complete")
			return nil
		}

		// Check if all remaining stories are skipped (no next story)
		story := GetNextStory(def, state)
		if story == nil {
			logger.LogPrintln()
			fmt.Println(strings.Repeat("=", 60))
			logger.LogPrintln(" ! All remaining stories are skipped")
			fmt.Println(strings.Repeat("=", 60))
			logger.LogPrintln()
			logger.LogPrintln("Skipped stories:")
			for _, sk := range state.Skipped {
				s := GetStoryByID(def, sk)
				if s != nil {
					logger.LogPrint("  - %s: %s\n", s.ID, s.Title)
					if reason := state.GetLastFailure(sk); reason != "" {
						logger.LogPrint("    └─ %s\n", reason)
					}
				}
			}
			logger.LogPrintln()
			logger.LogPrintln("Manual intervention required.")
			logger.RunEnd(false, "all remaining stories skipped")
			return fmt.Errorf("all remaining stories skipped")
		}

		// Verify-at-top: if this story already passes, mark it and skip implementation.
		// Only runs when the branch has non-.ralph/ changes (real implementation work).
		// On fresh branches with only PRD state, skip verify-at-top to avoid false positives
		// where the existing test suite passes vacuously (it doesn't test the new feature yet).
		if !state.IsPassed(story.ID) && git.HasNonRalphChanges() {
			verifyResult, verifyErr := runStoryVerification(cfg, featureDir, story, svcMgr, logger)
			if verifyErr == nil && verifyResult.passed {
				logger.LogPrint("\n✓ %s already passes verification, marking complete\n", story.ID)
				state.MarkPassed(story.ID)
				if err := SaveRunState(statePath, state); err != nil {
					return fmt.Errorf("failed to save state: %w", err)
				}
				if cfg.Config.Commits.PrdChanges {
					if commitErr := commitPrdOnly(cfg.ProjectRoot, statePath, fmt.Sprintf("ralph: %s pre-verified", story.ID)); commitErr != nil {
						logger.Warning("failed to commit state: " + commitErr.Error())
					}
				}
				continue
			}
		}

		logger.LogPrintln()
		fmt.Println(strings.Repeat("=", 60))
		logger.LogPrint(" Iteration %d: %s - %s\n", iteration, story.ID, story.Title)
		fmt.Println(strings.Repeat("=", 60))

		// Log iteration and story start
		logger.SetCurrentStory(story.ID)
		logger.IterationStart(story.ID, story.Title, state.GetRetries(story.ID))

		// Commit state change
		if cfg.Config.Commits.PrdChanges {
			if err := commitPrdOnly(cfg.ProjectRoot, statePath, fmt.Sprintf("ralph: start %s", story.ID)); err != nil {
				fmt.Printf("Warning: failed to commit state: %v\n", err)
			}
		}

		// Capture commit hash AFTER state commit, BEFORE provider runs
		preRunCommit := git.GetLastCommit()

		// Compute diff summary per-iteration (changes as provider commits)
		diffSummary := ""
		if diffStat := git.GetDiffSummary(); diffStat != "" {
			diffSummary = "## Changes on Branch\n\n```\n" + truncateOutput(diffStat, 60) + "\n```\n"
		}

		// Resource consultation (before spawning main agent)
		var resourceGuidance string
		if rm != nil && rm.HasDetectedResources() {
			consultResult := ConsultResources(context.Background(), cfg, story, rm, codebaseCtx, featureDir.Path)
			resourceGuidance = FormatGuidance(consultResult)
			if len(consultResult.Consultations) > 0 {
				logger.LogPrint("  Consulted %d framework(s) for %s\n", len(consultResult.Consultations), story.ID)
			}
		} else {
			resourceGuidance = buildResourceFallbackInstructions()
		}

		// Generate and send prompt
		prompt := generateRunPrompt(cfg, featureDir, def, state, story, codebaseStr, diffSummary, resourceGuidance)
		logger.LogPrintln("Provider running...")
		logger.ProviderStart()
		providerStartTime := time.Now()
		result, err := runProvider(cfg, prompt, logger, cleanup)

		// Collect markers for logging
		var detectedMarkers []string
		if result != nil {
			if result.Done {
				detectedMarkers = append(detectedMarkers, "DONE")
			}
			if result.Stuck {
				detectedMarkers = append(detectedMarkers, "STUCK")
			}
			if len(result.Learnings) > 0 {
				detectedMarkers = append(detectedMarkers, "LEARNING")
			}
		}
		if result != nil {
			logger.ProviderEnd(result.ExitCode, result.TimedOut, detectedMarkers)
		} else {
			logger.ProviderEnd(0, false, nil)
		}
		logger.LogPrint("Provider done (%s)\n", FormatDuration(time.Since(providerStartTime)))

		// Process learnings even on error
		if result != nil {
			for _, learning := range result.Learnings {
				state.AddLearning(learning)
				logger.Learning(learning)
			}
		}

		if err != nil {
			logger.Error("provider error", err)
			if saveErr := SaveRunState(statePath, state); saveErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save state: %v\n", saveErr)
			}
			if cfg.Config.Commits.PrdChanges {
				if commitErr := commitPrdOnly(cfg.ProjectRoot, statePath, fmt.Sprintf("ralph: %s provider error", story.ID)); commitErr != nil {
					logger.Warning("failed to commit state: " + commitErr.Error())
				}
			}
			logger.IterationEnd(false)
			logger.RunEnd(false, "provider error")
			return fmt.Errorf("provider error: %w", err)
		}

		// Check for STUCK marker
		if result.Stuck {
			reason := result.StuckNote
			if reason == "" {
				reason = "Provider signaled STUCK"
			}
			logger.LogPrint("\n! Provider stuck on %s: %s\n", story.ID, reason)
			logger.StateChange(story.ID, "pending", "failed", map[string]interface{}{"reason": reason})
			state.MarkFailed(story.ID, reason, cfg.Config.MaxRetries)
			if err := SaveRunState(statePath, state); err != nil {
				logger.IterationEnd(false)
				return fmt.Errorf("failed to save state: %w", err)
			}
			if cfg.Config.Commits.PrdChanges {
				if commitErr := commitPrdOnly(cfg.ProjectRoot, statePath, fmt.Sprintf("ralph: %s stuck", story.ID)); commitErr != nil {
					logger.Warning("failed to commit state: " + commitErr.Error())
				}
			}
			logger.IterationEnd(false)
			continue
		}

		// Check for DONE marker
		if !result.Done {
			logger.LogPrintln("\nProvider did not signal completion. Retrying...")
			logger.Warning("provider did not signal completion")
			state.MarkFailed(story.ID, "Provider did not signal completion", cfg.Config.MaxRetries)
			if err := SaveRunState(statePath, state); err != nil {
				logger.IterationEnd(false)
				return fmt.Errorf("failed to save state: %w", err)
			}
			if cfg.Config.Commits.PrdChanges {
				if commitErr := commitPrdOnly(cfg.ProjectRoot, statePath, fmt.Sprintf("ralph: %s no completion signal", story.ID)); commitErr != nil {
					logger.Warning("failed to commit state: " + commitErr.Error())
				}
			}
			logger.IterationEnd(false)
			continue
		}

		// Check that provider actually committed something
		if !git.HasNewCommitSince(preRunCommit) {
			logger.LogPrintln("\n! Provider signaled DONE but made no new commit.")
			logger.Warning("provider signaled DONE but made no new commit")
			state.MarkFailed(story.ID, "No commit made — provider signaled DONE without committing code", cfg.Config.MaxRetries)
			if err := SaveRunState(statePath, state); err != nil {
				logger.IterationEnd(false)
				return fmt.Errorf("failed to save state: %w", err)
			}
			if cfg.Config.Commits.PrdChanges {
				if commitErr := commitPrdOnly(cfg.ProjectRoot, statePath, fmt.Sprintf("ralph: %s no commit", story.ID)); commitErr != nil {
					logger.Warning("failed to commit state: " + commitErr.Error())
				}
			}
			logger.IterationEnd(false)
			continue
		}

		// Warn if working tree is dirty
		if !git.IsWorkingTreeClean() {
			logger.LogPrintln("\n! Working tree has uncommitted changes after provider finished.")
			logger.Warning("working tree has uncommitted changes after provider finished")
		}

		// Run verification
		logger.LogPrintln("\nRunning verification...")
		logger.VerifyStart()
		verifyResult, err := runStoryVerification(cfg, featureDir, story, svcMgr, logger)
		if err != nil {
			logger.Error("verification error", err)
			logger.VerifyEnd(false)
			logger.IterationEnd(false)
			_ = SaveRunState(statePath, state) // preserve learnings
			logger.RunEnd(false, "verification error")
			return fmt.Errorf("verification error: %w", err)
		}

		if !verifyResult.passed {
			logger.LogPrint("\nVerification failed: %s\n", verifyResult.reason)
			logger.VerifyEnd(false)
			logger.StateChange(story.ID, "pending", "failed", map[string]interface{}{"reason": verifyResult.reason})
			state.MarkFailed(story.ID, verifyResult.reason, cfg.Config.MaxRetries)
			if err := SaveRunState(statePath, state); err != nil {
				logger.IterationEnd(false)
				return fmt.Errorf("failed to save state: %w", err)
			}
			if cfg.Config.Commits.PrdChanges {
				if commitErr := commitPrdOnly(cfg.ProjectRoot, statePath, fmt.Sprintf("ralph: %s failed verification", story.ID)); commitErr != nil {
					logger.Warning("failed to commit state: " + commitErr.Error())
				}
			}
			logger.IterationEnd(false)
			continue
		}
		logger.VerifyEnd(true)

		// Story passed!
		state.MarkPassed(story.ID)
		logger.StateChange(story.ID, "pending", "passed", nil)

		if err := SaveRunState(statePath, state); err != nil {
			logger.IterationEnd(false)
			return fmt.Errorf("failed to save state: %w", err)
		}

		if cfg.Config.Commits.PrdChanges {
			if commitErr := commitPrdOnly(cfg.ProjectRoot, statePath, fmt.Sprintf("ralph: %s complete", story.ID)); commitErr != nil {
				logger.Warning("failed to commit state: " + commitErr.Error())
			}
		}

		logger.LogPrint("\n✓ %s complete\n", story.ID)
		logger.IterationEnd(true)
	}
}

// buildProviderArgs builds the final argument list for a provider subprocess.
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
func runProvider(cfg *ResolvedConfig, prompt string, logger *RunLogger, cleanup *CleanupCoordinator) (*ProviderResult, error) {
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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 5 * time.Second

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

	// Register provider with cleanup coordinator for signal handling
	if cleanup != nil {
		cleanup.SetProvider(cmd)
		defer cleanup.ClearProvider()
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
	var stdoutBuilder, stderrBuilder, outputBuilder strings.Builder
	result := &ProviderResult{}

	// Use WaitGroup for stderr goroutine
	var wg sync.WaitGroup
	var stderrScanErr error
	wg.Add(1)

	go func() {
		defer wg.Done()
		s := bufio.NewScanner(stderr)
		s.Buffer(make([]byte, 64*1024), 1024*1024)
		for s.Scan() {
			line := s.Text()
			if logger != nil {
				logger.ProviderLine("stderr", line)
			}
			mu.Lock()
			stderrBuilder.WriteString(line + "\n")
			outputBuilder.WriteString(line + "\n")
			processLine(line, result, logger)
			mu.Unlock()
		}
		stderrScanErr = s.Err()
	}()

	// Process stdout
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if logger != nil {
			logger.ProviderLine("stdout", line)
		}
		mu.Lock()
		stdoutBuilder.WriteString(line + "\n")
		outputBuilder.WriteString(line + "\n")
		processLine(line, result, logger)
		mu.Unlock()
	}

	// Check stdout scanner for errors
	if scanErr := scanner.Err(); scanErr != nil && logger != nil {
		logger.Warning(fmt.Sprintf("stdout scanner error (possible line >1MB): %v", scanErr))
	}

	// Wait for stderr to finish
	wg.Wait()

	// Check stderr scanner for errors
	if stderrScanErr != nil && logger != nil {
		logger.Warning(fmt.Sprintf("stderr scanner error (possible line >1MB): %v", stderrScanErr))
	}

	// Always wait for process
	err = cmd.Wait()
	result.Output = outputBuilder.String()

	// Log provider output
	if logger != nil {
		logger.ProviderOutput(stdoutBuilder.String(), stderrBuilder.String())
	}

	if ctx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		return result, fmt.Errorf("provider timed out after %v", timeout)
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
		return result, nil // Non-zero exit is not an error
	}

	result.ExitCode = 0
	return result, nil
}

// processLine processes a line of output for markers.
// Uses whole-line matching (after trimming whitespace) to prevent marker spoofing.
func processLine(line string, result *ProviderResult, logger *RunLogger) {
	trimmed := strings.TrimSpace(line)

	if trimmed == DoneMarker {
		result.Done = true
		if logger != nil {
			logger.MarkerDetected("DONE", "")
			logger.LogPrintln("  ◆ DONE")
		}
	}
	if trimmed == StuckMarker {
		result.Stuck = true
		if logger != nil {
			logger.MarkerDetected("STUCK", "")
			logger.LogPrintln("  ◆ STUCK")
		}
	}

	// STUCK with reason: <ralph>STUCK:reason text</ralph>
	if matches := StuckNotePattern.FindStringSubmatch(trimmed); len(matches) > 1 {
		result.Stuck = true
		result.StuckNote = strings.TrimSpace(matches[1])
		if logger != nil {
			logger.MarkerDetected("STUCK", result.StuckNote)
			logger.LogPrint("  ◆ STUCK: %s\n", result.StuckNote)
		}
	}

	// Extract learnings
	if matches := LearningPattern.FindStringSubmatch(trimmed); len(matches) > 1 {
		value := strings.TrimSpace(matches[1])
		result.Learnings = append(result.Learnings, value)
		if logger != nil {
			logger.MarkerDetected("LEARNING", value)
			logger.LogPrint("  ~ LEARNING: %s\n", value)
		}
	}
}

// StoryVerifyResult contains the result of story verification
type StoryVerifyResult struct {
	passed bool
	reason string
}

// runStoryVerification runs verification for a single story
func runStoryVerification(cfg *ResolvedConfig, featureDir *FeatureDir, story *StoryDefinition, svcMgr *ServiceManager, logger *RunLogger) (*StoryVerifyResult, error) {
	result := &StoryVerifyResult{passed: true}

	// Run default verification commands
	for _, cmd := range cfg.Config.Verify.Default {
		logger.LogPrint("  → %s\n", cmd)
		logger.VerifyCmdStart(cmd)
		startTime := time.Now()
		output, err := runCommand(cfg.ProjectRoot, cmd, cfg.Config.Verify.Timeout)
		duration := time.Since(startTime)
		if err != nil {
			logger.VerifyCmdEnd(cmd, false, output, duration.Nanoseconds())
			result.passed = false
			result.reason = fmt.Sprintf("%s failed: %v\n\n--- Output (last 50 lines) ---\n%s", cmd, err, output)
			return result, nil
		}
		logger.VerifyCmdEnd(cmd, true, output, duration.Nanoseconds())
		if logger.config != nil && logger.config.ConsoleDurations {
			logger.LogPrint("    ✓ (%s)\n", FormatDuration(duration))
		}
	}

	// Run UI verification if story has UI tag
	if IsUIStory(story) {
		// Restart services before UI verification (fresh state)
		if svcMgr != nil && svcMgr.HasUIServices() {
			logger.LogPrintln("  → Restarting services for UI verification...")
			if err := svcMgr.RestartForVerify(); err != nil {
				logger.ServiceRestart("all", false)
				result.passed = false
				result.reason = fmt.Sprintf("service restart failed: %v", err)
				return result, nil
			}
			logger.ServiceRestart("all", true)
		}

		// Run UI verification commands
		for _, cmd := range cfg.Config.Verify.UI {
			logger.LogPrint("  → %s\n", cmd)
			logger.VerifyCmdStart(cmd)
			startTime := time.Now()
			output, err := runCommand(cfg.ProjectRoot, cmd, cfg.Config.Verify.Timeout)
			duration := time.Since(startTime)
			if err != nil {
				logger.VerifyCmdEnd(cmd, false, output, duration.Nanoseconds())
				result.passed = false
				result.reason = fmt.Sprintf("%s failed: %v\n\n--- Output (last 50 lines) ---\n%s", cmd, err, output)
				return result, nil
			}
			logger.VerifyCmdEnd(cmd, true, output, duration.Nanoseconds())
			if logger.config != nil && logger.config.ConsoleDurations {
				logger.LogPrint("    ✓ (%s)\n", FormatDuration(duration))
			}
		}
	}

	// Check service health after all verification
	if svcMgr != nil && svcMgr.HasServices() {
		if healthIssues := svcMgr.CheckServiceHealth(); len(healthIssues) > 0 {
			reason := fmt.Sprintf("service health check failed: %s", strings.Join(healthIssues, "; "))
			for _, issue := range healthIssues {
				logger.ServiceHealth("", false, issue)
			}
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
		for _, svc := range cfg.Config.Services {
			logger.ServiceHealth(svc.Name, true, "")
		}
	}

	return result, nil
}

// --- VerifyReport for ralph verify ---

// VerifyItem represents a single verification check result
type VerifyItem struct {
	Name    string
	Passed  bool
	Output  string // truncated command output for failures
	Warning bool   // true for WARN-type items
}

// VerifyReport collects all verification results
type VerifyReport struct {
	Items     []VerifyItem
	AllPassed bool
	FailCount int
	WarnCount int
}

func (r *VerifyReport) AddPass(name string) {
	r.Items = append(r.Items, VerifyItem{Name: name, Passed: true})
}

func (r *VerifyReport) AddFail(name, output string) {
	r.Items = append(r.Items, VerifyItem{Name: name, Passed: false, Output: output})
}

func (r *VerifyReport) AddWarn(name, detail string) {
	r.Items = append(r.Items, VerifyItem{Name: name, Warning: true, Output: detail})
}

func (r *VerifyReport) Finalize() {
	r.AllPassed = true
	r.FailCount = 0
	r.WarnCount = 0
	for _, item := range r.Items {
		if item.Warning {
			r.WarnCount++
		} else if !item.Passed {
			r.FailCount++
			r.AllPassed = false
		}
	}
}

// FormatForConsole returns human-readable output for terminal
func (r *VerifyReport) FormatForConsole() string {
	var lines []string
	lines = append(lines, "Verification Results")
	lines = append(lines, strings.Repeat("=", 60))
	for _, item := range r.Items {
		if item.Warning {
			lines = append(lines, fmt.Sprintf("  ⚠ %s", item.Name))
			if item.Output != "" {
				lines = append(lines, fmt.Sprintf("      %s", item.Output))
			}
		} else if item.Passed {
			lines = append(lines, fmt.Sprintf("  ✓ %s", item.Name))
		} else {
			lines = append(lines, fmt.Sprintf("  ✗ %s", item.Name))
			if item.Output != "" {
				outputLines := strings.Split(item.Output, "\n")
				if len(outputLines) > 10 {
					outputLines = outputLines[len(outputLines)-10:]
				}
				for _, ol := range outputLines {
					if ol != "" {
						lines = append(lines, fmt.Sprintf("      %s", ol))
					}
				}
			}
		}
	}
	lines = append(lines, strings.Repeat("=", 60))

	passCount := 0
	for _, item := range r.Items {
		if item.Passed && !item.Warning {
			passCount++
		}
	}
	lines = append(lines, fmt.Sprintf("  %d passed, %d failed, %d warnings", passCount, r.FailCount, r.WarnCount))
	return strings.Join(lines, "\n")
}

// FormatForPrompt returns detailed output suitable for AI prompt context
func (r *VerifyReport) FormatForPrompt() string {
	var lines []string
	for _, item := range r.Items {
		if item.Warning {
			lines = append(lines, fmt.Sprintf("WARN: %s — %s", item.Name, item.Output))
		} else if item.Passed {
			lines = append(lines, fmt.Sprintf("PASS: %s", item.Name))
		} else {
			lines = append(lines, fmt.Sprintf("FAIL: %s", item.Name))
			if item.Output != "" {
				lines = append(lines, item.Output)
			}
		}
	}
	return strings.Join(lines, "\n")
}

// runVerifyChecks runs all verification gates and returns structured results.
func runVerifyChecks(cfg *ResolvedConfig, featureDir *FeatureDir, def *PRDDefinition, state *RunState, svcMgr *ServiceManager, logger *RunLogger, resourceGuidance string) (*VerifyReport, error) {
	report := &VerifyReport{}

	// 1. Run verify.default commands
	for _, cmd := range cfg.Config.Verify.Default {
		logger.LogPrint("  → %s\n", cmd)
		logger.VerifyCmdStart(cmd)
		startTime := time.Now()
		output, err := runCommand(cfg.ProjectRoot, cmd, cfg.Config.Verify.Timeout)
		duration := time.Since(startTime)
		if err != nil {
			logger.VerifyCmdEnd(cmd, false, output, duration.Nanoseconds())
			report.AddFail(fmt.Sprintf("%s (%s)", cmd, FormatDuration(duration)), output)
		} else {
			logger.VerifyCmdEnd(cmd, true, output, duration.Nanoseconds())
			report.AddPass(fmt.Sprintf("%s (%s)", cmd, FormatDuration(duration)))
		}
	}

	// 2. Run verify.ui commands
	for _, cmd := range cfg.Config.Verify.UI {
		logger.LogPrint("  → %s\n", cmd)
		logger.VerifyCmdStart(cmd)
		startTime := time.Now()
		output, err := runCommand(cfg.ProjectRoot, cmd, cfg.Config.Verify.Timeout)
		duration := time.Since(startTime)
		if err != nil {
			logger.VerifyCmdEnd(cmd, false, output, duration.Nanoseconds())
			report.AddFail(fmt.Sprintf("%s (%s)", cmd, FormatDuration(duration)), output)
		} else {
			logger.VerifyCmdEnd(cmd, true, output, duration.Nanoseconds())
			report.AddPass(fmt.Sprintf("%s (%s)", cmd, FormatDuration(duration)))
		}
	}

	// 3. Service health checks
	if svcMgr != nil && svcMgr.HasServices() {
		if healthIssues := svcMgr.CheckServiceHealth(); len(healthIssues) > 0 {
			for _, issue := range healthIssues {
				logger.ServiceHealth("", false, issue)
			}
			report.AddFail("service health", strings.Join(healthIssues, "; "))
		} else {
			for _, svc := range cfg.Config.Services {
				logger.ServiceHealth(svc.Name, true, "")
			}
			report.AddPass("service health")
		}
	}

	// 4. Knowledge file check
	git := NewGitOps(cfg.ProjectRoot)
	knowledgeFile := cfg.Config.Provider.KnowledgeFile
	if knowledgeFile != "" {
		if git.HasFileChanged(knowledgeFile) {
			report.AddPass(fmt.Sprintf("%s was updated", knowledgeFile))
		} else {
			report.AddWarn(knowledgeFile, "was NOT modified on this branch")
		}
	}

	// 5. Test file heuristic
	if git.HasTestFileChanges() {
		report.AddPass("test files were modified")
	} else {
		report.AddWarn("test files", "no test files were modified on this branch")
	}

	// 6. AI deep verification (always runs during ralph verify)
	logger.LogPrintln("  → AI verification analysis...")
	analyzePrompt := generateVerifyAnalyzePrompt(cfg, featureDir, def, state, report, resourceGuidance)
	aiResult, aiErr := runVerifySubagent(cfg, analyzePrompt)
	if aiErr != nil {
		report.AddFail("AI analysis", aiErr.Error())
	} else if !aiResult.Passed {
		for _, f := range aiResult.Failures {
			report.AddFail("AI analysis", f)
		}
	} else {
		report.AddPass("AI analysis")
	}

	report.Finalize()
	return report, nil
}

// runVerify runs verification checks and opens an interactive session on failure.
// resourceGuidance is pre-computed consultation guidance passed from cmdVerify.
func runVerify(cfg *ResolvedConfig, featureDir *FeatureDir, resourceGuidance string) error {
	def, err := LoadPRDDefinition(featureDir.PrdJsonPath())
	if err != nil {
		return err
	}
	state, err := LoadRunState(featureDir.RunStatePath())
	if err != nil {
		return err
	}

	// Ensure we're on the correct branch
	git := NewGitOps(cfg.ProjectRoot)
	if err := git.EnsureBranch(def.BranchName); err != nil {
		return fmt.Errorf("failed to switch to branch %s: %w", def.BranchName, err)
	}

	// Initialize logger
	logger, err := NewRunLogger(featureDir.Path, cfg.Config.Logging)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer logger.Close()

	logger.RunStart(featureDir.Feature, def.BranchName, len(def.UserStories))

	// Start services for verification
	svcMgr := NewServiceManager(cfg.ProjectRoot, cfg.Config.Services)
	defer svcMgr.StopAll()
	if svcMgr.HasServices() {
		logger.LogPrintln("Starting services...")
		for _, svc := range cfg.Config.Services {
			logger.ServiceStart(svc.Name, svc.Start)
		}
		startTime := time.Now()
		if err := svcMgr.EnsureRunning(); err != nil {
			logger.Error("failed to start services", err)
			logger.RunEnd(false, "service start failed")
			return fmt.Errorf("failed to start services: %w", err)
		}
		for _, svc := range cfg.Config.Services {
			logger.ServiceReady(svc.Name, svc.Ready, time.Since(startTime).Nanoseconds())
		}
	}

	// Run all verification checks
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println(" Ralph Verify")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf(" Feature: %s\n", featureDir.Feature)
	fmt.Printf(" Branch: %s\n", def.BranchName)
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	report, err := runVerifyChecks(cfg, featureDir, def, state, svcMgr, logger, resourceGuidance)
	if err != nil {
		logger.RunEnd(false, "verification error")
		return err
	}

	// Print results to console
	fmt.Println()
	fmt.Print(report.FormatForConsole())
	fmt.Println()

	if report.AllPassed {
		fmt.Println(strings.Repeat("=", 60))
		logger.LogPrintln(" ✓ All verification passed!")
		fmt.Println(strings.Repeat("=", 60))
		fmt.Println()
		fmt.Println("Ready to merge. Review changes with:")
		fmt.Printf("  git log --oneline %s..HEAD\n", git.DefaultBranch())
		logger.RunEnd(true, "verification passed")
		return nil
	}

	// Verification failed — offer interactive session
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf(" ✗ Verification found %d failure(s), %d warning(s)\n", report.FailCount, report.WarnCount)
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	// Stop services before interactive session (provider may need to restart them)
	svcMgr.StopAll()

	if promptYesNo("Open interactive AI session to investigate and fix?") {
		prompt := generateVerifyFixPrompt(cfg, featureDir, def, state, report, resourceGuidance)
		if err := runProviderInteractive(cfg, prompt); err != nil {
			logger.RunEnd(false, "interactive session error")
			return err
		}
		fmt.Println()
		fmt.Printf("Run 'ralph run %s' to continue implementation, or 'ralph verify %s' to re-check.\n", featureDir.Feature, featureDir.Feature)
	} else {
		fmt.Println()
		fmt.Println("Review the failures above and fix manually.")
		fmt.Printf("Run 'ralph run %s' to continue implementation, or 'ralph verify %s' to re-check.\n", featureDir.Feature, featureDir.Feature)
	}

	logger.RunEnd(false, "verification found issues")
	return nil
}

// runCommand runs a shell command with a per-command timeout.
func runCommand(dir, cmdStr string, timeoutSec int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 100 * time.Millisecond
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
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

// VerifyAnalysisResult contains the result of AI deep verification
type VerifyAnalysisResult struct {
	Passed   bool
	Failures []string
	Output   string
}

// runVerifySubagent spawns a non-interactive provider to perform AI deep verification.
// Scans output for VERIFY_PASS/VERIFY_FAIL markers.
func runVerifySubagent(cfg *ResolvedConfig, prompt string) (*VerifyAnalysisResult, error) {
	timeout := time.Duration(cfg.Config.Provider.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	p := cfg.Config.Provider
	args, promptFile, err := buildProviderArgs(p.Args, p.PromptMode, p.PromptFlag, prompt)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, p.Command, args...)
	cmd.Dir = cfg.ProjectRoot
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 5 * time.Second

	// Setup stdin pipe for stdin mode
	var stdinPipe io.WriteCloser
	if p.PromptMode == "stdin" || p.PromptMode == "" {
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
		return nil, fmt.Errorf("failed to start verify subagent: %w", err)
	}

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

	// Collect output and scan for markers
	result := &VerifyAnalysisResult{}
	var mu sync.Mutex
	var outputBuilder strings.Builder

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		s := bufio.NewScanner(stderr)
		s.Buffer(make([]byte, 64*1024), 1024*1024)
		for s.Scan() {
			line := s.Text()
			mu.Lock()
			outputBuilder.WriteString(line + "\n")
			processVerifyLine(line, result)
			mu.Unlock()
		}
	}()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		mu.Lock()
		outputBuilder.WriteString(line + "\n")
		processVerifyLine(line, result)
		mu.Unlock()
	}

	wg.Wait()
	cmd.Wait()
	result.Output = outputBuilder.String()

	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("verify subagent timed out after %v", timeout)
	}

	// If no markers detected at all, treat as a failure
	if !result.Passed && len(result.Failures) == 0 {
		result.Failures = append(result.Failures, "AI verification did not produce a verdict (no VERIFY_PASS or VERIFY_FAIL marker)")
	}

	return result, nil
}

// processVerifyLine scans a line for VERIFY_PASS/VERIFY_FAIL markers.
func processVerifyLine(line string, result *VerifyAnalysisResult) {
	trimmed := strings.TrimSpace(line)

	if VerifyPassPattern.MatchString(trimmed) {
		result.Passed = true
	}
	if matches := VerifyFailPattern.FindStringSubmatch(trimmed); len(matches) > 1 {
		result.Failures = append(result.Failures, strings.TrimSpace(matches[1]))
	}
}

// commitPrdOnly commits a single file (typically run-state.json during runs)
func commitPrdOnly(projectRoot, filePath, message string) error {
	git := NewGitOps(projectRoot)
	return git.CommitFile(filePath, message)
}

