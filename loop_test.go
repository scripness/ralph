package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessLine_DoneMarker(t *testing.T) {
	result := &ProviderResult{}
	processLine("Some output <ralph>DONE</ralph> more text", result, nil)

	if !result.Done {
		t.Error("expected Done=true")
	}
}

func TestProcessLine_VerifiedMarker(t *testing.T) {
	result := &ProviderResult{}
	processLine("Review complete <ralph>VERIFIED</ralph>", result, nil)

	if !result.Verified {
		t.Error("expected Verified=true")
	}
}

func TestProcessLine_LearningMarker(t *testing.T) {
	result := &ProviderResult{}
	processLine("<ralph>LEARNING:Always use escapeHtml for user data</ralph>", result, nil)

	if len(result.Learnings) != 1 {
		t.Fatalf("expected 1 learning, got %d", len(result.Learnings))
	}
	if result.Learnings[0] != "Always use escapeHtml for user data" {
		t.Errorf("unexpected learning: %s", result.Learnings[0])
	}
}

func TestProcessLine_MultipleLearnings(t *testing.T) {
	result := &ProviderResult{}
	processLine("<ralph>LEARNING:First learning</ralph>", result, nil)
	processLine("<ralph>LEARNING:Second learning</ralph>", result, nil)

	if len(result.Learnings) != 2 {
		t.Fatalf("expected 2 learnings, got %d", len(result.Learnings))
	}
}

func TestProcessLine_ResetMarker(t *testing.T) {
	result := &ProviderResult{}
	processLine("<ralph>RESET:US-001,US-003</ralph>", result, nil)

	if len(result.Resets) != 2 {
		t.Fatalf("expected 2 resets, got %d", len(result.Resets))
	}
	if result.Resets[0] != "US-001" {
		t.Errorf("expected first reset='US-001', got '%s'", result.Resets[0])
	}
	if result.Resets[1] != "US-003" {
		t.Errorf("expected second reset='US-003', got '%s'", result.Resets[1])
	}
}

func TestProcessLine_ReasonMarker(t *testing.T) {
	result := &ProviderResult{}
	processLine("<ralph>REASON:Missing test coverage for auth module</ralph>", result, nil)

	if result.Reason != "Missing test coverage for auth module" {
		t.Errorf("unexpected reason: %s", result.Reason)
	}
}

func TestProcessLine_NoMarkers(t *testing.T) {
	result := &ProviderResult{}
	processLine("Regular output without any markers", result, nil)

	if result.Done || result.Verified || len(result.Learnings) > 0 || len(result.Resets) > 0 {
		t.Error("expected no markers to be detected")
	}
}

func TestProcessLine_MarkerWithWhitespace(t *testing.T) {
	result := &ProviderResult{}
	processLine("<ralph>LEARNING:  Trimmed learning  </ralph>", result, nil)

	if len(result.Learnings) != 1 {
		t.Fatalf("expected 1 learning, got %d", len(result.Learnings))
	}
	if result.Learnings[0] != "Trimmed learning" {
		t.Errorf("expected trimmed learning, got '%s'", result.Learnings[0])
	}
}

func TestLearningPattern(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<ralph>LEARNING:test</ralph>", "test"},
		{"<ralph>LEARNING:multi word learning</ralph>", "multi word learning"},
		{"<ralph>LEARNING: spaces around </ralph>", "spaces around"},
	}

	for _, tt := range tests {
		matches := LearningPattern.FindStringSubmatch(tt.input)
		if len(matches) < 2 {
			t.Errorf("expected match for %s", tt.input)
			continue
		}
		// Note: actual trimming happens in processLine
		if matches[1] != tt.expected && matches[1] != " "+tt.expected+" " {
			t.Errorf("for %s: expected '%s', got '%s'", tt.input, tt.expected, matches[1])
		}
	}
}

func TestResetPattern(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<ralph>RESET:US-001</ralph>", "US-001"},
		{"<ralph>RESET:US-001,US-002</ralph>", "US-001,US-002"},
		{"<ralph>RESET:US-001, US-002, US-003</ralph>", "US-001, US-002, US-003"},
	}

	for _, tt := range tests {
		matches := ResetPattern.FindStringSubmatch(tt.input)
		if len(matches) < 2 {
			t.Errorf("expected match for %s", tt.input)
			continue
		}
		if matches[1] != tt.expected {
			t.Errorf("for %s: expected '%s', got '%s'", tt.input, tt.expected, matches[1])
		}
	}
}

func TestProcessLine_StuckMarker(t *testing.T) {
	result := &ProviderResult{}
	processLine("I can't figure this out <ralph>STUCK</ralph>", result, nil)

	if !result.Stuck {
		t.Error("expected Stuck=true")
	}
}

