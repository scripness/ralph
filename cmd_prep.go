package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// cmdPrep implements "scrip prep" — detect project, generate config,
// cache dependencies, audit harness. Non-interactive and idempotent.
func cmdPrep(args []string) {
	_ = args
	projectRoot := GetProjectRoot()

	fmt.Printf("Preparing project: %s\n\n", filepath.Base(projectRoot))

	// Detect project type and package manager
	techStack, packageManager := detectTechStack(projectRoot)
	if techStack == "unknown" {
		fmt.Fprintln(os.Stderr, "Could not detect project type.")
		fmt.Fprintln(os.Stderr, "Supported: go, typescript, javascript, python, rust, elixir")
		os.Exit(1)
	}
	fmt.Printf("  Detected: %s", techStack)
	if packageManager != "" && packageManager != techStack {
		fmt.Printf(" (%s)", packageManager)
	}
	fmt.Println()

	// Detect verify commands
	typecheck, lint, test := DetectVerifyCommands(projectRoot)

	// Build config from detection
	cfg := &ScripConfig{
		Project: ProjectConfig{
			Name: filepath.Base(projectRoot),
			Type: techStack,
		},
		Verify: ScripVerifyConfig{
			Typecheck: typecheck,
			Lint:      lint,
			Test:      test,
		},
	}

	// Merge with existing config to preserve user customizations
	if existing, err := loadExistingScripConfig(projectRoot); err == nil {
		mergeWithExisting(cfg, existing)
		fmt.Println("  Merged with existing config")
	}

	// Write .scrip/config.json
	if err := WriteDefaultScripConfig(projectRoot, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write config: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  Written: .scrip/config.json")

	// Write .scrip/.gitignore
	writeScripGitignore(projectRoot)
	fmt.Println("  Written: .scrip/.gitignore")

	// Resolve dependencies to ~/.scrip/resources/
	fmt.Println()
	resolved := &ScripResolvedConfig{ProjectRoot: projectRoot, Config: *cfg}
	codebaseCtx := DiscoverScripCodebase(projectRoot, cfg)
	ensureScripResourceSync(resolved, codebaseCtx)

	// Harness audit
	fmt.Println()
	auditHarness(projectRoot, techStack, cfg)

	// Next steps
	fmt.Println()
	fmt.Println("Ready. Next: scrip plan <feature> \"description\"")
}

// loadExistingScripConfig reads .scrip/config.json without validation.
// Used to preserve user customizations when re-running scrip prep.
func loadExistingScripConfig(projectRoot string) (*ScripConfig, error) {
	data, err := os.ReadFile(ScripConfigPath(projectRoot))
	if err != nil {
		return nil, err
	}
	var cfg ScripConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// mergeWithExisting preserves user customizations from an existing config.
// Detection fills empty slots; existing non-empty values are kept.
func mergeWithExisting(detected, existing *ScripConfig) {
	// Preserve user-customized verify commands
	if existing.Verify.Typecheck != "" {
		detected.Verify.Typecheck = existing.Verify.Typecheck
	}
	if existing.Verify.Lint != "" {
		detected.Verify.Lint = existing.Verify.Lint
	}
	if existing.Verify.Test != "" {
		detected.Verify.Test = existing.Verify.Test
	}
	// Preserve services (always user-configured)
	if len(existing.Services) > 0 {
		detected.Services = existing.Services
	}
	// Preserve non-default provider timeouts
	if existing.Provider.Timeout > 0 && existing.Provider.Timeout != 1800 {
		detected.Provider.Timeout = existing.Provider.Timeout
	}
	if existing.Provider.StallTimeout > 0 && existing.Provider.StallTimeout != 300 {
		detected.Provider.StallTimeout = existing.Provider.StallTimeout
	}
}

// writeScripGitignore creates .scrip/.gitignore for temporary files.
func writeScripGitignore(projectRoot string) {
	gitignorePath := filepath.Join(ScripConfigDir(projectRoot), ".gitignore")
	content := `# Scrip temporary files
scrip.lock
*/logs/
state.json
plan.jsonl
`
	if err := os.WriteFile(gitignorePath, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write .gitignore: %v\n", err)
	}
}

// auditHarness checks project harness for gaps and prints recommendations.
// Report-only — does not auto-fix.
func auditHarness(projectRoot, techStack string, cfg *ScripConfig) {
	fmt.Println("Harness audit:")
	gaps := 0

	// Check test command
	if cfg.Verify.Test == "" {
		fmt.Println("  ! No test command detected. Add verify.test to .scrip/config.json")
		gaps++
	}

	// Check typecheck
	if cfg.Verify.Typecheck == "" {
		switch techStack {
		case "typescript":
			fmt.Println("  ! No typecheck configured. Consider: npx tsc --noEmit")
			gaps++
		}
	}

	// Check linter
	if cfg.Verify.Lint == "" {
		suggestion := ""
		switch techStack {
		case "go":
			suggestion = "golangci-lint run"
		case "typescript", "javascript":
			suggestion = "add a lint script to package.json"
		case "python":
			suggestion = "ruff check ."
		case "rust":
			suggestion = "cargo clippy"
		case "elixir":
			suggestion = "mix credo"
		}
		if suggestion != "" {
			fmt.Printf("  ! No linter configured. Consider: %s\n", suggestion)
			gaps++
		}
	}

	// Check SAST tools
	if !isCommandAvailable("semgrep") {
		fmt.Println("  ! semgrep not installed. Recommended for cross-language SAST")
		gaps++
	}

	switch techStack {
	case "go":
		if !isCommandAvailable("gosec") {
			fmt.Println("  ! gosec not installed. Recommended for Go security analysis")
			gaps++
		}
	case "python":
		if !isCommandAvailable("bandit") {
			fmt.Println("  ! bandit not installed. Recommended for Python security analysis")
			gaps++
		}
	}

	if gaps == 0 {
		fmt.Println("  All checks passed")
	} else {
		fmt.Printf("  %d recommendation(s) — not blockers\n", gaps)
	}
}
