package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runPrdStateMachine runs the smart PRD workflow
func runPrdStateMachine(cfg *ResolvedConfig, featureDir *FeatureDir) error {
	// Determine current state
	hasMd := featureDir.HasPrdMd
	hasJson := featureDir.HasPrdJson

	// Ensure feature directory exists
	if err := featureDir.EnsureExists(); err != nil {
		return err
	}

	// State 1: New feature (no markdown)
	if !hasMd && !hasJson {
		return prdStateNew(cfg, featureDir)
	}

	// State 2: Markdown exists, not finalized
	if hasMd && !hasJson {
		return prdStateNeedsFinalize(cfg, featureDir)
	}

	// State 3: Both exist (already finalized)
	return prdStateFinalized(cfg, featureDir)
}

// prdStateNew handles creating a new PRD from scratch
func prdStateNew(cfg *ResolvedConfig, featureDir *FeatureDir) error {
	fmt.Printf("Starting PRD for '%s'...\n\n", featureDir.Feature)

	// Discover codebase context
	codebaseCtx := DiscoverCodebase(cfg.ProjectRoot, &cfg.Config)

	// Generate and run brainstorming prompt
	prompt := generatePrdCreatePrompt(cfg, featureDir, codebaseCtx)
	if err := runProviderInteractive(cfg, prompt); err != nil {
		return err
	}

	// Check if prd.md was created
	featureDir.HasPrdMd = fileExists(featureDir.PrdMdPath())
	if !featureDir.HasPrdMd {
		fmt.Println("\nPRD was not saved. Run 'ralph prd " + featureDir.Feature + "' again.")
		return nil
	}

	fmt.Printf("\nSaved to %s\n\n", featureDir.PrdMdPath())

	// Commit prd.md
	commitPrdFile(cfg, featureDir.PrdMdPath(), "ralph: create prd.md for "+featureDir.Feature)

	// Ask to finalize
	if promptYesNo("Ready to finalize for execution?") {
		return prdFinalize(cfg, featureDir)
	}

	fmt.Printf("\nRun 'ralph prd %s' to continue.\n", featureDir.Feature)
	return nil
}

// prdStateNeedsFinalize handles PRD that needs finalization
func prdStateNeedsFinalize(cfg *ResolvedConfig, featureDir *FeatureDir) error {
	fmt.Printf("PRD exists: %s\n\n", featureDir.PrdMdPath())
	fmt.Println("What would you like to do?")
	fmt.Println("  A) Finalize for execution")
	fmt.Println("  B) Refine with AI")
	fmt.Println("  C) Edit prd.md ($EDITOR)")
	fmt.Println("  Q) Quit")
	fmt.Println()

	choice := promptChoice("Choose", []string{"a", "b", "c", "q"})

	switch choice {
	case "a":
		return prdFinalize(cfg, featureDir)
	case "b":
		return prdRefineDraft(cfg, featureDir)
	case "c":
		return prdEditManual(featureDir.PrdMdPath())
	case "q":
		return nil
	}

	return nil
}

// prdStateFinalized handles PRD that is already finalized
func prdStateFinalized(cfg *ResolvedConfig, featureDir *FeatureDir) error {
	fmt.Printf("PRD is ready: %s\n", featureDir.Path)

	// Show progress if any stories have been worked on
	def, err := LoadPRDDefinition(featureDir.PrdJsonPath())
	if err != nil {
		fmt.Printf("\nError loading prd.json: %v\n", err)
		fmt.Println()
		fmt.Println("What would you like to do?")
		fmt.Println("  A) Regenerate prd.json from prd.md")
		fmt.Println("  B) Edit prd.json ($EDITOR)")
		fmt.Println("  C) Edit prd.md ($EDITOR)")
		fmt.Println("  Q) Quit")
		fmt.Println()

		choice := promptChoice("Choose", []string{"a", "b", "c", "q"})
		switch choice {
		case "a":
			return prdRegenerateJson(cfg, featureDir)
		case "b":
			return prdEditManual(featureDir.PrdJsonPath())
		case "c":
			return prdEditManual(featureDir.PrdMdPath())
		}
		return nil
	}

	state, _ := LoadRunState(featureDir.RunStatePath())
	passed := CountPassed(state)
	skipped := CountSkipped(state)
	if passed > 0 || skipped > 0 {
		fmt.Printf("Progress: %s\n", buildProgress(def, state))
	}

	fmt.Println()
	fmt.Println("What would you like to do?")
	fmt.Println("  A) Refine with AI")
	fmt.Println("  B) Regenerate prd.json from prd.md")
	fmt.Println("  C) Edit prd.md ($EDITOR)")
	fmt.Println("  D) Edit prd.json ($EDITOR)")
	fmt.Println("  Q) Quit")
	fmt.Println()

	choice := promptChoice("Choose", []string{"a", "b", "c", "d", "q"})

	switch choice {
	case "a":
		return prdRefineInteractive(cfg, featureDir)
	case "b":
		return prdRegenerateJson(cfg, featureDir)
	case "c":
		return prdEditManual(featureDir.PrdMdPath())
	case "d":
		return prdEditManual(featureDir.PrdJsonPath())
	case "q":
		return nil
	}

	return nil
}

