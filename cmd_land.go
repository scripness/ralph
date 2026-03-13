package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// Scrip summary markers for landing
var (
	scripSummaryStartRe = regexp.MustCompile(`^\s*<scrip>SUMMARY_START</scrip>\s*$`)
	scripSummaryEndRe   = regexp.MustCompile(`^\s*<scrip>SUMMARY_END</scrip>\s*$`)
)

const landMaxFixAttempts = 3

// cmdLand handles the "scrip land <feature>" command.
// Runs comprehensive verification, AI analysis, fix loop, summary generation,
// artifact purge, and push.
func cmdLand(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: scrip land <feature>")
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

	if err := landFeature(cfg, featureDir, plan); err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}
}

// landFeature orchestrates the landing verification pipeline:
// verify commands → AI analysis → fix loop (if needed) → summary → purge → commit → push.
func landFeature(cfg *ScripResolvedConfig, featureDir *FeatureDir, plan *Plan) error {
	git := NewGitOps(cfg.ProjectRoot)
	progressPath := filepath.Join(featureDir.Path, "progress.jsonl")
	progressMdFile := filepath.Join(featureDir.Path, "progress.md")
	statePath := filepath.Join(featureDir.Path, "state.json")
	planPath := filepath.Join(featureDir.Path, "plan.md")

	// Initialize logger
	logger, err := NewRunLogger(featureDir.Path, DefaultLoggingConfig())
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer logger.Close()

	// Cleanup coordinator for signal handling
	cleanup := NewCleanupCoordinator()
	cleanup.SetLogger(logger)

	// Acquire lock
	lock := NewLockFile(cfg.ProjectRoot)
	if err := lock.Acquire(featureDir.Feature, "land/"+featureDir.Feature); err != nil {
		return err
	}
	cleanup.SetLock(lock)
	defer lock.Release()

	// Signal handling
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

	// Banner
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println(" Scrip — Landing Verification")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf(" Feature:  %s\n", featureDir.Feature)
	fmt.Printf(" Branch:   %s\n", branchName)
	fmt.Printf(" Items:    %d\n", len(plan.Items))
	fmt.Println(strings.Repeat("=", 60))

	// Step 1: Run verification commands
	fmt.Println("\n  Running verification commands...")
	verifyFormatted, verifyPassed := landRunVerifyCommands(cfg.ProjectRoot, &cfg.Config.Verify, cfg.Config.Provider.Timeout)
	if verifyPassed {
		fmt.Println("  All verification commands passed")
	} else {
		fmt.Println("  Some verification commands failed")
	}

	// Step 2: Gather context for analysis
	fullDiff := git.GetFullDiff()
	allCriteria := landBuildCriteria(plan)
	events, _ := LoadProgressEvents(progressPath)
	learnings := CollectLearnings(events)

	// Build consultation
	codebaseCtx := DiscoverScripCodebase(cfg.ProjectRoot, &cfg.Config)
	rm := ensureScripResourceSync(cfg, codebaseCtx)
	consultation := buildResourceFallbackInstructions()
	_ = rm // consultation falls back to web search instructions when no cached resources

	// Step 3: AI deep analysis
	fmt.Println("\n  Running AI deep analysis...")
	startTime := time.Now()
	analyzePrompt := generateLandAnalyzePrompt(allCriteria, fullDiff, verifyFormatted, consultation)
	analyzeResult, analyzeErr := scripSpawnProvider(cfg.ProjectRoot, analyzePrompt, cfg.Config.Provider.Timeout, false, logger, cleanup)
	fmt.Printf("  Analysis done (%s)\n", FormatDuration(time.Since(startTime)))

	if analyzeErr != nil {
		return fmt.Errorf("analysis provider error: %w", analyzeErr)
	}

	passed, failures := landParseAnalysis(analyzeResult)

	if passed {
		fmt.Println("  Analysis passed")
		return landSuccessPath(cfg, featureDir, plan, git, events, learnings, fullDiff, consultation,
			progressPath, progressMdFile, statePath, planPath, logger, cleanup)
	}

	// Step 4: Analysis failed — run fix loop
	fmt.Printf("\n  Analysis found %d issue(s):\n", len(failures))
	for _, f := range failures {
		fmt.Printf("    - %s\n", f)
	}

	for attempt := 1; attempt <= landMaxFixAttempts; attempt++ {
		fmt.Printf("\n  Fix attempt %d/%d...\n", attempt, landMaxFixAttempts)

		currentDiff := git.GetFullDiff()
		currentVerify, _ := landRunVerifyCommands(cfg.ProjectRoot, &cfg.Config.Verify, cfg.Config.Provider.Timeout)

		fixPrompt := generateLandFixPrompt(failures, currentVerify, currentDiff, consultation)
		fixStart := time.Now()
		fixResult, fixErr := scripSpawnProvider(cfg.ProjectRoot, fixPrompt, cfg.Config.Provider.Timeout, true, logger, cleanup)
		fmt.Printf("    Fix agent done (%s)\n", FormatDuration(time.Since(fixStart)))

		if fixErr != nil {
			fmt.Printf("    Fix provider error: %v\n", fixErr)
			continue
		}

		// Process learnings from fix agent
		if fixResult != nil {
			for _, l := range fixResult.Learnings {
				_ = AppendProgressEvent(progressPath, &ProgressEvent{
					Event: ProgressLearning,
					Text:  l,
				})
			}
		}

		if fixResult == nil || !fixResult.Done {
			stuckReason := "Fix agent did not signal completion"
			if fixResult != nil && fixResult.StuckNote != "" {
				stuckReason = fixResult.StuckNote
			}
			fmt.Printf("    %s\n", stuckReason)
			continue
		}

		// Re-run verification
		fmt.Println("    Re-running verification...")
		_, reVerifyPassed := landRunVerifyCommands(cfg.ProjectRoot, &cfg.Config.Verify, cfg.Config.Provider.Timeout)
		if reVerifyPassed {
			fmt.Println("\n  Verification passed after fix")
			events, _ = LoadProgressEvents(progressPath)
			learnings = CollectLearnings(events)
			currentDiff = git.GetFullDiff()
			return landSuccessPath(cfg, featureDir, plan, git, events, learnings, currentDiff, consultation,
				progressPath, progressMdFile, statePath, planPath, logger, cleanup)
		}
		fmt.Println("    Verification still failing")
	}

	// Fix loop exhausted
	fmt.Println("\n  Landing failed — fix attempts exhausted")
	_ = AppendProgressEvent(progressPath, &ProgressEvent{
		Event:    ProgressLandFailed,
		Findings: failures,
	})

	fmt.Println("\n  Next steps:")
	fmt.Printf("    - Run 'scrip exec %s' to implement targeted fixes\n", featureDir.Feature)
	fmt.Printf("    - Run 'scrip plan %s' to rethink if needed\n", featureDir.Feature)
	fmt.Println("    - Review findings in progress.jsonl")
	return fmt.Errorf("landing failed: fix attempts exhausted")
}

