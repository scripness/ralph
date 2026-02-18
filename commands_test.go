package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptProviderSelection_KnownProvider(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"aider", "1\n", "aider"},
		{"amp", "2\n", "amp"},
		{"claude", "3\n", "claude"},
		{"codex", "4\n", "codex"},
		{"opencode", "5\n", "opencode"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.input))
			got := promptProviderSelection(reader)
			if got != tt.want {
				t.Errorf("promptProviderSelection() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPromptProviderSelection_CustomProvider(t *testing.T) {
	// Select "other" (6), then enter custom command
	input := "6\nmy-custom-ai\n"
	reader := bufio.NewReader(strings.NewReader(input))
	got := promptProviderSelection(reader)
	if got != "my-custom-ai" {
		t.Errorf("promptProviderSelection() = %q, want %q", got, "my-custom-ai")
	}
}

func TestPromptProviderSelection_InvalidThenValid(t *testing.T) {
	// Invalid input first, then valid
	input := "0\n99\nabc\n3\n"
	reader := bufio.NewReader(strings.NewReader(input))
	got := promptProviderSelection(reader)
	if got != "claude" {
		t.Errorf("promptProviderSelection() = %q, want %q", got, "claude")
	}
}

func TestPromptVerifyCommands_AllProvided(t *testing.T) {
	input := "go vet ./...\ngolangci-lint run\ngo test ./...\n"
	reader := bufio.NewReader(strings.NewReader(input))
	got := promptVerifyCommands(reader, [3]string{})
	want := []string{"go vet ./...", "golangci-lint run", "go test ./..."}
	if len(got) != len(want) {
		t.Fatalf("expected %d commands, got %d: %v", len(want), len(got), got)
	}
	for i, cmd := range want {
		if got[i] != cmd {
			t.Errorf("command[%d]: got %q, want %q", i, got[i], cmd)
		}
	}
}

func TestPromptVerifyCommands_AllSkipped(t *testing.T) {
	input := "\n\n\n"
	reader := bufio.NewReader(strings.NewReader(input))
	got := promptVerifyCommands(reader, [3]string{})
	if len(got) != 0 {
		t.Errorf("expected empty slice when all skipped, got %v", got)
	}
}

func TestPromptVerifyCommands_PartialSkip(t *testing.T) {
	input := "bun run typecheck\n\nbun run test\n"
	reader := bufio.NewReader(strings.NewReader(input))
	got := promptVerifyCommands(reader, [3]string{})
	want := []string{"bun run typecheck", "bun run test"}
	if len(got) != len(want) {
		t.Fatalf("expected %d commands, got %d: %v", len(want), len(got), got)
	}
	for i, cmd := range want {
		if got[i] != cmd {
			t.Errorf("command[%d]: got %q, want %q", i, got[i], cmd)
		}
	}
}

func TestPromptVerifyCommands_WhitespaceOnly(t *testing.T) {
	input := "   \n\t\n  \t  \n"
	reader := bufio.NewReader(strings.NewReader(input))
	got := promptVerifyCommands(reader, [3]string{})
	if len(got) != 0 {
		t.Errorf("expected empty slice for whitespace-only input, got %v", got)
	}
}

func TestProviderChoices_Sorted(t *testing.T) {
	// Verify provider choices are in alphabetical order
	for i := 1; i < len(providerChoices); i++ {
		if providerChoices[i] < providerChoices[i-1] {
			t.Errorf("providerChoices not sorted: %q comes after %q", providerChoices[i], providerChoices[i-1])
		}
	}
}

func TestProviderChoices_MatchKnownProviders(t *testing.T) {
	// Every choice should be in the knownProviders map
	for _, choice := range providerChoices {
		if _, ok := knownProviders[choice]; !ok {
			t.Errorf("providerChoices contains %q which is not in knownProviders", choice)
		}
	}
	// Every known provider should be in the choices
	for name := range knownProviders {
		found := false
		for _, choice := range providerChoices {
			if choice == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("knownProviders has %q which is not in providerChoices", name)
		}
	}
}

func TestPromptVerifyCommands_AcceptsDetectedDefaults(t *testing.T) {
	// All Enter = accept detected defaults
	input := "\n\n\n"
	reader := bufio.NewReader(strings.NewReader(input))
	detected := [3]string{"go vet ./...", "", "go test ./..."}
	got := promptVerifyCommands(reader, detected)
	want := []string{"go vet ./...", "go test ./..."}
	if len(got) != len(want) {
		t.Fatalf("expected %d commands, got %d: %v", len(want), len(got), got)
	}
	for i, cmd := range want {
		if got[i] != cmd {
			t.Errorf("command[%d]: got %q, want %q", i, got[i], cmd)
		}
	}
}

func TestPromptVerifyCommands_OverridesDetectedDefaults(t *testing.T) {
	// Override first and third, accept second
	input := "bun run typecheck\n\nbun run test:unit\n"
	reader := bufio.NewReader(strings.NewReader(input))
	detected := [3]string{"npm run typecheck", "npm run lint", "npm run test"}
	got := promptVerifyCommands(reader, detected)
	want := []string{"bun run typecheck", "npm run lint", "bun run test:unit"}
	if len(got) != len(want) {
		t.Fatalf("expected %d commands, got %d: %v", len(want), len(got), got)
	}
	for i, cmd := range want {
		if got[i] != cmd {
			t.Errorf("command[%d]: got %q, want %q", i, got[i], cmd)
		}
	}
}

func TestPromptServiceConfig_WithInput(t *testing.T) {
	input := "npm run dev\nlocalhost:4000\n"
	reader := bufio.NewReader(strings.NewReader(input))
	svc := promptServiceConfig(reader)
	if svc == nil {
		t.Fatal("expected non-nil service config")
	}
	if svc.Start != "npm run dev" {
		t.Errorf("expected start='npm run dev', got %q", svc.Start)
	}
	if svc.Ready != "http://localhost:4000" {
		t.Errorf("expected ready='http://localhost:4000', got %q", svc.Ready)
	}
	if svc.Name != "dev" {
		t.Errorf("expected name='dev', got %q", svc.Name)
	}
	if !svc.RestartBeforeVerify {
		t.Error("expected RestartBeforeVerify=true")
	}
}

func TestPromptServiceConfig_DefaultURL(t *testing.T) {
	input := "bun run dev\n\n"
	reader := bufio.NewReader(strings.NewReader(input))
	svc := promptServiceConfig(reader)
	if svc == nil {
		t.Fatal("expected non-nil service config")
	}
	if svc.Ready != "http://localhost:3000" {
		t.Errorf("expected default ready URL, got %q", svc.Ready)
	}
}

func TestPromptServiceConfig_Skip(t *testing.T) {
	input := "\n"
	reader := bufio.NewReader(strings.NewReader(input))
	svc := promptServiceConfig(reader)
	if svc != nil {
		t.Errorf("expected nil when start command is empty, got %+v", svc)
	}
}

func TestPromptServiceConfig_URLWithScheme(t *testing.T) {
	input := "mix phx.server\nhttps://localhost:4001\n"
	reader := bufio.NewReader(strings.NewReader(input))
	svc := promptServiceConfig(reader)
	if svc == nil {
		t.Fatal("expected non-nil service config")
	}
	if svc.Ready != "https://localhost:4001" {
		t.Errorf("expected URL with scheme preserved, got %q", svc.Ready)
	}
}

func TestCmdInit_CreatesGitignore(t *testing.T) {
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	os.MkdirAll(ralphDir, 0755)

	// Replicate the gitignore creation logic from cmdInit
	gitignorePath := filepath.Join(ralphDir, ".gitignore")
	gitignoreContent := "# Ralph temporary files\nralph.lock\n*.tmp\n*/logs/\n"
	if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644); err != nil {
		t.Fatalf("failed to write .gitignore: %v", err)
	}

	// Verify the file exists and has correct content
	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	content := string(data)

	expectedPatterns := []string{"ralph.lock", "*.tmp", "*/logs/"}
	for _, pattern := range expectedPatterns {
		if !strings.Contains(content, pattern) {
			t.Errorf(".gitignore should contain %q, got:\n%s", pattern, content)
		}
	}
}
