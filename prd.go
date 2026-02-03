package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
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

	// Generate and run brainstorming prompt
	prompt := generatePrdCreatePrompt(cfg, featureDir)
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
	fmt.Printf("PRD is ready: %s\n\n", featureDir.Path)
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

	prompt := generatePrdRefinePrompt(cfg, featureDir, string(content))
	if err := runProviderInteractive(cfg, prompt); err != nil {
		return err
	}

	fmt.Printf("\nUpdated %s\n", featureDir.PrdMdPath())

	if promptYesNo("Finalize for execution?") {
		return prdFinalize(cfg, featureDir)
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
		// Validate it
		if _, err := LoadPRD(featureDir.PrdJsonPath()); err != nil {
			fmt.Printf("\nWarning: prd.json validation failed: %v\n", err)
			fmt.Println("Edit manually or run 'ralph prd " + featureDir.Feature + "' again.")
			return nil
		}

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

	cmd := runCommandInteractive(editor, path)
	return cmd.Run()
}

// runProviderInteractive runs the provider with stdin/stdout connected
func runProviderInteractive(cfg *ResolvedConfig, prompt string) error {
	args := append([]string{}, cfg.Config.Provider.Args...)
	if cfg.Config.Provider.PromptFlag != "" {
		args = append(args, cfg.Config.Provider.PromptFlag)
	}
	args = append(args, prompt)
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
