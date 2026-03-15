package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	scripVerifyPassRe = regexp.MustCompile(`^\s*<scrip>VERIFY_PASS</scrip>\s*$`)
	scripVerifyFailRe = regexp.MustCompile(`^\s*<scrip>VERIFY_FAIL:(.+)</scrip>\s*$`)
)

// cmdPlan implements "scrip plan <feature> [description]" — CLI-mediated
// planning rounds. Spawns Claude for each round, captures output, prompts
// user for feedback, and finalizes when ready.
func cmdPlan(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: scrip plan <feature> [\"description\"]")
		os.Exit(1)
	}

	feature := args[0]
	description := ""
	if len(args) > 1 {
		description = strings.Join(args[1:], " ")
	}

	projectRoot := GetProjectRoot()

	resolved, err := LoadScripConfig(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config not found: %v\nRun 'scrip prep' first.\n", err)
		os.Exit(1)
	}

	featureDir, err := FindFeatureDir(projectRoot, feature, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Feature directory error: %v\n", err)
		os.Exit(1)
	}
	if err := featureDir.EnsureExists(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create feature directory: %v\n", err)
		os.Exit(1)
	}

	planMdPath := planMdPathFor(featureDir)
	planJsonlPath := planJsonlPathFor(featureDir)
	progressJsonlPath := progressJsonlPathFor(featureDir)
	progressMdPath := progressMdPathFor(featureDir)

	// Already finalized — nothing to do
	if _, err := os.Stat(planMdPath); err == nil {
		fmt.Printf("Plan already finalized: %s\n", planMdPath)
		fmt.Println("Delete plan.md to re-plan, or run 'scrip exec' to execute.")
		return
	}

	// Resume from existing rounds or require description for new plan
	rounds, _ := LoadPlanRounds(planJsonlPath)
	isResume := len(rounds) > 0

	if !isResume && description == "" {
		fmt.Fprintln(os.Stderr, "Usage: scrip plan <feature> \"description\"")
		fmt.Fprintln(os.Stderr, "Description required for new plan.")
		os.Exit(1)
	}

	git := NewGitOps(projectRoot)
	if err := git.EnsureBranch("plan/"+feature, git.DefaultBranch()); err != nil {
		fmt.Fprintf(os.Stderr, "Branch error: %v\n", err)
		os.Exit(1)
	}

	// Pre-compute context (done once, reused across rounds)
	codebaseCtx := DiscoverScripCodebase(projectRoot, &resolved.Config)
	codebaseStr := FormatCodebaseContext(codebaseCtx)
	consultation := buildPlanConsultation(resolved, featureDir, codebaseCtx)
	progressCtx := GetProgressContext(progressMdPath)
	landFailureCtx := buildLandFailureContext(progressJsonlPath)

	fullProgressCtx := progressCtx
	if landFailureCtx != "" {
		if fullProgressCtx != "" {
			fullProgressCtx += "\n\n"
		}
		fullProgressCtx += landFailureCtx
	}

	if isResume {
		fmt.Printf("Resuming plan for %s (round %d)\n", feature, len(rounds)+1)
	} else {
		fmt.Printf("Planning: %s\n", feature)
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		roundNum := len(rounds) + 1
		var prompt, userInput string

		if roundNum == 1 && !isResume {
			// First round: use plan-create.md with the feature description
			userInput = description
			prompt = generatePlanCreatePrompt(featureDir, description, codebaseStr, consultation, fullProgressCtx)
		} else {
			// Subsequent rounds: get user feedback
			fmt.Print("\nFeedback (or 'done' to finalize, 'quit' to abandon): ")
			input, err := reader.ReadString('\n')
			if err != nil {
				fmt.Println()
				return
			}
			userInput = strings.TrimSpace(input)

			switch strings.ToLower(userInput) {
			case "quit", "abandon", "q":
				fmt.Println("Planning abandoned.")
				return
			case "done", "finalize", "d":
				if err := finalizePlanFromRounds(resolved, featureDir, rounds, planMdPath, progressJsonlPath, codebaseStr); err != nil {
					fmt.Fprintf(os.Stderr, "Finalization failed: %v\n", err)
					os.Exit(1)
				}
				return
			}

			planHistory := BuildPlanHistory(rounds)
			prompt = generatePlanRoundPrompt(featureDir, userInput, codebaseStr, consultation, planHistory, fullProgressCtx)
		}

		fmt.Printf("\nRound %d: thinking...\n", roundNum)
		response, err := runPlanProvider(projectRoot, prompt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "AI agent failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Println()
		fmt.Println(response)

		hasDraft := containsPlanDraft(response)

		round := &PlanRound{
			Round:        roundNum,
			Timestamp:    time.Now().UTC().Format(time.RFC3339),
			UserInput:    userInput,
			AIResponse:   response,
			HasPlanDraft: hasDraft,
		}
		if err := AppendPlanRound(planJsonlPath, round); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save round: %v\n", err)
		}
		rounds = append(rounds, *round)

		if hasDraft {
			fmt.Println("\n--- Plan draft detected ---")
			fmt.Print("Finalize? (yes/no): ")
			input, err := reader.ReadString('\n')
			if err != nil {
				fmt.Println()
				return
			}
			if choice := strings.TrimSpace(strings.ToLower(input)); choice == "y" || choice == "yes" {
				if err := finalizePlanFromRounds(resolved, featureDir, rounds, planMdPath, progressJsonlPath, codebaseStr); err != nil {
					fmt.Fprintf(os.Stderr, "Finalization failed: %v\n", err)
					os.Exit(1)
				}
				return
			}
		}
	}
}

