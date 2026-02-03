package main

import (
	"embed"
	"fmt"
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

// generateRunPrompt generates the prompt for story implementation
func generateRunPrompt(cfg *ResolvedConfig, featureDir *FeatureDir, prd *PRD, story *UserStory) string {
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

	// Build learnings
	learningsStr := ""
	if len(prd.Run.Learnings) > 0 {
		learningsStr = "## Learnings from Previous Work\n\n"
		for _, l := range prd.Run.Learnings {
			learningsStr += "- " + l + "\n"
		}
	}

	// Build tags info
	tagsStr := ""
	if len(story.Tags) > 0 {
		tagsStr = fmt.Sprintf("**Tags:** %s\n", strings.Join(story.Tags, ", "))
	}

	// Build retry info
	retryStr := ""
	if story.Retries > 0 {
		retryStr = fmt.Sprintf("\n**Previous Attempts:** %d\n", story.Retries)
		if story.Notes != "" {
			retryStr += fmt.Sprintf("**Previous Issue:** %s\n", story.Notes)
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
	})
}

// generateVerifyPrompt generates the prompt for final verification
func generateVerifyPrompt(cfg *ResolvedConfig, featureDir *FeatureDir, prd *PRD) string {
	// Build story summaries
	var summaries []string
	for _, s := range prd.UserStories {
		status := "✓"
		if s.Blocked {
			status = "✗ (blocked)"
		} else if !s.Passes {
			status = "○"
		}
		line := fmt.Sprintf("- %s %s: %s", status, s.ID, s.Title)
		if s.LastResult != nil && s.LastResult.Summary != "" {
			line += fmt.Sprintf("\n  └─ %s", s.LastResult.Summary)
		}
		summaries = append(summaries, line)
	}
	summariesStr := strings.Join(summaries, "\n")

	// Build verify commands
	var verifyLines []string
	for _, cmd := range cfg.Config.Verify.Default {
		verifyLines = append(verifyLines, "- "+cmd)
	}
	for _, cmd := range cfg.Config.Verify.UI {
		verifyLines = append(verifyLines, "- "+cmd+" (UI)")
	}
	verifyStr := strings.Join(verifyLines, "\n")

	// Build learnings
	learningsStr := ""
	if len(prd.Run.Learnings) > 0 {
		learningsStr = "## Learnings\n\n"
		for _, l := range prd.Run.Learnings {
			learningsStr += "- " + l + "\n"
		}
	}

	return getPrompt("verify", map[string]string{
		"project":        prd.Project,
		"description":    prd.Description,
		"storySummaries": summariesStr,
		"verifyCommands": verifyStr,
		"learnings":      learningsStr,
		"knowledgeFile":  cfg.Config.Provider.KnowledgeFile,
	})
}

// generatePrdCreatePrompt generates the prompt for creating a new PRD
func generatePrdCreatePrompt(cfg *ResolvedConfig, featureDir *FeatureDir) string {
	return getPrompt("prd-create", map[string]string{
		"feature":    featureDir.Feature,
		"outputPath": featureDir.PrdMdPath(),
	})
}

// generatePrdRefinePrompt generates the prompt for refining a PRD
func generatePrdRefinePrompt(cfg *ResolvedConfig, featureDir *FeatureDir, content string) string {
	return getPrompt("prd-refine", map[string]string{
		"feature":    featureDir.Feature,
		"prdContent": content,
		"outputPath": featureDir.PrdMdPath(),
	})
}

// generatePrdFinalizePrompt generates the prompt for finalizing a PRD
func generatePrdFinalizePrompt(cfg *ResolvedConfig, featureDir *FeatureDir, content string) string {
	return getPrompt("prd-finalize", map[string]string{
		"feature":    featureDir.Feature,
		"prdContent": content,
		"outputPath": featureDir.PrdJsonPath(),
	})
}