// prdFinalize converts prd.md to prd.json
func prdFinalize(cfg *ResolvedConfig, featureDir *FeatureDir) error {
	fmt.Println("\nFinalizing PRD...")

	content, err := os.ReadFile(featureDir.PrdMdPath())
	if err != nil {
		return fmt.Errorf("failed to read prd.md: %w", err)
	}

	prompt := generatePrdFinalizePrompt(cfg, featureDir, string(content))
	if err := runProviderInteractive(cfg, prompt); err != nil {
		return err
	}

	// Check if prd.json was created
	if fileExists(featureDir.PrdJsonPath()) {
		// Validate as v3 definition (no runtime fields expected)
		if _, err := LoadPRDDefinition(featureDir.PrdJsonPath()); err != nil {
			fmt.Printf("\nWarning: prd.json validation failed: %v\n", err)
			fmt.Println("Edit manually or run 'ralph prd " + featureDir.Feature + "' again.")
			return nil
		}

		featureDir.HasPrdJson = true

		// Commit both prd.md and prd.json
		commitPrdFile(cfg, featureDir.PrdMdPath(), "ralph: finalize prd.md for "+featureDir.Feature)
		commitPrdFile(cfg, featureDir.PrdJsonPath(), "ralph: finalize prd.json for "+featureDir.Feature)

		fmt.Printf("\n✓ PRD finalized: %s\n", featureDir.PrdJsonPath())
		fmt.Printf("\nRun 'ralph run %s' to start implementation.\n", featureDir.Feature)
	} else {
		fmt.Println("\nprd.json was not created. Try again.")
	}

	return nil
}

// prdRefineDraft opens an AI session for refining draft PRD (pre-finalization).
// Re-runs the brainstorming prompt — provider sees existing prd.md and iterates on it.
func prdRefineDraft(cfg *ResolvedConfig, featureDir *FeatureDir) error {
	fmt.Printf("\nRefining PRD with AI...\n\n")

	codebaseCtx := DiscoverCodebase(cfg.ProjectRoot, &cfg.Config)
	prompt := generatePrdCreatePrompt(cfg, featureDir, codebaseCtx)
	if err := runProviderInteractive(cfg, prompt); err != nil {
		return err
	}

	// Check if prd.md was updated
	featureDir.HasPrdMd = fileExists(featureDir.PrdMdPath())
	if featureDir.HasPrdMd {
		commitPrdFile(cfg, featureDir.PrdMdPath(), "ralph: refine prd.md for "+featureDir.Feature)
	}

	fmt.Printf("\nRun 'ralph prd %s' to continue.\n", featureDir.Feature)
	return nil
}

// prdRefineInteractive opens an interactive AI session with full feature context (post-finalization).
// This is the former cmdRefine logic, now integrated into ralph prd.
func prdRefineInteractive(cfg *ResolvedConfig, featureDir *FeatureDir) error {
	def, err := LoadPRDDefinition(featureDir.PrdJsonPath())
	if err != nil {
		return fmt.Errorf("failed to load PRD: %w", err)
	}
	state, _ := LoadRunState(featureDir.RunStatePath())

	// Soft warnings (don't block interactive session)
	if warnings := CheckReadinessWarnings(&cfg.Config); len(warnings) > 0 {
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "  Warning: %s\n", w)
		}
		fmt.Fprintln(os.Stderr, "")
	}

	// Ensure we're on the feature branch
	git := NewGitOps(cfg.ProjectRoot)
	if err := git.EnsureBranch(def.BranchName); err != nil {
		return fmt.Errorf("failed to switch to branch %s: %w", def.BranchName, err)
	}

	// Show progress summary
	fmt.Printf("\nFeature: %s\n", featureDir.Feature)
	fmt.Printf("Branch: %s\n", def.BranchName)
	fmt.Printf("Progress: %s\n", buildProgress(def, state))
	fmt.Println()

	// Generate prompt and open interactive session
	prompt := generateRefinePrompt(cfg, featureDir, def, state)
	return runProviderInteractive(cfg, prompt)
}

