package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestGetPrompt_Run(t *testing.T) {
	prompt := getPrompt("run", map[string]string{
		"storyId":            "US-001",
		"storyTitle":         "Test Story",
		"storyDescription":   "As a user...",
		"acceptanceCriteria": "- Criterion 1\n- Criterion 2",
		"tags":               "",
		"retryInfo":          "",
		"verifyCommands":     "- bun run test",
		"learnings":          "",
		"knowledgeFile":      "AGENTS.md",
		"project":            "TestProject",
		"description":        "Test feature",
		"branchName":         "ralph/test",
		"progress":           "1/3 stories complete",
		"storyMap":           "✓ US-000: Setup\n→ US-001: Test Story [CURRENT]",
		"browserSteps":       "",
		"resourceVerificationInstructions": "",
	})

	if !strings.Contains(prompt, "US-001") {
		t.Error("prompt should contain story ID")
	}
	if !strings.Contains(prompt, "Test Story") {
		t.Error("prompt should contain story title")
	}
	if !strings.Contains(prompt, "<ralph>DONE</ralph>") {
		t.Error("prompt should contain DONE marker")
	}
	if !strings.Contains(prompt, "AGENTS.md") {
		t.Error("prompt should contain knowledge file name")
	}
	if !strings.Contains(prompt, "CLI handles") {
		t.Error("prompt should contain responsibility boundaries")
	}
	if !strings.Contains(prompt, "TestProject") {
		t.Error("prompt should contain project name")
	}
	if !strings.Contains(prompt, "ralph/test") {
		t.Error("prompt should contain branch name")
	}
	if !strings.Contains(prompt, "1/3 stories complete") {
		t.Error("prompt should contain progress")
	}
	if !strings.Contains(prompt, "[CURRENT]") {
		t.Error("prompt should contain story map with CURRENT marker")
	}
	if !strings.Contains(prompt, "git log --oneline") {
		t.Error("prompt should contain git history hint")
	}
}

func TestGetPrompt_Verify(t *testing.T) {
	prompt := getPrompt("verify", map[string]string{
		"project":          "TestProject",
		"description":      "Test description",
		"storySummaries":   "- US-001: Complete",
		"verifyCommands":   "- bun run test",
		"learnings":        "",
		"knowledgeFile":      "CLAUDE.md",
		"prdPath":            "/project/.ralph/2024-01-15-auth/prd.json",
		"branchName":         "ralph/test",
		"serviceURLs":        "",
		"diffSummary":        "",
		"resourceVerificationInstructions": "",
		"verifySummary":      "",
		"criteriaChecklist":  "",
	})

	if !strings.Contains(prompt, "TestProject") {
		t.Error("prompt should contain project name")
	}
	if !strings.Contains(prompt, "<ralph>VERIFIED</ralph>") {
		t.Error("prompt should contain VERIFIED marker")
	}
	if !strings.Contains(prompt, "<ralph>RESET:") {
		t.Error("prompt should contain RESET marker example")
	}
	if !strings.Contains(prompt, "CLAUDE.md") {
		t.Error("prompt should contain knowledge file name")
	}
	if !strings.Contains(prompt, "prd.json") {
		t.Error("prompt should contain prd.json pointer")
	}
	if !strings.Contains(prompt, "do NOT modify any code") {
		t.Error("prompt should contain clarified report-only wording")
	}
}

func TestGetPrompt_PrdCreate(t *testing.T) {
	prompt := getPrompt("prd-create", map[string]string{
		"feature":    "auth",
		"outputPath": "/path/to/prd.md",
	})

	if !strings.Contains(prompt, "auth") {
		t.Error("prompt should contain feature name")
	}
	if !strings.Contains(prompt, "Clarifying Questions") {
		t.Error("prompt should contain clarifying questions section")
	}
	if !strings.Contains(prompt, "Do NOT start implementing") {
		t.Error("prompt should prohibit implementation")
	}
	if !strings.Contains(prompt, "US-XXX") {
		t.Error("prompt should contain story format example")
	}
	if !strings.Contains(prompt, "browserSteps") {
		t.Error("prompt should mention browserSteps for UI stories")
	}
	if !strings.Contains(prompt, "Typecheck passes") {
		t.Error("prompt should enforce typecheck criterion")
	}
}

