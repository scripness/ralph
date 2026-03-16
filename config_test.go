package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindGitRoot(t *testing.T) {
	// Create a temp dir structure with .git
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	os.Mkdir(gitDir, 0755)

	subDir := filepath.Join(dir, "sub", "deep")
	os.MkdirAll(subDir, 0755)

	// Find git root from subdirectory
	root := findGitRoot(subDir)
	if root != dir {
		t.Errorf("expected '%s', got '%s'", dir, root)
	}
}

func TestFindGitRoot_NoGit(t *testing.T) {
	dir := t.TempDir()

	// Should return start dir when no .git found
	root := findGitRoot(dir)
	if root != dir {
		t.Errorf("expected '%s', got '%s'", dir, root)
	}
}

func TestIsCommandAvailable(t *testing.T) {
	// 'go' should exist on any system running these tests
	if !isCommandAvailable("go") {
		t.Error("expected 'go' to be available")
	}

	if isCommandAvailable("definitely-not-a-real-command-12345") {
		t.Error("expected fake command to not be available")
	}
}

func TestGetProjectRoot_WithGitDir(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".git"), 0755)

	root := findGitRoot(dir)
	if root != dir {
		t.Errorf("expected '%s', got '%s'", dir, root)
	}
}

// --- Scrip v1 config tests ---

func TestScripConfigPath(t *testing.T) {
	got := ScripConfigPath("/my/project")
	want := "/my/project/.scrip/config.json"
	if got != want {
		t.Errorf("ScripConfigPath() = %q, want %q", got, want)
	}
}

func TestScripConfigDir(t *testing.T) {
	got := ScripConfigDir("/my/project")
	want := "/my/project/.scrip"
	if got != want {
		t.Errorf("ScripConfigDir() = %q, want %q", got, want)
	}
}

func TestLoadScripConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	scripDir := filepath.Join(dir, ".scrip")
	os.MkdirAll(scripDir, 0755)

	configContent := `{
		"$schema": "https://scrip.dev/config.schema.json",
		"project": {"name": "my-app", "type": "go"},
		"provider": {"command": "claude", "timeout": 900, "stallTimeout": 120},
		"verify": {"typecheck": "go vet ./...", "lint": "golangci-lint run", "test": "go test ./..."},
		"services": [{"name": "api", "command": "go run ./cmd/server", "ready": "http://localhost:8080/health", "timeout": 15}]
	}`
	os.WriteFile(filepath.Join(scripDir, "config.json"), []byte(configContent), 0644)

	cfg, err := LoadScripConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Config.Project.Name != "my-app" {
		t.Errorf("expected project.name='my-app', got %q", cfg.Config.Project.Name)
	}
	if cfg.Config.Project.Type != "go" {
		t.Errorf("expected project.type='go', got %q", cfg.Config.Project.Type)
	}
	if cfg.Config.Provider.Timeout != 900 {
		t.Errorf("expected provider.timeout=900, got %d", cfg.Config.Provider.Timeout)
	}
	if cfg.Config.Provider.StallTimeout != 120 {
		t.Errorf("expected provider.stallTimeout=120, got %d", cfg.Config.Provider.StallTimeout)
	}
	if cfg.Config.Verify.Typecheck != "go vet ./..." {
		t.Errorf("expected verify.typecheck='go vet ./...', got %q", cfg.Config.Verify.Typecheck)
	}
	if cfg.Config.Verify.Test != "go test ./..." {
		t.Errorf("expected verify.test='go test ./...', got %q", cfg.Config.Verify.Test)
	}
	if len(cfg.Config.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(cfg.Config.Services))
	}
	if cfg.Config.Services[0].Command != "go run ./cmd/server" {
		t.Errorf("expected service command, got %q", cfg.Config.Services[0].Command)
	}
	if cfg.Config.Services[0].Timeout != 15 {
		t.Errorf("expected service timeout=15, got %d", cfg.Config.Services[0].Timeout)
	}
	if cfg.ProjectRoot != dir {
		t.Errorf("expected ProjectRoot=%q, got %q", dir, cfg.ProjectRoot)
	}
}

func TestLoadScripConfig_NotFound(t *testing.T) {
	dir := t.TempDir()

	_, err := LoadScripConfig(dir)
	if err == nil {
		t.Error("expected error for missing config")
	}
	if !strings.Contains(err.Error(), "scrip prep") {
		t.Errorf("expected hint to run 'scrip prep', got: %v", err)
	}
}

func TestLoadScripConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	scripDir := filepath.Join(dir, ".scrip")
	os.MkdirAll(scripDir, 0755)
	os.WriteFile(filepath.Join(scripDir, "config.json"), []byte("not json"), 0644)

	_, err := LoadScripConfig(dir)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadScripConfig_Defaults(t *testing.T) {
	dir := t.TempDir()
	scripDir := filepath.Join(dir, ".scrip")
	os.MkdirAll(scripDir, 0755)

	// Minimal valid config — defaults should be applied
	configContent := `{
		"project": {"name": "test", "type": "node"},
		"verify": {"test": "npm test"}
	}`
	os.WriteFile(filepath.Join(scripDir, "config.json"), []byte(configContent), 0644)

	cfg, err := LoadScripConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Config.Project.Root != "." {
		t.Errorf("expected default project.root='.', got %q", cfg.Config.Project.Root)
	}
	if cfg.Config.Provider.Command != "claude" {
		t.Errorf("expected default provider.command='claude', got %q", cfg.Config.Provider.Command)
	}
	if cfg.Config.Provider.Timeout != 1800 {
		t.Errorf("expected default provider.timeout=1800, got %d", cfg.Config.Provider.Timeout)
	}
	if cfg.Config.Provider.StallTimeout != 300 {
		t.Errorf("expected default provider.stallTimeout=300, got %d", cfg.Config.Provider.StallTimeout)
	}
}

func TestValidateScripConfig_MissingProjectName(t *testing.T) {
	cfg := &ScripConfig{
		Project: ProjectConfig{Type: "go"},
		Verify:  ScripVerifyConfig{Test: "go test ./..."},
	}
	err := validateScripConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "project.name") {
		t.Errorf("expected project.name error, got: %v", err)
	}
}

func TestValidateScripConfig_MissingProjectType(t *testing.T) {
	cfg := &ScripConfig{
		Project: ProjectConfig{Name: "test"},
		Verify:  ScripVerifyConfig{Test: "go test ./..."},
	}
	err := validateScripConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "project.type") {
		t.Errorf("expected project.type error, got: %v", err)
	}
}

func TestValidateScripConfig_MissingVerifyTest(t *testing.T) {
	cfg := &ScripConfig{
		Project: ProjectConfig{Name: "test", Type: "go"},
		Verify:  ScripVerifyConfig{Typecheck: "go vet ./..."},
	}
	err := validateScripConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "verify.test") {
		t.Errorf("expected verify.test error, got: %v", err)
	}
}

