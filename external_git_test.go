package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExternalGitOps_NewWithDefaults(t *testing.T) {
	ops := NewExternalGitOps("/path/to/repo", "https://github.com/example/repo", "")

	if ops.repoPath != "/path/to/repo" {
		t.Errorf("unexpected repoPath: %s", ops.repoPath)
	}
	if ops.url != "https://github.com/example/repo" {
		t.Errorf("unexpected url: %s", ops.url)
	}
	if ops.branch != "main" {
		t.Errorf("expected default branch 'main', got '%s'", ops.branch)
	}
}

func TestExternalGitOps_NewWithBranch(t *testing.T) {
	ops := NewExternalGitOps("/path/to/repo", "https://github.com/example/repo", "canary")

	if ops.branch != "canary" {
		t.Errorf("expected branch 'canary', got '%s'", ops.branch)
	}
}

func TestExternalGitOps_Exists_False(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "nonexistent")

	ops := NewExternalGitOps(repoPath, "https://example.com/repo", "main")

	if ops.Exists() {
		t.Error("expected Exists() to return false for nonexistent repo")
	}
}

func TestExternalGitOps_Exists_True(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "repo")

	// Create a fake git repo
	os.MkdirAll(filepath.Join(repoPath, ".git"), 0755)

	ops := NewExternalGitOps(repoPath, "https://example.com/repo", "main")

	if !ops.Exists() {
		t.Error("expected Exists() to return true for existing repo with .git")
	}
}

func TestExternalGitOps_GetCurrentCommit_NoRepo(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "nonexistent")

	ops := NewExternalGitOps(repoPath, "https://example.com/repo", "main")

	commit := ops.GetCurrentCommit()
	if commit != "" {
		t.Errorf("expected empty commit for nonexistent repo, got '%s'", commit)
	}
}

func TestExternalGitOps_Clone_CreatesParentDir(t *testing.T) {
	// This test doesn't actually clone (would need network)
	// but we can verify the error handling and that it tries to create parent

	dir := t.TempDir()
	repoPath := filepath.Join(dir, "nested", "path", "repo")

	ops := NewExternalGitOps(repoPath, "https://invalid-url-that-wont-work.example.com/repo", "main")

	// The clone will fail but should create parent directory first
	err := ops.Clone(true)
	if err == nil {
		t.Error("expected clone to fail with invalid URL")
	}

	// Parent directory should have been created
	parentDir := filepath.Dir(repoPath)
	if _, err := os.Stat(parentDir); os.IsNotExist(err) {
		t.Error("expected parent directory to be created")
	}
}

func TestExternalGitOps_GetRepoSize_NoRepo(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "nonexistent")

	ops := NewExternalGitOps(repoPath, "https://example.com/repo", "main")

	_, err := ops.GetRepoSize()
	if err == nil {
		t.Error("expected error for nonexistent repo")
	}
}

func TestExternalGitOps_GetRepoSize_WithFiles(t *testing.T) {
	dir := t.TempDir()
	repoPath := dir

	// Create some files
	os.WriteFile(filepath.Join(repoPath, "file1.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(repoPath, "file2.txt"), []byte("world!"), 0644)

	ops := NewExternalGitOps(repoPath, "https://example.com/repo", "main")

	size, err := ops.GetRepoSize()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be at least 11 bytes (hello + world!)
	if size < 11 {
		t.Errorf("expected size >= 11, got %d", size)
	}
}
