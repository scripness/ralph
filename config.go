package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ProviderConfig configures the AI provider CLI
type ProviderConfig struct {
	Command       string   `json:"command"`
	Args          []string `json:"args"`
	Timeout       int      `json:"timeout"`       // seconds per iteration
	PromptMode    string   `json:"promptMode"`    // "stdin", "arg", or "file" (auto-detected if empty)
	PromptFlag    string   `json:"promptFlag"`    // flag before prompt in arg/file modes (e.g. "--message")
	KnowledgeFile string   `json:"knowledgeFile"` // "AGENTS.md", "CLAUDE.md", etc. (auto-detected if empty)
}

// ProviderDefaults contains default settings for known providers
type ProviderDefaults struct {
	PromptMode    string
	PromptFlag    string
	DefaultArgs   []string
	KnowledgeFile string
}

// knownProviders maps provider commands to their defaults
var knownProviders = map[string]ProviderDefaults{
	"amp":      {PromptMode: "stdin", DefaultArgs: []string{"--dangerously-allow-all"}, KnowledgeFile: "AGENTS.md"},
	"claude":   {PromptMode: "stdin", DefaultArgs: []string{"--print", "--dangerously-skip-permissions"}, KnowledgeFile: "CLAUDE.md"},
	"opencode": {PromptMode: "arg", DefaultArgs: []string{"run"}, KnowledgeFile: "AGENTS.md"},
	"aider":    {PromptMode: "arg", PromptFlag: "--message", DefaultArgs: []string{"--yes-always"}, KnowledgeFile: "AGENTS.md"},
	"codex":    {PromptMode: "arg", DefaultArgs: []string{"exec", "--full-auto"}, KnowledgeFile: "AGENTS.md"},
}

// defaultProviderDefaults is used for unknown providers
var defaultProviderDefaults = ProviderDefaults{
	PromptMode:    "stdin",
	KnowledgeFile: "AGENTS.md",
}

// ServiceConfig configures a managed service (e.g., dev server)
type ServiceConfig struct {
	Name                string `json:"name"`
	Start               string `json:"start,omitempty"`
	Ready               string `json:"ready"` // URL to check
	ReadyTimeout        int    `json:"readyTimeout,omitempty"`
	RestartBeforeVerify bool   `json:"restartBeforeVerify,omitempty"`
}

// VerifyConfig configures verification commands
type VerifyConfig struct {
	Default []string `json:"default"`
	UI      []string `json:"ui,omitempty"`
	Timeout int      `json:"timeout,omitempty"` // seconds per command, default 300
}

// BrowserConfig configures built-in browser verification
type BrowserConfig struct {
	Enabled        bool   `json:"enabled,omitempty"`
	ExecutablePath string `json:"executablePath,omitempty"`
	Headless       bool   `json:"headless,omitempty"`
	ScreenshotDir  string `json:"screenshotDir,omitempty"`
}

// CommitsConfig configures git commit behavior
type CommitsConfig struct {
	PrdChanges bool   `json:"prdChanges,omitempty"`
	Message    string `json:"message,omitempty"`
}

// RalphConfig is the main configuration loaded from ralph.config.json
type RalphConfig struct {
	MaxRetries int              `json:"maxRetries,omitempty"`
	Provider   ProviderConfig   `json:"provider"`
	Services   []ServiceConfig  `json:"services,omitempty"`
	Verify     VerifyConfig     `json:"verify"`
	Browser    *BrowserConfig   `json:"browser,omitempty"`
	Commits    *CommitsConfig   `json:"commits,omitempty"`
	Logging    *LoggingConfig   `json:"logging,omitempty"`
}

// ResolvedConfig is the fully resolved configuration
type ResolvedConfig struct {
	ProjectRoot string
	Config      RalphConfig
}

// ConfigPath returns the path to ralph.config.json
func ConfigPath(projectRoot string) string {
	return filepath.Join(projectRoot, "ralph.config.json")
}