func TestGetPrompt_PrdFinalize(t *testing.T) {
	prompt := getPrompt("prd-finalize", map[string]string{
		"feature":    "auth",
		"prdContent": "# Auth Feature\n\n## User Stories\n...",
		"outputPath": "/path/to/prd.json",
	})

	if !strings.Contains(prompt, "schemaVersion") {
		t.Error("prompt should contain schema version")
	}
	if !strings.Contains(prompt, "browserSteps") {
		t.Error("prompt should document browserSteps")
	}
	if !strings.Contains(prompt, "navigate") {
		t.Error("prompt should list available actions")
	}
	if !strings.Contains(prompt, "Typecheck passes") {
		t.Error("prompt should enforce typecheck in checklist")
	}
	if !strings.Contains(prompt, "Tests pass") {
		t.Error("prompt should enforce tests pass in checklist")
	}
}

func TestGetPrompt_Refine(t *testing.T) {
	prompt := getPrompt("refine", map[string]string{
		"feature":        "auth",
		"prdMdContent":   "# Auth Feature\n\nUser stories...",
		"prdJsonContent": "```json\n{}\n```",
		"progress":       "2/5 stories complete",
		"storyDetails":   "",
		"learnings":      "",
		"diffSummary":    "",
		"codebaseContext": "",
		"verifyCommands": "- bun run test",
		"serviceURLs":    "",
		"knowledgeFile":  "CLAUDE.md",
		"branchName":     "ralph/auth",
		"featureDir":     "/project/.ralph/2024-01-15-auth",
	})

	if !strings.Contains(prompt, "auth") {
		t.Error("prompt should contain feature name")
	}
	if !strings.Contains(prompt, "Auth Feature") {
		t.Error("prompt should contain prd.md content")
	}
	if !strings.Contains(prompt, "2/5 stories complete") {
		t.Error("prompt should contain progress")
	}
	if !strings.Contains(prompt, "ralph/auth") {
		t.Error("prompt should contain branch name")
	}
	if !strings.Contains(prompt, "CLAUDE.md") {
		t.Error("prompt should contain knowledge file")
	}
	if !strings.Contains(prompt, "ralph run") {
		t.Error("prompt should mention ralph run for resuming")
	}
}

func TestGetPrompt_ProviderAgnostic(t *testing.T) {
	// Verify prompts don't contain provider-specific references
	prompts := []string{"run", "verify", "prd-create", "prd-finalize", "refine"}
	forbiddenTerms := []string{
		"$AMP_CURRENT_THREAD_ID",
		"read_thread",
		"dev-browser skill",
		"oracle",
		"MCP",
		"skill",
	}

	for _, name := range prompts {
		prompt := getPrompt(name, map[string]string{
			"feature":            "test",
			"storyId":            "US-001",
			"storyTitle":         "Test",
			"storyDescription":   "Test",
			"acceptanceCriteria": "Test",
			"tags":               "",
			"retryInfo":          "",
			"verifyCommands":     "Test",
			"learnings":          "",
			"knowledgeFile":      "AGENTS.md",
			"project":            "Test",
			"description":        "Test",
			"branchName":         "ralph/test",
			"progress":           "0/1",
			"storyMap":           "→ US-001: Test [CURRENT]",
			"browserSteps":       "",
			"storySummaries":     "Test",
			"prdContent":         "Test",
			"outputPath":         "/test",
			"prdPath":            "/test/prd.json",
			"storyDetails":       "",
			"serviceURLs":        "",
			"resourceVerificationInstructions": "",
			"diffSummary":        "",
			"verifySummary":      "",
			"criteriaChecklist":  "",
			"prdMdContent":       "Test",
			"prdJsonContent":     "Test",
			"codebaseContext":    "",
			"featureDir":         "/test",
		})

		for _, term := range forbiddenTerms {
			if strings.Contains(prompt, term) {
				t.Errorf("prompt '%s' contains provider-specific term: %s", name, term)
			}
		}
	}
}

func TestGetPrompt_NotFound(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for non-existent prompt")
		}
	}()

	getPrompt("nonexistent", nil)
}

