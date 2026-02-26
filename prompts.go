package main

import (
	"embed"
	"fmt"
	"os"
	"strings"
)

//go:embed prompts/*
var promptsFS embed.FS

func getPrompt(name string, vars map[string]string) string {
	data, err := promptsFS.ReadFile("prompts/" + name + ".md")
	if err != nil {
		panic("prompt not found: " + name)
	}

	content := string(data)
	for key, value := range vars {
		content = strings.ReplaceAll(content, "{{"+key+"}}", value)
	}
	return content
}

const maxLearningsInPrompt = 50

// buildLearnings formats learnings for prompt injection, capped at maxLearningsInPrompt most recent.
func buildLearnings(learnings []string, heading string) string {
	if len(learnings) == 0 {
		return ""
	}
	s := heading + "\n\n"
	start := 0
	if len(learnings) > maxLearningsInPrompt {
		s += fmt.Sprintf("_(showing %d most recent of %d learnings)_\n\n", maxLearningsInPrompt, len(learnings))
		start = len(learnings) - maxLearningsInPrompt
	}
	for _, l := range learnings[start:] {
		s += "- " + l + "\n"
	}
	return s
}

// buildProgress returns a one-line progress summary like "3/6 stories complete (1 skipped)"
func buildProgress(def *PRDDefinition, state *RunState) string {
	total := len(def.UserStories)
	passed := CountPassed(state)
	skipped := CountSkipped(state)

	s := fmt.Sprintf("%d/%d stories complete", passed, total)
	if skipped > 0 {
		s += fmt.Sprintf(" (%d skipped)", skipped)
	}
	return s
}

