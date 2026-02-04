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

func TestGitOps_HasFileChanged(t *testing.T) {
	dir, git := initTestRepo(t)

	// Create a branch and modify a file
	git.CreateBranch("ralph/feature")

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

	// Create AGENTS.md on the branch
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Agents"), 0644)
	run("add", "AGENTS.md")
	run("commit", "-m", "add agents doc")

	// AGENTS.md should show as changed
	if !git.HasFileChanged("AGENTS.md") {
		t.Error("expected AGENTS.md to show as changed on branch")
	}

	// README.md was not modified on this branch
	if git.HasFileChanged("README.md") {
		t.Error("expected README.md to NOT show as changed on branch")
	}

	// Non-existent file should not show as changed
	if git.HasFileChanged("nonexistent.txt") {
		t.Error("expected nonexistent file to NOT show as changed")
	}
}

func TestGitOps_HasFileChanged_SameBranch(t *testing.T) {
	_, git := initTestRepo(t)

	// On main with no divergence, nothing changed
	if git.HasFileChanged("README.md") {
		t.Error("expected no changes on same branch")
	}
}

func TestGitOps_HasNewCommitSince(t *testing.T) {
	dir, git := initTestRepo(t)

	hash1 := git.GetLastCommit()

	// Same hash → false
	if git.HasNewCommitSince(hash1) {
		t.Error("expected false when HEAD matches given hash")
	}

	// Make a new commit
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

	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("data"), 0644)
	run("add", "new.txt")
	run("commit", "-m", "second commit")

	// Different hash → true
	if !git.HasNewCommitSince(hash1) {
		t.Error("expected true after new commit")
	}

	// Empty hash input → true (current != "")
	if !git.HasNewCommitSince("") {
		t.Error("expected true when given empty hash")
	}
}

func TestGitOps_IsWorkingTreeClean(t *testing.T) {
	dir, git := initTestRepo(t)

	// Clean repo → true
	if !git.IsWorkingTreeClean() {
		t.Error("expected clean working tree after init")
	}

	// Untracked file → false
	os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("data"), 0644)
	if git.IsWorkingTreeClean() {
		t.Error("expected dirty with untracked file")
	}

	// Stage the file
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

	run("add", "untracked.txt")
	// Staged but not committed → false
	if git.IsWorkingTreeClean() {
		t.Error("expected dirty with staged but uncommitted file")
	}

	// Commit → clean
	run("commit", "-m", "add file")
	if !git.IsWorkingTreeClean() {
		t.Error("expected clean after commit")
	}
}

func TestGitOps_GetChangedFiles(t *testing.T) {
	dir, git := initTestRepo(t)

	// Same branch, no divergence → nil
	files := git.GetChangedFiles()
	if files != nil {
		t.Errorf("expected nil on same branch, got %v", files)
	}

	// Create branch and add files
	git.CreateBranch("ralph/feature")

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

	os.WriteFile(filepath.Join(dir, "file1.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "file2.go"), []byte("package main"), 0644)
	run("add", ".")
	run("commit", "-m", "add files")

	files = git.GetChangedFiles()
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}

	// Delete README.md (which existed on main) and commit
	os.Remove(filepath.Join(dir, "README.md"))
	run("add", "README.md")
	run("commit", "-m", "delete README")

	// Deleted file from main should appear in the diff
	files = git.GetChangedFiles()
	found := false
	for _, f := range files {
		if f == "README.md" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected deleted README.md in changed files, got %v", files)
	}
}

func TestGitOps_HasTestFileChanges(t *testing.T) {
	dir, git := initTestRepo(t)
	git.CreateBranch("ralph/feature")

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

	// No changes → false
	if git.HasTestFileChanges() {
		t.Error("expected false with no changes")
	}

	// Source files only → false
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	run("add", "main.go")
	run("commit", "-m", "add source")
	if git.HasTestFileChanges() {
		t.Error("expected false with only source files")
	}

	// _test.go → true
	os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main"), 0644)
	run("add", "main_test.go")
	run("commit", "-m", "add Go test")
	if !git.HasTestFileChanges() {
		t.Error("expected true with _test.go file")
	}
}

func TestGitOps_HasTestFileChanges_JSPatterns(t *testing.T) {
	dir, git := initTestRepo(t)
	git.CreateBranch("ralph/feature")

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

	// .test.ts file → true
	os.WriteFile(filepath.Join(dir, "app.test.ts"), []byte("test"), 0644)
	run("add", "app.test.ts")
	run("commit", "-m", "add TS test")
	if !git.HasTestFileChanges() {
		t.Error("expected true with .test.ts file")
	}
}

func TestGitOps_HasTestFileChanges_SpecPattern(t *testing.T) {
	dir, git := initTestRepo(t)
	git.CreateBranch("ralph/feature")

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

	os.WriteFile(filepath.Join(dir, "widget.spec.js"), []byte("test"), 0644)
	run("add", "widget.spec.js")
	run("commit", "-m", "add spec")
	if !git.HasTestFileChanges() {
		t.Error("expected true with .spec.js file")
	}
}

func TestGitOps_HasTestFileChanges_TestsDir(t *testing.T) {
	dir, git := initTestRepo(t)
	git.CreateBranch("ralph/feature")

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

	os.MkdirAll(filepath.Join(dir, "__tests__"), 0755)
	os.WriteFile(filepath.Join(dir, "__tests__", "app.js"), []byte("test"), 0644)
	run("add", "__tests__/app.js")
	run("commit", "-m", "add __tests__")
	if !git.HasTestFileChanges() {
		t.Error("expected true with __tests__/ file")
	}
}