func TestGenerateRunPrompt(t *testing.T) {
	cfg := &ResolvedConfig{
		ProjectRoot: "/project",
		Config: RalphConfig{
			Provider: ProviderConfig{
				Command:       "amp",
				KnowledgeFile: "AGENTS.md",
			},
			Verify: VerifyConfig{
				Default: []string{"bun run test"},
				UI:      []string{"bun run test:e2e"},
			},
		},
	}

	featureDir := &FeatureDir{
		Feature: "auth",
		Path:    "/project/.ralph/2024-01-15-auth",
	}

	prd := &PRD{
		Project:     "MyApp",
		Description: "Authentication feature",
		BranchName:  "ralph/auth",
		UserStories: []UserStory{
			{ID: "US-001", Title: "Login form", Passes: false, Tags: []string{"ui"}, Retries: 1, Notes: "Previous attempt failed"},
			{ID: "US-002", Title: "Session handling", Passes: false},
		},
		Run: Run{
			Learnings: []string{"Use bcrypt for passwords"},
		},
	}

	story := &prd.UserStories[0]

	prompt := generateRunPrompt(cfg, featureDir, prd, story)

	if !strings.Contains(prompt, "US-001") {
		t.Error("prompt should contain story ID")
	}
	if !strings.Contains(prompt, "Login form") {
		t.Error("prompt should contain story title")
	}
	if !strings.Contains(prompt, "bcrypt") {
		t.Error("prompt should contain learnings")
	}
	if !strings.Contains(prompt, "Previous Attempts") {
		t.Error("prompt should contain retry info")
	}
	if !strings.Contains(prompt, "AGENTS.md") {
		t.Error("prompt should contain knowledge file")
	}
	if !strings.Contains(prompt, "MyApp") {
		t.Error("prompt should contain project name")
	}
	if !strings.Contains(prompt, "Authentication feature") {
		t.Error("prompt should contain feature description")
	}
	if !strings.Contains(prompt, "ralph/auth") {
		t.Error("prompt should contain branch name")
	}
	if !strings.Contains(prompt, "0/2 stories complete") {
		t.Error("prompt should contain progress")
	}
}

func TestGenerateRunPrompt_StoryMap(t *testing.T) {
	cfg := &ResolvedConfig{
		Config: RalphConfig{
			Provider: ProviderConfig{KnowledgeFile: "AGENTS.md"},
			Verify:   VerifyConfig{Default: []string{"echo ok"}},
		},
	}

	featureDir := &FeatureDir{Feature: "test", Path: "/project/.ralph/test"}

	prd := &PRD{
		Project:     "Test",
		Description: "Test feature",
		BranchName:  "ralph/test",
		UserStories: []UserStory{
			{
				ID: "US-001", Title: "Database setup", Passes: true,
				LastResult: &LastResult{Commit: "abc1234def", Summary: "Added migration"},
			},
			{
				ID: "US-002", Title: "API endpoint", Passes: false,
				AcceptanceCriteria: []string{"Returns 200"},
			},
			{
				ID: "US-003", Title: "UI component", Passes: false,
			},
			{
				ID: "US-004", Title: "Broken feature", Blocked: true, Notes: "Missing dependency",
			},
		},
	}

	story := &prd.UserStories[1] // US-002 is current

	prompt := generateRunPrompt(cfg, featureDir, prd, story)

	// Completed story with summary and short commit
	if !strings.Contains(prompt, "✓ US-001: Database setup") {
		t.Error("prompt should show completed story with checkmark")
	}
	if !strings.Contains(prompt, "Added migration") {
		t.Error("prompt should show completed story summary")
	}
	if !strings.Contains(prompt, "abc1234") {
		t.Error("prompt should show short commit hash")
	}

	// Current story
	if !strings.Contains(prompt, "→ US-002: API endpoint [CURRENT]") {
		t.Error("prompt should mark current story")
	}

	// Pending story
	if !strings.Contains(prompt, "○ US-003: UI component") {
		t.Error("prompt should show pending story")
	}

	// Blocked story
	if !strings.Contains(prompt, "✗ US-004: Broken feature (blocked: Missing dependency)") {
		t.Error("prompt should show blocked story with notes")
	}

	// Progress
	if !strings.Contains(prompt, "1/4 stories complete (1 blocked)") {
		t.Error("prompt should show progress with blocked count")
	}
}