func TestValidateScripConfig_ServiceReadyURL(t *testing.T) {
	tests := []struct {
		name    string
		ready   string
		wantErr bool
	}{
		{"http URL", "http://localhost:3000", false},
		{"https URL", "https://localhost:3000", false},
		{"missing scheme", "localhost:3000", true},
		{"bare port", ":3000", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ScripConfig{
				Project: ProjectConfig{Name: "test", Type: "go"},
				Verify:  ScripVerifyConfig{Test: "go test ./..."},
				Services: []ScripServiceConfig{
					{Name: "dev", Command: "go run .", Ready: tt.ready},
				},
			}
			err := validateScripConfig(cfg)
			if tt.wantErr && err == nil {
				t.Error("expected error for invalid service ready URL")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateScripConfig_ServicesOptional(t *testing.T) {
	cfg := &ScripConfig{
		Project: ProjectConfig{Name: "test", Type: "go"},
		Verify:  ScripVerifyConfig{Test: "go test ./..."},
		// No services — should be valid (services are optional in scrip)
	}
	err := validateScripConfig(cfg)
	if err != nil {
		t.Errorf("services should be optional, got error: %v", err)
	}
}

func TestWriteDefaultScripConfig(t *testing.T) {
	dir := t.TempDir()

	cfg := &ScripConfig{
		Project: ProjectConfig{Name: "my-app", Type: "go"},
		Verify:  ScripVerifyConfig{Typecheck: "go vet ./...", Test: "go test ./..."},
	}

	if err := WriteDefaultScripConfig(dir, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should create .scrip directory
	if _, err := os.Stat(filepath.Join(dir, ".scrip")); err != nil {
		t.Error(".scrip directory not created")
	}

	// Should be loadable
	loaded, err := LoadScripConfig(dir)
	if err != nil {
		t.Fatalf("failed to load written config: %v", err)
	}
	if loaded.Config.Project.Name != "my-app" {
		t.Errorf("expected project.name='my-app', got %q", loaded.Config.Project.Name)
	}
	if loaded.Config.Schema != "https://scrip.dev/config.schema.json" {
		t.Errorf("expected schema URL, got %q", loaded.Config.Schema)
	}
}

func TestWriteDefaultScripConfig_AppliesDefaults(t *testing.T) {
	dir := t.TempDir()

	cfg := &ScripConfig{
		Project: ProjectConfig{Name: "test", Type: "node"},
		Verify:  ScripVerifyConfig{Test: "npm test"},
	}

	if err := WriteDefaultScripConfig(dir, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, err := LoadScripConfig(dir)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	if loaded.Config.Provider.Command != "claude" {
		t.Errorf("expected default command='claude', got %q", loaded.Config.Provider.Command)
	}
	if loaded.Config.Provider.Timeout != 1800 {
		t.Errorf("expected default timeout=1800, got %d", loaded.Config.Provider.Timeout)
	}
	if loaded.Config.Provider.StallTimeout != 300 {
		t.Errorf("expected default stallTimeout=300, got %d", loaded.Config.Provider.StallTimeout)
	}
}

func TestScripProviderArgs_Autonomous(t *testing.T) {
	args := ScripProviderArgs(true)

	// Must include all required flags
	expected := []string{"--print", "--model", "opus", "--effort", "max", "--dangerously-skip-permissions"}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, want := range expected {
		if args[i] != want {
			t.Errorf("args[%d] = %q, want %q", i, args[i], want)
		}
	}
}

func TestScripProviderArgs_NonAutonomous(t *testing.T) {
	args := ScripProviderArgs(false)

	// Should NOT include --dangerously-skip-permissions
	expected := []string{"--print", "--model", "opus", "--effort", "max"}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, want := range expected {
		if args[i] != want {
			t.Errorf("args[%d] = %q, want %q", i, args[i], want)
		}
	}

	// Verify --dangerously-skip-permissions is absent
	for _, a := range args {
		if a == "--dangerously-skip-permissions" {
			t.Error("non-autonomous mode should not include --dangerously-skip-permissions")
		}
	}
}

func TestScripVerifyCommands(t *testing.T) {
	t.Run("all commands present", func(t *testing.T) {
		v := &ScripVerifyConfig{
			Typecheck: "go vet ./...",
			Lint:      "golangci-lint run",
			Test:      "go test ./...",
		}
		cmds := v.VerifyCommands()
		if len(cmds) != 3 {
			t.Fatalf("expected 3 commands, got %d", len(cmds))
		}
		if cmds[0] != "go vet ./..." {
			t.Errorf("expected typecheck first, got %q", cmds[0])
		}
		if cmds[1] != "golangci-lint run" {
			t.Errorf("expected lint second, got %q", cmds[1])
		}
		if cmds[2] != "go test ./..." {
			t.Errorf("expected test third, got %q", cmds[2])
		}
	})

	t.Run("only test", func(t *testing.T) {
		v := &ScripVerifyConfig{Test: "npm test"}
		cmds := v.VerifyCommands()
		if len(cmds) != 1 {
			t.Fatalf("expected 1 command, got %d", len(cmds))
		}
		if cmds[0] != "npm test" {
			t.Errorf("expected 'npm test', got %q", cmds[0])
		}
	})

	t.Run("typecheck and test only", func(t *testing.T) {
		v := &ScripVerifyConfig{Typecheck: "tsc --noEmit", Test: "jest"}
		cmds := v.VerifyCommands()
		if len(cmds) != 2 {
			t.Fatalf("expected 2 commands, got %d", len(cmds))
		}
		if cmds[0] != "tsc --noEmit" {
			t.Errorf("expected typecheck first, got %q", cmds[0])
		}
		if cmds[1] != "jest" {
			t.Errorf("expected test second, got %q", cmds[1])
		}
	})

	t.Run("empty config", func(t *testing.T) {
		v := &ScripVerifyConfig{}
		cmds := v.VerifyCommands()
		if len(cmds) != 0 {
			t.Errorf("expected 0 commands for empty config, got %d", len(cmds))
		}
	})
}

func TestScripConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()

	original := &ScripConfig{
		Project:  ProjectConfig{Name: "roundtrip-app", Type: "elixir", Root: "apps/web"},
		Provider: ScripProviderConfig{Command: "claude", Timeout: 2400, StallTimeout: 600},
		Verify:   ScripVerifyConfig{Typecheck: "mix compile --warnings-as-errors", Lint: "mix credo", Test: "mix test"},
		Services: []ScripServiceConfig{
			{Name: "phoenix", Command: "mix phx.server", Ready: "http://localhost:4000/health", Timeout: 45},
		},
	}

	if err := WriteDefaultScripConfig(dir, original); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	loaded, err := LoadScripConfig(dir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	cfg := loaded.Config
	if cfg.Project.Name != "roundtrip-app" {
		t.Errorf("project.name: got %q, want 'roundtrip-app'", cfg.Project.Name)
	}
	if cfg.Project.Type != "elixir" {
		t.Errorf("project.type: got %q, want 'elixir'", cfg.Project.Type)
	}
	if cfg.Project.Root != "apps/web" {
		t.Errorf("project.root: got %q, want 'apps/web'", cfg.Project.Root)
	}
	if cfg.Provider.Timeout != 2400 {
		t.Errorf("provider.timeout: got %d, want 2400", cfg.Provider.Timeout)
	}
	if cfg.Provider.StallTimeout != 600 {
		t.Errorf("provider.stallTimeout: got %d, want 600", cfg.Provider.StallTimeout)
	}
	if cfg.Verify.Typecheck != "mix compile --warnings-as-errors" {
		t.Errorf("verify.typecheck: got %q", cfg.Verify.Typecheck)
	}
	if cfg.Verify.Lint != "mix credo" {
		t.Errorf("verify.lint: got %q", cfg.Verify.Lint)
	}
	if cfg.Verify.Test != "mix test" {
		t.Errorf("verify.test: got %q", cfg.Verify.Test)
	}
	if len(cfg.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(cfg.Services))
	}
	svc := cfg.Services[0]
	if svc.Name != "phoenix" || svc.Command != "mix phx.server" || svc.Ready != "http://localhost:4000/health" || svc.Timeout != 45 {
		t.Errorf("service mismatch: %+v", svc)
	}
}

func TestScripVerifyConfig_DeepVerifyDefault(t *testing.T) {
	dir := t.TempDir()
	scripDir := filepath.Join(dir, ".scrip")
	os.MkdirAll(scripDir, 0755)

	// Config without deepVerify field — should default to false
	configContent := `{
		"project": {"name": "test", "type": "go"},
		"verify": {"test": "go test ./..."}
	}`
	os.WriteFile(filepath.Join(scripDir, "config.json"), []byte(configContent), 0644)

	loaded, err := LoadScripConfig(dir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded.Config.Verify.DeepVerify {
		t.Error("DeepVerify should default to false when absent from config")
	}

	// Config with deepVerify: true
	configContent = `{
		"project": {"name": "test", "type": "go"},
		"verify": {"test": "go test ./...", "deepVerify": true}
	}`
	os.WriteFile(filepath.Join(scripDir, "config.json"), []byte(configContent), 0644)

	loaded, err = LoadScripConfig(dir)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if !loaded.Config.Verify.DeepVerify {
		t.Error("DeepVerify should be true when set in config")
	}
}

func TestLoadScripConfig_ServiceDefaults(t *testing.T) {
	dir := t.TempDir()
	scripDir := filepath.Join(dir, ".scrip")
	os.MkdirAll(scripDir, 0755)

	configContent := `{
		"project": {"name": "test", "type": "go"},
		"verify": {"test": "go test ./..."},
		"services": [{"name": "api", "command": "go run .", "ready": "http://localhost:8080"}]
	}`
	os.WriteFile(filepath.Join(scripDir, "config.json"), []byte(configContent), 0644)

	cfg, err := LoadScripConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Config.Services[0].Timeout != 30 {
		t.Errorf("expected default service timeout=30, got %d", cfg.Config.Services[0].Timeout)
	}
}

func TestValidateScripConfig_ServiceMissingName(t *testing.T) {
	cfg := &ScripConfig{
		Project:  ProjectConfig{Name: "test", Type: "go"},
		Verify:   ScripVerifyConfig{Test: "go test ./..."},
		Services: []ScripServiceConfig{{Ready: "http://localhost:3000"}},
	}
	err := validateScripConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "services[0].name") {
		t.Errorf("expected service name error, got: %v", err)
	}
}

func TestValidateScripConfig_ServiceMissingReady(t *testing.T) {
	cfg := &ScripConfig{
		Project:  ProjectConfig{Name: "test", Type: "go"},
		Verify:   ScripVerifyConfig{Test: "go test ./..."},
		Services: []ScripServiceConfig{{Name: "dev"}},
	}
	err := validateScripConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "services[0].ready") {
		t.Errorf("expected service ready error, got: %v", err)
	}
}
