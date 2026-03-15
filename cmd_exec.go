package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Scrip provider signal markers
const (
	ScripDoneMarker  = "<scrip>DONE</scrip>"
	ScripStuckMarker = "<scrip>STUCK</scrip>"
	scripMaxRetries  = 3
)

var (
	scripLearningRe  = regexp.MustCompile(`^<scrip>LEARNING:(.+?)</scrip>$`)
	scripStuckNoteRe = regexp.MustCompile(`^<scrip>STUCK:(.+?)</scrip>$`)
)

// cmdExec handles the "scrip exec <feature>" command.
// Also supports quick-fix mode: scrip exec "fix something"
func cmdExec(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: scrip exec <feature>")
		fmt.Fprintln(os.Stderr, "       scrip exec \"fix description\"")
		os.Exit(1)
	}

	feature := args[0]
	projectRoot := GetProjectRoot()

	cfg, err := LoadScripConfig(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	featureDir, err := FindFeatureDir(projectRoot, feature, false)
	if err != nil {
		// Quick-fix mode: if feature contains spaces, treat as a description
		if strings.Contains(feature, " ") {
			if qfErr := scripQuickFix(cfg, projectRoot, feature); qfErr != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", qfErr)
				os.Exit(1)
			}
			return
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	planPath := filepath.Join(featureDir.Path, "plan.md")
	planContent, err := os.ReadFile(planPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: plan.md not found in %s\n\nRun 'scrip plan %s' first.\n", featureDir.Path, feature)
		os.Exit(1)
	}

	plan, err := ParsePlan(string(planContent))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing plan.md: %v\n", err)
		os.Exit(1)
	}

	if len(plan.Items) == 0 {
		fmt.Fprintln(os.Stderr, "Error: plan.md contains no items")
		os.Exit(1)
	}

	if err := scripExecLoop(cfg, featureDir, plan); err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}
}

// scripQuickFix creates a 1-item plan from a description and executes it
// in the most recent feature directory.
func scripQuickFix(cfg *ScripResolvedConfig, projectRoot, description string) error {
	features, err := ListFeatures(projectRoot)
	if err != nil || len(features) == 0 {
		return fmt.Errorf("no feature directory found — run 'scrip plan <feature>' first")
	}

	featureDir := &features[0]
	plan := &Plan{
		Feature:   featureDir.Feature,
		Created:   time.Now().UTC().Format(time.RFC3339),
		ItemCount: 1,
		Items: []PlanItem{
			{
				Title:      description,
				Acceptance: []string{"Fix applied and all verification commands pass"},
			},
		},
	}

	return scripExecLoop(cfg, featureDir, plan)
}

