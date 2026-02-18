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

// generateRunPrompt generates the prompt for story implementation
func generateRunPrompt(cfg *ResolvedConfig, featureDir *FeatureDir, def *PRDDefinition, state *RunState, story *StoryDefinition) string {
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

	// Build resource verification instructions
	resourceStr := buildResourceVerificationInstructions(cfg)

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
		"serviceURLs":                      serviceURLsStr,
		"timeout":                          fmt.Sprintf("%d minutes", cfg.Config.Provider.Timeout/60),
		"resourceVerificationInstructions": resourceStr,
	})
}

// generateVerifyFixPrompt generates the prompt for an interactive fix session after verification failure.
func generateVerifyFixPrompt(cfg *ResolvedConfig, featureDir *FeatureDir, def *PRDDefinition, state *RunState, report *VerifyReport) string {
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

	// Build resource verification instructions
	resourceStr := buildResourceVerificationInstructions(cfg)

	return getPrompt("verify-fix", map[string]string{
		"project":                           def.Project,
		"description":                       def.Description,
		"branchName":                        def.BranchName,
		"progress":                          buildProgress(def, state),
		"storyDetails":                      buildRefinementStoryDetails(def, state),
		"verifyResults":                     report.FormatForPrompt(),
		"verifyCommands":                    verifyStr,
		"learnings":                         learningsStr,
		"diffSummary":                       diffStr,
		"serviceURLs":                       serviceURLsStr,
		"knowledgeFile":                     cfg.Config.Provider.KnowledgeFile,
		"featureDir":                        featureDir.Path,
		"resourceVerificationInstructions":  resourceStr,
	})
}

// generatePrdCreatePrompt generates the prompt for creating a new PRD
func generatePrdCreatePrompt(cfg *ResolvedConfig, featureDir *FeatureDir, codebaseCtx *CodebaseContext) string {
	return getPrompt("prd-create", map[string]string{
		"feature":         featureDir.Feature,
		"outputPath":      featureDir.PrdMdPath(),
		"codebaseContext": FormatCodebaseContext(codebaseCtx),
	})
}