func TestGenerateRunPrompt_BrowserSteps(t *testing.T) {
	cfg := &ResolvedConfig{
		Config: RalphConfig{
			Provider: ProviderConfig{KnowledgeFile: "AGENTS.md"},
			Verify:   VerifyConfig{Default: []string{"echo ok"}},
		},
	}

	featureDir := &FeatureDir{Feature: "test", Path: "/project/.ralph/test"}

	prd := &PRD{
		Project:     "Test",
		Description: "Test",
		BranchName:  "ralph/test",
		UserStories: []UserStory{
			{
				ID:                 "US-001",
				Title:              "Login form",
				AcceptanceCriteria: []string{"Form works"},
				Tags:               []string{"ui"},
				BrowserSteps: []BrowserStep{
					{Action: "navigate", URL: "/login"},
					{Action: "type", Selector: "#email", Value: "test@example.com"},
					{Action: "click", Selector: "button[type=submit]"},
					{Action: "assertText", Selector: "h1", Contains: "Welcome"},
				},
			},
		},
	}

	story := &prd.UserStories[0]

	prompt := generateRunPrompt(cfg, featureDir, prd, story)

	if !strings.Contains(prompt, "Browser Verification") {
		t.Error("prompt should contain browser verification section")
	}
	if !strings.Contains(prompt, "navigate → /login") {
		t.Error("prompt should show navigate step")
	}
	if !strings.Contains(prompt, `type → #email = "test@example.com"`) {
		t.Error("prompt should show type step with value")
	}
	if !strings.Contains(prompt, "click → button[type=submit]") {
		t.Error("prompt should show click step")
	}
	if !strings.Contains(prompt, `assertText → h1 contains "Welcome"`) {
		t.Error("prompt should show assertText step")
	}
	if !strings.Contains(prompt, "Design your implementation so these steps will pass") {
		t.Error("prompt should contain design guidance for browser steps")
	}
}

func TestGenerateRunPrompt_NoBrowserSteps(t *testing.T) {
	cfg := &ResolvedConfig{
		Config: RalphConfig{
			Provider: ProviderConfig{KnowledgeFile: "AGENTS.md"},
			Verify:   VerifyConfig{Default: []string{"echo ok"}},
		},
	}

	featureDir := &FeatureDir{Feature: "test", Path: "/project/.ralph/test"}

	prd := &PRD{
		Project:     "Test",
		Description: "Test",
		BranchName:  "ralph/test",
		UserStories: []UserStory{
			{ID: "US-001", Title: "Backend task", AcceptanceCriteria: []string{"Works"}},
		},
	}

	prompt := generateRunPrompt(cfg, featureDir, prd, &prd.UserStories[0])

	if strings.Contains(prompt, "Browser Verification") {
		t.Error("prompt should NOT contain browser verification for non-UI stories")
	}
}

func TestGenerateVerifyPrompt(t *testing.T) {
	cfg := &ResolvedConfig{
		ProjectRoot: "/project",
		Config: RalphConfig{
			Provider: ProviderConfig{
				KnowledgeFile: "CLAUDE.md",
			},
			Verify: VerifyConfig{
				Default: []string{"bun run test"},
			},
		},
	}

	featureDir := &FeatureDir{
		Feature: "auth",
		Path:    "/project/.ralph/2024-01-15-auth",
	}

	prd := &PRD{
		Project:     "MyProject",
		Description: "Authentication feature",
		Run: Run{
			Learnings: []string{"Learned something"},
		},
		UserStories: []UserStory{
			{
				ID: "US-001", Title: "Login", Passes: true,
				AcceptanceCriteria: []string{"Form validates", "Token returned"},
				LastResult:         &LastResult{Commit: "abc1234", Summary: "Login form done"},
			},
			{
				ID: "US-002", Title: "Logout", Passes: true,
				AcceptanceCriteria: []string{"Session cleared"},
			},
		},
	}

	prompt := generateVerifyPrompt(cfg, featureDir, prd, "")

	if !strings.Contains(prompt, "MyProject") {
		t.Error("prompt should contain project name")
	}
	if !strings.Contains(prompt, "US-001") {
		t.Error("prompt should contain story IDs")
	}
	if !strings.Contains(prompt, "CLAUDE.md") {
		t.Error("prompt should contain knowledge file")
	}
	// Verify acceptance criteria appear
	if !strings.Contains(prompt, "Form validates") {
		t.Error("prompt should contain acceptance criteria")
	}
	if !strings.Contains(prompt, "Token returned") {
		t.Error("prompt should contain all criteria")
	}
	// Verify commit info appears
	if !strings.Contains(prompt, "abc1234") {
		t.Error("prompt should contain commit hash")
	}
	if !strings.Contains(prompt, "Login form done") {
		t.Error("prompt should contain commit summary")
	}
	// Verify prd.json pointer
	if !strings.Contains(prompt, "prd.json") {
		t.Error("prompt should contain prd.json pointer")
	}
}