// scripExecLoop is the autonomous execution loop for scrip exec.
// Iterates plan items, spawns claude per item, tracks progress in progress.jsonl.
func scripExecLoop(cfg *ScripResolvedConfig, featureDir *FeatureDir, plan *Plan) error {
	git := NewGitOps(cfg.ProjectRoot)
	progressPath := filepath.Join(featureDir.Path, "progress.jsonl")
	progressMdFile := filepath.Join(featureDir.Path, "progress.md")
	statePath := filepath.Join(featureDir.Path, "state.json")

	// Create cleanup coordinator for signal handling
	cleanup := NewCleanupCoordinator()

	// Initialize logger
	logger, err := NewRunLogger(featureDir.Path, DefaultLoggingConfig())
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	cleanup.SetLogger(logger)
	defer logger.Close()

	// Acquire lock
	lock := NewLockFile(cfg.ProjectRoot)
	if err := lock.Acquire(featureDir.Feature, "plan/"+featureDir.Feature); err != nil {
		return err
	}
	cleanup.SetLock(lock)
	defer lock.Release()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n\nInterrupted. Cleaning up...")
		cleanup.Cleanup()
		os.Exit(130)
	}()

	// Ensure feature branch
	branchName := "plan/" + featureDir.Feature
	if err := git.EnsureBranch(branchName); err != nil {
		return fmt.Errorf("failed to switch to branch %s: %w", branchName, err)
	}

	// Print banner
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println(" Scrip — Autonomous Execution Loop")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf(" Feature:  %s\n", featureDir.Feature)
	fmt.Printf(" Branch:   %s\n", branchName)
	fmt.Printf(" Items:    %d\n", len(plan.Items))
	if logger.LogPath() != "" {
		fmt.Printf(" Logs:     %s\n", logger.LogPath())
	}
	fmt.Println(strings.Repeat("=", 60))

	// Start services if configured
	var svcMgr *ServiceManager
	if len(cfg.Config.Services) > 0 {
		svcs := toServiceConfigs(cfg.Config.Services)
		svcMgr = NewServiceManager(cfg.ProjectRoot, svcs)
		cleanup.SetServiceManager(svcMgr)
		defer svcMgr.StopAll()

		fmt.Println("\nStarting services...")
		if err := svcMgr.EnsureRunning(); err != nil {
			return fmt.Errorf("failed to start services: %w", err)
		}
	}

	// Discover codebase context
	codebaseCtx := DiscoverScripCodebase(cfg.ProjectRoot, &cfg.Config)
	codebaseStr := FormatCodebaseContext(codebaseCtx)

	// Sync resources
	rm := ensureScripResourceSync(cfg, codebaseCtx)

	// Log exec start
	logger.RunStart(featureDir.Feature, branchName, len(plan.Items))
	_ = AppendProgressEvent(progressPath, &ProgressEvent{
		Event:     ProgressExecStart,
		Feature:   featureDir.Feature,
		PlanItems: len(plan.Items),
	})

	// Clean up stale state.json from previous crash
	if stale, _ := LoadSessionState(statePath); stale != nil {
		if !IsProviderAlive(stale) {
			_ = DeleteSessionState(statePath)
		}
	}

	iteration := 0
	var sessionLearnings []string

	for {
		iteration++
		logger.SetIteration(iteration)

		// Reload progress events at start of each iteration
		events, err := LoadProgressEvents(progressPath)
		if err != nil {
			return fmt.Errorf("failed to load progress: %w", err)
		}

		// Check if all items complete
		if AllItemsComplete(plan, events) {
			passed := CountItemsPassed(events)
			skipped := CountItemsSkipped(events)

			fmt.Println()
			fmt.Println(strings.Repeat("=", 60))
			fmt.Println(" All items complete!")
			fmt.Println(strings.Repeat("=", 60))
			fmt.Printf(" %d passed, %d skipped\n", passed, skipped)

			if len(sessionLearnings) > 0 {
				fmt.Printf("\nLearnings captured (%d):\n", len(sessionLearnings))
				for i, l := range sessionLearnings {
					fmt.Printf("  %d. %s\n", i+1, l)
				}
			}

			_ = AppendProgressEvent(progressPath, &ProgressEvent{
				Event:   ProgressExecEnd,
				Passed:  passed,
				Skipped: skipped,
			})

			// Append session narrative to progress.md
			narrative := scripBuildSessionNarrative(plan, events, sessionLearnings)
			if narrative != "" {
				_ = AppendProgressMd(progressMdFile, narrative)
			}

			_ = DeleteSessionState(statePath)
			logger.RunEnd(true, "all items complete")
			fmt.Printf("\nRun 'scrip land %s' to verify and push.\n", featureDir.Feature)
			return nil
		}

		// Get next item (first non-passed, non-skipped with deps resolved)
		item := GetNextItem(plan, events)
		if item == nil {
			passed := CountItemsPassed(events)
			skipped := CountItemsSkipped(events)

			fmt.Println()
			fmt.Println(strings.Repeat("=", 60))
			fmt.Println(" All remaining items are blocked or skipped")
			fmt.Println(strings.Repeat("=", 60))

			states := ComputeAllItemStates(plan, events)
			for _, pi := range plan.Items {
				s := states[pi.Title]
				if s.Skipped {
					line := fmt.Sprintf("  ✗ %s", pi.Title)
					if s.LastFailure != "" {
						line += " — " + s.LastFailure
					}
					fmt.Println(line)
				} else if !s.Passed {
					fmt.Printf("  ○ %s (blocked)\n", pi.Title)
				}
			}

			_ = AppendProgressEvent(progressPath, &ProgressEvent{
				Event:   ProgressExecEnd,
				Passed:  passed,
				Skipped: skipped,
			})

			narrative := scripBuildSessionNarrative(plan, events, sessionLearnings)
			if narrative != "" {
				_ = AppendProgressMd(progressMdFile, narrative)
			}

			_ = DeleteSessionState(statePath)
			logger.RunEnd(false, "all remaining items blocked or skipped")
			return fmt.Errorf("all remaining items blocked or skipped")
		}

		itemState := ComputeItemState(item.Title, events)

		// Verify-at-top: if previously attempted, check if it already passes.
		// Prevents re-implementing items that now pass due to other items' work.
		if itemState.Attempted && !itemState.Passed {
			fmt.Printf("\n  Verifying %s (previously attempted)...\n", item.Title)
			vResult := scripRunVerify(cfg.ProjectRoot, &cfg.Config.Verify, cfg.Config.Provider.Timeout, logger)
			if vResult.passed {
				fmt.Printf("  ✓ %s already passes verification\n", item.Title)
				_ = AppendProgressEvent(progressPath, &ProgressEvent{
					Event:  ProgressItemDone,
					Item:   item.Title,
					Status: "passed",
				})
				continue
			}
		}

		// Print iteration header
		fmt.Println()
		fmt.Println(strings.Repeat("=", 60))
		fmt.Printf(" Item %d/%d: %s\n", scripItemIndex(plan, item)+1, len(plan.Items), item.Title)
		if itemState.Attempts > 0 {
			fmt.Printf(" Attempt: %d/%d\n", itemState.Attempts+1, scripMaxRetries)
		}
		fmt.Println(strings.Repeat("=", 60))

		logger.SetCurrentItem(item.Title)
		logger.IterationStart(item.Title, item.Title, itemState.Attempts)

		// Log item_start to progress
		_ = AppendProgressEvent(progressPath, &ProgressEvent{
			Event:    ProgressItemStart,
			Item:     item.Title,
			Criteria: item.Acceptance,
			Attempt:  itemState.Attempts + 1,
		})

		// Save session state (crash recovery hint)
		sessState := NewSessionState(item.Title, itemState.Attempts+1, "scrip-exec")
		_ = SaveSessionState(statePath, sessState)

		// Capture commit hash BEFORE provider runs
		preRunCommit := git.GetLastCommit()

		// Build resource guidance via per-item consultation
		var resourceGuidance string
		if rm != nil && rm.HasDetectedResources() {
			fmt.Println("  Consulting frameworks...")
			resourceGuidance = consultForItem(cfg.ProjectRoot, featureDir, item, scripItemIndex(plan, item), rm, codebaseCtx.TechStack, logger)
		} else {
			resourceGuidance = buildResourceFallbackInstructions()
		}

		// Build retry context
		retryContext := ""
		if itemState.Attempts > 0 && itemState.LastFailure != "" {
			retryContext = fmt.Sprintf(
				"## Retry Context\n\nYou are retrying (attempt %d of %d). Previous attempt failed:\n%s\n\nDo NOT re-implement from scratch. Focus on the specific failure and try a different approach.",
				itemState.Attempts+1, scripMaxRetries, itemState.LastFailure,
			)
			// Append diff from previous failed attempt if available
			if itemState.LastCommit != "" {
				diff, err := git.DiffBetweenCommits(preRunCommit, itemState.LastCommit)
				if err == nil && diff != "" {
					const maxDiffBytes = 8192
					if len(diff) > maxDiffBytes {
						diff = diff[:maxDiffBytes] + "\n... (truncated)"
					}
					retryContext += fmt.Sprintf("\n\n## Previous Attempt Diff\n\n```diff\n%s\n```", diff)
				}
			}
		}

		// Collect learnings from progress events for prompt injection
		allLearnings := CollectLearnings(events)
		learningsStr := buildLearnings(allLearnings, "## Learnings from Previous Items")

		// Get progress context from progress.md
		progressCtx := GetProgressContext(progressMdFile)

		// Build prompt
		prompt := generateExecBuildPrompt(item, codebaseStr, resourceGuidance, learningsStr, retryContext, progressCtx)

		// Spawn provider
		fmt.Println("  Provider running...")
		logger.ProviderStart()
		providerStart := time.Now()

		result, provErr := scripSpawnProvider(cfg.ProjectRoot, prompt, cfg.Config.Provider.Timeout, cfg.Config.Provider.StallTimeout, true, logger, cleanup, sessState, statePath)

		// Log provider end
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
			logger.ProviderEnd(result.ExitCode, result.TimedOut, detectedMarkers)
		} else {
			logger.ProviderEnd(0, false, nil)
		}
		fmt.Printf("  Provider done (%s)\n", FormatDuration(time.Since(providerStart)))

		// Process learnings (always, even on error)
		if result != nil {
			for _, learning := range result.Learnings {
				sessionLearnings = appendLearningDeduped(sessionLearnings, learning)
				logger.Learning(learning)
				_ = AppendProgressEvent(progressPath, &ProgressEvent{
					Event: ProgressLearning,
					Text:  learning,
				})
			}
		}

		// Clear provider from session state
		sessState.ClearProvider()
		_ = SaveSessionState(statePath, sessState)

		// Handle provider timeout — retryable (same path as STUCK)
		if provErr != nil && result != nil && result.TimedOut {
			reason := fmt.Sprintf("Provider timed out after %d seconds", cfg.Config.Provider.Timeout)
			fmt.Printf("\n  ⏱ %s\n", reason)

			_ = AppendProgressEvent(progressPath, &ProgressEvent{
				Event:   ProgressItemStuck,
				Item:    item.Title,
				Attempt: itemState.Attempts + 1,
				Reason:  reason,
			})

			if itemState.Attempts+1 >= scripMaxRetries {
				fmt.Printf("  ! Skipping %s after %d attempts\n", item.Title, scripMaxRetries)
				_ = AppendProgressEvent(progressPath, &ProgressEvent{
					Event:  ProgressItemDone,
					Item:   item.Title,
					Status: "skipped",
				})
			}

			logger.IterationEnd(false)
			continue
		}

		// Handle provider error (failed to start — result is nil)
		if provErr != nil {
			logger.Error("provider error", provErr)
			logger.IterationEnd(false)
			logger.RunEnd(false, "provider error")
			_ = DeleteSessionState(statePath)
			return fmt.Errorf("provider error: %w", provErr)
		}

		// Handle STUCK marker
		if result.Stuck {
			reason := result.StuckNote
			if reason == "" {
				reason = "Provider signaled STUCK"
			}
			fmt.Printf("\n  ! Stuck: %s\n", reason)

			_ = AppendProgressEvent(progressPath, &ProgressEvent{
				Event:  ProgressItemStuck,
				Item:   item.Title,
				Reason: reason,
			})

			// Auto-skip if exceeded retries
			if itemState.Attempts+1 >= scripMaxRetries {
				fmt.Printf("  ! Skipping %s after %d attempts\n", item.Title, scripMaxRetries)
				_ = AppendProgressEvent(progressPath, &ProgressEvent{
					Event:  ProgressItemDone,
					Item:   item.Title,
					Status: "skipped",
				})
			}

			logger.IterationEnd(false)
			continue
		}

		// Handle missing DONE marker
		if !result.Done {
			fmt.Println("\n  ! Provider did not signal completion. Retrying...")
			_ = AppendProgressEvent(progressPath, &ProgressEvent{
				Event:  ProgressItemStuck,
				Item:   item.Title,
				Reason: "Provider did not signal completion",
			})

			if itemState.Attempts+1 >= scripMaxRetries {
				_ = AppendProgressEvent(progressPath, &ProgressEvent{
					Event:  ProgressItemDone,
					Item:   item.Title,
					Status: "skipped",
				})
			}

			logger.IterationEnd(false)
			continue
		}

		// Check that provider actually committed something
		if !git.HasNewCommitSince(preRunCommit) {
			fmt.Println("\n  ! Provider signaled DONE but made no new commit.")
			_ = AppendProgressEvent(progressPath, &ProgressEvent{
				Event:  ProgressItemStuck,
				Item:   item.Title,
				Reason: "No commit made — provider signaled DONE without committing code",
			})

			if itemState.Attempts+1 >= scripMaxRetries {
				_ = AppendProgressEvent(progressPath, &ProgressEvent{
					Event:  ProgressItemDone,
					Item:   item.Title,
					Status: "skipped",
				})
			}

			logger.IterationEnd(false)
			continue
		}

		// Warn if working tree is dirty
		if !git.IsWorkingTreeClean() {
			fmt.Println("\n  ! Working tree has uncommitted changes after provider finished.")
			logger.Warning("working tree has uncommitted changes after provider finished")
		}

		// Run verification commands
		fmt.Println("\n  Running verification...")
		logger.VerifyStart()
		vResult := scripRunVerify(cfg.ProjectRoot, &cfg.Config.Verify, cfg.Config.Provider.Timeout, logger)

		if !vResult.passed {
			fmt.Printf("\n  Verification failed: %s\n", vResult.reason)
			logger.VerifyEnd(false)

			_ = AppendProgressEvent(progressPath, &ProgressEvent{
				Event:  ProgressItemStuck,
				Item:   item.Title,
				Reason: vResult.reason,
			})

			if itemState.Attempts+1 >= scripMaxRetries {
				_ = AppendProgressEvent(progressPath, &ProgressEvent{
					Event:  ProgressItemDone,
					Item:   item.Title,
					Status: "skipped",
				})
			}

			logger.IterationEnd(false)
			continue
		}
		logger.VerifyEnd(true)

		// AI deep analysis — adversarial verification of the implementation
		fmt.Println("\n  Running AI deep analysis...")
		itemDiff := git.DiffSince(preRunCommit)
		verifyPrompt := generateExecVerifyPrompt(item, itemDiff, vResult.output)
		analyzeResult, analyzeErr := scripSpawnProvider(cfg.ProjectRoot, verifyPrompt, cfg.Config.Provider.Timeout, cfg.Config.Provider.StallTimeout, false, logger, cleanup, nil, "")

		if analyzeErr == nil && analyzeResult != nil {
			deepPassed, failures := landParseAnalysis(analyzeResult)
			if !deepPassed {
				reason := fmt.Sprintf("AI deep analysis: %s", strings.Join(failures, "; "))
				fmt.Printf("\n  ! %s\n", reason)

				_ = AppendProgressEvent(progressPath, &ProgressEvent{
					Event:  ProgressItemStuck,
					Item:   item.Title,
					Reason: reason,
				})

				if itemState.Attempts+1 >= scripMaxRetries {
					_ = AppendProgressEvent(progressPath, &ProgressEvent{
						Event:  ProgressItemDone,
						Item:   item.Title,
						Status: "skipped",
					})
				}

				logger.IterationEnd(false)
				continue
			}
		} else if analyzeErr != nil {
			// AI deep analysis failed to run — log warning but don't block
			fmt.Printf("\n  ! AI deep analysis unavailable: %v\n", analyzeErr)
		}

		// Item passed!
		lastCommit := git.GetLastCommit()
		_ = AppendProgressEvent(progressPath, &ProgressEvent{
			Event:     ProgressItemDone,
			Item:      item.Title,
			Status:    "passed",
			Commit:    lastCommit,
			Learnings: result.Learnings,
		})

		fmt.Printf("\n  ✓ %s complete\n", item.Title)
		logger.IterationEnd(true)
	}
}