func TestProcessLine_BlockMarker(t *testing.T) {
	result := &ProviderResult{}
	processLine("<ralph>BLOCK:US-007</ralph>", result, nil)

	if len(result.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result.Blocks))
	}
	if result.Blocks[0] != "US-007" {
		t.Errorf("expected block='US-007', got '%s'", result.Blocks[0])
	}
}

func TestProcessLine_BlockMarkerMultiple(t *testing.T) {
	result := &ProviderResult{}
	processLine("<ralph>BLOCK:US-007,US-008</ralph>", result, nil)

	if len(result.Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(result.Blocks))
	}
	if result.Blocks[0] != "US-007" || result.Blocks[1] != "US-008" {
		t.Errorf("unexpected blocks: %v", result.Blocks)
	}
}

func TestProcessLine_SuggestNextMarker(t *testing.T) {
	result := &ProviderResult{}
	processLine("<ralph>SUGGEST_NEXT:US-012</ralph>", result, nil)

	if result.SuggestNext != "US-012" {
		t.Errorf("expected SuggestNext='US-012', got '%s'", result.SuggestNext)
	}
}

func TestProcessLine_CombinedMarkers(t *testing.T) {
	result := &ProviderResult{}
	processLine("<ralph>BLOCK:US-007</ralph>", result, nil)
	processLine("<ralph>REASON:Depends on US-003 which isn't complete</ralph>", result, nil)

	if len(result.Blocks) != 1 || result.Blocks[0] != "US-007" {
		t.Errorf("expected block US-007, got %v", result.Blocks)
	}
	if result.Reason != "Depends on US-003 which isn't complete" {
		t.Errorf("unexpected reason: %s", result.Reason)
	}
}

