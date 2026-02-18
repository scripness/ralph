package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func checkProviderAvailable(cfg *ResolvedConfig) {
	if !isCommandAvailable(cfg.Config.Provider.Command) {
		fmt.Fprintf(os.Stderr, "Error: provider command '%s' not found in PATH\n", cfg.Config.Provider.Command)
		fmt.Fprintln(os.Stderr, "Install it or update provider.command in ralph.config.json.")
		os.Exit(1)
	}
}

func checkGitAvailable() {
	if !isCommandAvailable("git") {
		fmt.Fprintln(os.Stderr, "Error: git not found in PATH")
		fmt.Fprintln(os.Stderr, "Git is required for branch management and commits.")
		os.Exit(1)
	}
}

// providerChoices is the ordered list of known providers shown during init
var providerChoices = []string{"aider", "amp", "claude", "codex", "opencode"}

// promptProviderSelection prompts the user to select a provider from the known list or enter a custom command.
// reader is accepted as a parameter so tests can inject a bufio.Reader over a controlled input.
func promptProviderSelection(reader *bufio.Reader) string {
	fmt.Println("Select your AI provider:")
	fmt.Println()
	for i, name := range providerChoices {
		fmt.Printf("  %d) %s\n", i+1, name)
	}
	fmt.Printf("  %d) other\n", len(providerChoices)+1)
	fmt.Println()

	for {
		fmt.Printf("Enter choice (1-%d): ", len(providerChoices)+1)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		// Parse numeric choice
		var choice int
		if _, err := fmt.Sscanf(input, "%d", &choice); err != nil || choice < 1 || choice > len(providerChoices)+1 {
			fmt.Printf("Please enter a number between 1 and %d\n", len(providerChoices)+1)
			continue
		}

		// Known provider selected
		if choice <= len(providerChoices) {
			return providerChoices[choice-1]
		}

		// "other" selected — prompt for custom command
		for {
			fmt.Print("Enter provider command: ")
			custom, _ := reader.ReadString('\n')
			custom = strings.TrimSpace(custom)
			if custom != "" {
				return custom
			}
			fmt.Println("Command cannot be empty")
		}
	}
}

// promptVerifyCommands prompts the user for typecheck, lint, and test commands.
// Returns only the non-empty commands. reader is accepted as a parameter so tests can inject controlled input.
// detected is [typecheck, lint, test] from DetectVerifyCommands; empty strings mean no default detected.
func promptVerifyCommands(reader *bufio.Reader, detected [3]string) []string {
	fmt.Println()
	hasDefaults := detected[0] != "" || detected[1] != "" || detected[2] != ""
	if hasDefaults {
		fmt.Println("Verify commands (detected from project config, press Enter to accept or type to override):")
	} else {
		fmt.Println("Verify commands (press Enter to skip any):")
	}
	fmt.Println()

	prompts := []struct {
		label   string
		example string
	}{
		{"Typecheck", "e.g. bun run typecheck, go vet ./..., npx tsc --noEmit"},
		{"Lint", "e.g. bun run lint, golangci-lint run, npx eslint ."},
		{"Test", "e.g. bun run test:unit, go test ./..., pytest"},
	}

	var commands []string
	for i, p := range prompts {
		if detected[i] != "" {
			fmt.Printf("  %s [%s]\n", p.label, detected[i])
		} else {
			fmt.Printf("  %s (%s)\n", p.label, p.example)
		}
		fmt.Print("  > ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			commands = append(commands, input)
		} else if detected[i] != "" {
			commands = append(commands, detected[i])
		}
	}

	return commands
}