// buildStoryMap builds a formatted story map showing all stories with status icons.
// The current story is marked with [CURRENT].
func buildStoryMap(def *PRDDefinition, state *RunState, current *StoryDefinition) string {
	var lines []string
	for _, s := range def.UserStories {
		var line string
		switch {
		case s.ID == current.ID:
			line = fmt.Sprintf("→ %s: %s [CURRENT]", s.ID, s.Title)
		case state.IsPassed(s.ID):
			line = fmt.Sprintf("✓ %s: %s", s.ID, s.Title)
		case state.IsSkipped(s.ID):
			line = fmt.Sprintf("✗ %s: %s", s.ID, s.Title)
			if reason := state.GetLastFailure(s.ID); reason != "" {
				line += " (skipped: " + reason + ")"
			} else {
				line += " (skipped)"
			}
		default:
			line = fmt.Sprintf("○ %s: %s", s.ID, s.Title)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// generateRunPrompt generates the prompt for story implementation.
// codebaseStr and diffSummary are pre-computed in runLoop to avoid redundant per-iteration I/O.
// resourceGuidance is the pre-computed consultation guidance (or fallback instructions).
func generateRunPrompt(cfg *ResolvedConfig, featureDir *FeatureDir, def *PRDDefinition, state *RunState, story *StoryDefinition, codebaseStr, diffSummary, resourceGuidance string) string {
	// Build acceptance criteria list
	var criteria []string
	for _, c := range story.AcceptanceCriteria {
		criteria = append(criteria, "- "+c)
	}
	criteriaStr := strings.Join(criteria, "\n")

	// Build verify commands list
	var verifyLines []string
	for _, cmd := range cfg.Config.Verify.Default {
		verifyLines = append(verifyLines, "- "+cmd)
	}
	if IsUIStory(story) {
		for _, cmd := range cfg.Config.Verify.UI {
			verifyLines = append(verifyLines, "- "+cmd+" (UI)")
		}
	}
	verifyStr := strings.Join(verifyLines, "\n")

	// Build learnings (capped at maxLearningsInPrompt most recent)
	learningsStr := buildLearnings(state.Learnings, "## Learnings from Previous Work")

	// Previous work from summary.md (archived features)
	previousWork := LoadSummary(cfg.ProjectRoot)

	// Build tags info
	tagsStr := ""
	if len(story.Tags) > 0 {
		tagsStr = fmt.Sprintf("**Tags:** %s\n", strings.Join(story.Tags, ", "))
	}

	// Build retry info with remaining retries context
	retryStr := ""
	retries := state.GetRetries(story.ID)
	if retries > 0 {
		remaining := cfg.Config.MaxRetries - retries
		retryStr = fmt.Sprintf("\n**Previous Attempts:** %d of %d (%d remaining before skipped)\n", retries, cfg.Config.MaxRetries, remaining)
		if lastFailure := state.GetLastFailure(story.ID); lastFailure != "" {
			retryStr += fmt.Sprintf("**Previous Issue:** %s\n", lastFailure)
		}
	}

	// Build service URLs
	serviceURLsStr := ""
	if len(cfg.Config.Services) > 0 {
		serviceURLsStr = "\n**Services:**\n"
		for _, svc := range cfg.Config.Services {
			serviceURLsStr += fmt.Sprintf("- %s: %s\n", svc.Name, svc.Ready)
		}
	}

	return getPrompt("run", map[string]string{
		"storyId":            story.ID,
		"storyTitle":         story.Title,
		"storyDescription":   story.Description,
		"acceptanceCriteria": criteriaStr,
		"tags":               tagsStr,
		"retryInfo":          retryStr,
		"verifyCommands":     verifyStr,
		"learnings":          learningsStr,
		"knowledgeFile":      cfg.Config.Provider.KnowledgeFile,
		"project":            def.Project,
		"description":        def.Description,
		"branchName":         def.BranchName,
		"progress":           buildProgress(def, state),
		"storyMap":           buildStoryMap(def, state, story),
		"serviceURLs":       serviceURLsStr,
		"timeout":           fmt.Sprintf("%d minutes", cfg.Config.Provider.Timeout/60),
		"codebaseContext":   codebaseStr,
		"diffSummary":       diffSummary,
		"resourceGuidance":  resourceGuidance,
		"previousWork":      previousWork,
	})
}

// generateVerifyFixPrompt generates the prompt for an interactive fix session after verification failure.
func generateVerifyFixPrompt(cfg *ResolvedConfig, featureDir *FeatureDir, def *PRDDefinition, state *RunState, report *VerifyReport, resourceGuidance string) string {
	// Build verify commands list
	var verifyLines []string
	for _, cmd := range cfg.Config.Verify.Default {
		verifyLines = append(verifyLines, "- "+cmd)
	}
	for _, cmd := range cfg.Config.Verify.UI {
		verifyLines = append(verifyLines, "- "+cmd+" (UI)")
	}
	verifyStr := strings.Join(verifyLines, "\n")

	// Build learnings
	learningsStr := buildLearnings(state.Learnings, "## Learnings from Previous Runs")

	// Build git diff summary
	git := NewGitOps(cfg.ProjectRoot)
	diffStat := git.GetDiffSummary()
	diffStr := ""
	if diffStat != "" {
		diffStr = "## Changes on Branch\n\n```\n" + truncateOutput(diffStat, 60) + "\n```\n"
	}

	// Build service URLs
	serviceURLsStr := ""
	if len(cfg.Config.Services) > 0 {
		serviceURLsStr = "\n**Services:**\n"
		for _, svc := range cfg.Config.Services {
			serviceURLsStr += fmt.Sprintf("- %s: %s\n", svc.Name, svc.Ready)
		}
	}

	return getPrompt("verify-fix", map[string]string{
		"project":          def.Project,
		"description":      def.Description,
		"branchName":       def.BranchName,
		"progress":         buildProgress(def, state),
		"storyDetails":     buildRefinementStoryDetails(def, state),
		"verifyResults":    report.FormatForPrompt(),
		"verifyCommands":   verifyStr,
		"learnings":        learningsStr,
		"diffSummary":      diffStr,
		"serviceURLs":      serviceURLsStr,
		"knowledgeFile":    cfg.Config.Provider.KnowledgeFile,
		"featureDir":       featureDir.Path,
		"resourceGuidance": resourceGuidance,
	})
}

// generatePrdCreatePrompt generates the prompt for creating a new PRD
func generatePrdCreatePrompt(cfg *ResolvedConfig, featureDir *FeatureDir, codebaseCtx *CodebaseContext, resourceGuidance string) string {
	return getPrompt("prd-create", map[string]string{
		"feature":          featureDir.Feature,
		"outputPath":       featureDir.PrdMdPath(),
		"codebaseContext":  FormatCodebaseContext(codebaseCtx),
		"resourceGuidance": resourceGuidance,
	})
}

// generatePrdRefinePrompt generates the prompt for an interactive PRD refinement session.
func generatePrdRefinePrompt(cfg *ResolvedConfig, featureDir *FeatureDir, codebaseCtx *CodebaseContext, resourceGuidance string) string {
	prdMdContent := "(prd.md not found)"
	if data, err := os.ReadFile(featureDir.PrdMdPath()); err == nil {
		prdMdContent = string(data)
	}

	return getPrompt("prd-refine", map[string]string{
		"feature":          featureDir.Feature,
		"outputPath":       featureDir.PrdMdPath(),
		"prdMdContent":     prdMdContent,
		"codebaseContext":  FormatCodebaseContext(codebaseCtx),
		"resourceGuidance": resourceGuidance,
	})
}

// generateRefineSessionPrompt generates the prompt for an interactive refine session.
// Uses summary.md as historical context instead of PRD files.
func generateRefineSessionPrompt(cfg *ResolvedConfig, featureDir *FeatureDir, summaryContent, codebaseCtx, resourceGuidance string) string {
	// Build git diff summary
	git := NewGitOps(cfg.ProjectRoot)
	diffStat := git.GetDiffSummary()
	diffStr := ""
	if diffStat != "" {
		diffStr = "## Changes on Branch\n\n```\n" + truncateOutput(diffStat, 60) + "\n```\n"
	}

	// Build verify commands list
	var verifyLines []string
	for _, cmd := range cfg.Config.Verify.Default {
		verifyLines = append(verifyLines, "- "+cmd)
	}
	for _, cmd := range cfg.Config.Verify.UI {
		verifyLines = append(verifyLines, "- "+cmd+" (UI)")
	}
	verifyStr := strings.Join(verifyLines, "\n")

	// Build service URLs
	serviceURLsStr := ""
	if len(cfg.Config.Services) > 0 {
		serviceURLsStr = "\n**Services:**\n"
		for _, svc := range cfg.Config.Services {
			serviceURLsStr += fmt.Sprintf("- %s: %s\n", svc.Name, svc.Ready)
		}
	}

	return getPrompt("refine-session", map[string]string{
		"feature":          featureDir.Feature,
		"summary":          summaryContent,
		"diffSummary":      diffStr,
		"codebaseContext":  codebaseCtx,
		"branchName":       "ralph/" + featureDir.Feature,
		"featureDir":       featureDir.Path,
		"knowledgeFile":    cfg.Config.Provider.KnowledgeFile,
		"verifyCommands":   verifyStr,
		"serviceURLs":      serviceURLsStr,
		"resourceGuidance": resourceGuidance,
	})
}

// generateRefineSummarizePrompt generates the prompt for post-refine-session summary generation.
func generateRefineSummarizePrompt(feature, gitLog, diffStat, previousSummary, timestamp string) string {
	return getPrompt("refine-summarize", map[string]string{
		"feature":         feature,
		"gitLog":          gitLog,
		"diffStat":        diffStat,
		"previousSummary": previousSummary,
		"timestamp":       timestamp,
	})
}

// generateSummaryPrompt generates the prompt for post-verify summary generation.
func generateSummaryPrompt(cfg *ResolvedConfig, featureDir *FeatureDir, def *PRDDefinition, state *RunState) string {
	// Read prd.md content
	prdMdContent := "(prd.md not found)"
	if data, err := os.ReadFile(featureDir.PrdMdPath()); err == nil {
		prdMdContent = string(data)
	}

	// Build story details
	storyDetails := buildRefinementStoryDetails(def, state)

	// Build retry details
	retryDetails := ""
	for _, s := range def.UserStories {
		retries := state.GetRetries(s.ID)
		if retries > 0 {
			retryDetails += fmt.Sprintf("- %s: %d retries", s.ID, retries)
			if note := state.GetLastFailure(s.ID); note != "" {
				retryDetails += fmt.Sprintf(" (last issue: %s)", note)
			}
			retryDetails += "\n"
		}
	}

	// Build learnings
	learningsStr := buildLearnings(state.Learnings, "## Learnings Captured During Implementation")

	// Build git diff summary
	git := NewGitOps(cfg.ProjectRoot)
	diffStat := git.GetDiffSummary()
	diffStr := ""
	if diffStat != "" {
		diffStr = "```\n" + truncateOutput(diffStat, 60) + "\n```"
	}

	// Changed files
	changedFiles := ""
	files := git.GetChangedFiles()
	for _, f := range files {
		changedFiles += "- " + f + "\n"
	}
	if changedFiles == "" {
		changedFiles = "(no changed files detected)"
	}

	return getPrompt("summary", map[string]string{
		"project":      def.Project,
		"feature":      featureDir.Feature,
		"description":  def.Description,
		"branchName":   def.BranchName,
		"prdMdContent": prdMdContent,
		"storyDetails": storyDetails,
		"passedCount":  fmt.Sprintf("%d", CountPassed(state)),
		"skippedCount": fmt.Sprintf("%d", CountSkipped(state)),
		"retryDetails": retryDetails,
		"learnings":    learningsStr,
		"diffSummary":  diffStr,
		"changedFiles": changedFiles,
	})
}

// buildRefinementStoryDetails formats each story with execution status for the refine prompt.
func buildRefinementStoryDetails(def *PRDDefinition, state *RunState) string {
	if len(def.UserStories) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, "## Story Execution Status")
	lines = append(lines, "")

	for _, s := range def.UserStories {
		status := "PENDING"
		if state.IsPassed(s.ID) {
			status = "PASSED"
		} else if state.IsSkipped(s.ID) {
			status = "SKIPPED"
		}

		line := fmt.Sprintf("- **%s: %s** — %s", s.ID, s.Title, status)
		retries := state.GetRetries(s.ID)
		if retries > 0 {
			line += fmt.Sprintf(" (%d retries)", retries)
		}
		lines = append(lines, line)

		if lastFailure := state.GetLastFailure(s.ID); lastFailure != "" {
			lines = append(lines, fmt.Sprintf("  Notes: %s", lastFailure))
		}
	}

	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

// generatePrdFinalizePrompt generates the prompt for finalizing a PRD
func generatePrdFinalizePrompt(cfg *ResolvedConfig, featureDir *FeatureDir, content, resourceGuidance string) string {
	return getPrompt("prd-finalize", map[string]string{
		"feature":          featureDir.Feature,
		"project":          cfg.Config.Project,
		"prdContent":       content,
		"outputPath":       featureDir.PrdJsonPath(),
		"resourceGuidance": resourceGuidance,
	})
}

// generateVerifyAnalyzePrompt generates the prompt for AI deep verification.
func generateVerifyAnalyzePrompt(cfg *ResolvedConfig, featureDir *FeatureDir, def *PRDDefinition, state *RunState, report *VerifyReport, resourceGuidance string) string {
	// Build git diff summary
	git := NewGitOps(cfg.ProjectRoot)
	diffStat := git.GetDiffSummary()
	diffStr := ""
	if diffStat != "" {
		diffStr = "```\n" + truncateOutput(diffStat, 60) + "\n```"
	}

	return getPrompt("verify-analyze", map[string]string{
		"project":            def.Project,
		"description":        def.Description,
		"branchName":         def.BranchName,
		"criteriaChecklist":  buildCriteriaChecklist(def, state),
		"verifyResults":      report.FormatForPrompt(),
		"diffSummary":        diffStr,
		"resourceGuidance":   resourceGuidance,
	})
}

// buildCriteriaChecklist builds a structured checklist of acceptance criteria per story.
func buildCriteriaChecklist(def *PRDDefinition, state *RunState) string {
	var lines []string
	for _, s := range def.UserStories {
		status := "PENDING"
		if state.IsPassed(s.ID) {
			status = "PASSED"
		} else if state.IsSkipped(s.ID) {
			status = "SKIPPED"
		}

		lines = append(lines, fmt.Sprintf("### %s: %s (%s)", s.ID, s.Title, status))
		for _, c := range s.AcceptanceCriteria {
			lines = append(lines, fmt.Sprintf("- [ ] %s", c))
		}
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

