package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

func cmdInit(args []string) {
	force := false
	for _, arg := range args {
		if arg == "-f" || arg == "--force" {
			force = true
		}
	}

	cfg := resolveConfig(0, true)
	ralphDir := filepath.Join(cfg.ProjectRoot, "scripts", "ralph")
	prdPath := filepath.Join(ralphDir, "prd.json")

	if fileExists(prdPath) && !force {
		fmt.Fprintf(os.Stderr, "prd.json already exists at %s\n", prdPath)
		fmt.Fprintln(os.Stderr, "Use --force to overwrite.")
		os.Exit(1)
	}

	os.MkdirAll(ralphDir, 0755)

	// Write example PRD
	if err := os.WriteFile(prdPath, getExamplePRD(), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write prd.json: %v\n", err)
		os.Exit(1)
	}

	// Write .gitignore
	gitignorePath := filepath.Join(ralphDir, ".gitignore")
	if !fileExists(gitignorePath) {
		os.WriteFile(gitignorePath, []byte("*.backup\n*.tmp\n"), 0644)
	}

	fmt.Println("Initialized Ralph:")
	fmt.Printf("  - Created %s\n", prdPath)
	fmt.Printf("  - Created %s\n", gitignorePath)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Run 'ralph prd' to create a PRD interactively, or")
	fmt.Println("  2. Edit prd.json directly with your user stories")
	fmt.Println("  3. Run 'ralph run' to start the agent loop")
}

