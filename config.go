package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ServiceConfig configures a managed service (e.g., dev server)
type ServiceConfig struct {
	Name                string `json:"name"`
	Start               string `json:"start,omitempty"`
	Ready               string `json:"ready"` // URL to check
	ReadyTimeout        int    `json:"readyTimeout,omitempty"`
	RestartBeforeVerify bool   `json:"restartBeforeVerify,omitempty"`
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

// ProjectConfig identifies the project.
type ProjectConfig struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Root string `json:"root,omitempty"`
}

// ScripProviderConfig configures the AI provider (always Claude Code in scrip).
type ScripProviderConfig struct {
	Command      string `json:"command"`
	Timeout      int    `json:"timeout"`      // hard timeout per spawn, default 1800s
	StallTimeout int    `json:"stallTimeout"`  // no-output timeout, default 300s
}

// ScripVerifyConfig configures verification commands.
type ScripVerifyConfig struct {
	Typecheck string `json:"typecheck,omitempty"`
	Lint      string `json:"lint,omitempty"`
	Test      string `json:"test"`
}

// VerifyCommands returns the ordered list of non-empty verify commands.
func (v *ScripVerifyConfig) VerifyCommands() []string {
	var cmds []string
	if v.Typecheck != "" {
		cmds = append(cmds, v.Typecheck)
	}
	if v.Lint != "" {
		cmds = append(cmds, v.Lint)
	}
	if v.Test != "" {
		cmds = append(cmds, v.Test)
	}
	return cmds
}

// ScripServiceConfig configures a managed service (e.g., dev server).
type ScripServiceConfig struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Ready   string `json:"ready"`
	Timeout int    `json:"timeout,omitempty"` // ready timeout, default 30s
}

// ScripConfig is the main configuration loaded from .scrip/config.json.
type ScripConfig struct {
	Schema   string               `json:"$schema,omitempty"`
	Project  ProjectConfig        `json:"project"`
	Provider ScripProviderConfig  `json:"provider"`
	Verify   ScripVerifyConfig    `json:"verify"`
	Services []ScripServiceConfig `json:"services,omitempty"`
}

// ScripResolvedConfig is the fully resolved configuration with project root.
type ScripResolvedConfig struct {
	ProjectRoot string
	Config      ScripConfig
}

// ScripConfigDir returns the .scrip directory path.
func ScripConfigDir(projectRoot string) string {
	return filepath.Join(projectRoot, ".scrip")
}

// ScripConfigPath returns the path to .scrip/config.json.
func ScripConfigPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".scrip", "config.json")
}

// LoadScripConfig loads and validates .scrip/config.json.
func LoadScripConfig(projectRoot string) (*ScripResolvedConfig, error) {
	configPath := ScripConfigPath(projectRoot)

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config not found: .scrip/config.json\n\nRun 'scrip prep' to set up your project")
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg ScripConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid .scrip/config.json: %w", err)
	}

	applyScripDefaults(&cfg)

	if err := validateScripConfig(&cfg); err != nil {
		return nil, err
	}

	return &ScripResolvedConfig{
		ProjectRoot: projectRoot,
		Config:      cfg,
	}, nil
}

// applyScripDefaults fills in zero-value fields with sensible defaults.
func applyScripDefaults(cfg *ScripConfig) {
	if cfg.Project.Root == "" {
		cfg.Project.Root = "."
	}
	if cfg.Provider.Command == "" {
		cfg.Provider.Command = "claude"
	}
	if cfg.Provider.Timeout <= 0 {
		cfg.Provider.Timeout = 1800
	}
	if cfg.Provider.StallTimeout <= 0 {
		cfg.Provider.StallTimeout = 300
	}
	for i := range cfg.Services {
		if cfg.Services[i].Timeout <= 0 {
			cfg.Services[i].Timeout = 30
		}
	}
}

// validateScripConfig validates required fields in the configuration.
func validateScripConfig(cfg *ScripConfig) error {
	if cfg.Project.Name == "" {
		return fmt.Errorf("project.name is required")
	}
	if cfg.Project.Type == "" {
		return fmt.Errorf("project.type is required")
	}
	if cfg.Verify.Test == "" {
		return fmt.Errorf("verify.test is required")
	}
	for i, svc := range cfg.Services {
		if svc.Name == "" {
			return fmt.Errorf("services[%d].name is required", i)
		}
		if svc.Ready == "" {
			return fmt.Errorf("services[%d].ready is required", i)
		}
		if !strings.HasPrefix(svc.Ready, "http://") && !strings.HasPrefix(svc.Ready, "https://") {
			return fmt.Errorf("services[%d].ready must be an HTTP URL (got: %s)", i, svc.Ready)
		}
	}
	return nil
}

// WriteDefaultScripConfig writes a .scrip/config.json with the given configuration.
// Creates the .scrip directory if needed.
func WriteDefaultScripConfig(projectRoot string, cfg *ScripConfig) error {
	if cfg.Schema == "" {
		cfg.Schema = "https://scrip.dev/config.schema.json"
	}
	applyScripDefaults(cfg)

	configPath := ScripConfigPath(projectRoot)
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("create .scrip directory: %w", err)
	}
	return AtomicWriteJSON(configPath, cfg)
}

// ScripProviderArgs returns the argument list for spawning claude.
// Autonomous mode (exec build, land fix) adds --dangerously-skip-permissions.
// Non-autonomous mode (consultation, verification, planning) omits it.
func ScripProviderArgs(autonomous bool) []string {
	args := []string{"--print", "--model", "opus", "--effort", "max"}
	if autonomous {
		args = append(args, "--dangerously-skip-permissions")
	}
	return args
}