func TestBuildProgress(t *testing.T) {
	tests := []struct {
		name   string
		prd    *PRD
		expect string
	}{
		{
			"no stories",
			&PRD{UserStories: []UserStory{}},
			"0/0 stories complete",
		},
		{
			"some complete",
			&PRD{UserStories: []UserStory{
				{Passes: true}, {Passes: false}, {Passes: true},
			}},
			"2/3 stories complete",
		},
		{
			"with blocked",
			&PRD{UserStories: []UserStory{
				{Passes: true}, {Blocked: true}, {Passes: false},
			}},
			"1/3 stories complete (1 blocked)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildProgress(tt.prd)
			if result != tt.expect {
				t.Errorf("expected %q, got %q", tt.expect, result)
			}
		})
	}
}

func TestBuildStoryMap(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Title: "Done", Passes: true, LastResult: &LastResult{Commit: "abcdef1234567", Summary: "Finished"}},
			{ID: "US-002", Title: "Current", Passes: false},
			{ID: "US-003", Title: "Pending", Passes: false},
			{ID: "US-004", Title: "Stuck", Blocked: true, Notes: "Need API"},
		},
	}

	current := &UserStory{ID: "US-002"}
	result := buildStoryMap(prd, current)

	if !strings.Contains(result, "✓ US-001: Done") {
		t.Error("should show completed story")
	}
	if !strings.Contains(result, "Finished (abcdef1)") {
		t.Error("should show summary with truncated commit hash")
	}
	if !strings.Contains(result, "→ US-002: Current [CURRENT]") {
		t.Error("should mark current story")
	}
	if !strings.Contains(result, "○ US-003: Pending") {
		t.Error("should show pending story")
	}
	if !strings.Contains(result, "✗ US-004: Stuck (blocked: Need API)") {
		t.Error("should show blocked story with notes")
	}
}

func TestBuildBrowserSteps(t *testing.T) {
	story := &UserStory{
		BrowserSteps: []BrowserStep{
			{Action: "navigate", URL: "/page"},
			{Action: "click", Selector: "#btn"},
			{Action: "type", Selector: "#input", Value: "hello"},
			{Action: "waitFor", Selector: ".loaded"},
			{Action: "assertVisible", Selector: ".item"},
			{Action: "assertText", Selector: "h1", Contains: "Title"},
			{Action: "assertNotVisible", Selector: ".modal"},
			{Action: "submit", Selector: "form"},
			{Action: "screenshot"},
			{Action: "wait", Timeout: 3},
		},
	}

	result := buildBrowserSteps(story)

	expectations := []string{
		"navigate → /page",
		"click → #btn",
		`type → #input = "hello"`,
		"waitFor → .loaded",
		"assertVisible → .item",
		`assertText → h1 contains "Title"`,
		"assertNotVisible → .modal",
		"submit → form",
		"screenshot",
		"wait 3s",
	}

	for _, e := range expectations {
		if !strings.Contains(result, e) {
			t.Errorf("expected browser steps to contain %q", e)
		}
	}
}

func TestBuildBrowserSteps_Empty(t *testing.T) {
	story := &UserStory{}
	result := buildBrowserSteps(story)
	if result != "" {
		t.Errorf("expected empty string for no browser steps, got %q", result)
	}
}

func TestBuildLearnings_Empty(t *testing.T) {
	result := buildLearnings(nil, "## Learnings")
	if result != "" {
		t.Errorf("expected empty string for nil learnings, got %q", result)
	}
}

func TestBuildLearnings_WithItems(t *testing.T) {
	learnings := []string{"first", "second", "third"}
	result := buildLearnings(learnings, "## Learnings from Previous Work")
	if !strings.Contains(result, "## Learnings from Previous Work") {
		t.Error("expected heading in output")
	}
	if !strings.Contains(result, "- first") {
		t.Error("expected first learning in output")
	}
	if strings.Contains(result, "showing") {
		t.Error("should not show truncation notice for small list")
	}
}

