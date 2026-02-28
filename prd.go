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
func runPrdStateMachine(cfg *ResolvedConfig, featureDir *FeatureDir, resourceGuidance string) error {
	// Determine current state
	hasMd := featureDir.HasPrdMd
	hasJson := featureDir.HasPrdJson

	// Ensure feature directory exists
	if err := featureDir.EnsureExists(); err != nil {
		return err
	}

	// State 1: New feature (no markdown)
	if !hasMd && !hasJson {
		return prdStateNew(cfg, featureDir, resourceGuidance)
	}

	// State 2+3: PRD exists — refine via AI session then auto-finalize
	return prdRefineAndFinalize(cfg, featureDir, resourceGuidance)
}

// prdStateNew handles creating a new PRD from scratch
func prdStateNew(cfg *ResolvedConfig, featureDir *FeatureDir, resourceGuidance string) error {
	fmt.Printf("Starting PRD for '%s'...\n\n", featureDir.Feature)

	// Discover codebase context
	codebaseCtx := DiscoverCodebase(cfg.ProjectRoot, &cfg.Config)

	// Generate and run brainstorming prompt
	prompt := generatePrdCreatePrompt(cfg, featureDir, codebaseCtx, resourceGuidance)
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
		return prdFinalize(cfg, featureDir, resourceGuidance)
	}

	fmt.Printf("\nRun 'ralph prd %s' to continue.\n", featureDir.Feature)
	return nil
}

// prdRefineAndFinalize opens an interactive AI session to refine prd.md, then auto-finalizes.
// Used when prd.md already exists (with or without prd.json).
func prdRefineAndFinalize(cfg *ResolvedConfig, featureDir *FeatureDir, resourceGuidance string) error {
	fmt.Printf("PRD exists: %s\n", featureDir.PrdMdPath())
	fmt.Println("Opening AI session to refine...")
	fmt.Println()

	// Discover codebase context
	codebaseCtx := DiscoverCodebase(cfg.ProjectRoot, &cfg.Config)

	// Generate and run refine prompt
	prompt := generatePrdRefinePrompt(cfg, featureDir, codebaseCtx, resourceGuidance)
	if err := runProviderInteractive(cfg, prompt); err != nil {
		return err
	}

	// Commit updated prd.md
	commitPrdFile(cfg, featureDir.PrdMdPath(), "ralph: refine prd.md for "+featureDir.Feature)

	// Auto-finalize
	fmt.Println("\nFinalizing PRD from updated prd.md...")
	return prdFinalize(cfg, featureDir, resourceGuidance)
}

// prdFinalize converts prd.md to prd.json
func prdFinalize(cfg *ResolvedConfig, featureDir *FeatureDir, resourceGuidance string) error {
	fmt.Println("\nFinalizing PRD...")

	content, err := os.ReadFile(featureDir.PrdMdPath())
	if err != nil {
		return fmt.Errorf("failed to read prd.md: %w", err)
	}

	prompt := generatePrdFinalizePrompt(cfg, featureDir, string(content), resourceGuidance)
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

// promptYesNo prompts for a yes/no answer.
// Returns false on EOF/error (safe default — declines interactive sessions).
func promptYesNo(question string) bool {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s (y/n): ", question)
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println()
			return false
		}
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

// promptChoice prompts for a choice from options.
// Returns empty string on EOF/error (callers handle unknown values).
func promptChoice(question string, options []string) string {
	reader := bufio.NewReader(os.Stdin)
	optStr := strings.Join(options, "/")
	for {
		fmt.Printf("%s (%s): ", question, optStr)
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println()
			return ""
		}
		input = strings.TrimSpace(strings.ToLower(input))
		for _, opt := range options {
			if input == opt {
				return opt
			}
		}
		fmt.Printf("Please enter one of: %s\n", optStr)
	}
}