// scripSpawnProvider spawns claude with scrip-specific args and processes output.
// Always uses stdin mode (claude reads prompt from stdin).
// stallTimeoutSec controls idle-output detection: if the provider produces no output
// for this many seconds, it is killed (same as hard timeout). 0 disables stall detection.
func scripSpawnProvider(projectRoot, prompt string, timeoutSec, stallTimeoutSec int, autonomous bool, logger *RunLogger, cleanup *CleanupCoordinator, sessState *SessionState, statePath string) (*ProviderResult, error) {
	timeout := time.Duration(timeoutSec) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := ScripProviderArgs(autonomous)
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = projectRoot
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 5 * time.Second

	stdinPipe, err := cmd.StdinPipe()
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
		return nil, fmt.Errorf("failed to start claude: %w", err)
	}

	if cleanup != nil {
		cleanup.SetProvider(cmd)
		defer cleanup.ClearProvider()
	}

	// Record provider PID in session state for crash recovery
	if sessState != nil && statePath != "" {
		sessState.SetProvider(cmd.Process.Pid)
		_ = SaveSessionState(statePath, sessState)
	}

	// Write prompt to stdin
	go func() {
		defer stdinPipe.Close()
		io.WriteString(stdinPipe, prompt)
	}()

	// Collect output with marker detection
	var mu sync.Mutex
	var outputBuilder strings.Builder
	result := &ProviderResult{}

	// Stall timeout: kill provider if no output for stallTimeoutSec seconds.
	// Activity channel is buffered so scanner loops never block on send.
	var stallTimedOut atomic.Bool
	activityCh := make(chan struct{}, 1)
	if stallTimeoutSec > 0 {
		stallDuration := time.Duration(stallTimeoutSec) * time.Second
		go func() {
			stallTimer := time.NewTimer(stallDuration)
			defer stallTimer.Stop()
			for {
				select {
				case <-activityCh:
					if !stallTimer.Stop() {
						select {
						case <-stallTimer.C:
						default:
						}
					}
					stallTimer.Reset(stallDuration)
				case <-stallTimer.C:
					stallTimedOut.Store(true)
					cancel()
					return
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// notifyActivity sends a non-blocking signal on the activity channel.
	notifyActivity := func() {
		select {
		case activityCh <- struct{}{}:
		default:
		}
	}

	var wg sync.WaitGroup
	wg.Add(1)

	// Read stderr
	go func() {
		defer wg.Done()
		s := bufio.NewScanner(stderr)
		s.Buffer(make([]byte, 64*1024), 1024*1024)
		for s.Scan() {
			line := s.Text()
			notifyActivity()
			if logger != nil {
				logger.ProviderLine("stderr", line)
			}
			mu.Lock()
			outputBuilder.WriteString(line + "\n")
			scripProcessLine(line, result, logger)
			mu.Unlock()
		}
	}()

	// Read stdout
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		notifyActivity()
		if logger != nil {
			logger.ProviderLine("stdout", line)
		}
		mu.Lock()
		outputBuilder.WriteString(line + "\n")
		scripProcessLine(line, result, logger)
		mu.Unlock()
	}

	if scanErr := scanner.Err(); scanErr != nil && logger != nil {
		logger.Warning(fmt.Sprintf("stdout scanner error (possible line >1MB): %v", scanErr))
	}

	wg.Wait()

	err = cmd.Wait()
	result.Output = outputBuilder.String()

	if ctx.Err() == context.DeadlineExceeded || stallTimedOut.Load() {
		result.TimedOut = true
		if stallTimedOut.Load() {
			return result, fmt.Errorf("provider stalled (no output for %ds)", stallTimeoutSec)
		}
		return result, fmt.Errorf("provider timed out after %v", timeout)
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
		return result, nil // Non-zero exit is not an error — markers determine outcome
	}

	result.ExitCode = 0
	return result, nil
}

// scripProcessLine detects scrip markers in provider output.
// Uses whole-line matching (after trimming) to prevent marker spoofing.
func scripProcessLine(line string, result *ProviderResult, logger *RunLogger) {
	trimmed := strings.TrimSpace(line)

	if trimmed == ScripDoneMarker {
		result.Done = true
		if logger != nil {
			logger.MarkerDetected("DONE", "")
			logger.LogPrintln("  ◆ DONE")
		}
	}

	if trimmed == ScripStuckMarker {
		result.Stuck = true
		if logger != nil {
			logger.MarkerDetected("STUCK", "")
			logger.LogPrintln("  ◆ STUCK")
		}
	}

	if matches := scripStuckNoteRe.FindStringSubmatch(trimmed); len(matches) > 1 {
		result.Stuck = true
		result.StuckNote = strings.TrimSpace(matches[1])
		if logger != nil {
			logger.MarkerDetected("STUCK", result.StuckNote)
			logger.LogPrint("  ◆ STUCK: %s\n", result.StuckNote)
		}
	}

	if matches := scripLearningRe.FindStringSubmatch(trimmed); len(matches) > 1 {
		value := strings.TrimSpace(matches[1])
		result.Learnings = append(result.Learnings, value)
		if logger != nil {
			logger.MarkerDetected("LEARNING", value)
			logger.LogPrint("  ~ LEARNING: %s\n", value)
		}
	}
}

// scripRunVerify runs verification commands from ScripVerifyConfig.
// Returns a result indicating pass/fail with reason.
func scripRunVerify(projectRoot string, verify *ScripVerifyConfig, timeoutSec int, logger *RunLogger) *ItemVerifyResult {
	result := &ItemVerifyResult{passed: true}
	var allOutput []string

	for _, cmd := range verify.VerifyCommands() {
		if logger != nil {
			logger.LogPrint("  → %s\n", cmd)
			logger.VerifyCmdStart(cmd)
		}
		startTime := time.Now()
		output, err := runCommand(projectRoot, cmd, timeoutSec)
		duration := time.Since(startTime)

		if err != nil {
			if logger != nil {
				logger.VerifyCmdEnd(cmd, false, output, duration.Nanoseconds())
			}
			result.passed = false
			result.reason = fmt.Sprintf("%s failed: %v\n\n--- Output (last 50 lines) ---\n%s", cmd, err, output)
			return result
		}

		allOutput = append(allOutput, fmt.Sprintf("$ %s\n%s", cmd, output))

		if logger != nil {
			logger.VerifyCmdEnd(cmd, true, output, duration.Nanoseconds())
			if logger.config != nil && logger.config.ConsoleDurations {
				logger.LogPrint("    ✓ (%s)\n", FormatDuration(duration))
			}
		}
	}

	result.output = strings.Join(allOutput, "\n\n")
	return result
}

// generateExecBuildPrompt builds the exec-build.md prompt for item implementation.
func generateExecBuildPrompt(item *PlanItem, codebaseStr, resourceGuidance, learningsStr, retryContext, progressCtx string) string {
	var criteriaLines []string
	for _, c := range item.Acceptance {
		criteriaLines = append(criteriaLines, "- "+c)
	}

	return getPrompt("exec-build", map[string]string{
		"item":            item.Title,
		"criteria":        strings.Join(criteriaLines, "\n"),
		"consultation":    resourceGuidance,
		"learnings":       learningsStr,
		"retryContext":    retryContext,
		"codebaseContext": codebaseStr,
		"progressContext": progressCtx,
	})
}

// generateExecVerifyPrompt builds the exec-verify.md prompt for AI deep analysis.
func generateExecVerifyPrompt(item *PlanItem, diff, testOutput string) string {
	var criteriaLines []string
	for _, c := range item.Acceptance {
		criteriaLines = append(criteriaLines, "- "+c)
	}

	return getPrompt("exec-verify", map[string]string{
		"item":       item.Title,
		"criteria":   strings.Join(criteriaLines, "\n"),
		"diff":       diff,
		"testOutput": testOutput,
	})
}

// toServiceConfigs converts ScripServiceConfig to ServiceConfig for the ServiceManager.
func toServiceConfigs(services []ScripServiceConfig) []ServiceConfig {
	var result []ServiceConfig
	for _, svc := range services {
		result = append(result, ServiceConfig{
			Name:         svc.Name,
			Start:        svc.Command,
			Ready:        svc.Ready,
			ReadyTimeout: svc.Timeout,
		})
	}
	return result
}

// scripBuildSessionNarrative generates a narrative section for progress.md
// summarizing what was completed, what remains, and what was learned.
func scripBuildSessionNarrative(plan *Plan, events []ProgressEvent, learnings []string) string {
	states := ComputeAllItemStates(plan, events)
	var completed, remaining []string

	for _, item := range plan.Items {
		s := states[item.Title]
		if s.Passed {
			completed = append(completed, "- ✓ "+item.Title)
		} else if s.Skipped {
			reason := s.LastFailure
			if reason == "" {
				reason = "exceeded retries"
			}
			completed = append(completed, fmt.Sprintf("- ✗ %s (skipped: %s)", item.Title, reason))
		} else {
			remaining = append(remaining, "- "+item.Title)
		}
	}

	timestamp := time.Now().UTC().Format("2006-01-02 15:04")
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("## %s — Execution Session\n\n", timestamp))

	if len(completed) > 0 {
		buf.WriteString("### Completed\n")
		buf.WriteString(strings.Join(completed, "\n"))
		buf.WriteString("\n\n")
	}

	if len(remaining) > 0 {
		buf.WriteString("### Remaining\n")
		buf.WriteString(strings.Join(remaining, "\n"))
		buf.WriteString("\n\n")
	}

	if len(learnings) > 0 {
		buf.WriteString("### Learnings\n")
		for _, l := range learnings {
			buf.WriteString("- " + l + "\n")
		}
	}

	return buf.String()
}

// scripItemIndex returns the 0-based index of an item in the plan.
func scripItemIndex(plan *Plan, item *PlanItem) int {
	for i := range plan.Items {
		if plan.Items[i].Title == item.Title {
			return i
		}
	}
	return 0
}

// appendLearningDeduped appends a learning to the slice if not already present.
func appendLearningDeduped(learnings []string, learning string) []string {
	normalized := normalizeLearning(learning)
	for _, existing := range learnings {
		if normalizeLearning(existing) == normalized {
			return learnings
		}
	}
	return append(learnings, learning)
}
