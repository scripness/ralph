package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initTestRepo creates a temp git repo with an initial commit
func initTestRepo(t *testing.T) (string, *GitOps) {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %s\n%s", args, err, out)
		}
	}

	run("init", "-b", "main")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0644)
	run("add", "README.md")
	run("commit", "-m", "initial commit")

	return dir, NewGitOps(dir)
}

func TestGitOps_CurrentBranch(t *testing.T) {
	_, git := initTestRepo(t)

	branch, err := git.CurrentBranch()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "main" {
		t.Errorf("expected branch='main', got '%s'", branch)
	}
}

func TestGitOps_BranchExists(t *testing.T) {
	_, git := initTestRepo(t)

	if !git.BranchExists("main") {
		t.Error("expected main branch to exist")
	}
	if git.BranchExists("nonexistent-branch") {
		t.Error("expected nonexistent branch to not exist")
	}
}

func TestGitOps_CreateBranch(t *testing.T) {
	_, git := initTestRepo(t)

	err := git.CreateBranch("feature/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !git.BranchExists("feature/test") {
		t.Error("expected feature/test branch to exist after creation")
	}
}

func TestGitOps_Checkout(t *testing.T) {
	_, git := initTestRepo(t)

	git.CreateBranch("feature/test")
	// CreateBranch uses checkout -b, so we're already on it.
	// Go back to main first.
	git.Checkout("main")

	err := git.Checkout("feature/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	current, _ := git.CurrentBranch()
	if current != "feature/test" {
		t.Errorf("expected branch='feature/test', got '%s'", current)
	}
}

func TestGitOps_EnsureBranch(t *testing.T) {
	_, git := initTestRepo(t)

	// Creates new branch
	err := git.EnsureBranch("ralph/auth")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	current, _ := git.CurrentBranch()
	if current != "ralph/auth" {
		t.Errorf("expected branch='ralph/auth', got '%s'", current)
	}

	// Switch away and back
	git.Checkout("main")
	err = git.EnsureBranch("ralph/auth")
	if err != nil {
		t.Fatalf("unexpected error switching back: %v", err)
	}
	current, _ = git.CurrentBranch()
	if current != "ralph/auth" {
		t.Errorf("expected branch='ralph/auth', got '%s'", current)
	}
}

func TestGitOps_CommitFile(t *testing.T) {
	dir, git := initTestRepo(t)

	testFile := filepath.Join(dir, "test.txt")
	os.WriteFile(testFile, []byte("hello"), 0644)

	err := git.CommitFile(testFile, "add test file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGitOps_GetLastCommit(t *testing.T) {
	_, git := initTestRepo(t)

	hash := git.GetLastCommit()
	if hash == "" {
		t.Error("expected non-empty commit hash")
	}
	if len(hash) < 7 {
		t.Errorf("expected short hash >= 7 chars, got '%s'", hash)
	}
}

func TestGitOps_DefaultBranch(t *testing.T) {
	_, git := initTestRepo(t)

	branch := git.DefaultBranch()
	if branch != "main" && branch != "master" {
		t.Errorf("expected 'main' or 'master', got '%s'", branch)
	}
}

func TestGitOps_GetDiffSummary(t *testing.T) {
	dir, git := initTestRepo(t)

	// Create a branch and add a commit
	git.CreateBranch("ralph/feature")
	os.WriteFile(filepath.Join(dir, "new-file.txt"), []byte("content"), 0644)

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %s\n%s", args, err, out)
		}
	}

	run("add", "new-file.txt")
	run("commit", "-m", "add new file")

	summary := git.GetDiffSummary()
	if summary == "" {
		t.Error("expected non-empty diff summary")
	}
	if !strings.Contains(summary, "new-file.txt") {
		t.Errorf("expected summary to mention new-file.txt, got: %s", summary)
	}
	if !strings.Contains(summary, "1 file changed") {
		t.Errorf("expected summary to contain file change count, got: %s", summary)
	}
}

func TestGitOps_GetDiffSummary_SameBranch(t *testing.T) {
	_, git := initTestRepo(t)

	// On main with no divergence, diff summary should be empty
	summary := git.GetDiffSummary()
	if summary != "" {
		t.Errorf("expected empty diff summary on same branch, got: %s", summary)
	}
}