// --- Path helpers ---

func planMdPathFor(fd *FeatureDir) string {
	return filepath.Join(fd.Path, "plan.md")
}

func planJsonlPathFor(fd *FeatureDir) string {
	return filepath.Join(fd.Path, "plan.jsonl")
}

func progressJsonlPathFor(fd *FeatureDir) string {
	return filepath.Join(fd.Path, "progress.jsonl")
}

func progressMdPathFor(fd *FeatureDir) string {
	return filepath.Join(fd.Path, "progress.md")
}

// --- Prompt generation ---

func generatePlanCreatePrompt(fd *FeatureDir, description, codebaseStr, consultation, progressCtx string) string {
	return getPrompt("plan-create", map[string]string{
		"feature":         fd.Feature,
		"description":     description,
		"codebaseContext": codebaseStr,
		"consultation":    consultation,
		"progressContext": progressCtx,
	})
}

func generatePlanRoundPrompt(fd *FeatureDir, userInput, codebaseStr, consultation, planHistory, progressCtx string) string {
	return getPrompt("plan-round", map[string]string{
		"feature":         fd.Feature,
		"codebaseContext": codebaseStr,
		"consultation":    consultation,
		"planHistory":     planHistory,
		"progressContext": progressCtx,
		"userInput":       userInput,
	})
}

func generatePlanVerifyPrompt(planContent, codebaseStr string) string {
	return getPrompt("plan-verify", map[string]string{
		"planContent":     planContent,
		"codebaseContext": codebaseStr,
	})
}

// --- Provider spawning ---

func runPlanProvider(projectRoot, prompt string) (string, error) {
	args := ScripProviderArgs(false)
	cmd := exec.Command("claude", args...)
	cmd.Dir = projectRoot
	cmd.Stdin = strings.NewReader(prompt)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude exited with error: %w", err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// --- Plan detection and extraction ---

// containsPlanDraft checks whether an AI response contains a plan draft
// by looking for the expected YAML frontmatter fields and Items section.
func containsPlanDraft(response string) bool {
	return strings.Contains(response, "feature:") &&
		strings.Contains(response, "item_count:") &&
		strings.Contains(response, "## Items")
}

// extractPlanContent extracts plan markdown from an AI response that may
// contain discussion text before the actual plan. Looks for YAML frontmatter
// starting with "---" followed by a "feature:" field.
func extractPlanContent(response string) string {
	lines := strings.Split(response, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			// Check if "feature:" appears within the next few lines
			for j := i + 1; j < len(lines) && j < i+10; j++ {
				if strings.HasPrefix(strings.TrimSpace(lines[j]), "feature:") {
					return strings.Join(lines[i:], "\n")
				}
				if strings.TrimSpace(lines[j]) == "---" {
					break
				}
			}
		}
	}
	return response
}

// --- Consultation ---

// buildPlanConsultation syncs cached framework resources and runs feature-level
// consultation via subagents. Returns consultation guidance with citations, or
// falls back to web search instructions when no resources are cached.
func buildPlanConsultation(cfg *ScripResolvedConfig, featureDir *FeatureDir, codebaseCtx *CodebaseContext) string {
	rm := ensureScripResourceSync(cfg, codebaseCtx)
	if rm == nil || !rm.HasDetectedResources() {
		return buildResourceFallbackInstructions()
	}
	return consultForFeature(cfg.ProjectRoot, featureDir, rm, codebaseCtx.TechStack, nil)
}

