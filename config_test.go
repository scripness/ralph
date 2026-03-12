package main

import (
	"fmt"
	"os"
	"os/user"
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
		},
		"services": [{"name": "dev", "ready": "http://localhost:3000"}]
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
		},
		"services": [{"name": "dev", "ready": "http://localhost:3000"}]
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

	err := WriteDefaultConfig(dir, "", "claude", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be able to load it
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("failed to load written config: %v", err)
	}
	if cfg.Config.Provider.Command != "claude" {
		t.Errorf("expected provider.command='claude', got '%s'", cfg.Config.Provider.Command)
	}
}

func TestWriteDefaultConfig_PersistsProject(t *testing.T) {
	dir := t.TempDir()

	err := WriteDefaultConfig(dir, "my-app", "claude", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("failed to load written config: %v", err)
	}

	if cfg.Config.Project != "my-app" {
		t.Errorf("expected project='my-app', got %q", cfg.Config.Project)
	}
}

func TestLoadConfig_BackfillsProjectFromDirName(t *testing.T) {
	dir := t.TempDir()

	// Write config without project field
	err := WriteDefaultConfig(dir, "", "claude", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Should backfill from directory name
	expected := filepath.Base(dir)
	if cfg.Config.Project != expected {
		t.Errorf("expected project=%q (from dir name), got %q", expected, cfg.Config.Project)
	}
}

func TestApplyProviderDefaults_AllProviders(t *testing.T) {
	tests := []struct {
		name          string
		command       string
		wantMode      string
		wantFlag      string
		wantArgs      []string
		wantKnowledge string
	}{
		{"amp", "amp", "stdin", "", []string{"--dangerously-allow-all"}, "AGENTS.md"},
		{"claude", "claude", "stdin", "", []string{"--print", "--dangerously-skip-permissions"}, "CLAUDE.md"},
		{"opencode", "opencode", "arg", "", []string{"run"}, "AGENTS.md"},
		{"aider", "aider", "arg", "--message", []string{"--yes-always"}, "AGENTS.md"},
		{"codex", "codex", "arg", "", []string{"exec", "--full-auto"}, "AGENTS.md"},
		{"unknown", "my-custom-ai", "stdin", "", nil, "AGENTS.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ProviderConfig{Command: tt.command}
			applyProviderDefaults(p)

			if p.PromptMode != tt.wantMode {
				t.Errorf("promptMode: got %q, want %q", p.PromptMode, tt.wantMode)
			}
			if p.PromptFlag != tt.wantFlag {
				t.Errorf("promptFlag: got %q, want %q", p.PromptFlag, tt.wantFlag)
			}
			if p.KnowledgeFile != tt.wantKnowledge {
				t.Errorf("knowledgeFile: got %q, want %q", p.KnowledgeFile, tt.wantKnowledge)
			}
			if tt.wantArgs == nil {
				if p.Args != nil {
					t.Errorf("args: got %v, want nil", p.Args)
				}
			} else {
				if len(p.Args) != len(tt.wantArgs) {
					t.Fatalf("args length: got %d, want %d", len(p.Args), len(tt.wantArgs))
				}
				for i, a := range tt.wantArgs {
					if p.Args[i] != a {
						t.Errorf("args[%d]: got %q, want %q", i, p.Args[i], a)
					}
				}
			}
		})
	}
}

func TestApplyProviderDefaults_NilArgsGetsDefaults(t *testing.T) {
	p := &ProviderConfig{Command: "amp"}
	// Args is nil (JSON key absent)
	applyProviderDefaults(p)

	if len(p.Args) != 1 || p.Args[0] != "--dangerously-allow-all" {
		t.Errorf("expected default args for amp, got %v", p.Args)
	}
}

func TestApplyProviderDefaults_EmptyArgsPreserved(t *testing.T) {
	p := &ProviderConfig{Command: "amp", Args: []string{}}
	// Args is explicitly empty — user intent to have no args
	applyProviderDefaults(p)

	if len(p.Args) != 0 {
		t.Errorf("expected empty args preserved, got %v", p.Args)
	}
}

