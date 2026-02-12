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

	err := WriteDefaultConfig(dir, "claude", nil, nil)
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

			err := WriteDefaultConfig(dir, tt.command, nil, nil)
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
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Tags: []string{"backend"}},
		},
	}

	issues := CheckReadiness(cfg, prd)
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
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Tags: []string{"ui"}},
		},
	}

	issues := CheckReadiness(cfg, prd)
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
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Tags: []string{"ui"}},
		},
	}

	issues := CheckReadiness(cfg, prd)
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
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Tags: []string{"backend"}},
		},
	}

	issues := CheckReadiness(cfg, prd)
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
	err := WriteDefaultConfig(dir, "claude", commands, nil)
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

	err := WriteDefaultConfig(dir, "claude", []string{}, nil)
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

	err := WriteDefaultConfig(dir, "claude", nil, nil)
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

func TestCheckReadiness_BrowserExecutablePath(t *testing.T) {
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
		Browser: &BrowserConfig{
			Enabled:        true,
			ExecutablePath: "/nonexistent/path/to/chromium",
		},
	}

	issues := CheckReadiness(cfg, nil)
	found := false
	for _, issue := range issues {
		if strings.Contains(issue, "browser.executablePath not found") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'browser.executablePath not found' issue, got %v", issues)
	}
}

func TestCheckReadiness_BrowserExecutablePathValid(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".git"), 0755)
	os.Mkdir(filepath.Join(dir, ".ralph"), 0755)

	// Create a fake browser binary
	fakeBin := filepath.Join(dir, "fake-chromium")
	os.WriteFile(fakeBin, []byte("#!/bin/sh"), 0755)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(origDir) })

	cfg := &RalphConfig{
		Verify: VerifyConfig{
			Default: []string{"go version"},
		},
		Browser: &BrowserConfig{
			Enabled:        true,
			ExecutablePath: fakeBin,
		},
	}

	issues := CheckReadiness(cfg, nil)
	for _, issue := range issues {
		if strings.Contains(issue, "browser.executablePath") {
			t.Errorf("unexpected browser issue for valid path: %s", issue)
		}
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

func TestCheckReadiness_BrowserStepsButBrowserDisabled(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".git"), 0755)
	os.Mkdir(filepath.Join(dir, ".ralph"), 0755)
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(origDir) })

	cfg := &RalphConfig{
		Verify: VerifyConfig{
			Default: []string{"go version"},
			UI:      []string{"go version"},
		},
		Browser: &BrowserConfig{Enabled: false},
	}
	prd := &PRD{
		UserStories: []UserStory{
			{
				ID:   "US-001",
				Tags: []string{"ui"},
				BrowserSteps: []BrowserStep{
					{Action: "navigate", URL: "/"},
				},
			},
		},
	}

	issues := CheckReadiness(cfg, prd)
	found := false
	for _, issue := range issues {
		if strings.Contains(issue, "browserSteps") && strings.Contains(issue, "browser is disabled") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected browserSteps+disabled browser issue, got %v", issues)
	}
}
