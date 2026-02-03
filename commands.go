package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func cmdInit(args []string) {
	force := false
	for _, arg := range args {
		if arg == "-f" || arg == "--force" {
			force = true
		}
	}

	projectRoot := GetProjectRoot()
	configPath := ConfigPath(projectRoot)
	ralphDir := filepath.Join(projectRoot, ".ralph")

	// Check if already initialized
	if fileExists(configPath) && !force {
		fmt.Fprintf(os.Stderr, "ralph.config.json already exists at %s\n", configPath)
		fmt.Fprintln(os.Stderr, "Use --force to overwrite.")
		os.Exit(1)
	}

	// Create ralph.config.json
	if err := WriteDefaultConfig(projectRoot); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write config: %v\n", err)
		os.Exit(1)
	}

	// Create .ralph directory
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create .ralph directory: %v\n", err)
		os.Exit(1)
	}

	// Create .ralph/.gitignore
	gitignorePath := filepath.Join(ralphDir, ".gitignore")
	gitignoreContent := `# Ralph temporary files
ralph.lock
*.tmp
screenshots/
`
	if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write .gitignore: %v\n", err)
	}

	fmt.Println("Initialized Ralph:")
	fmt.Printf("  - Created %s\n", configPath)
	fmt.Printf("  - Created %s\n", ralphDir)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Edit ralph.config.json with your provider and verify commands")
	fmt.Println("  2. Run 'ralph prd <feature>' to create a PRD")
	fmt.Println("  3. Run 'ralph run <feature>' to start the agent loop")
}