// LoadConfig loads and validates ralph.config.json
func LoadConfig(projectRoot string) (*ResolvedConfig, error) {
	configPath := ConfigPath(projectRoot)
	
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("ralph.config.json not found\n\nRun 'ralph init' to create one")
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg RalphConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid ralph.config.json: %w", err)
	}

	// Apply defaults
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.Provider.Timeout <= 0 {
		cfg.Provider.Timeout = 1800 // 30 minutes
	}

	// Auto-detect provider defaults based on command
	applyProviderDefaults(&cfg.Provider)
	if cfg.Browser == nil {
		cfg.Browser = &BrowserConfig{
			Enabled:       true,
			Headless:      true,
			ScreenshotDir: ".ralph/screenshots",
		}
	}
	if cfg.Commits == nil {
		cfg.Commits = &CommitsConfig{
			PrdChanges: true,
			Message:    "chore: update prd.json",
		}
	}
	if cfg.Logging == nil {
		cfg.Logging = DefaultLoggingConfig()
	}

	// Apply service defaults
	for i := range cfg.Services {
		if cfg.Services[i].ReadyTimeout <= 0 {
			cfg.Services[i].ReadyTimeout = 30
		}
	}

	// Apply verify timeout default
	if cfg.Verify.Timeout <= 0 {
		cfg.Verify.Timeout = 300 // 5 minutes per command
	}

	// Validate required fields
	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}

	return &ResolvedConfig{
		ProjectRoot: projectRoot,
		Config:      cfg,
	}, nil
}

// validateConfig validates the configuration
func validateConfig(cfg *RalphConfig) error {
	if cfg.Provider.Command == "" {
		return fmt.Errorf("provider.command is required")
	}
	if len(cfg.Verify.Default) == 0 {
		return fmt.Errorf("verify.default must have at least one command")
	}
	for i, svc := range cfg.Services {
		if svc.Name == "" {
			return fmt.Errorf("services[%d].name is required", i)
		}
		if svc.Ready == "" {
			return fmt.Errorf("services[%d].ready is required", i)
		}
	}
	return nil
}

// findGitRoot finds the git root from a starting directory
func findGitRoot(start string) string {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return start
		}
		dir = parent
	}
}

// GetProjectRoot returns the project root (git root or cwd)
func GetProjectRoot() string {
	cwd, _ := os.Getwd()
	return findGitRoot(cwd)
}