func TestApplyProviderDefaults_CustomArgsPreserved(t *testing.T) {
	p := &ProviderConfig{Command: "amp", Args: []string{"--custom"}}
	applyProviderDefaults(p)

	if len(p.Args) != 1 || p.Args[0] != "--custom" {
		t.Errorf("expected custom args preserved, got %v", p.Args)
	}
}

func TestApplyProviderDefaults_UserOverride(t *testing.T) {
	p := &ProviderConfig{
		Command:       "amp",
		PromptMode:    "file",
		PromptFlag:    "--prompt",
		KnowledgeFile: "CUSTOM.md",
		Args:          []string{"--my-flag"},
	}
	applyProviderDefaults(p)

	if p.PromptMode != "file" {
		t.Errorf("expected user-set promptMode='file', got '%s'", p.PromptMode)
	}
	if p.PromptFlag != "--prompt" {
		t.Errorf("expected user-set promptFlag='--prompt', got '%s'", p.PromptFlag)
	}
	if p.KnowledgeFile != "CUSTOM.md" {
		t.Errorf("expected user-set knowledgeFile='CUSTOM.md', got '%s'", p.KnowledgeFile)
	}
	if len(p.Args) != 1 || p.Args[0] != "--my-flag" {
		t.Errorf("expected user-set args preserved, got %v", p.Args)
	}
}

func TestApplyProviderDefaults_InvalidPromptMode(t *testing.T) {
	p := &ProviderConfig{
		Command:    "amp",
		PromptMode: "invalid",
	}
	applyProviderDefaults(p)

	if p.PromptMode != "stdin" {
		t.Errorf("expected fallback promptMode='stdin', got '%s'", p.PromptMode)
	}
}

