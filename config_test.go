package main

import (
	"os"
	"path/filepath"
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

func TestLoadConfig_Valid(t *testing.T) {
	dir := t.TempDir()

	configContent := `{
		"maxRetries": 5,
		"provider": {
			"command": "amp",
			"args": ["--test"],
			"timeout": 600
		},
		"verify": {
			"default": ["bun run test"],
			"ui": ["bun run test:e2e"]
		}
	}`
	os.WriteFile(filepath.Join(dir, "ralph.config.json"), []byte(configContent), 0644)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("expected config, got error: %v", err)
	}
	if cfg.Config.MaxRetries != 5 {
		t.Errorf("expected maxRetries=5, got %d", cfg.Config.MaxRetries)
	}
	if cfg.Config.Provider.Command != "amp" {
		t.Errorf("expected provider.command='amp', got '%s'", cfg.Config.Provider.Command)
	}
	if cfg.Config.Provider.Timeout != 600 {
		t.Errorf("expected provider.timeout=600, got %d", cfg.Config.Provider.Timeout)
	}
	if len(cfg.Config.Verify.Default) != 1 {
		t.Errorf("expected 1 default verify command, got %d", len(cfg.Config.Verify.Default))
	}
}

func TestLoadConfig_NotFound(t *testing.T) {
	dir := t.TempDir()

	_, err := LoadConfig(dir)
	if err == nil {
		t.Error("expected error for missing config")
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "ralph.config.json"), []byte("not json"), 0644)

	_, err := LoadConfig(dir)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadConfig_MissingProvider(t *testing.T) {
	dir := t.TempDir()

	configContent := `{
		"verify": {
			"default": ["bun run test"]
		}
	}`
	os.WriteFile(filepath.Join(dir, "ralph.config.json"), []byte(configContent), 0644)

	_, err := LoadConfig(dir)
	if err == nil {
		t.Error("expected error for missing provider.command")
	}
}

func TestLoadConfig_MissingVerifyDefault(t *testing.T) {
	dir := t.TempDir()

	configContent := `{
		"provider": {
			"command": "amp"
		},
		"verify": {}
	}`
	os.WriteFile(filepath.Join(dir, "ralph.config.json"), []byte(configContent), 0644)

	_, err := LoadConfig(dir)
	if err == nil {
		t.Error("expected error for missing verify.default")
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	dir := t.TempDir()

	// Minimal valid config
	configContent := `{
		"provider": {
			"command": "amp"
		},
		"verify": {
			"default": ["bun run test"]
		}
	}`
	os.WriteFile(filepath.Join(dir, "ralph.config.json"), []byte(configContent), 0644)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check defaults were applied
	if cfg.Config.MaxRetries != 3 {
		t.Errorf("expected default maxRetries=3, got %d", cfg.Config.MaxRetries)
	}
	if cfg.Config.Provider.Timeout != 1800 {
		t.Errorf("expected default timeout=1800, got %d", cfg.Config.Provider.Timeout)
	}
	if cfg.Config.Browser == nil || !cfg.Config.Browser.Enabled {
		t.Error("expected browser.enabled=true by default")
	}
	if cfg.Config.Commits == nil || !cfg.Config.Commits.PrdChanges {
		t.Error("expected commits.prdChanges=true by default")
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

func TestWriteDefaultConfig(t *testing.T) {
	dir := t.TempDir()

	err := WriteDefaultConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be able to load it
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("failed to load written config: %v", err)
	}
	if cfg.Config.Provider.Command != "amp" {
		t.Errorf("expected provider.command='amp', got '%s'", cfg.Config.Provider.Command)
	}
}
