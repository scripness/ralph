package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ExternalGitOps handles cloning and syncing external source repositories.
// This is separate from GitOps (which handles the user's project repo).
type ExternalGitOps struct {
	repoPath string
	url      string
	ref      string // branch name OR tag name
}

// NewExternalGitOps creates a git operations helper for an external repo.
func NewExternalGitOps(repoPath, url, ref string) *ExternalGitOps {
	if ref == "" {
		ref = "main"
	}
	return &ExternalGitOps{
		repoPath: repoPath,
		url:      url,
		ref:      ref,
	}
}

// Clone clones a repo to a local path.
// Uses shallow clone (--depth 1) to save space/time.
// Single branch only (--single-branch).
func (g *ExternalGitOps) Clone(shallow bool) error {
	// Ensure parent directory exists
	parentDir := filepath.Dir(g.repoPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	args := []string{"clone"}
	if shallow {
		args = append(args, "--depth", "1", "--single-branch")
	}
	args = append(args, "--branch", g.ref, g.url, g.repoPath)

	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %w\n%s", err, string(output))
	}
	return nil
}

// Fetch fetches latest from remote without merging.
func (g *ExternalGitOps) Fetch() error {
	cmd := exec.Command("git", "fetch", "origin", g.ref)
	cmd.Dir = g.repoPath
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git fetch failed: %w\n%s", err, string(output))
	}
	return nil
}

// Pull fast-forwards to latest (fetch + reset --hard origin/ref).
// For shallow clones, this updates to the latest commit.
func (g *ExternalGitOps) Pull() error {
	// Fetch first
	if err := g.Fetch(); err != nil {
		return err
	}

	// Reset to remote ref
	cmd := exec.Command("git", "reset", "--hard", "origin/"+g.ref)
	cmd.Dir = g.repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git reset failed: %w\n%s", err, string(output))
	}
	return nil
}

// GetCurrentCommit returns the current HEAD commit hash.
func (g *ExternalGitOps) GetCurrentCommit() string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = g.repoPath
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// GetCurrentCommitShort returns the short form of the current HEAD commit hash.
func (g *ExternalGitOps) GetCurrentCommitShort() string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = g.repoPath
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// GetRemoteHeadCommit returns remote HEAD without full fetch (ls-remote).
// This is faster than fetch for checking if update is needed.
func (g *ExternalGitOps) GetRemoteHeadCommit() (string, error) {
	cmd := exec.Command("git", "ls-remote", g.url, "refs/heads/"+g.ref)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git ls-remote failed: %w", err)
	}
	// Output format: "commit_hash\trefs/heads/branch"
	parts := strings.Fields(string(output))
	if len(parts) == 0 {
		return "", fmt.Errorf("no remote HEAD found for ref %s", g.ref)
	}
	return parts[0], nil
}

// IsUpToDate compares local HEAD with remote HEAD.
// Returns true if local is at the same commit as remote.
func (g *ExternalGitOps) IsUpToDate() (bool, error) {
	localCommit := g.GetCurrentCommit()
	if localCommit == "" {
		return false, fmt.Errorf("could not get local commit")
	}

	remoteCommit, err := g.GetRemoteHeadCommit()
	if err != nil {
		return false, err
	}

	return localCommit == remoteCommit, nil
}

// Exists checks if the repo is already cloned.
func (g *ExternalGitOps) Exists() bool {
	gitDir := filepath.Join(g.repoPath, ".git")
	info, err := os.Stat(gitDir)
	return err == nil && info.IsDir()
}

// GetRepoSize returns the approximate size of the repo in bytes.
func (g *ExternalGitOps) GetRepoSize() (int64, error) {
	var size int64
	err := filepath.Walk(g.repoPath, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}