// isCommandAvailable checks if a command is available in PATH
func isCommandAvailable(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// applyProviderDefaults sets PromptMode, PromptFlag, Args, and KnowledgeFile based on known providers
func applyProviderDefaults(p *ProviderConfig) {
	// Get defaults for this provider (or use fallback)
	defaults, ok := knownProviders[p.Command]
	if !ok {
		defaults = defaultProviderDefaults
	}

	// Only apply if not already set by user
	if p.PromptMode == "" {
		p.PromptMode = defaults.PromptMode
	}
	if p.PromptFlag == "" {
		p.PromptFlag = defaults.PromptFlag
	}
	if p.KnowledgeFile == "" {
		p.KnowledgeFile = defaults.KnowledgeFile
	}

	// Apply default args only when Args is nil (JSON key absent).
	// "args": [] (explicit empty) preserves user intent — no defaults applied.
	if p.Args == nil && len(defaults.DefaultArgs) > 0 {
		p.Args = append([]string{}, defaults.DefaultArgs...)
	}

	// Validate promptMode
	switch p.PromptMode {
	case "stdin", "arg", "file":
		// valid
	default:
		p.PromptMode = "stdin" // fallback to safest
	}
}

// WriteDefaultConfig writes a default ralph.config.json
func WriteDefaultConfig(projectRoot string) error {
	cfg := RalphConfig{
		MaxRetries: 3,
		Provider: ProviderConfig{
			Command: "amp",
			Timeout: 1800,
		},
		Verify: VerifyConfig{
			Default: []string{
				"echo 'Add your typecheck command'",
				"echo 'Add your lint command'",
				"echo 'Add your test command'",
			},
		},
		Services: []ServiceConfig{},
		Browser: &BrowserConfig{
			Enabled:       true,
			Headless:      true,
			ScreenshotDir: ".ralph/screenshots",
		},
		Commits: &CommitsConfig{
			PrdChanges: true,
			Message:    "chore: update prd.json",
		},
	}

	return AtomicWriteJSON(ConfigPath(projectRoot), cfg)
}

// HasPlaceholderVerifyCommands returns true if any verify commands are placeholders.
func HasPlaceholderVerifyCommands(cfg *RalphConfig) bool {
	for _, cmd := range cfg.Verify.Default {
		if isPlaceholderCommand(cmd) {
			return true
		}
	}
	return false
}

// extractBaseCommand returns the first word of a shell command string.
// e.g. "bun run test" → "bun", "./scripts/test.sh arg" → "./scripts/test.sh"
func extractBaseCommand(cmdStr string) string {
	fields := strings.Fields(cmdStr)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// isPlaceholderCommand returns true if the command is an echo placeholder.
func isPlaceholderCommand(cmd string) bool {
	return strings.HasPrefix(cmd, "echo '") || strings.HasPrefix(cmd, "echo \"")
}

// CheckBtcaAvailable returns true if btca is in PATH.
func CheckBtcaAvailable() bool {
	return isCommandAvailable("btca")
}

// CheckReadinessWarnings returns non-blocking warnings about the environment.
func CheckReadinessWarnings() []string {
	var warnings []string
	if !CheckBtcaAvailable() {
		warnings = append(warnings, "btca not found in PATH \u2014 agents cannot verify against latest docs. Install: https://github.com/nicobailon/btca-tool")
	}
	return warnings
}

// CheckReadiness validates that the project is ready for Ralph.
// Returns a list of issues. Empty list means ready.
func CheckReadiness(cfg *RalphConfig, prd *PRD) []string {
	var issues []string

	// verify.default must have real commands (not placeholders)
	if HasPlaceholderVerifyCommands(cfg) {
		issues = append(issues, "verify.default contains placeholder commands (echo '...'). Add real typecheck/lint/test commands.")
	}

	// Check verify.default command binaries are available (skip placeholders)
	for _, cmd := range cfg.Verify.Default {
		if isPlaceholderCommand(cmd) {
			continue
		}
		base := extractBaseCommand(cmd)
		if base != "" && !isCommandAvailable(base) {
			issues = append(issues, fmt.Sprintf("verify.default: '%s' not found in PATH (from: %s)", base, cmd))
		}
	}

	// Check verify.ui command binaries are available
	for _, cmd := range cfg.Verify.UI {
		base := extractBaseCommand(cmd)
		if base != "" && !isCommandAvailable(base) {
			issues = append(issues, fmt.Sprintf("verify.ui: '%s' not found in PATH (from: %s)", base, cmd))
		}
	}

	// Check service start command binaries are available
	for _, svc := range cfg.Services {
		if svc.Start != "" {
			base := extractBaseCommand(svc.Start)
			if base != "" && !isCommandAvailable(base) {
				issues = append(issues, fmt.Sprintf("service '%s': '%s' not found in PATH (from: %s)", svc.Name, base, svc.Start))
			}
		}
	}

	// UI stories require verify.ui commands
	if prd != nil {
		hasUIStories := false
		for _, s := range prd.UserStories {
			if IsUIStory(&s) {
				hasUIStories = true
				break
			}
		}
		if hasUIStories && len(cfg.Verify.UI) == 0 {
			issues = append(issues, "PRD has UI stories but verify.ui has no commands. Add e2e test commands (e.g., 'bun run test:e2e').")
		}
	}

	return issues
}