func TestBuildLearnings_Capped(t *testing.T) {
	// Create more than maxLearningsInPrompt learnings
	learnings := make([]string, maxLearningsInPrompt+10)
	for i := range learnings {
		learnings[i] = fmt.Sprintf("learning %d", i)
	}

	result := buildLearnings(learnings, "## Learnings")
	if !strings.Contains(result, "showing") {
		t.Error("expected truncation notice for large list")
	}
	// Should contain the last one but not the first
	if !strings.Contains(result, fmt.Sprintf("learning %d", maxLearningsInPrompt+9)) {
		t.Error("expected most recent learning present")
	}
	if strings.Contains(result, "- learning 0\n") {
		t.Error("expected oldest learning to be truncated")
	}
}

func TestGetPrompt_VerifyWithSummary(t *testing.T) {
	summary := "PASS: bun run typecheck\nFAIL: bun run test\nPASS: service health"
	prompt := getPrompt("verify", map[string]string{
		"project":            "TestProject",
		"description":        "Test description",
		"storySummaries":     "- US-001: Complete",
		"verifyCommands":     "- bun run typecheck\n- bun run test",
		"learnings":          "",
		"knowledgeFile":      "CLAUDE.md",
		"prdPath":            "/project/.ralph/2024-01-15-auth/prd.json",
		"branchName":         "ralph/test",
		"serviceURLs":        "",
		"diffSummary":        "## Changes Summary\n\n```\n 3 files changed, 50 insertions(+)\n```\n",
		"resourceVerificationInstructions": "",
		"verifySummary":      summary,
		"criteriaChecklist":  "### US-001: Login\n- [ ] Form validates\n- [ ] Token returned\n",
	})

	if !strings.Contains(prompt, "PASS: bun run typecheck") {
		t.Error("prompt should contain passing verification result")
	}
	if !strings.Contains(prompt, "FAIL: bun run test") {
		t.Error("prompt should contain failing verification result")
	}
	if !strings.Contains(prompt, "PASS: service health") {
		t.Error("prompt should contain service health result")
	}
	if !strings.Contains(prompt, "3 files changed") {
		t.Error("prompt should contain diff summary")
	}
	if !strings.Contains(prompt, "Changes Summary") {
		t.Error("prompt should contain changes summary heading")
	}
	if !strings.Contains(prompt, "### US-001: Login") {
		t.Error("prompt should contain criteria checklist story header")
	}
	if !strings.Contains(prompt, "- [ ] Form validates") {
		t.Error("prompt should contain criteria checkbox items")
	}
}

func TestGetPrompt_RunWithResourcesCache(t *testing.T) {
	resourceInstr := "## Documentation Verification\n\nThe following framework source code is cached locally:\nnext, react\n\n**Available at:** ~/.ralph/resources/<framework>/\n"
	prompt := getPrompt("run", map[string]string{
		"storyId":                           "US-001",
		"storyTitle":                        "Test Story",
		"storyDescription":                  "As a user...",
		"acceptanceCriteria":                "- Criterion 1",
		"tags":                              "",
		"retryInfo":                         "",
		"verifyCommands":                    "- bun run test",
		"learnings":                         "",
		"knowledgeFile":                     "AGENTS.md",
		"project":                           "TestProject",
		"description":                       "Test feature",
		"branchName":                        "ralph/test",
		"progress":                          "0/1",
		"storyMap":                          "→ US-001: Test [CURRENT]",
		"browserSteps":                      "",
		"resourceVerificationInstructions":  resourceInstr,
	})

	if !strings.Contains(prompt, "Documentation Verification") {
		t.Error("prompt should contain documentation verification section")
	}
	if !strings.Contains(prompt, "source code is cached") {
		t.Error("prompt should contain resource cache instructions")
	}
}

