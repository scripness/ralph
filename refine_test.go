package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateRefineSummary_NoCommits(t *testing.T) {
	dir, _ := initTestRepo(t)

	featureDir := &FeatureDir{
		Feature: "auth",
		Path:    filepath.Join(dir, ".ralph", "2024-01-15-auth"),
	}
	os.MkdirAll(featureDir.Path, 0755)

	cfg := &ResolvedConfig{
		ProjectRoot: dir,
		Config: RalphConfig{
			Provider: ProviderConfig{Command: "echo", Timeout: 30},
		},
	}

	// Use HEAD as preCommit — no new commits since then
	git := NewGitOps(dir)
	head, _ := git.run("rev-parse", "HEAD")

	err := generateRefineSummary(cfg, featureDir, head)
	if err != nil {
		t.Errorf("expected nil error for no commits, got %v", err)
	}

	// summary.md should NOT be created
	if fileExists(featureDir.SummaryMdPath()) {
		t.Error("summary.md should not be created when no commits were made")
	}
}

func TestLoadFeatureSummary_AppendSeparator(t *testing.T) {
	dir := t.TempDir()
	fd := &FeatureDir{Path: dir, Feature: "auth"}

	// Write initial summary
	initial := "## auth (2026-02-25)\n\nBuilt login and logout."
	os.WriteFile(fd.SummaryMdPath(), []byte(initial), 0644)

	// Verify LoadFeatureSummary returns existing content
	got := LoadFeatureSummary(fd)
	if got != initial {
		t.Errorf("LoadFeatureSummary() = %q, want %q", got, initial)
	}

	// Simulate what generateRefineSummary does: append with separator
	newSummary := "## auth refine (2026-02-28)\n\nAdded OAuth support."
	content := got + "\n---\n\n" + newSummary + "\n"
	os.WriteFile(fd.SummaryMdPath(), []byte(content), 0644)

	// Verify the appended content
	result := LoadFeatureSummary(fd)
	if result != content {
		t.Errorf("expected appended content, got %q", result)
	}
}