// prdRegenerateJson re-runs finalization from prd.md. Safe because run-state.json is separate.
func prdRegenerateJson(cfg *ResolvedConfig, featureDir *FeatureDir) error {
	// Warn if stories have been worked on
	if fileExists(featureDir.RunStatePath()) {
		rDef, rErr := LoadPRDDefinition(featureDir.PrdJsonPath())
		rState, _ := LoadRunState(featureDir.RunStatePath())
		if rErr == nil {
			passed := CountPassed(rState)
			skipped := CountSkipped(rState)
			if passed > 0 || skipped > 0 {
				fmt.Printf("\nWarning: %s\n", buildProgress(rDef, rState))
				fmt.Println("Execution state (passes, retries, skips) is preserved in run-state.json.")
				fmt.Println("Story state will be matched by ID after regeneration.")
				fmt.Println()
				if !promptYesNo("Proceed with regeneration?") {
					return nil
				}
			}
		}
	}

	return prdFinalize(cfg, featureDir)
}

// prdEditManual opens a file in the user's editor
func prdEditManual(path string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nano"
	}
	if !isCommandAvailable(editor) {
		return fmt.Errorf("editor '%s' not found in PATH. Set $EDITOR to your preferred editor", editor)
	}

	cmd := runCommandInteractive(editor, path)
	return cmd.Run()
}

// runProviderInteractive runs the provider with stdin/stdout connected.
// Interactive mode needs stdin for user input, so stdin promptMode
// falls back to arg mode. File mode is preserved for providers that
// need it (e.g., to avoid shell argument length limits).
// Non-interactive flags like --print are stripped so the provider runs
// as a full interactive CLI session (user answers questions, then exits).
func runProviderInteractive(cfg *ResolvedConfig, prompt string) error {
	promptMode := cfg.Config.Provider.PromptMode
	if promptMode == "stdin" || promptMode == "" {
		promptMode = "arg"
	}

	// Strip non-interactive flags so the provider runs interactively.
	// e.g., claude's --print suppresses streaming and prevents conversation.
	interactiveArgs := stripNonInteractiveArgs(cfg.Config.Provider.Args)

	args, promptFile, err := buildProviderArgs(
		interactiveArgs,
		promptMode,
		cfg.Config.Provider.PromptFlag,
		prompt,
	)
	if err != nil {
		return err
	}
	if promptFile != "" {
		defer os.Remove(promptFile)
	}

	cmd := runCommandInteractive(cfg.Config.Provider.Command, args...)
	cmd.dir = cfg.ProjectRoot
	return cmd.Run()
}

// runCommandInteractive creates an interactive command
func runCommandInteractive(name string, args ...string) *Command {
	cmd := &Command{
		name: name,
		args: args,
	}
	return cmd
}

// Command wraps exec.Command with interactive support
type Command struct {
	name string
	args []string
	dir  string
}

func (c *Command) Run() error {
	cmd := exec.Command(c.name, c.args...)
	if c.dir != "" {
		cmd.Dir = c.dir
	}
	// NOTE: no Setpgid here. The provider must stay in ralph's process group
	// so it can read from the controlling terminal. Setpgid would put it in a
	// background group, causing SIGTTIN on any stdin read (freezing the process).
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// nonInteractiveArgs are flags that prevent interactive provider sessions.
// These are stripped by runProviderInteractive so the user can converse
// with the provider during PRD creation.
var nonInteractiveArgs = map[string]bool{
	"--print": true,
	"-p":      true,
}

// stripNonInteractiveArgs removes flags that prevent interactive sessions.
func stripNonInteractiveArgs(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if !nonInteractiveArgs[arg] {
			filtered = append(filtered, arg)
		}
	}
	return filtered
}

// commitPrdFile commits a PRD file if commits.prdChanges is enabled.
// Safety: refuses to commit unless currently on a ralph/ branch.
func commitPrdFile(cfg *ResolvedConfig, path, message string) {
	if cfg.Config.Commits == nil || !cfg.Config.Commits.PrdChanges {
		return
	}
	git := NewGitOps(cfg.ProjectRoot)
	branch, err := git.CurrentBranch()
	if err != nil || !strings.HasPrefix(branch, "ralph/") {
		fmt.Printf("Warning: not on a ralph/ branch (on %s), skipping commit of %s\n", branch, filepath.Base(path))
		return
	}
	if err := git.CommitFile(path, message); err != nil {
		fmt.Printf("Warning: failed to commit %s: %v\n", filepath.Base(path), err)
	}
}

// promptYesNo prompts for a yes/no answer
func promptYesNo(question string) bool {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s (y/n): ", question)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		if input == "y" || input == "yes" {
			return true
		}
		if input == "n" || input == "no" {
			return false
		}
		fmt.Println("Please enter 'y' or 'n'")
	}
}

// promptChoice prompts for a choice from options
func promptChoice(question string, options []string) string {
	reader := bufio.NewReader(os.Stdin)
	optStr := strings.Join(options, "/")
	for {
		fmt.Printf("%s (%s): ", question, optStr)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		for _, opt := range options {
			if input == opt {
				return opt
			}
		}
		fmt.Printf("Please enter one of: %s\n", optStr)
	}
}