func TestGetPrompt_RunWithoutResourcesCache(t *testing.T) {
	// When no resources cached, use web search fallback
	webSearchInstr := "## Documentation Verification\n\nBefore committing, verify your implementation against current official documentation using web search:\n"
	prompt := getPrompt("run", map[string]string{
		"storyId":                           "US-001",
		"storyTitle":                        "Test Story",
		"storyDescription":                  "As a user...",
		"acceptanceCriteria":                "- Criterion 1",
		"tags":                              "",
		"retryInfo":                         "",
		"verifyCommands":                    "- bun run test",
		"learnings":                         "",
		"knowledgeFile":                     "AGENTS.md",
		"project":                           "TestProject",
		"description":                       "Test feature",
		"branchName":                        "ralph/test",
		"progress":                          "0/1",
		"storyMap":                          "→ US-001: Test [CURRENT]",
		"browserSteps":                      "",
		"resourceVerificationInstructions":  webSearchInstr,
	})

	if !strings.Contains(prompt, "Documentation Verification") {
		t.Error("prompt should contain documentation verification section")
	}
	if !strings.Contains(prompt, "web search") {
		t.Error("prompt should contain web search fallback instructions")
	}
}

func TestBuildCriteriaChecklist(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{
				ID:                 "US-001",
				Title:              "Login form",
				AcceptanceCriteria: []string{"Form validates email", "Token returned on success"},
			},
			{
				ID:                 "US-002",
				Title:              "Logout",
				AcceptanceCriteria: []string{"Session cleared"},
			},
			{
				ID:      "US-003",
				Title:   "Broken",
				Blocked: true,
				AcceptanceCriteria: []string{"Should not appear"},
			},
		},
	}

	result := buildCriteriaChecklist(prd)

	if !strings.Contains(result, "### US-001: Login form") {
		t.Error("expected US-001 header")
	}
	if !strings.Contains(result, "- [ ] Form validates email") {
		t.Error("expected US-001 criterion checkbox")
	}
	if !strings.Contains(result, "- [ ] Token returned on success") {
		t.Error("expected US-001 second criterion checkbox")
	}
	if !strings.Contains(result, "### US-002: Logout") {
		t.Error("expected US-002 header")
	}
	if !strings.Contains(result, "- [ ] Session cleared") {
		t.Error("expected US-002 criterion checkbox")
	}
	if strings.Contains(result, "US-003") {
		t.Error("blocked story should not appear in checklist")
	}
	if strings.Contains(result, "Should not appear") {
		t.Error("blocked story criteria should not appear")
	}
}

func TestBuildCriteriaChecklist_Empty(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Title: "No criteria"},
		},
	}

	result := buildCriteriaChecklist(prd)
	if result != "" {
		t.Errorf("expected empty string for stories without criteria, got %q", result)
	}
}

func TestGetPrompt_VerifyWithCriteriaChecklist(t *testing.T) {
	checklist := "### US-001: Login\n- [ ] Form validates\n- [ ] Token returned\n"
	prompt := getPrompt("verify", map[string]string{
		"project":            "TestProject",
		"description":        "Test description",
		"storySummaries":     "- US-001: Complete",
		"verifyCommands":     "- bun run test",
		"learnings":          "",
		"knowledgeFile":      "CLAUDE.md",
		"prdPath":            "/test/prd.json",
		"branchName":         "ralph/test",
		"serviceURLs":        "",
		"diffSummary":        "",
		"resourceVerificationInstructions": "",
		"verifySummary":      "",
		"criteriaChecklist":  checklist,
	})

	if !strings.Contains(prompt, "### US-001: Login") {
		t.Error("prompt should contain criteria checklist story header")
	}
	if !strings.Contains(prompt, "- [ ] Form validates") {
		t.Error("prompt should contain criteria checkbox")
	}
	if !strings.Contains(prompt, "Acceptance Criteria Checklist") {
		t.Error("prompt should contain acceptance criteria checklist heading")
	}
}