// landSuccessPath handles the post-verification success flow:
// summary generation → write summary.md → narrative → purge plan → commit → push.
func landSuccessPath(
	cfg *ScripResolvedConfig,
	featureDir *FeatureDir,
	plan *Plan,
	git *GitOps,
	events []ProgressEvent,
	learnings []string,
	fullDiff, consultation string,
	progressPath, progressMdFile, statePath, planPath string,
	logger *RunLogger,
	cleanup *CleanupCoordinator,
) error {
	// Generate summary via AI
	fmt.Println("\n  Generating feature summary...")
	summaryPrompt := generateLandSummaryPrompt(featureDir.Feature, events, fullDiff, learnings)
	summaryStart := time.Now()
	summaryResult, summaryErr := scripSpawnProvider(cfg.ProjectRoot, summaryPrompt, cfg.Config.Provider.Timeout, false, logger, cleanup)
	fmt.Printf("  Summary generation done (%s)\n", FormatDuration(time.Since(summaryStart)))

	var summaryText string
	if summaryErr == nil && summaryResult != nil {
		summaryText, _ = landExtractSummary(summaryResult.Output)
	}
	if summaryText == "" {
		// Fallback: basic summary if AI extraction fails
		summaryText = fmt.Sprintf("# %s\n\nFeature landed successfully. %d plan items completed.", featureDir.Feature, len(plan.Items))
	}

	// Write summary.md BEFORE deleting other files (fail-safe)
	summaryPath := featureDir.SummaryMdPath()
	if err := AtomicWriteFile(summaryPath, []byte(summaryText)); err != nil {
		return fmt.Errorf("failed to write summary.md: %w", err)
	}
	fmt.Println("  Summary written")

	// Append landing narrative to progress.md
	narrative := landBuildNarrative(plan, events, learnings)
	if narrative != "" {
		_ = AppendProgressMd(progressMdFile, narrative)
	}

	// Record plan purge event
	_ = AppendProgressEvent(progressPath, &ProgressEvent{
		Event: ProgressPlanPurged,
	})

	// Purge plan.md and state.json
	os.Remove(planPath)
	_ = DeleteSessionState(statePath)
	fmt.Println("  Plan artifacts purged")

	// Record success
	_ = AppendProgressEvent(progressPath, &ProgressEvent{
		Event:           ProgressLandPassed,
		SummaryAppended: true,
	})

	// Commit artifacts (summary addition, plan/state deletion, progress updates)
	commitFiles := []string{summaryPath, planPath, statePath, progressPath, progressMdFile}
	commitMsg := fmt.Sprintf("chore: land %s — summary appended", featureDir.Feature)
	if err := git.CommitFiles(commitFiles, commitMsg); err != nil {
		fmt.Printf("  Warning: commit failed: %v\n", err)
	} else {
		fmt.Println("  Changes committed")
	}

	// Push to remote
	if err := git.Push(); err != nil {
		fmt.Printf("  Warning: push failed: %v\n", err)
		fmt.Println("    Run 'git push' manually to push your changes.")
	} else {
		fmt.Println("  Pushed to remote")
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf(" Feature '%s' landed successfully!\n", featureDir.Feature)
	fmt.Println(strings.Repeat("=", 60))
	return nil
}

// landRunVerifyCommands runs all verification commands and returns formatted results
// for template injection, plus a bool indicating whether all commands passed.
func landRunVerifyCommands(projectRoot string, verify *ScripVerifyConfig, timeout int) (formatted string, allPassed bool) {
	commands := verify.VerifyCommands()
	if len(commands) == 0 {
		return "No verification commands configured.\n", true
	}

	allPassed = true
	var buf strings.Builder
	for _, cmd := range commands {
		fmt.Printf("    %s\n", cmd)
		output, err := runCommand(projectRoot, cmd, timeout)

		status := "PASSED"
		if err != nil {
			status = "FAILED"
			allPassed = false
		}

		buf.WriteString(fmt.Sprintf("### %s — %s\n", cmd, status))
		if output != "" {
			buf.WriteString("```\n")
			buf.WriteString(strings.TrimRight(output, "\n"))
			buf.WriteString("\n```\n\n")
		} else {
			buf.WriteString("\n")
		}
	}

	return buf.String(), allPassed
}

// landBuildCriteria builds the {{allCriteria}} template variable from plan items.
func landBuildCriteria(plan *Plan) string {
	var buf strings.Builder
	for i, item := range plan.Items {
		buf.WriteString(fmt.Sprintf("### Item %d: %s\n", i+1, item.Title))
		for _, a := range item.Acceptance {
			buf.WriteString(fmt.Sprintf("- %s\n", a))
		}
		if i < len(plan.Items)-1 {
			buf.WriteString("\n")
		}
	}
	return buf.String()
}

// landParseAnalysis extracts VERIFY_PASS/VERIFY_FAIL markers from analysis output.
// Multiple VERIFY_FAIL markers may be present. Failures override pass.
func landParseAnalysis(result *ProviderResult) (passed bool, failures []string) {
	if result == nil || result.Output == "" {
		return false, []string{"analysis produced no output"}
	}

	for _, line := range strings.Split(result.Output, "\n") {
		trimmed := strings.TrimSpace(line)
		if scripVerifyPassRe.MatchString(trimmed) {
			passed = true
		}
		if m := scripVerifyFailRe.FindStringSubmatch(trimmed); len(m) == 2 {
			failures = append(failures, strings.TrimSpace(m[1]))
		}
	}

	// Failures override pass
	if len(failures) > 0 {
		passed = false
	}

	return passed, failures
}

// landExtractSummary extracts text between SUMMARY_START and SUMMARY_END markers.
// Returns (summary, true) on success, ("", false) if markers missing or content empty.
func landExtractSummary(output string) (string, bool) {
	lines := strings.Split(output, "\n")
	var collecting bool
	var summary strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if scripSummaryStartRe.MatchString(trimmed) {
			collecting = true
			continue
		}
		if scripSummaryEndRe.MatchString(trimmed) {
			result := strings.TrimSpace(summary.String())
			if result == "" {
				return "", false
			}
			return result, true
		}
		if collecting {
			summary.WriteString(line + "\n")
		}
	}

	return "", false
}

