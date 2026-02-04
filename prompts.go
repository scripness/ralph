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

// buildProgress returns a one-line progress summary like "3/6 stories complete (1 blocked)"
func buildProgress(prd *PRD) string {
	total := len(prd.UserStories)
	complete := CountComplete(prd)
	blocked := CountBlocked(prd)

	s := fmt.Sprintf("%d/%d stories complete", complete, total)
	if blocked > 0 {
		s += fmt.Sprintf(" (%d blocked)", blocked)
	}
	return s
}

// buildStoryMap builds a formatted story map showing all stories with status icons.
// The current story is marked with [CURRENT]. Completed stories show their commit summary.
func buildStoryMap(prd *PRD, current *UserStory) string {
	var lines []string
	for _, s := range prd.UserStories {
		var line string
		switch {
		case s.ID == current.ID:
			line = fmt.Sprintf("→ %s: %s [CURRENT]", s.ID, s.Title)
		case s.Passes:
			line = fmt.Sprintf("✓ %s: %s", s.ID, s.Title)
			if s.LastResult != nil {
				detail := ""
				if s.LastResult.Summary != "" {
					detail = s.LastResult.Summary
				}
				if s.LastResult.Commit != "" {
					commit := s.LastResult.Commit
					if len(commit) > 7 {
						commit = commit[:7]
					}
					if detail != "" {
						detail += " (" + commit + ")"
					} else {
						detail = commit
					}
				}
				if detail != "" {
					line += "\n  └─ " + detail
				}
			}
		case s.Blocked:
			line = fmt.Sprintf("✗ %s: %s", s.ID, s.Title)
			if s.Notes != "" {
				line += " (blocked: " + s.Notes + ")"
			} else {
				line += " (blocked)"
			}
		default:
			line = fmt.Sprintf("○ %s: %s", s.ID, s.Title)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// buildBrowserSteps formats browser verification steps for the run prompt.
// Returns an empty string if the story has no browser steps.
func buildBrowserSteps(story *UserStory) string {
	if len(story.BrowserSteps) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, "## Browser Verification")
	lines = append(lines, "")
	lines = append(lines, "After you signal DONE, the CLI will run these browser steps to verify your UI:")
	lines = append(lines, "")

	for i, step := range story.BrowserSteps {
		var desc string
		switch step.Action {
		case "navigate":
			desc = fmt.Sprintf("navigate → %s", step.URL)
		case "click":
			desc = fmt.Sprintf("click → %s", step.Selector)
		case "type":
			desc = fmt.Sprintf("type → %s = %q", step.Selector, step.Value)
		case "waitFor":
			desc = fmt.Sprintf("waitFor → %s", step.Selector)
		case "assertVisible":
			desc = fmt.Sprintf("assertVisible → %s", step.Selector)
		case "assertText":
			desc = fmt.Sprintf("assertText → %s contains %q", step.Selector, step.Contains)
		case "assertNotVisible":
			desc = fmt.Sprintf("assertNotVisible → %s", step.Selector)
		case "submit":
			desc = fmt.Sprintf("submit → %s", step.Selector)
		case "screenshot":
			desc = "screenshot"
		case "wait":
			desc = fmt.Sprintf("wait %ds", step.Timeout)
		default:
			desc = step.Action
		}
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, desc))
	}

	lines = append(lines, "")
	lines = append(lines, "Design your implementation so these steps will pass.")
	lines = append(lines, "")
	return strings.Join(lines, "\n")
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

	// Build learnings (capped at maxLearningsInPrompt most recent)
	learningsStr := buildLearnings(prd.Run.Learnings, "## Learnings from Previous Work")

	// Build tags info
	tagsStr := ""
	if len(story.Tags) > 0 {
		tagsStr = fmt.Sprintf("**Tags:** %s\n", strings.Join(story.Tags, ", "))
	}

	// Build retry info with remaining retries context
	retryStr := ""
	if story.Retries > 0 {
		remaining := cfg.Config.MaxRetries - story.Retries
		retryStr = fmt.Sprintf("\n**Previous Attempts:** %d of %d (%d remaining before blocked)\n", story.Retries, cfg.Config.MaxRetries, remaining)
		if story.Notes != "" {
			retryStr += fmt.Sprintf("**Previous Issue:** %s\n", story.Notes)
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

	// Build btca instructions (conditional on availability)
	btcaStr := ""
	if CheckBtcaAvailable() {
		btcaStr = "## Documentation Verification\n\n" +
			"Before committing, verify your implementation against current documentation using `btca`:\n\n" +
			"```\nbtca ask --resource <library> --question \"Is this the correct pattern for <what you built>?\"\n```\n\n" +
			"Check:\n" +
			"- APIs you used (current? deprecated?)\n" +
			"- Configuration patterns (best practices?)\n" +
			"- Security patterns (input validation, auth, etc.)\n\n" +
			"If btca has no relevant resource, use web search against official docs instead.\n"
	} else {
		btcaStr = "## Documentation Verification\n\n" +
			"Before committing, verify your implementation against current official documentation using web search:\n\n" +
			"- Search for the official docs of any library or framework you used\n" +
			"- Confirm APIs you used are current and not deprecated\n" +
			"- Verify configuration patterns follow current best practices\n" +
			"- Check security patterns (input validation, auth, etc.) are up to date\n\n" +
			"Do not rely on memory alone — docs change between versions. Verify against the latest.\n"
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
		"project":            prd.Project,
		"description":        prd.Description,
		"branchName":         prd.BranchName,
		"progress":           buildProgress(prd),
		"storyMap":           buildStoryMap(prd, story),
		"browserSteps":       buildBrowserSteps(story),
		"serviceURLs":        serviceURLsStr,
		"timeout":            fmt.Sprintf("%d minutes", cfg.Config.Provider.Timeout/60),
		"btcaInstructions":   btcaStr,
	})
}

// buildCriteriaChecklist builds a structured acceptance criteria checklist for the verify prompt.
// Each story gets an explicit list of criteria the verify agent must confirm.
func buildCriteriaChecklist(prd *PRD) string {
	var lines []string
	for _, s := range prd.UserStories {
		if s.Blocked {
			continue
		}
		if len(s.AcceptanceCriteria) == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("### %s: %s", s.ID, s.Title))
		for _, c := range s.AcceptanceCriteria {
			lines = append(lines, fmt.Sprintf("- [ ] %s", c))
		}
		lines = append(lines, "")
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

// generateVerifyPrompt generates the prompt for final verification
func generateVerifyPrompt(cfg *ResolvedConfig, featureDir *FeatureDir, prd *PRD, verifySummary string) string {
	// Build story summaries with acceptance criteria
	var summaries []string
	for _, s := range prd.UserStories {
		status := "✓"
		if s.Blocked {
			status = "✗ (blocked)"
		} else if !s.Passes {
			status = "○"
		}
		line := fmt.Sprintf("- %s %s: %s", status, s.ID, s.Title)

		// Add compact acceptance criteria
		if len(s.AcceptanceCriteria) > 0 {
			line += "\n  Criteria: " + strings.Join(s.AcceptanceCriteria, " | ")
		}

		if s.LastResult != nil {
			detail := ""
			if s.LastResult.Commit != "" {
				commit := s.LastResult.Commit
				if len(commit) > 7 {
					commit = commit[:7]
				}
				detail = commit
			}
			if s.LastResult.Summary != "" {
				if detail != "" {
					detail += ": " + s.LastResult.Summary
				} else {
					detail = s.LastResult.Summary
				}
			}
			if detail != "" {
				line += "\n  └─ " + detail
			}
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

	// Build learnings (capped)
	learningsStr := buildLearnings(prd.Run.Learnings, "## Learnings")

	// Build service URLs for verify prompt
	verifyServiceURLs := ""
	if len(cfg.Config.Services) > 0 {
		verifyServiceURLs = "\n**Services:**\n"
		for _, svc := range cfg.Config.Services {
			verifyServiceURLs += fmt.Sprintf("- %s: %s\n", svc.Name, svc.Ready)
		}
	}

	// Build git diff summary
	git := NewGitOps(cfg.ProjectRoot)
	diffStat := git.GetDiffSummary()
	diffStr := ""
	if diffStat != "" {
		diffStr = "## Changes Summary\n\n```\n" + truncateOutput(diffStat, 60) + "\n```\n\nFor full diff: `git diff " + git.DefaultBranch() + "...HEAD`\n"
	}

	// Build btca instructions for verify context
	btcaStr := ""
	if CheckBtcaAvailable() {
		btcaStr = "### Documentation Compliance\n\n" +
			"Use `btca` to verify implementations follow current best practices:\n\n" +
			"```\nbtca ask --resource <library> --question \"Does this follow current best practices for <pattern>?\"\n```\n\n" +
			"Check: API patterns are current, no deprecated usage, security practices are up to date.\n" +
			"If btca has no relevant resource, use web search. Deprecated patterns = RESET.\n"
	} else {
		btcaStr = "### Documentation Compliance\n\n" +
			"Use web search to verify implementations follow current best practices:\n\n" +
			"- Search official docs for each library/framework used in the implementation\n" +
			"- Confirm API patterns are current and not deprecated\n" +
			"- Verify security practices (auth, validation, etc.) are up to date\n\n" +
			"Deprecated patterns or outdated API usage = RESET.\n"
	}

	return getPrompt("verify", map[string]string{
		"project":            prd.Project,
		"description":        prd.Description,
		"storySummaries":     summariesStr,
		"verifyCommands":     verifyStr,
		"learnings":          learningsStr,
		"knowledgeFile":      cfg.Config.Provider.KnowledgeFile,
		"prdPath":            featureDir.PrdJsonPath(),
		"branchName":         prd.BranchName,
		"serviceURLs":        verifyServiceURLs,
		"diffSummary":        diffStr,
		"btcaInstructions":   btcaStr,
		"verifySummary":      verifySummary,
		"criteriaChecklist":  buildCriteriaChecklist(prd),
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