func TestGenerateRefinePrompt(t *testing.T) {
	// Create temp dir with prd.md and prd.json
	dir := t.TempDir()
	featureDir := &FeatureDir{
		Feature:    "auth",
		Path:       dir,
		HasPrdMd:   true,
		HasPrdJson: true,
	}

	prdMdContent := "# Auth Feature\n\nBuild login and logout."
	if err := os.WriteFile(featureDir.PrdMdPath(), []byte(prdMdContent), 0644); err != nil {
		t.Fatal(err)
	}

	prd := &PRD{
		SchemaVersion: 2,
		Project:       "MyApp",
		BranchName:    "ralph/auth",
		Description:   "Auth feature",
		UserStories: []UserStory{
			{ID: "US-001", Title: "Login", Passes: true, AcceptanceCriteria: []string{"Works"}},
			{ID: "US-002", Title: "Logout", Blocked: true, Notes: "Depends on sessions", Retries: 3, AcceptanceCriteria: []string{"Works"}},
			{ID: "US-003", Title: "Register", Retries: 2, Notes: "Type errors", AcceptanceCriteria: []string{"Works"}},
		},
		Run: Run{
			Learnings: []string{"Use bcrypt for passwords"},
		},
	}

	prdJSON, _ := json.Marshal(prd)
	if err := os.WriteFile(featureDir.PrdJsonPath(), prdJSON, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &ResolvedConfig{
		ProjectRoot: dir,
		Config: RalphConfig{
			Provider: ProviderConfig{
				Command:       "claude",
				KnowledgeFile: "CLAUDE.md",
			},
			Verify: VerifyConfig{
				Default: []string{"bun run typecheck", "bun run test"},
			},
			Services: []ServiceConfig{
				{Name: "dev", Ready: "http://localhost:3000"},
			},
		},
	}

	prompt := generateRefinePrompt(cfg, featureDir, prd)

	// Check prd.md content is included
	if !strings.Contains(prompt, "Auth Feature") {
		t.Error("prompt should contain prd.md content")
	}
	if !strings.Contains(prompt, "Build login and logout") {
		t.Error("prompt should contain prd.md body")
	}

	// Check prd.json content is included
	if !strings.Contains(prompt, "schemaVersion") {
		t.Error("prompt should contain prd.json content")
	}

	// Check progress
	if !strings.Contains(prompt, "1/3 stories complete (1 blocked)") {
		t.Error("prompt should contain progress")
	}

	// Check story details
	if !strings.Contains(prompt, "US-001: Login** — PASSED") {
		t.Error("prompt should show US-001 as PASSED")
	}
	if !strings.Contains(prompt, "US-002: Logout** — BLOCKED") {
		t.Error("prompt should show US-002 as BLOCKED")
	}
	if !strings.Contains(prompt, "Depends on sessions") {
		t.Error("prompt should include story notes")
	}

	// Check learnings
	if !strings.Contains(prompt, "bcrypt") {
		t.Error("prompt should include learnings")
	}

	// Check environment info
	if !strings.Contains(prompt, "ralph/auth") {
		t.Error("prompt should contain branch name")
	}
	if !strings.Contains(prompt, "CLAUDE.md") {
		t.Error("prompt should contain knowledge file")
	}

	// Check verify commands
	if !strings.Contains(prompt, "bun run typecheck") {
		t.Error("prompt should contain verify commands")
	}

	// Check service URLs
	if !strings.Contains(prompt, "localhost:3000") {
		t.Error("prompt should contain service URLs")
	}

	// Check guidance
	if !strings.Contains(prompt, "ralph run auth") {
		t.Error("prompt should mention ralph run for resuming")
	}
}

func TestBuildRefinementStoryDetails(t *testing.T) {
	prd := &PRD{
		UserStories: []UserStory{
			{ID: "US-001", Title: "Login", Passes: true, LastResult: &LastResult{Summary: "Implemented"}},
			{ID: "US-002", Title: "Logout", Blocked: true, Retries: 3, Notes: "Cannot implement"},
			{ID: "US-003", Title: "Register"},
		},
	}

	result := buildRefinementStoryDetails(prd)

	if !strings.Contains(result, "Story Execution Status") {
		t.Error("expected heading")
	}
	if !strings.Contains(result, "US-001: Login** — PASSED") {
		t.Error("expected PASSED status for US-001")
	}
	if !strings.Contains(result, "Last result: Implemented") {
		t.Error("expected last result for US-001")
	}
	if !strings.Contains(result, "US-002: Logout** — BLOCKED (3 retries)") {
		t.Error("expected BLOCKED with retries for US-002")
	}
	if !strings.Contains(result, "Notes: Cannot implement") {
		t.Error("expected notes for US-002")
	}
	if !strings.Contains(result, "US-003: Register** — PENDING") {
		t.Error("expected PENDING for US-003")
	}
}

func TestBuildRefinementStoryDetails_Empty(t *testing.T) {
	prd := &PRD{UserStories: []UserStory{}}
	result := buildRefinementStoryDetails(prd)
	if result != "" {
		t.Errorf("expected empty string for no stories, got %q", result)
	}
}
