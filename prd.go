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
	fmt.Println("  A) Keep refining")
	fmt.Println("  B) Finalize for execution")
	fmt.Println("  C) Edit manually ($EDITOR)")
	fmt.Println("  Q) Quit")
	fmt.Println()

	choice := promptChoice("Choose", []string{"a", "b", "c", "q"})

	switch choice {
	case "a":
		return prdRefine(cfg, featureDir)
	case "b":
		return prdFinalize(cfg, featureDir)
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
	if prd, err := LoadPRD(featureDir.PrdJsonPath()); err == nil {
		complete := CountComplete(prd)
		blocked := CountBlocked(prd)
		if complete > 0 || blocked > 0 {
			fmt.Printf("Progress: %s\n", buildProgress(prd))
		}
	}

	fmt.Println()
	fmt.Println("What would you like to do?")
	fmt.Println("  A) Refine further")
	fmt.Println("  B) Regenerate prd.json from prd.md")
	fmt.Println("  C) Edit prd.md ($EDITOR)")
	fmt.Println("  D) Edit prd.json ($EDITOR)")
	fmt.Println("  E) Start execution (ralph run)")
	fmt.Println("  Q) Quit")
	fmt.Println()

	choice := promptChoice("Choose", []string{"a", "b", "c", "d", "e", "q"})

	switch choice {
	case "a":
		return prdRefine(cfg, featureDir)
	case "b":
		return prdFinalize(cfg, featureDir)
	case "c":
		return prdEditManual(featureDir.PrdMdPath())
	case "d":
		return prdEditManual(featureDir.PrdJsonPath())
	case "e":
		fmt.Printf("\nRun: ralph run %s\n", featureDir.Feature)
		return nil
	case "q":
		return nil
	}

	return nil
}

// prdRefine refines an existing PRD
func prdRefine(cfg *ResolvedConfig, featureDir *FeatureDir) error {
	fmt.Println("\nRefining PRD...")

	// Read current PRD content
	content, err := os.ReadFile(featureDir.PrdMdPath())
	if err != nil {
		return fmt.Errorf("failed to read prd.md: %w", err)
	}

	// Load prd.json for run state context (nil if not finalized yet)
	var prd *PRD
	if featureDir.HasPrdJson {
		prd, _ = LoadPRD(featureDir.PrdJsonPath())
	}

	prompt := generatePrdRefinePrompt(cfg, featureDir, string(content), prd)
	if err := runProviderInteractive(cfg, prompt); err != nil {
		return err
	}

	fmt.Printf("\nUpdated %s\n", featureDir.PrdMdPath())

	// Commit refined prd.md
	commitPrdFile(cfg, featureDir.PrdMdPath(), "ralph: refine prd.md for "+featureDir.Feature)

	if promptYesNo("Finalize for execution?") {
		return prdFinalize(cfg, featureDir)
	}

	return nil
}

// prdFinalize converts prd.md to prd.json
func prdFinalize(cfg *ResolvedConfig, featureDir *FeatureDir) error {
	fmt.Println("\nFinalizing PRD...")

	// Load existing prd.json before regeneration (for state preservation)
	var oldPRD *PRD
	if fileExists(featureDir.PrdJsonPath()) {
		oldPRD, _ = LoadPRD(featureDir.PrdJsonPath())
	}

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
		// Validate it
		newPRD, err := LoadPRD(featureDir.PrdJsonPath())
		if err != nil {
			fmt.Printf("\nWarning: prd.json validation failed: %v\n", err)
			fmt.Println("Edit manually or run 'ralph prd " + featureDir.Feature + "' again.")
			return nil
		}

		// Merge state from old PRD into new PRD
		if oldPRD != nil {
			summary := MergePRDStateSummary(oldPRD, newPRD)
			MergePRDState(oldPRD, newPRD)
			if summary != "" {
				fmt.Printf("\n%s\n", summary)
			}
			if err := SavePRD(featureDir.PrdJsonPath(), newPRD); err != nil {
				fmt.Printf("Warning: failed to save merged prd.json: %v\n", err)
			}
		}

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
	cmd := newExecCommand(c.name, c.args...)
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

// newExecCommand is a helper to create exec.Command
func newExecCommand(name string, args ...string) *execCmd {
	return &execCmd{Cmd: *execCommand(name, args...)}
}

type execCmd struct {
	exec.Cmd
}

func execCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
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