// promptServiceConfig prompts the user for dev server configuration.
// Returns nil if the user skips (placeholder will be used).
func promptServiceConfig(reader *bufio.Reader) *ServiceConfig {
	fmt.Println("\nDev server (for service health checks and e2e tests):")
	fmt.Print("  Start command (e.g. npm run dev, mix phx.server): ")
	startCmd, _ := reader.ReadString('\n')
	startCmd = strings.TrimSpace(startCmd)
	if startCmd == "" {
		return nil
	}
	fmt.Print("  Ready URL [http://localhost:3000]: ")
	readyURL, _ := reader.ReadString('\n')
	readyURL = strings.TrimSpace(readyURL)
	if readyURL == "" {
		readyURL = "http://localhost:3000"
	}
	if !strings.HasPrefix(readyURL, "http://") && !strings.HasPrefix(readyURL, "https://") {
		readyURL = "http://" + readyURL
	}
	return &ServiceConfig{Name: "dev", Start: startCmd, Ready: readyURL, ReadyTimeout: 30, RestartBeforeVerify: true}
}

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

	// Prompt user to select a provider
	reader := bufio.NewReader(os.Stdin)
	providerCommand := promptProviderSelection(reader)

	// Detect verify commands from project config files
	tc, lint, test := DetectVerifyCommands(projectRoot)
	detected := [3]string{tc, lint, test}

	// Prompt for verify commands (with auto-detected defaults)
	verifyCommands := promptVerifyCommands(reader, detected)

	// Prompt for service config
	svcConfig := promptServiceConfig(reader)

	// Create ralph.config.json
	if err := WriteDefaultConfig(projectRoot, providerCommand, verifyCommands, svcConfig); err != nil {
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
*/logs/
`
	if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write .gitignore: %v\n", err)
	}

	fmt.Println()
	fmt.Println("Initialized Ralph:")
	fmt.Printf("  Provider: %s\n", providerCommand)
	if len(verifyCommands) > 0 {
		fmt.Printf("  Verify commands: %s\n", strings.Join(verifyCommands, ", "))
	} else {
		fmt.Println("  Verify commands: (placeholders — edit ralph.config.json)")
	}
	fmt.Printf("  Config: %s\n", configPath)
	fmt.Printf("  Data dir: %s\n", ralphDir)
	fmt.Println()
	fmt.Println("Next steps:")
	if len(verifyCommands) == 0 {
		fmt.Println("  1. Edit ralph.config.json with your verify commands")
		fmt.Println("  2. Run 'ralph prd <feature>' to create a PRD")
		fmt.Println("  3. Run 'ralph run <feature>' to start the agent loop")
	} else {
		fmt.Println("  1. Run 'ralph prd <feature>' to create a PRD")
		fmt.Println("  2. Run 'ralph run <feature>' to start the agent loop")
	}
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

	checkProviderAvailable(cfg)
	checkGitAvailable()

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

	def, err := LoadPRDDefinition(featureDir.PrdJsonPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Enforce codebase readiness
	if issues := CheckReadiness(&cfg.Config, def); len(issues) > 0 {
		fmt.Fprintln(os.Stderr, "Error: codebase is not ready for Ralph")
		fmt.Fprintln(os.Stderr, "")
		for _, issue := range issues {
			fmt.Fprintf(os.Stderr, "  ✗ %s\n", issue)
		}
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Prepare your project for agentic work, then try again.")
		fmt.Fprintln(os.Stderr, "Run 'ralph doctor' for a full environment check.")
		os.Exit(1)
	}

	// Environment warnings (soft — warn but don't block)
	if warnings := CheckReadinessWarnings(&cfg.Config); len(warnings) > 0 {
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "  Warning: %s\n", w)
		}
		fmt.Fprintln(os.Stderr, "")
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

	checkProviderAvailable(cfg)
	checkGitAvailable()

	featureDir, err := FindFeatureDir(projectRoot, feature, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if !featureDir.HasPrdJson {
		fmt.Fprintf(os.Stderr, "No prd.json found for feature '%s'\n", feature)
		os.Exit(1)
	}

	def, err := LoadPRDDefinition(featureDir.PrdJsonPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Enforce codebase readiness
	if issues := CheckReadiness(&cfg.Config, def); len(issues) > 0 {
		fmt.Fprintln(os.Stderr, "Error: codebase is not ready for Ralph")
		fmt.Fprintln(os.Stderr, "")
		for _, issue := range issues {
			fmt.Fprintf(os.Stderr, "  ✗ %s\n", issue)
		}
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Prepare your project for agentic work, then try again.")
		fmt.Fprintln(os.Stderr, "Run 'ralph doctor' for a full environment check.")
		os.Exit(1)
	}

	// Environment warnings (soft — warn but don't block)
	if warnings := CheckReadinessWarnings(&cfg.Config); len(warnings) > 0 {
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "  Warning: %s\n", w)
		}
		fmt.Fprintln(os.Stderr, "")
	}

	// Acquire lock to prevent concurrent run+verify
	lock := NewLockFile(projectRoot)
	if err := lock.Acquire(feature, def.BranchName); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer lock.Release()

	fmt.Printf("Feature: %s\n", feature)
	fmt.Printf("Project: %s\n", def.Project)
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

	checkProviderAvailable(cfg)
	checkGitAvailable()

	// Warn about placeholder verify commands (soft — don't block PRD creation)
	if HasPlaceholderVerifyCommands(&cfg.Config) {
		fmt.Fprintln(os.Stderr, "Warning: verify.default contains placeholder commands.")
		fmt.Fprintln(os.Stderr, "Edit ralph.config.json before running 'ralph run'.")
		fmt.Fprintln(os.Stderr, "")
	}

	// Find or create feature directory
	featureDir, err := FindFeatureDir(projectRoot, feature, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Ensure we're on the feature branch before any commits.
	// New branches start from the default branch (main/master), not current HEAD.
	branchName := "ralph/" + feature
	git := NewGitOps(projectRoot)
	if err := git.EnsureBranch(branchName, git.DefaultBranch()); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to switch to branch %s: %v\n", branchName, err)
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
				def, defErr := LoadPRDDefinition(f.PrdJsonPath())
				st, _ := LoadRunState(f.RunStatePath())
				if defErr == nil {
					passed := CountPassed(st)
					total := len(def.UserStories)
					skipped := CountSkipped(st)
					if passed == total {
						status = "✓"
					} else if skipped > 0 {
						status = "!"
					}
					fmt.Printf("  %s %s (%d/%d complete", status, f.Feature, passed, total)
					if skipped > 0 {
						fmt.Printf(", %d skipped", skipped)
					}
					fmt.Println(")")
					continue
				}
			}
			st := "draft"
			if f.HasPrdJson {
				st = "ready"
			} else if f.HasPrdMd {
				st = "needs finalize"
			}
			fmt.Printf("  %s %s (%s)\n", status, f.Feature, st)
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

	def, err := LoadPRDDefinition(featureDir.PrdJsonPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	state, _ := LoadRunState(featureDir.RunStatePath())

	fmt.Printf("Feature: %s\n", feature)
	fmt.Printf("Project: %s\n", def.Project)
	fmt.Printf("Branch: %s\n", def.BranchName)
	fmt.Printf("Description: %s\n", def.Description)
	fmt.Println()

	passed := CountPassed(state)
	skipped := CountSkipped(state)
	fmt.Printf("Progress: %d/%d stories complete", passed, len(def.UserStories))
	if skipped > 0 {
		fmt.Printf(" (%d skipped)", skipped)
	}
	fmt.Println()
	fmt.Println()

	fmt.Println("Stories:")
	for _, story := range def.UserStories {
		status := "○"
		if state.IsPassed(story.ID) {
			status = "✓"
		} else if state.IsSkipped(story.ID) {
			status = "✗"
		}
		retries := ""
		if r := state.GetRetries(story.ID); r > 0 {
			retries = fmt.Sprintf(" (retries: %d)", r)
		}
		tags := ""
		if len(story.Tags) > 0 {
			tags = fmt.Sprintf(" [%s]", story.Tags[0])
			for _, t := range story.Tags[1:] {
				tags += fmt.Sprintf(" [%s]", t)
			}
		}
		fmt.Printf("  %s %s: %s%s%s\n", status, story.ID, story.Title, tags, retries)
		if note := state.GetLastFailure(story.ID); note != "" {
			fmt.Printf("    └─ Note: %s\n", note)
		}
	}
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

	// Check sh
	if isCommandAvailable("sh") {
		fmt.Printf("✓ sh available\n")
	} else {
		fmt.Printf("✗ sh not found\n")
		issues++
	}

	// Check git
	if isCommandAvailable("git") {
		fmt.Printf("✓ git available\n")
	} else {
		fmt.Printf("✗ git not found\n")
		issues++
	}

	// Check git repository
	cwd, _ := os.Getwd()
	gitRoot := findGitRoot(cwd)
	if _, err := os.Stat(filepath.Join(gitRoot, ".git")); err == nil {
		fmt.Printf("✓ git repository found\n")
	} else {
		fmt.Printf("✗ not inside a git repository\n")
		issues++
	}

	// Check .ralph directory writability
	if fi, statErr := os.Stat(ralphDir); statErr == nil && fi.IsDir() {
		testFile := filepath.Join(ralphDir, ".write-test")
		if f, writeErr := os.Create(testFile); writeErr != nil {
			fmt.Printf("✗ .ralph directory not writable\n")
			issues++
		} else {
			f.Close()
			os.Remove(testFile)
			fmt.Printf("✓ .ralph directory writable\n")
		}
	}

	// Check verify commands
	if err == nil {
		if HasPlaceholderVerifyCommands(&cfg.Config) {
			fmt.Printf("✗ verify.default: placeholder commands (replace with real typecheck/lint/test)\n")
			issues++
		} else {
			fmt.Printf("✓ verify.default: %d commands configured\n", len(cfg.Config.Verify.Default))
		}

		if len(cfg.Config.Verify.UI) > 0 {
			fmt.Printf("✓ verify.ui: %d commands configured\n", len(cfg.Config.Verify.UI))
		} else {
			fmt.Printf("○ verify.ui: no commands (required for UI stories)\n")
		}

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
			fmt.Printf("! Ralph is currently running (PID %d, feature: %s)\n", lock.PID, lock.Feature)
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

func cmdLogs(args []string) {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	runNum := fs.Int("run", 0, "Show specific run number (default: latest)")
	listRuns := fs.Bool("list", false, "List all runs with summary")
	tail := fs.Int("tail", 50, "Show last N events")
	follow := fs.Bool("follow", false, "Follow log in real-time")
	fs.BoolVar(follow, "f", false, "Follow log in real-time (shorthand)")
	eventType := fs.String("type", "", "Filter by event type")
	storyID := fs.String("story", "", "Filter by story ID")
	jsonOutput := fs.Bool("json", false, "Output raw JSONL")
	summaryMode := fs.Bool("summary", false, "Show run summary only")

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: ralph logs <feature> [options]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Options:")
		fs.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  ralph logs auth                    # Latest run, last 50 events")
		fmt.Fprintln(os.Stderr, "  ralph logs auth --list             # List all runs")
		fmt.Fprintln(os.Stderr, "  ralph logs auth --run 2            # Show run #2")
		fmt.Fprintln(os.Stderr, "  ralph logs auth --follow           # Watch current run live")
		fmt.Fprintln(os.Stderr, "  ralph logs auth --type error       # Show only errors")
		fmt.Fprintln(os.Stderr, "  ralph logs auth --story US-001     # Events for specific story")
		fmt.Fprintln(os.Stderr, "  ralph logs auth --summary          # Quick summary of latest run")
	}

	// Find feature argument before flags
	var feature string
	var flagArgs []string
	for i, arg := range args {
		if strings.HasPrefix(arg, "-") {
			flagArgs = args[i:]
			break
		}
		if feature == "" {
			feature = arg
		}
	}
	if feature == "" && len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		feature = args[0]
		flagArgs = args[1:]
	}

	if feature == "" {
		fmt.Fprintln(os.Stderr, "Usage: ralph logs <feature> [options]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Example: ralph logs auth")
		os.Exit(1)
	}

	fs.Parse(flagArgs)

	projectRoot := GetProjectRoot()
	featureDir, err := FindFeatureDir(projectRoot, feature, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	runs, err := ListRuns(featureDir.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading logs: %v\n", err)
		os.Exit(1)
	}

	if len(runs) == 0 {
		fmt.Printf("No logs found for feature '%s'\n", feature)
		fmt.Printf("Run 'ralph run %s' to create logs.\n", feature)
		return
	}

	// --list mode: show all runs
	if *listRuns {
		fmt.Printf("Runs for feature '%s':\n\n", feature)
		for _, run := range runs {
			status := "○"
			if run.Success != nil {
				if *run.Success {
					status = "✓"
				} else {
					status = "✗"
				}
			}

			duration := ""
			if run.EndTime != nil {
				d := run.EndTime.Sub(run.StartTime)
				duration = fmt.Sprintf(" (%s)", FormatDuration(d))
			}

			fmt.Printf("  %s Run #%d - %s%s\n", status, run.RunNumber,
				run.StartTime.Format("2006-01-02 15:04:05"), duration)
			if run.Summary != "" {
				fmt.Printf("    └─ %s\n", run.Summary)
			}
		}
		return
	}

	// Find the target run
	var targetRun *RunSummary
	if *runNum > 0 {
		for i := range runs {
			if runs[i].RunNumber == *runNum {
				targetRun = &runs[i]
				break
			}
		}
		if targetRun == nil {
			fmt.Fprintf(os.Stderr, "Run #%d not found\n", *runNum)
			os.Exit(1)
		}
	} else {
		// Default to latest run
		targetRun = &runs[0]
	}

	// --summary mode: show detailed summary
	if *summaryMode {
		printRunSummary(targetRun.LogPath)
		return
	}

	// --follow mode: tail the log file
	if *follow {
		followLog(targetRun.LogPath, *eventType, *storyID, *jsonOutput)
		return
	}

	// Default: show last N events
	printEvents(targetRun.LogPath, *tail, *eventType, *storyID, *jsonOutput)
}

func printRunSummary(logPath string) {
	summary, err := GetRunSummary(logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading log: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Run #%d - %s\n", summary.RunNumber, summary.StartTime.Format("2006-01-02 15:04:05"))
	if summary.Duration != nil {
		fmt.Printf("Duration: %s\n", FormatDuration(*summary.Duration))
	}
	if summary.Success != nil {
		result := "FAILED"
		if *summary.Success {
			result = "PASSED"
		}
		fmt.Printf("Result: %s\n", result)
	}
	if summary.Result != "" {
		fmt.Printf("Summary: %s\n", summary.Result)
	}

	fmt.Println()
	fmt.Printf("Stories: %d total\n", len(summary.Stories))
	for _, story := range summary.Stories {
		status := "○"
		if story.Success != nil {
			if *story.Success {
				status = "✓"
			} else {
				status = "✗"
			}
		}
		duration := ""
		if story.Duration != nil {
			duration = fmt.Sprintf(" (%s", FormatDuration(*story.Duration))
			if story.Retries > 0 {
				duration += fmt.Sprintf(", %d retries", story.Retries)
			}
			duration += ")"
		} else if story.Retries > 0 {
			duration = fmt.Sprintf(" (%d retries)", story.Retries)
		}
		fmt.Printf("  %s %s: %s%s\n", status, story.ID, story.Title, duration)
	}

	if len(summary.VerifyResults) > 0 {
		fmt.Println()
		fmt.Println("Verification:")
		for _, v := range summary.VerifyResults {
			status := "✓"
			if !v.Success {
				status = "✗"
			}
			fmt.Printf("  %s %s (%s)\n", status, v.Command, FormatDuration(v.Duration))
		}
	}

	fmt.Println()
	if len(summary.Learnings) > 0 {
		fmt.Printf("Learnings captured: %d\n", len(summary.Learnings))
	}
	fmt.Printf("Warnings: %d\n", summary.Warnings)
	fmt.Printf("Errors: %d\n", summary.Errors)
}

func printEvents(logPath string, tailN int, eventTypeFilter, storyFilter string, jsonOutput bool) {
	events, err := ReadEvents(logPath, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading log: %v\n", err)
		os.Exit(1)
	}

	// Apply filters
	var filtered []Event
	for _, e := range events {
		if eventTypeFilter != "" && string(e.Type) != eventTypeFilter {
			continue
		}
		if storyFilter != "" && e.StoryID != storyFilter {
			continue
		}
		filtered = append(filtered, e)
	}

	// Take last N
	if len(filtered) > tailN {
		filtered = filtered[len(filtered)-tailN:]
	}

	for _, e := range filtered {
		if jsonOutput {
			data, _ := json.Marshal(e)
			fmt.Println(string(data))
		} else {
			printEvent(&e)
		}
	}
}

func printEvent(e *Event) {
	timestamp := e.Timestamp.Format("15:04:05")

	// Format based on event type
	switch e.Type {
	case EventRunStart:
		feature, _ := e.Data["feature"].(string)
		fmt.Printf("[%s] === Run started: %s ===\n", timestamp, feature)

	case EventRunEnd:
		result := "failed"
		if e.Success != nil && *e.Success {
			result = "success"
		}
		fmt.Printf("[%s] === Run ended: %s ===\n", timestamp, result)
		if e.Message != "" {
			fmt.Printf("         %s\n", e.Message)
		}

	case EventIterationStart:
		title, _ := e.Data["title"].(string)
		fmt.Printf("[%s] ─── Iteration %d: %s - %s ───\n", timestamp, e.Iteration, e.StoryID, title)

	case EventIterationEnd:
		status := "✗"
		if e.Success != nil && *e.Success {
			status = "✓"
		}
		duration := ""
		if e.Duration != nil {
			duration = fmt.Sprintf(" (%s)", FormatDuration(time.Duration(*e.Duration)))
		}
		fmt.Printf("[%s] %s Iteration %d complete%s\n", timestamp, status, e.Iteration, duration)

	case EventProviderStart:
		fmt.Printf("[%s] → Provider started\n", timestamp)

	case EventProviderEnd:
		status := "✗"
		if e.Success != nil && *e.Success {
			status = "✓"
		}
		duration := ""
		if e.Duration != nil {
			duration = fmt.Sprintf(" (%s)", FormatDuration(time.Duration(*e.Duration)))
		}
		markers := ""
		if m, ok := e.Data["markers"].([]interface{}); ok && len(m) > 0 {
			var ms []string
			for _, v := range m {
				if s, ok := v.(string); ok {
					ms = append(ms, s)
				}
			}
			markers = fmt.Sprintf(" [%s]", strings.Join(ms, ", "))
		}
		fmt.Printf("[%s] %s Provider complete%s%s\n", timestamp, status, duration, markers)

	case EventMarkerDetected:
		marker, _ := e.Data["marker"].(string)
		value, _ := e.Data["value"].(string)
		if value != "" {
			fmt.Printf("[%s]   ◆ %s: %s\n", timestamp, marker, value)
		} else {
			fmt.Printf("[%s]   ◆ %s\n", timestamp, marker)
		}

	case EventVerifyStart:
		fmt.Printf("[%s] → Verification started\n", timestamp)

	case EventVerifyEnd:
		status := "✗"
		if e.Success != nil && *e.Success {
			status = "✓"
		}
		duration := ""
		if e.Duration != nil {
			duration = fmt.Sprintf(" (%s)", FormatDuration(time.Duration(*e.Duration)))
		}
		fmt.Printf("[%s] %s Verification complete%s\n", timestamp, status, duration)

	case EventVerifyCmdStart:
		cmd, _ := e.Data["cmd"].(string)
		fmt.Printf("[%s]   → %s\n", timestamp, cmd)

	case EventVerifyCmdEnd:
		cmd, _ := e.Data["cmd"].(string)
		status := "✗"
		if e.Success != nil && *e.Success {
			status = "✓"
		}
		duration := ""
		if e.Duration != nil {
			duration = fmt.Sprintf(" (%s)", FormatDuration(time.Duration(*e.Duration)))
		}
		fmt.Printf("[%s]   %s %s%s\n", timestamp, status, cmd, duration)

	case EventServiceStart:
		name, _ := e.Data["name"].(string)
		fmt.Printf("[%s] → Service starting: %s\n", timestamp, name)

	case EventServiceReady:
		name, _ := e.Data["name"].(string)
		duration := ""
		if e.Duration != nil {
			duration = fmt.Sprintf(" (%s)", FormatDuration(time.Duration(*e.Duration)))
		}
		fmt.Printf("[%s] ✓ Service ready: %s%s\n", timestamp, name, duration)

	case EventStateChange:
		from, _ := e.Data["from"].(string)
		to, _ := e.Data["to"].(string)
		fmt.Printf("[%s] ↔ State: %s → %s\n", timestamp, from, to)

	case EventProviderLine:
		line, _ := e.Data["line"].(string)
		fmt.Printf("[%s]   %s\n", timestamp, line)

	case EventLearning:
		fmt.Printf("[%s] ~ Learning: %s\n", timestamp, e.Message)

	case EventWarning:
		fmt.Printf("[%s] ! Warning: %s\n", timestamp, e.Message)

	case EventError:
		fmt.Printf("[%s] ✗ Error: %s\n", timestamp, e.Message)
		if errMsg, ok := e.Data["error"].(string); ok {
			fmt.Printf("         %s\n", errMsg)
		}

	default:
		fmt.Printf("[%s] %s", timestamp, e.Type)
		if e.StoryID != "" {
			fmt.Printf(" [%s]", e.StoryID)
		}
		if e.Message != "" {
			fmt.Printf(": %s", e.Message)
		}
		fmt.Println()
	}
}

func followLog(logPath, eventTypeFilter, storyFilter string, jsonOutput bool) {
	file, err := os.Open(logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening log: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	// Seek to end
	file.Seek(0, io.SeekEnd)

	fmt.Printf("Following %s (Ctrl+C to stop)\n\n", logPath)

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			return
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		// Apply filters
		if eventTypeFilter != "" && string(event.Type) != eventTypeFilter {
			continue
		}
		if storyFilter != "" && event.StoryID != storyFilter {
			continue
		}

		if jsonOutput {
			fmt.Println(line)
		} else {
			printEvent(&event)
		}
	}
}

