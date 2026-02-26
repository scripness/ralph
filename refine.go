package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// generateRefineSummary spawns a non-interactive subagent to summarize refine session changes,
// extracts the summary, appends to summary.md, and commits.
func generateRefineSummary(cfg *ResolvedConfig, featureDir *FeatureDir, preCommit string) error {
	git := NewGitOps(cfg.ProjectRoot)

	// Get git log since pre-session commit
	gitLog, err := git.run("log", "--oneline", preCommit+"..HEAD")
	if err != nil || strings.TrimSpace(gitLog) == "" {
		return nil // No commits made during session — nothing to summarize
	}

	// Get diff stat
	diffStat, _ := git.run("diff", "--stat", preCommit+"..HEAD")

	// Load existing summary
	summaryContent := LoadSummary(cfg.ProjectRoot)

	// Generate prompt
	timestamp := time.Now().Format("2006-01-02")
	prompt := generateRefineSummarizePrompt(featureDir.Feature, gitLog, diffStat, summaryContent, timestamp)

	// Run summary subagent
	summary, err := runSummarySubagent(cfg, prompt)
	if err != nil {
		return fmt.Errorf("failed to generate refine summary: %w", err)
	}

	// Append to summary.md
	summaryPath := SummaryPath(cfg.ProjectRoot)
	existing := LoadSummary(cfg.ProjectRoot)

	var content string
	if existing == "" {
		content = "# Feature Summaries\n\n---\n\n" + summary + "\n"
	} else {
		content = existing + "\n---\n\n" + summary + "\n"
	}

	if err := os.WriteFile(summaryPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write summary.md: %w", err)
	}

	// Commit
	if err := git.CommitFiles([]string{summaryPath}, fmt.Sprintf("ralph: update summary for %s (refine)", featureDir.Feature)); err != nil {
		return fmt.Errorf("failed to commit summary: %w", err)
	}

	return nil
}