func cmdRun(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: ralph run <feature>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Example: ralph run auth")
		os.Exit(1)
	}

	feature := args[0]
	projectRoot := GetProjectRoot()

	cfg, err := LoadConfig(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	featureDir, err := FindFeatureDir(projectRoot, feature, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if !featureDir.HasPrdJson {
		fmt.Fprintf(os.Stderr, "No prd.json found for feature '%s'\n", feature)
		fmt.Fprintf(os.Stderr, "Run 'ralph prd %s' to create and finalize a PRD first.\n", feature)
		os.Exit(1)
	}

	if err := runLoop(cfg, featureDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func cmdVerify(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: ralph verify <feature>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Example: ralph verify auth")
		os.Exit(1)
	}

	feature := args[0]
	projectRoot := GetProjectRoot()

	cfg, err := LoadConfig(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	featureDir, err := FindFeatureDir(projectRoot, feature, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if !featureDir.HasPrdJson {
		fmt.Fprintf(os.Stderr, "No prd.json found for feature '%s'\n", feature)
		os.Exit(1)
	}

	prd, err := LoadPRD(featureDir.PrdJsonPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Acquire lock to prevent concurrent run+verify
	lock := NewLockFile(projectRoot)
	if err := lock.Acquire(feature, prd.BranchName); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer lock.Release()

	fmt.Printf("Feature: %s\n", feature)
	fmt.Printf("Project: %s\n", prd.Project)
	fmt.Printf("Path: %s\n", featureDir.Path)
	fmt.Println()

	if err := runVerify(cfg, featureDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func cmdPrd(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: ralph prd <feature>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Example: ralph prd auth")
		os.Exit(1)
	}

	feature := args[0]
	projectRoot := GetProjectRoot()

	cfg, err := LoadConfig(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Find or create feature directory
	featureDir, err := FindFeatureDir(projectRoot, feature, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := runPrdStateMachine(cfg, featureDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func cmdStatus(args []string) {
	projectRoot := GetProjectRoot()

	// If no feature specified, show all features
	if len(args) == 0 {
		features, err := ListFeatures(projectRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if len(features) == 0 {
			fmt.Println("No features found.")
			fmt.Println("Run 'ralph prd <feature>' to create one.")
			return
		}

		fmt.Println("Features:")
		for _, f := range features {
			status := "○"
			if f.HasPrdJson {
				prd, err := LoadPRD(f.PrdJsonPath())
				if err == nil {
					complete := CountComplete(prd)
					total := len(prd.UserStories)
					blocked := CountBlocked(prd)
					if complete == total {
						status = "✓"
					} else if blocked > 0 {
						status = "!"
					}
					fmt.Printf("  %s %s (%d/%d complete", status, f.Feature, complete, total)
					if blocked > 0 {
						fmt.Printf(", %d blocked", blocked)
					}
					fmt.Println(")")
					continue
				}
			}
			state := "draft"
			if f.HasPrdJson {
				state = "ready"
			} else if f.HasPrdMd {
				state = "needs finalize"
			}
			fmt.Printf("  %s %s (%s)\n", status, f.Feature, state)
		}
		return
	}

	// Show specific feature
	feature := args[0]
	featureDir, err := FindFeatureDir(projectRoot, feature, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if !featureDir.HasPrdJson {
		fmt.Printf("Feature: %s\n", feature)
		fmt.Printf("Path: %s\n", featureDir.Path)
		fmt.Printf("Status: ")
		if featureDir.HasPrdMd {
			fmt.Println("PRD drafted, not finalized")
			fmt.Printf("\nRun 'ralph prd %s' to finalize.\n", feature)
		} else {
			fmt.Println("No PRD")
			fmt.Printf("\nRun 'ralph prd %s' to create one.\n", feature)
		}
		return
	}

	prd, err := LoadPRD(featureDir.PrdJsonPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Feature: %s\n", feature)
	fmt.Printf("Project: %s\n", prd.Project)
	fmt.Printf("Branch: %s\n", prd.BranchName)
	fmt.Printf("Description: %s\n", prd.Description)
	fmt.Println()

	complete := CountComplete(prd)
	blocked := CountBlocked(prd)
	fmt.Printf("Progress: %d/%d stories complete", complete, len(prd.UserStories))
	if blocked > 0 {
		fmt.Printf(" (%d blocked)", blocked)
	}
	fmt.Println()
	fmt.Println()

	fmt.Println("Stories:")
	for _, story := range prd.UserStories {
		status := "○"
		if story.Passes {
			status = "✓"
		} else if story.Blocked {
			status = "✗"
		}
		retries := ""
		if story.Retries > 0 {
			retries = fmt.Sprintf(" (retries: %d)", story.Retries)
		}
		tags := ""
		if len(story.Tags) > 0 {
			tags = fmt.Sprintf(" [%s]", story.Tags[0])
			for _, t := range story.Tags[1:] {
				tags += fmt.Sprintf(" [%s]", t)
			}
		}
		fmt.Printf("  %s %s: %s%s%s\n", status, story.ID, story.Title, tags, retries)
		if story.Notes != "" {
			fmt.Printf("    └─ Note: %s\n", story.Notes)
		}
	}
}

func cmdNext(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: ralph next <feature>")
		os.Exit(1)
	}

	feature := args[0]
	projectRoot := GetProjectRoot()

	featureDir, err := FindFeatureDir(projectRoot, feature, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if !featureDir.HasPrdJson {
		fmt.Fprintf(os.Stderr, "No prd.json found for feature '%s'\n", feature)
		os.Exit(1)
	}

	prd, err := LoadPRD(featureDir.PrdJsonPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	next := GetNextStory(prd)
	if next == nil {
		if HasBlockedStories(prd) {
			fmt.Println("All remaining stories are blocked.")
			fmt.Println("Blocked stories:")
			for _, s := range GetBlockedStories(prd) {
				fmt.Printf("  - %s: %s\n", s.ID, s.Title)
				if s.Notes != "" {
					fmt.Printf("    └─ %s\n", s.Notes)
				}
			}
		} else {
			fmt.Println("All stories complete!")
		}
		return
	}

	fmt.Printf("%s: %s\n", next.ID, next.Title)
	fmt.Printf("Priority: %d\n", next.Priority)
	if len(next.Tags) > 0 {
		fmt.Printf("Tags: %v\n", next.Tags)
	}
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
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: ralph validate <feature>")
		os.Exit(1)
	}

	feature := args[0]
	projectRoot := GetProjectRoot()

	featureDir, err := FindFeatureDir(projectRoot, feature, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if !featureDir.HasPrdJson {
		fmt.Fprintf(os.Stderr, "No prd.json found for feature '%s'\n", feature)
		os.Exit(1)
	}

	prd, err := LoadPRD(featureDir.PrdJsonPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ prd.json is valid")
	fmt.Printf("  - %d stories\n", len(prd.UserStories))
	fmt.Printf("  - Schema version: %d\n", prd.SchemaVersion)
}

func cmdDoctor(args []string) {
	projectRoot := GetProjectRoot()
	issues := 0

	fmt.Println("Ralph Environment Check")
	fmt.Println()

	// Check ralph.config.json
	cfg, err := LoadConfig(projectRoot)
	if err != nil {
		fmt.Printf("✗ ralph.config.json: %v\n", err)
		issues++
	} else {
		fmt.Printf("✓ ralph.config.json found\n")

		// Check provider command
		if isCommandAvailable(cfg.Config.Provider.Command) {
			fmt.Printf("✓ Provider command: %s\n", cfg.Config.Provider.Command)
		} else {
			fmt.Printf("✗ Provider command not found: %s\n", cfg.Config.Provider.Command)
			issues++
		}
	}

	// Check .ralph directory
	ralphDir := filepath.Join(projectRoot, ".ralph")
	if fileExists(ralphDir) {
		fmt.Printf("✓ .ralph directory exists\n")
	} else {
		fmt.Printf("○ .ralph directory: not found (run 'ralph init')\n")
	}

	// Check git
	if isCommandAvailable("git") {
		fmt.Printf("✓ git available\n")
	} else {
		fmt.Printf("✗ git not found\n")
		issues++
	}

	// List features
	features, _ := ListFeatures(projectRoot)
	fmt.Println()
	if len(features) > 0 {
		fmt.Printf("Features: %d\n", len(features))
		for _, f := range features {
			state := "draft"
			if f.HasPrdJson {
				state = "ready"
			} else if f.HasPrdMd {
				state = "drafted"
			}
			fmt.Printf("  - %s (%s)\n", f.Feature, state)
		}
	} else {
		fmt.Println("Features: none")
	}

	// Check lock status
	lock, _ := ReadLockStatus(projectRoot)
	if lock != nil {
		fmt.Println()
		if isProcessAlive(lock.PID) {
			fmt.Printf("⚠ Ralph is currently running (PID %d, feature: %s)\n", lock.PID, lock.Feature)
		} else {
			fmt.Printf("○ Stale lock found (PID %d no longer running)\n", lock.PID)
		}
	}

	fmt.Println()
	if issues > 0 {
		fmt.Printf("%d issue(s) found.\n", issues)
		os.Exit(1)
	} else {
		fmt.Println("All checks passed.")
	}
}
