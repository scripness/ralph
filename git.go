package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// GitOps handles git operations for Ralph
type GitOps struct {
	projectRoot string
}

// NewGitOps creates a new GitOps instance
func NewGitOps(projectRoot string) *GitOps {
	return &GitOps{projectRoot: projectRoot}
}

// EnsureBranch ensures we're on the correct branch, creating it if needed
func (g *GitOps) EnsureBranch(branchName string) error {
	current, err := g.CurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	// Already on the right branch
	if current == branchName {
		return nil
	}

	// Check if branch exists
	if g.BranchExists(branchName) {
		// Switch to existing branch
		return g.Checkout(branchName)
	}

	// Create new branch from current HEAD
	return g.CreateBranch(branchName)
}

// CurrentBranch returns the current branch name
func (g *GitOps) CurrentBranch() (string, error) {
	out, err := g.run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// BranchExists checks if a branch exists
func (g *GitOps) BranchExists(branchName string) bool {
	_, err := g.run("rev-parse", "--verify", branchName)
	return err == nil
}

// CreateBranch creates a new branch from current HEAD
func (g *GitOps) CreateBranch(branchName string) error {
	_, err := g.run("checkout", "-b", branchName)
	return err
}

// Checkout switches to a branch
func (g *GitOps) Checkout(branchName string) error {
	_, err := g.run("checkout", branchName)
	return err
}

// CommitFile commits a single file with a message
func (g *GitOps) CommitFile(filePath, message string) error {
	// Stage the file
	if _, err := g.run("add", filePath); err != nil {
		return fmt.Errorf("failed to stage file: %w", err)
	}

	// Check if there are changes to commit
	status, _ := g.run("status", "--porcelain", filePath)
	if strings.TrimSpace(status) == "" {
		// No changes to commit
		return nil
	}

	// Commit with --only to avoid including other staged files
	_, err := g.run("commit", "--only", filePath, "-m", message)
	return err
}

// GetLastCommit returns the last commit hash (short)
func (g *GitOps) GetLastCommit() string {
	out, err := g.run("rev-parse", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// GetCommitMessage returns the commit message for a hash
func (g *GitOps) GetCommitMessage(hash string) string {
	out, err := g.run("log", "-1", "--format=%s", hash)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// DefaultBranch returns the default branch name (main or master).
func (g *GitOps) DefaultBranch() string {
	// Try origin/HEAD
	out, err := g.run("symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		ref := strings.TrimSpace(out)
		parts := strings.Split(ref, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}
	// Fallback: check which exists
	if g.BranchExists("main") {
		return "main"
	}
	if g.BranchExists("master") {
		return "master"
	}
	return "main"
}

// HasFileChanged returns true if the given file path (relative to project root) was
// modified on the current branch compared to the default branch.
func (g *GitOps) HasFileChanged(relativePath string) bool {
	base := g.DefaultBranch()
	out, err := g.run("diff", "--name-only", base+"...HEAD", "--", relativePath)
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) != ""
}

// GetDiffSummary returns the stat summary of changes from the default branch to HEAD.
func (g *GitOps) GetDiffSummary() string {
	base := g.DefaultBranch()
	out, err := g.run("diff", "--stat", base+"...HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// run executes a git command and returns the output
func (g *GitOps) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.projectRoot
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s failed: %s", args[0], string(exitErr.Stderr))
		}
		return "", err
	}
	return string(out), nil
}