// generateRefinePrompt generates the prompt for an interactive refine session.
// Loads prd.md + prd.json content, discovers codebase context, builds git diff.
func generateRefinePrompt(cfg *ResolvedConfig, featureDir *FeatureDir, def *PRDDefinition, state *RunState) string {
	// Read prd.md content (expected to exist alongside prd.json)
	prdMdContent := "(prd.md not found)"
	if data, err := os.ReadFile(featureDir.PrdMdPath()); err == nil {
		prdMdContent = string(data)
	}

	// Read prd.json content
	prdJsonContent := ""
	if data, err := os.ReadFile(featureDir.PrdJsonPath()); err == nil {
		prdJsonContent = "```json\n" + string(data) + "\n```"
	}

	// Build progress and story details
	progress := buildProgress(def, state)
	storyDetails := buildRefinementStoryDetails(def, state)
	learnings := buildLearnings(state.Learnings, "## Learnings from Previous Runs")

	// Build git diff summary
	git := NewGitOps(cfg.ProjectRoot)
	diffStat := git.GetDiffSummary()
	diffStr := ""
	if diffStat != "" {
		diffStr = "## Changes on Branch\n\n```\n" + truncateOutput(diffStat, 60) + "\n```\n"
	}

	// Discover codebase context
	codebaseCtx := DiscoverCodebase(cfg.ProjectRoot, &cfg.Config)
	codebaseStr := FormatCodebaseContext(codebaseCtx)

	// Build verify commands list
	var verifyLines []string
	for _, cmd := range cfg.Config.Verify.Default {
		verifyLines = append(verifyLines, "- "+cmd)
	}
	for _, cmd := range cfg.Config.Verify.UI {
		verifyLines = append(verifyLines, "- "+cmd+" (UI)")
	}
	verifyStr := strings.Join(verifyLines, "\n")

	// Build service URLs (leading \n matches generateRunPrompt)
	serviceURLsStr := ""
	if len(cfg.Config.Services) > 0 {
		serviceURLsStr = "\n**Services:**\n"
		for _, svc := range cfg.Config.Services {
			serviceURLsStr += fmt.Sprintf("- %s: %s\n", svc.Name, svc.Ready)
		}
	}

	return getPrompt("refine", map[string]string{
		"feature":         featureDir.Feature,
		"prdMdContent":    prdMdContent,
		"prdJsonContent":  prdJsonContent,
		"progress":        progress,
		"storyDetails":    storyDetails,
		"learnings":       learnings,
		"diffSummary":     diffStr,
		"codebaseContext": codebaseStr,
		"verifyCommands":  verifyStr,
		"serviceURLs":     serviceURLsStr,
		"knowledgeFile":   cfg.Config.Provider.KnowledgeFile,
		"branchName":      def.BranchName,
		"featureDir":      featureDir.Path,
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
func generatePrdFinalizePrompt(cfg *ResolvedConfig, featureDir *FeatureDir, content string) string {
	return getPrompt("prd-finalize", map[string]string{
		"feature":    featureDir.Feature,
		"prdContent": content,
		"outputPath": featureDir.PrdJsonPath(),
	})
}

// buildResourceVerificationInstructions returns instructions for verifying
// implementations against cached source code resources.
func buildResourceVerificationInstructions(cfg *ResolvedConfig) string {
	if cfg.Config.Resources != nil && !cfg.Config.Resources.IsEnabled() {
		return buildFallbackVerificationInstructions()
	}

	// Detect dependencies from codebase
	codebaseCtx := DiscoverCodebase(cfg.ProjectRoot, &cfg.Config)
	depNames := GetDependencyNames(codebaseCtx.Dependencies)
	rm := NewResourceManager(cfg.Config.Resources, depNames)

	cached, _ := rm.ListCached()
	if len(cached) == 0 {
		// Resources enabled but none cached yet - show detected
		detected := rm.ListDetected()
		if len(detected) > 0 {
			return fmt.Sprintf(`## Documentation Verification

Framework source code will be available after first sync. Detected frameworks:
%s

For now, use web search to verify your implementation against official documentation.
`, strings.Join(detected, ", "))
		}
		return buildFallbackVerificationInstructions()
	}

	resourceList := strings.Join(cached, ", ")
	return fmt.Sprintf(`## Documentation Verification

The following framework source code is cached locally:
%s

**Available at:** %s/<framework>/

To verify your implementation:
1. Check how the framework implements similar patterns in its source
2. Look at tests for usage examples
3. Read inline comments for API intentions
4. Compare your patterns against framework conventions

For frameworks not cached, use web search against official repos.
`, resourceList, rm.GetCacheDir())
}

// generateVerifyAnalyzePrompt generates the prompt for AI deep verification.
func generateVerifyAnalyzePrompt(cfg *ResolvedConfig, featureDir *FeatureDir, def *PRDDefinition, state *RunState, report *VerifyReport) string {
	// Build git diff summary
	git := NewGitOps(cfg.ProjectRoot)
	diffStat := git.GetDiffSummary()
	diffStr := ""
	if diffStat != "" {
		diffStr = "```\n" + truncateOutput(diffStat, 60) + "\n```"
	}

	// Build resource verification instructions
	resourceStr := buildResourceVerificationInstructions(cfg)

	return getPrompt("verify-analyze", map[string]string{
		"project":                          def.Project,
		"description":                      def.Description,
		"branchName":                       def.BranchName,
		"criteriaChecklist":                buildCriteriaChecklist(def, state),
		"verifyResults":                    report.FormatForPrompt(),
		"diffSummary":                      diffStr,
		"resourceVerificationInstructions": resourceStr,
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

// buildFallbackVerificationInstructions returns web search instructions.
func buildFallbackVerificationInstructions() string {
	return `## Documentation Verification

Before committing, verify your implementation against current official documentation using web search:

- Search for the official docs of any library or framework you used
- Confirm APIs you used are current and not deprecated
- Verify configuration patterns follow current best practices
- Check security patterns (input validation, auth, etc.) are up to date

Do not rely on memory alone — docs change between versions. Verify against the latest.
`
}