func TestLoadConfig_ProviderDefaults_AllProviders(t *testing.T) {
	tests := []struct {
		name          string
		command       string
		wantMode      string
		wantFlag      string
		wantArgs      []string
		wantKnowledge string
	}{
		{"amp", "amp", "stdin", "", []string{"--dangerously-allow-all"}, "AGENTS.md"},
		{"claude", "claude", "stdin", "", []string{"--print", "--dangerously-skip-permissions"}, "CLAUDE.md"},
		{"opencode", "opencode", "arg", "", []string{"run"}, "AGENTS.md"},
		{"aider", "aider", "arg", "--message", []string{"--yes-always"}, "AGENTS.md"},
		{"codex", "codex", "arg", "", []string{"exec", "--full-auto"}, "AGENTS.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			configContent := fmt.Sprintf(`{
				"provider": {"command": %q},
				"verify": {"default": ["echo test"]},
				"services": [{"name": "dev", "ready": "http://localhost:3000"}]
			}`, tt.command)
			os.WriteFile(filepath.Join(dir, "ralph.config.json"), []byte(configContent), 0644)

			cfg, err := LoadConfig(dir)
			if err != nil {
				t.Fatalf("LoadConfig error: %v", err)
			}

			p := cfg.Config.Provider
			if p.PromptMode != tt.wantMode {
				t.Errorf("promptMode: got %q, want %q", p.PromptMode, tt.wantMode)
			}
			if p.PromptFlag != tt.wantFlag {
				t.Errorf("promptFlag: got %q, want %q", p.PromptFlag, tt.wantFlag)
			}
			if p.KnowledgeFile != tt.wantKnowledge {
				t.Errorf("knowledgeFile: got %q, want %q", p.KnowledgeFile, tt.wantKnowledge)
			}
			if len(p.Args) != len(tt.wantArgs) {
				t.Fatalf("args length: got %d (%v), want %d (%v)", len(p.Args), p.Args, len(tt.wantArgs), tt.wantArgs)
			}
			for i, a := range tt.wantArgs {
				if p.Args[i] != a {
					t.Errorf("args[%d]: got %q, want %q", i, p.Args[i], a)
				}
			}
		})
	}
}

func TestLoadConfig_ProviderExplicitEmptyArgs(t *testing.T) {
	dir := t.TempDir()

	configContent := `{
		"provider": {
			"command": "amp",
			"args": []
		},
		"verify": {
			"default": ["bun run test"]
		},
		"services": [{"name": "dev", "ready": "http://localhost:3000"}]
	}`
	os.WriteFile(filepath.Join(dir, "ralph.config.json"), []byte(configContent), 0644)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Explicit empty args should NOT get defaults
	if len(cfg.Config.Provider.Args) != 0 {
		t.Errorf("expected empty args preserved, got %v", cfg.Config.Provider.Args)
	}
}

func TestWriteDefaultConfig_AutoDetection(t *testing.T) {
	tests := []struct {
		name          string
		command       string
		wantMode      string
		wantKnowledge string
		wantArgs      []string
	}{
		{"amp", "amp", "stdin", "AGENTS.md", []string{"--dangerously-allow-all"}},
		{"claude", "claude", "stdin", "CLAUDE.md", []string{"--print", "--dangerously-skip-permissions"}},
		{"custom", "my-ai", "stdin", "AGENTS.md", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			err := WriteDefaultConfig(dir, "", tt.command, nil, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			cfg, err := LoadConfig(dir)
			if err != nil {
				t.Fatalf("failed to load written config: %v", err)
			}

			if cfg.Config.Provider.Command != tt.command {
				t.Errorf("expected command=%q, got %q", tt.command, cfg.Config.Provider.Command)
			}
			if cfg.Config.Provider.PromptMode != tt.wantMode {
				t.Errorf("expected auto-detected promptMode=%q, got %q", tt.wantMode, cfg.Config.Provider.PromptMode)
			}
			if cfg.Config.Provider.KnowledgeFile != tt.wantKnowledge {
				t.Errorf("expected auto-detected knowledgeFile=%q, got %q", tt.wantKnowledge, cfg.Config.Provider.KnowledgeFile)
			}
			if tt.wantArgs == nil {
				if cfg.Config.Provider.Args != nil {
					t.Errorf("expected nil args, got %v", cfg.Config.Provider.Args)
				}
			} else {
				if len(cfg.Config.Provider.Args) != len(tt.wantArgs) {
					t.Fatalf("args length: got %d (%v), want %d (%v)", len(cfg.Config.Provider.Args), cfg.Config.Provider.Args, len(tt.wantArgs), tt.wantArgs)
				}
				for i, a := range tt.wantArgs {
					if cfg.Config.Provider.Args[i] != a {
						t.Errorf("args[%d]: got %q, want %q", i, cfg.Config.Provider.Args[i], a)
					}
				}
			}
		})
	}
}

func TestHasPlaceholderVerifyCommands(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		want     bool
	}{
		{"echo single quote", []string{"echo 'Add your test command'"}, true},
		{"echo double quote", []string{`echo "Add your lint command"`}, true},
		{"real commands", []string{"bun run typecheck", "bun run lint", "bun run test:unit"}, false},
		{"mixed real and placeholder", []string{"bun run test", "echo 'Add your lint command'"}, true},
		{"echo without quote", []string{"echo test"}, false},
		{"empty", []string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &RalphConfig{Verify: VerifyConfig{Default: tt.commands}}
			got := HasPlaceholderVerifyCommands(cfg)
			if got != tt.want {
				t.Errorf("HasPlaceholderVerifyCommands() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckReadiness_PlaceholderCommands(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".git"), 0755)
	os.Mkdir(filepath.Join(dir, ".ralph"), 0755)
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(origDir) })

	cfg := &RalphConfig{
		Verify: VerifyConfig{
			Default: []string{"echo 'Add your test command'"},
		},
	}
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-001", Tags: []string{"backend"}},
		},
	}

	issues := CheckReadiness(cfg, def)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d: %v", len(issues), issues)
	}
	if issues[0] != "verify.default contains placeholder commands (echo '...'). Add real typecheck/lint/test commands." {
		t.Errorf("unexpected issue: %s", issues[0])
	}
}

func TestCheckReadiness_UIStoriesNoVerifyUI(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".git"), 0755)
	os.Mkdir(filepath.Join(dir, ".ralph"), 0755)
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(origDir) })

	cfg := &RalphConfig{
		Verify: VerifyConfig{
			Default: []string{"go version"},
		},
	}
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-001", Tags: []string{"ui"}},
		},
	}

	issues := CheckReadiness(cfg, def)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d: %v", len(issues), issues)
	}
	if issues[0] != "PRD has UI stories but verify.ui has no commands. Add e2e test commands (e.g., 'bun run test:e2e')." {
		t.Errorf("unexpected issue: %s", issues[0])
	}
}

func TestCheckReadiness_AllGood(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".git"), 0755)
	os.Mkdir(filepath.Join(dir, ".ralph"), 0755)
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(origDir) })

	cfg := &RalphConfig{
		Verify: VerifyConfig{
			Default: []string{"go vet ./..."},
			UI:      []string{"go test ./..."},
		},
	}
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-001", Tags: []string{"ui"}},
		},
	}

	issues := CheckReadiness(cfg, def)
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %v", issues)
	}
}

func TestCheckReadiness_NoUIStories(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".git"), 0755)
	os.Mkdir(filepath.Join(dir, ".ralph"), 0755)
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(origDir) })

	cfg := &RalphConfig{
		Verify: VerifyConfig{
			Default: []string{"go version"},
		},
	}
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-001", Tags: []string{"backend"}},
		},
	}

	issues := CheckReadiness(cfg, def)
	if len(issues) != 0 {
		t.Errorf("expected no issues for non-UI stories without verify.ui, got %v", issues)
	}
}

func TestCheckReadiness_VerifyCommandNotInPATH(t *testing.T) {
	cfg := &RalphConfig{
		Verify: VerifyConfig{
			Default: []string{"nonexistent-tool-xyz123 run test"},
		},
	}

	issues := CheckReadiness(cfg, nil)
	found := false
	for _, issue := range issues {
		if strings.Contains(issue, "nonexistent-tool-xyz123") && strings.Contains(issue, "not found in PATH") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected issue about unavailable command, got %v", issues)
	}
}

func TestCheckReadiness_VerifyUICommandNotInPATH(t *testing.T) {
	cfg := &RalphConfig{
		Verify: VerifyConfig{
			Default: []string{"go version"},
			UI:      []string{"nonexistent-e2e-xyz123 run test:e2e"},
		},
	}

	issues := CheckReadiness(cfg, nil)
	found := false
	for _, issue := range issues {
		if strings.Contains(issue, "nonexistent-e2e-xyz123") && strings.Contains(issue, "verify.ui") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected issue about unavailable verify.ui command, got %v", issues)
	}
}

func TestCheckReadiness_ServiceCommandNotInPATH(t *testing.T) {
	cfg := &RalphConfig{
		Verify: VerifyConfig{
			Default: []string{"go version"},
		},
		Services: []ServiceConfig{
			{Name: "dev", Start: "nonexistent-server-xyz123 run dev", Ready: "http://localhost:3000"},
		},
	}

	issues := CheckReadiness(cfg, nil)
	found := false
	for _, issue := range issues {
		if strings.Contains(issue, "nonexistent-server-xyz123") && strings.Contains(issue, "service 'dev'") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected issue about unavailable service command, got %v", issues)
	}
}

func TestConfigPath(t *testing.T) {
	got := ConfigPath("/my/project")
	want := "/my/project/ralph.config.json"
	if got != want {
		t.Errorf("ConfigPath() = %q, want %q", got, want)
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

func TestCheckReadinessWarnings_KnownProvider(t *testing.T) {
	cfg := &RalphConfig{Provider: ProviderConfig{Command: "claude"}}
	warnings := CheckReadinessWarnings(cfg)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for known provider, got %d: %v", len(warnings), warnings)
	}
}

func TestCheckReadinessWarnings_UnknownProvider(t *testing.T) {
	cfg := &RalphConfig{Provider: ProviderConfig{Command: "my-custom-ai"}}
	warnings := CheckReadinessWarnings(cfg)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning for unknown provider, got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "my-custom-ai") || !strings.Contains(warnings[0], "not a known provider") {
		t.Errorf("unexpected warning: %s", warnings[0])
	}
}

func TestVerifyTimeoutDefault(t *testing.T) {
	dir := t.TempDir()
	configContent := `{
		"provider": {"command": "amp"},
		"verify": {"default": ["echo ok"]},
		"services": [{"name": "dev", "ready": "http://localhost:3000"}]
	}`
	os.WriteFile(filepath.Join(dir, "ralph.config.json"), []byte(configContent), 0644)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Config.Verify.Timeout != 300 {
		t.Errorf("expected default verify.timeout=300, got %d", cfg.Config.Verify.Timeout)
	}
}

func TestVerifyTimeoutExplicit(t *testing.T) {
	dir := t.TempDir()
	configContent := `{
		"provider": {"command": "amp"},
		"verify": {"default": ["echo ok"], "timeout": 600},
		"services": [{"name": "dev", "ready": "http://localhost:3000"}]
	}`
	os.WriteFile(filepath.Join(dir, "ralph.config.json"), []byte(configContent), 0644)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Config.Verify.Timeout != 600 {
		t.Errorf("expected verify.timeout=600, got %d", cfg.Config.Verify.Timeout)
	}
}

func TestVerifyTimeoutZero(t *testing.T) {
	dir := t.TempDir()
	configContent := `{
		"provider": {"command": "amp"},
		"verify": {"default": ["echo ok"], "timeout": 0},
		"services": [{"name": "dev", "ready": "http://localhost:3000"}]
	}`
	os.WriteFile(filepath.Join(dir, "ralph.config.json"), []byte(configContent), 0644)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Config.Verify.Timeout != 300 {
		t.Errorf("expected default verify.timeout=300 for zero value, got %d", cfg.Config.Verify.Timeout)
	}
}

func TestExtractBaseCommand(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"bun run test", "bun"},
		{"go vet ./...", "go"},
		{"./scripts/test.sh arg1", "./scripts/test.sh"},
		{"", ""},
		{"  go  version  ", "go"},
	}

	for _, tt := range tests {
		got := extractBaseCommand(tt.input)
		if got != tt.want {
			t.Errorf("extractBaseCommand(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCheckReadiness_ShAvailable(t *testing.T) {
	cfg := &RalphConfig{
		Verify: VerifyConfig{
			Default: []string{"go version"},
		},
	}

	issues := CheckReadiness(cfg, nil)
	for _, issue := range issues {
		if strings.Contains(issue, "'sh' not found") {
			t.Error("sh should be available in test environments")
		}
	}
}

func TestCheckReadiness_GitRepoRequired(t *testing.T) {
	dir := t.TempDir()
	// No .git directory — should fail

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(origDir) })

	cfg := &RalphConfig{
		Verify: VerifyConfig{
			Default: []string{"go version"},
		},
	}

	issues := CheckReadiness(cfg, nil)
	found := false
	for _, issue := range issues {
		if strings.Contains(issue, "Not inside a git repository") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'not inside a git repository' issue, got %v", issues)
	}
}

func TestCheckReadiness_RalphDirWritability(t *testing.T) {
	// Skip if running as root — chmod 0555 has no effect for root
	if u, err := user.Current(); err == nil && u.Uid == "0" {
		t.Skip("skipping writability test as root")
	}

	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".git"), 0755)
	ralphDir := filepath.Join(dir, ".ralph")
	os.Mkdir(ralphDir, 0555) // read-only
	t.Cleanup(func() { os.Chmod(ralphDir, 0755) })

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(origDir) })

	cfg := &RalphConfig{
		Verify: VerifyConfig{
			Default: []string{"go version"},
		},
	}

	issues := CheckReadiness(cfg, nil)
	found := false
	for _, issue := range issues {
		if strings.Contains(issue, ".ralph/ directory is not writable") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected '.ralph/ directory is not writable' issue, got %v", issues)
	}
}

func TestWriteDefaultConfig_WithVerifyCommands(t *testing.T) {
	dir := t.TempDir()

	commands := []string{"go vet ./...", "golangci-lint run", "go test ./..."}
	err := WriteDefaultConfig(dir, "", "claude", commands, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("failed to load written config: %v", err)
	}

	if len(cfg.Config.Verify.Default) != 3 {
		t.Fatalf("expected 3 verify commands, got %d", len(cfg.Config.Verify.Default))
	}
	for i, cmd := range commands {
		if cfg.Config.Verify.Default[i] != cmd {
			t.Errorf("verify.default[%d]: got %q, want %q", i, cfg.Config.Verify.Default[i], cmd)
		}
	}
	// Should not have placeholder commands
	if HasPlaceholderVerifyCommands(&cfg.Config) {
		t.Error("expected no placeholder commands when real commands provided")
	}
}

func TestWriteDefaultConfig_EmptyVerifyCommands(t *testing.T) {
	dir := t.TempDir()

	err := WriteDefaultConfig(dir, "", "claude", []string{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("failed to load written config: %v", err)
	}

	// Empty slice should fall back to placeholders
	if !HasPlaceholderVerifyCommands(&cfg.Config) {
		t.Error("expected placeholder commands when empty slice provided")
	}
}

func TestWriteDefaultConfig_NoCommitsMessage(t *testing.T) {
	dir := t.TempDir()

	err := WriteDefaultConfig(dir, "", "claude", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "ralph.config.json"))
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	if strings.Contains(string(data), "message") {
		t.Error("expected no 'message' field in written config (commits.message was removed)")
	}
}

func TestValidateConfig_ServiceReadyURL(t *testing.T) {
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
			cfg := &RalphConfig{
				Provider: ProviderConfig{Command: "claude"},
				Verify:   VerifyConfig{Default: []string{"go test ./..."}},
				Services: []ServiceConfig{
					{Name: "dev", Ready: tt.ready},
				},
			}
			err := validateConfig(cfg)
			if tt.wantErr && err == nil {
				t.Error("expected error for invalid service ready URL")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.wantErr && err != nil && !strings.Contains(err.Error(), "must be an HTTP URL") {
				t.Errorf("expected 'must be an HTTP URL' error, got: %v", err)
			}
		})
	}
}

func TestCheckReadiness_RalphDirMissing(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".git"), 0755)
	// Deliberately NOT creating .ralph directory

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(origDir) })

	cfg := &RalphConfig{
		Verify: VerifyConfig{
			Default: []string{"go version"},
		},
	}

	issues := CheckReadiness(cfg, nil)
	found := false
	for _, issue := range issues {
		if strings.Contains(issue, ".ralph/ directory not found") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected '.ralph/ directory not found' issue, got %v", issues)
	}
}

func TestConfig_SchemaFieldIgnored(t *testing.T) {
	dir := t.TempDir()
	configJSON := `{
		"$schema": "https://raw.githubusercontent.com/scripness/ralph/main/ralph.schema.json",
		"provider": {"command": "claude"},
		"verify": {"default": ["go test ./..."]},
		"services": [{"name": "dev", "start": "npm run dev", "ready": "http://localhost:3000"}]
	}`
	os.WriteFile(filepath.Join(dir, "ralph.config.json"), []byte(configJSON), 0644)

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("failed to load config with $schema: %v", err)
	}
	if cfg.Config.Schema != "https://raw.githubusercontent.com/scripness/ralph/main/ralph.schema.json" {
		t.Errorf("expected $schema to be preserved, got %q", cfg.Config.Schema)
	}
}

func TestValidateConfig_RequiresServices(t *testing.T) {
	cfg := &RalphConfig{
		Provider: ProviderConfig{Command: "claude"},
		Verify:   VerifyConfig{Default: []string{"go test ./..."}},
		Services: []ServiceConfig{},
	}
	err := validateConfig(cfg)
	if err == nil {
		t.Error("expected error for empty services")
	}
	if err != nil && !strings.Contains(err.Error(), "services must have at least one entry") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateConfig_RequiresServicesNil(t *testing.T) {
	cfg := &RalphConfig{
		Provider: ProviderConfig{Command: "claude"},
		Verify:   VerifyConfig{Default: []string{"go test ./..."}},
		Services: nil,
	}
	err := validateConfig(cfg)
	if err == nil {
		t.Error("expected error for nil services")
	}
}

func TestCheckReadiness_PlaceholderServiceCommand(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".git"), 0755)
	os.Mkdir(filepath.Join(dir, ".ralph"), 0755)
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(origDir) })

	cfg := &RalphConfig{
		Verify: VerifyConfig{
			Default: []string{"go version"},
		},
		Services: []ServiceConfig{
			{Name: "dev", Start: "echo 'Replace with your dev server command'", Ready: "http://localhost:3000"},
		},
	}

	issues := CheckReadiness(cfg, nil)
	found := false
	for _, issue := range issues {
		if strings.Contains(issue, "placeholder") && strings.Contains(issue, "service 'dev'") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected placeholder service command issue, got %v", issues)
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
		// No services — should be valid (unlike ralph which requires >=1)
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