func cmdRun(args []string) {
	iterations := 10
	noVerify := false

	for i, arg := range args {
		if arg == "--no-verify" {
			noVerify = true
		} else if n, err := strconv.Atoi(arg); err == nil && i == 0 {
			iterations = n
		}
	}

	cfg := resolveConfig(iterations, !noVerify)

	if err := runLoop(cfg, noVerify); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func cmdVerify(args []string) {
	cfg := resolveConfig(0, true)

	prd, err := loadPRD(cfg.PrdPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("PRD: %s\n", cfg.PrdPath)
	fmt.Printf("Project: %s\n", prd.Project)

	if err := runVerify(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func cmdPrd(args []string) {
	cfg := resolveConfig(0, true)

	var featureName string
	var outputPath string

	for i, arg := range args {
		if arg == "-o" && i+1 < len(args) {
			outputPath = args[i+1]
		} else if !isFlag(arg) && featureName == "" {
			featureName = arg
		}
	}

	if outputPath == "" {
		tasksDir := filepath.Join(cfg.ProjectRoot, "tasks")
		if featureName != "" {
			outputPath = filepath.Join(tasksDir, "prd-"+featureName+".md")
		} else {
			outputPath = filepath.Join(tasksDir, "prd.md")
		}
	}

	// Ensure tasks directory exists
	os.MkdirAll(filepath.Dir(outputPath), 0755)

	prompt := getTemplate("prd", map[string]string{
		"outputPath":  outputPath,
		"featureName": featureName,
		"projectRoot": cfg.ProjectRoot,
	})

	fmt.Println("Starting PRD creation...")
	fmt.Printf("Output will be saved to: %s\n", outputPath)
	fmt.Println()

	if err := runAmpWithPrompt(cfg, prompt); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("PRD created successfully!")
	fmt.Printf("  File: %s\n", outputPath)
	fmt.Println()
	fmt.Printf("Next: Run 'ralph convert %s' to generate prd.json\n", outputPath)
}

func cmdConvert(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: ralph convert <prd-file.md> [-o output.json]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Example: ralph convert tasks/prd-auth.md")
		os.Exit(1)
	}

	cfg := resolveConfig(0, true)
	prdFile := args[0]
	prdPath := filepath.Join(cfg.ProjectRoot, prdFile)

	if !fileExists(prdPath) {
		fmt.Fprintf(os.Stderr, "PRD file not found: %s\n", prdPath)
		os.Exit(1)
	}

	prdContent, err := os.ReadFile(prdPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read PRD file: %v\n", err)
		os.Exit(1)
	}

	var outputPath string
	for i, arg := range args {
		if arg == "-o" && i+1 < len(args) {
			outputPath = args[i+1]
		}
	}
	if outputPath == "" {
		outputPath = filepath.Join(cfg.ProjectRoot, "scripts", "ralph", "prd.json")
	}

	// Ensure output directory exists
	os.MkdirAll(filepath.Dir(outputPath), 0755)

	prompt := getTemplate("convert", map[string]string{
		"prdContent":  string(prdContent),
		"outputPath":  outputPath,
		"projectRoot": cfg.ProjectRoot,
	})

	fmt.Printf("Converting PRD: %s\n", prdPath)
	fmt.Printf("Output: %s\n", outputPath)
	fmt.Println()

	if err := runAmpWithPrompt(cfg, prompt); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("Conversion complete!")
	fmt.Printf("  prd.json: %s\n", outputPath)
	fmt.Println()
	fmt.Println("Next: Run 'ralph run' to start implementing stories")
}

func cmdStatus(args []string) {
	cfg := resolveConfig(0, true)

	prd, err := loadPRD(cfg.PrdPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Project: %s\n", prd.Project)
	fmt.Printf("Branch: %s\n", prd.BranchName)
	fmt.Printf("Description: %s\n", prd.Description)
	fmt.Println()

	complete := 0
	for _, s := range prd.UserStories {
		if s.Passes {
			complete++
		}
	}
	fmt.Printf("Progress: %d/%d stories complete\n", complete, len(prd.UserStories))
	fmt.Println()

	fmt.Println("Stories:")
	for _, story := range prd.UserStories {
		status := "○"
		if story.Passes {
			status = "✓"
		}
		retries := ""
		if story.Retries > 0 {
			retries = fmt.Sprintf(" (retries: %d)", story.Retries)
		}
		fmt.Printf("  %s %s: %s%s\n", status, story.ID, story.Title, retries)
		if story.Notes != "" {
			fmt.Printf("    └─ Note: %s\n", story.Notes)
		}
	}
}

func cmdNext(args []string) {
	cfg := resolveConfig(0, true)

	prd, err := loadPRD(cfg.PrdPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	next := getNextStory(prd)
	if next == nil {
		fmt.Println("All stories complete!")
		return
	}

	fmt.Printf("%s: %s\n", next.ID, next.Title)
	fmt.Printf("Priority: %d\n", next.Priority)
	fmt.Printf("Retries: %d\n", next.Retries)
	if next.Notes != "" {
		fmt.Printf("Notes: %s\n", next.Notes)
	}
	fmt.Println()
	fmt.Println("Acceptance Criteria:")
	for _, criterion := range next.AcceptanceCriteria {
		fmt.Printf("  - %s\n", criterion)
	}
}

func cmdValidate(args []string) {
	cfg := resolveConfig(0, true)

	prd, err := loadPRD(cfg.PrdPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ prd.json is valid")
	fmt.Printf("  - %d stories\n", len(prd.UserStories))
	fmt.Printf("  - Schema version: %d\n", prd.SchemaVersion)
}

func cmdDoctor(args []string) {
	cfg := resolveConfig(0, true)
	issues := 0

	fmt.Println("Ralph Environment Check")
	fmt.Println()

	// Check amp is available
	if isCommandAvailable(cfg.Amp.Command) {
		fmt.Printf("✓ Amp command: %s\n", cfg.Amp.Command)
	} else {
		fmt.Printf("✗ Amp command not found: %s\n", cfg.Amp.Command)
		issues++
	}

	// Check project root
	fmt.Printf("✓ Project root: %s\n", cfg.ProjectRoot)
	fmt.Printf("✓ Project type: %s\n", cfg.ProjectType)

	// Check PRD
	if fileExists(cfg.PrdPath) {
		fmt.Printf("✓ PRD file: %s\n", cfg.PrdPath)
	} else {
		fmt.Printf("○ PRD file: %s (not found - run 'ralph init')\n", cfg.PrdPath)
	}

	// Check quality commands
	fmt.Println()
	fmt.Println("Quality commands:")
	if len(cfg.Quality) == 0 {
		fmt.Println("  (none detected - configure in ralph.config.json)")
	} else {
		for _, cmd := range cfg.Quality {
			fmt.Printf("  - %s: %s\n", cmd.Name, cmd.Cmd)
		}
	}

	if issues > 0 {
		fmt.Printf("\n%d issue(s) found.\n", issues)
		os.Exit(1)
	} else {
		fmt.Println("\nAll checks passed.")
	}
}

func runAmpWithPrompt(cfg ResolvedConfig, prompt string) error {
	cmd := exec.Command(cfg.Amp.Command, cfg.Amp.Args...)
	cmd.Dir = cfg.ProjectRoot
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	io.WriteString(stdin, prompt)
	stdin.Close()

	return cmd.Wait()
}

func isFlag(s string) bool {
	return len(s) > 0 && s[0] == '-'
}