// --- Land failure context ---

// buildLandFailureContext reads progress.jsonl for the most recent land_failed
// event and formats its findings for injection into planning prompts.
func buildLandFailureContext(progressJsonlPath string) string {
	events, err := LoadProgressEvents(progressJsonlPath)
	if err != nil || len(events) == 0 {
		return ""
	}

	lastFail := LastEventByType(events, ProgressLandFailed)
	if lastFail == nil {
		return ""
	}

	var lines []string
	lines = append(lines, "## Previous Land Failure")
	lines = append(lines, "")
	lines = append(lines, "The previous attempt to land this feature failed:")
	for _, f := range lastFail.Findings {
		lines = append(lines, "- "+f)
	}
	if lastFail.Analysis != "" {
		lines = append(lines, "")
		lines = append(lines, "**Analysis:** "+lastFail.Analysis)
	}
	return strings.Join(lines, "\n")
}

// --- Finalization ---

// finalizePlanFromRounds extracts the plan from the last draft round, writes
// plan.md, runs adversarial verification, and emits a plan_created event.
func finalizePlanFromRounds(cfg *ScripResolvedConfig, fd *FeatureDir, rounds []PlanRound, planMdPath, progressJsonlPath, codebaseStr string) error {
	// Find last round with a plan draft
	var planContent string
	for i := len(rounds) - 1; i >= 0; i-- {
		if rounds[i].HasPlanDraft {
			planContent = extractPlanContent(rounds[i].AIResponse)
			break
		}
	}
	if planContent == "" {
		return fmt.Errorf("no plan draft found — ask the AI to produce a plan first")
	}

	plan, err := ParsePlan(planContent)
	if err != nil {
		return fmt.Errorf("failed to parse plan: %w", err)
	}
	if len(plan.Items) == 0 {
		return fmt.Errorf("plan contains no items")
	}

	if err := WritePlanMd(planMdPath, plan); err != nil {
		return fmt.Errorf("failed to write plan.md: %w", err)
	}
	fmt.Printf("Written: %s (%d items)\n", planMdPath, len(plan.Items))

	// Run adversarial verification
	fmt.Println("\nRunning plan verification...")
	verification, err := runPlanVerification(cfg.ProjectRoot, planContent, codebaseStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: verification failed to run: %v\n", err)
	} else if len(verification.Warnings) > 0 {
		fmt.Printf("\nVerification found %d issue(s):\n", len(verification.Warnings))
		for _, w := range verification.Warnings {
			fmt.Printf("  ! %s\n", w)
		}
	} else {
		fmt.Println("Verification passed.")
	}

	// Record finalization round
	finalRound := &PlanRound{
		Round:        len(rounds) + 1,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		UserInput:    "(finalized)",
		AIResponse:   "(plan written to plan.md)",
		Finalized:    true,
		Verification: verification,
	}
	_ = AppendPlanRound(planJsonlPathFor(fd), finalRound)

	// Emit plan_created event
	_ = AppendProgressEvent(progressJsonlPath, &ProgressEvent{
		Event:     ProgressPlanCreated,
		ItemCount: len(plan.Items),
		Context:   fmt.Sprintf("plan for %s", fd.Feature),
	})

	fmt.Printf("\nPlan finalized. Next: scrip exec %s\n", fd.Feature)
	return nil
}

func runPlanVerification(projectRoot, planContent, codebaseStr string) (*PlanVerification, error) {
	prompt := generatePlanVerifyPrompt(planContent, codebaseStr)
	response, err := runPlanProvider(projectRoot, prompt)
	if err != nil {
		return nil, err
	}
	return parsePlanVerifyOutput(response), nil
}

// parsePlanVerifyOutput extracts VERIFY_PASS/VERIFY_FAIL markers from
// the verification agent's output.
func parsePlanVerifyOutput(output string) *PlanVerification {
	v := &PlanVerification{}
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if scripVerifyPassRe.MatchString(trimmed) {
			continue
		}
		if m := scripVerifyFailRe.FindStringSubmatch(trimmed); len(m) == 2 {
			v.Warnings = append(v.Warnings, strings.TrimSpace(m[1]))
		}
	}
	return v
}