// landBuildNarrative generates a narrative section for progress.md on successful landing.
func landBuildNarrative(plan *Plan, events []ProgressEvent, learnings []string) string {
	states := ComputeAllItemStates(plan, events)
	var completed []string

	for _, item := range plan.Items {
		s := states[item.Title]
		if s.Passed {
			line := "- " + item.Title
			if s.LastCommit != "" {
				line += fmt.Sprintf(" (%s)", s.LastCommit[:min(7, len(s.LastCommit))])
			}
			completed = append(completed, line)
		} else if s.Skipped {
			reason := s.LastFailure
			if reason == "" {
				reason = "exceeded retries"
			}
			completed = append(completed, fmt.Sprintf("- %s (skipped: %s)", item.Title, reason))
		}
	}

	timestamp := time.Now().UTC().Format("2006-01-02 15:04")
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("## %s — Landing\n\n", timestamp))

	if len(completed) > 0 {
		buf.WriteString("### Items\n")
		buf.WriteString(strings.Join(completed, "\n"))
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

// generateLandAnalyzePrompt builds the land-analyze.md prompt.
func generateLandAnalyzePrompt(allCriteria, fullDiff, verifyResults, consultation string) string {
	return getPrompt("land-analyze", map[string]string{
		"allCriteria":   allCriteria,
		"fullDiff":      fullDiff,
		"verifyResults": verifyResults,
		"consultation":  consultation,
	})
}

// generateLandFixPrompt builds the land-fix.md prompt.
func generateLandFixPrompt(findings []string, verifyResults, diff, consultation string) string {
	var findingsBuf strings.Builder
	for i, f := range findings {
		findingsBuf.WriteString(fmt.Sprintf("%d. %s\n", i+1, f))
	}

	return getPrompt("land-fix", map[string]string{
		"findings":      findingsBuf.String(),
		"verifyResults": verifyResults,
		"diff":          diff,
		"consultation":  consultation,
	})
}

// generateLandSummaryPrompt builds the land-summary.md prompt.
func generateLandSummaryPrompt(feature string, events []ProgressEvent, diff string, learnings []string) string {
	return getPrompt("land-summary", map[string]string{
		"feature":        feature,
		"progressEvents": landFormatProgressEvents(events),
		"diff":           diff,
		"learnings":      buildLearnings(learnings, "## Learnings"),
	})
}

// landFormatProgressEvents formats progress events for the summary template.
func landFormatProgressEvents(events []ProgressEvent) string {
	if len(events) == 0 {
		return "No execution history."
	}

	var buf strings.Builder
	for _, e := range events {
		switch e.Event {
		case ProgressItemDone:
			status := e.Status
			if status == "" {
				status = "completed"
			}
			line := fmt.Sprintf("- [%s] Item: %s — %s", e.Timestamp, e.Item, status)
			if e.Commit != "" {
				line += fmt.Sprintf(" (commit: %s)", e.Commit[:min(7, len(e.Commit))])
			}
			buf.WriteString(line + "\n")
		case ProgressItemStuck:
			buf.WriteString(fmt.Sprintf("- [%s] Item stuck: %s — %s\n", e.Timestamp, e.Item, e.Reason))
		case ProgressLearning:
			buf.WriteString(fmt.Sprintf("- [%s] Learning: %s\n", e.Timestamp, e.Text))
		case ProgressLandFailed:
			buf.WriteString(fmt.Sprintf("- [%s] Previous landing failed: %s\n", e.Timestamp, strings.Join(e.Findings, "; ")))
		}
	}

	result := buf.String()
	if result == "" {
		return "No significant execution events."
	}
	return result
}
