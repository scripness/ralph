package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ProviderConfig configures the AI provider CLI
type ProviderConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Timeout int      `json:"timeout"` // seconds per iteration
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

	// Apply service defaults
	for i := range cfg.Services {
		if cfg.Services[i].ReadyTimeout <= 0 {
			cfg.Services[i].ReadyTimeout = 30
		}
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

// WriteDefaultConfig writes a default ralph.config.json
func WriteDefaultConfig(projectRoot string) error {
	cfg := RalphConfig{
		MaxRetries: 3,
		Provider: ProviderConfig{
			Command: "amp",
			Args:    []string{"--dangerously-allow-all"},
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