func TestBuildProviderArgs_StdinMode(t *testing.T) {
	args, promptFile, err := buildProviderArgs([]string{"--flag"}, "stdin", "", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if promptFile != "" {
		t.Errorf("expected no prompt file for stdin mode, got %q", promptFile)
	}
	// stdin mode: args unchanged, prompt not appended
	if len(args) != 1 || args[0] != "--flag" {
		t.Errorf("expected [--flag], got %v", args)
	}
}

func TestBuildProviderArgs_ArgMode(t *testing.T) {
	args, promptFile, err := buildProviderArgs([]string{"run"}, "arg", "", "do stuff")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if promptFile != "" {
		t.Errorf("expected no prompt file for arg mode, got %q", promptFile)
	}
	if len(args) != 2 || args[0] != "run" || args[1] != "do stuff" {
		t.Errorf("expected [run, do stuff], got %v", args)
	}
}

func TestBuildProviderArgs_ArgModeWithFlag(t *testing.T) {
	args, _, err := buildProviderArgs([]string{"--yes-always"}, "arg", "--message", "do stuff")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(args) != 3 || args[0] != "--yes-always" || args[1] != "--message" || args[2] != "do stuff" {
		t.Errorf("expected [--yes-always, --message, do stuff], got %v", args)
	}
}

func TestBuildProviderArgs_FileMode(t *testing.T) {
	args, promptFile, err := buildProviderArgs([]string{"--flag"}, "file", "", "prompt content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(promptFile)

	if promptFile == "" {
		t.Fatal("expected prompt file to be created")
	}
	// Verify file contents
	data, err := os.ReadFile(promptFile)
	if err != nil {
		t.Fatalf("failed to read prompt file: %v", err)
	}
	if string(data) != "prompt content" {
		t.Errorf("expected prompt file content 'prompt content', got %q", string(data))
	}
	// Last arg should be the file path
	if len(args) != 2 || args[0] != "--flag" || args[1] != promptFile {
		t.Errorf("expected [--flag, %s], got %v", promptFile, args)
	}
}

func TestBuildProviderArgs_FileModeWithFlag(t *testing.T) {
	args, promptFile, err := buildProviderArgs([]string{"--base"}, "file", "--prompt-file", "content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(promptFile)

	if len(args) != 3 || args[0] != "--base" || args[1] != "--prompt-file" || args[2] != promptFile {
		t.Errorf("expected [--base, --prompt-file, %s], got %v", promptFile, args)
	}
}

func TestBuildProviderArgs_DoesNotMutateBaseArgs(t *testing.T) {
	base := []string{"--flag1", "--flag2"}
	origLen := len(base)

	_, _, err := buildProviderArgs(base, "arg", "", "prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(base) != origLen {
		t.Errorf("base args were mutated: expected len %d, got %d", origLen, len(base))
	}
}

func TestBuildProviderArgs_ProviderIntegration(t *testing.T) {
	// Simulate what each provider's final args look like after auto-detection
	tests := []struct {
		name     string
		base     []string
		mode     string
		flag     string
		prompt   string
		wantArgs []string
	}{
		{
			"amp",
			[]string{"--dangerously-allow-all"},
			"stdin", "", "implement story",
			[]string{"--dangerously-allow-all"},
		},
		{
			"claude",
			[]string{"--print", "--dangerously-skip-permissions"},
			"stdin", "", "implement story",
			[]string{"--print", "--dangerously-skip-permissions"},
		},
		{
			"opencode",
			[]string{"run"},
			"arg", "", "implement story",
			[]string{"run", "implement story"},
		},
		{
			"aider",
			[]string{"--yes-always"},
			"arg", "--message", "implement story",
			[]string{"--yes-always", "--message", "implement story"},
		},
		{
			"codex",
			[]string{"exec", "--full-auto"},
			"arg", "", "implement story",
			[]string{"exec", "--full-auto", "implement story"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, _, err := buildProviderArgs(tt.base, tt.mode, tt.flag, tt.prompt)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(args) != len(tt.wantArgs) {
				t.Fatalf("args length: got %d, want %d (%v vs %v)", len(args), len(tt.wantArgs), args, tt.wantArgs)
			}
			for i, a := range tt.wantArgs {
				if args[i] != a {
					t.Errorf("args[%d]: got %q, want %q", i, args[i], a)
				}
			}
		})
	}
}

func TestBuildProviderArgs_EndToEnd(t *testing.T) {
	// Full pipeline: JSON config → LoadConfig → buildProviderArgs
	// Verifies the final command each provider would execute
	tests := []struct {
		command  string
		wantArgs []string // expected args AFTER buildProviderArgs (prompt included for arg mode)
	}{
		{"amp", []string{"--dangerously-allow-all"}},
		{"claude", []string{"--print", "--dangerously-skip-permissions"}},
		{"opencode", []string{"run", "test prompt"}},
		{"aider", []string{"--yes-always", "--message", "test prompt"}},
		{"codex", []string{"exec", "--full-auto", "test prompt"}},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			dir := t.TempDir()
			configContent := fmt.Sprintf(`{
				"provider": {"command": %q},
				"verify": {"default": ["echo ok"]}
			}`, tt.command)
			os.WriteFile(filepath.Join(dir, "ralph.config.json"), []byte(configContent), 0644)

			cfg, err := LoadConfig(dir)
			if err != nil {
				t.Fatalf("LoadConfig error: %v", err)
			}

			p := cfg.Config.Provider
			args, promptFile, err := buildProviderArgs(p.Args, p.PromptMode, p.PromptFlag, "test prompt")
			if err != nil {
				t.Fatalf("buildProviderArgs error: %v", err)
			}
			if promptFile != "" {
				os.Remove(promptFile)
			}

			if len(args) != len(tt.wantArgs) {
				t.Fatalf("args: got %v, want %v", args, tt.wantArgs)
			}
			for i, a := range tt.wantArgs {
				if args[i] != a {
					t.Errorf("args[%d]: got %q, want %q", i, args[i], a)
				}
			}
		})
	}
}

func TestRunCommand_Timeout(t *testing.T) {
	dir := t.TempDir()
	output, err := runCommand(dir, "sleep 10", 1)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected 'timed out' in error, got: %v", err)
	}
	// Output should be captured (may be empty for sleep)
	_ = output
}

func TestRunCommand_Success(t *testing.T) {
	dir := t.TempDir()
	output, err := runCommand(dir, "echo hello", 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "hello") {
		t.Errorf("expected 'hello' in output, got: %q", output)
	}
}

func TestRunCommand_NonZeroExit(t *testing.T) {
	dir := t.TempDir()
	_, err := runCommand(dir, "exit 1", 30)
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
	// Should NOT be a timeout error
	if strings.Contains(err.Error(), "timed out") {
		t.Error("expected non-timeout error")
	}
}

func TestTruncateOutput(t *testing.T) {
	short := "line1\nline2\nline3"
	if got := truncateOutput(short, 10); got != short {
		t.Errorf("expected short output unchanged, got %q", got)
	}

	lines := make([]string, 100)
	for i := range lines {
		lines[i] = fmt.Sprintf("line%d", i)
	}
	long := strings.Join(lines, "\n")
	result := truncateOutput(long, 5)
	if !strings.Contains(result, "line99") {
		t.Error("expected last line present")
	}
	if strings.Contains(result, "line0\n") {
		t.Error("expected first line truncated")
	}
}
