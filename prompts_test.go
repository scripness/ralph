package main

import (
	"fmt"
	"os"
	"path/filepath"
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
		"codebaseContext":    "",
		"diffSummary":        "",
		"resourceGuidance": "",
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
	if !strings.Contains(prompt, "e2e tests") {
		t.Error("prompt should mention e2e tests for UI stories")
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
	if !strings.Contains(prompt, "e2e tests") {
		t.Error("prompt should mention e2e tests for UI stories")
	}
	if !strings.Contains(prompt, "Typecheck passes") {
		t.Error("prompt should enforce typecheck in checklist")
	}
	if !strings.Contains(prompt, "Tests pass") {
		t.Error("prompt should enforce tests pass in checklist")
	}
}

func TestGetPrompt_RefineSession(t *testing.T) {
	prompt := getPrompt("refine-session", map[string]string{
		"feature":          "auth",
		"summary":          "## auth (2026-02-25)\n\nBuilt login and logout.",
		"diffSummary":      "",
		"codebaseContext":  "",
		"branchName":       "ralph/auth",
		"featureDir":       "/project/.ralph/2024-01-15-auth",
		"knowledgeFile":    "CLAUDE.md",
		"verifyCommands":   "- bun run test",
		"serviceURLs":      "",
		"resourceGuidance": "",
	})

	if !strings.Contains(prompt, "auth") {
		t.Error("prompt should contain feature name")
	}
	if !strings.Contains(prompt, "Built login and logout") {
		t.Error("prompt should contain summary content")
	}
	if !strings.Contains(prompt, "ralph/auth") {
		t.Error("prompt should contain branch name")
	}
	if !strings.Contains(prompt, "CLAUDE.md") {
		t.Error("prompt should contain knowledge file")
	}
}

func TestGetPrompt_VerifyFix(t *testing.T) {
	prompt := getPrompt("verify-fix", map[string]string{
		"project":                          "TestProject",
		"description":                      "Test description",
		"branchName":                       "ralph/test",
		"progress":                         "3/5 stories complete",
		"storyDetails":                     "",
		"verifyResults":                    "FAIL: bun run test — exit code 1",
		"verifyCommands":                   "- bun run test",
		"learnings":                        "",
		"diffSummary":                      "",
		"serviceURLs":                      "",
		"knowledgeFile":                    "CLAUDE.md",
		"featureDir":                       "/project/.ralph/2024-01-15-auth",
		"resourceGuidance": "",
	})

	if !strings.Contains(prompt, "TestProject") {
		t.Error("prompt should contain project name")
	}
	if !strings.Contains(prompt, "FAIL: bun run test") {
		t.Error("prompt should contain verify results")
	}
	if !strings.Contains(prompt, "CLAUDE.md") {
		t.Error("prompt should contain knowledge file")
	}
}

func TestGetPrompt_ProviderAgnostic(t *testing.T) {
	// Verify prompts don't contain provider-specific references
	prompts := []string{"run", "prd-create", "prd-finalize", "prd-refine", "refine-session", "verify-fix"}
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
			"storySummaries":     "Test",
			"prdContent":         "Test",
			"outputPath":         "/test",
			"prdPath":            "/test/prd.json",
			"storyDetails":       "",
			"serviceURLs":        "",
			"resourceGuidance": "",
			"diffSummary":        "",
			"verifySummary":      "",
			"verifyResults":      "",
			"prdMdContent":       "Test",
			"prdJsonContent":     "Test",
			"codebaseContext":    "",
			"featureDir":         "/test",
			"summary":            "",
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
			MaxRetries: 3,
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

	def := &PRDDefinition{
		Project:     "MyApp",
		Description: "Authentication feature",
		BranchName:  "ralph/auth",
		UserStories: []StoryDefinition{
			{ID: "US-001", Title: "Login form", Tags: []string{"ui"}, AcceptanceCriteria: []string{"Form works"}},
			{ID: "US-002", Title: "Session handling"},
		},
	}

	state := NewRunState()
	state.Retries = map[string]int{"US-001": 1}
	state.LastFailure = map[string]string{"US-001": "Previous attempt failed"}
	state.Learnings = []string{"Use bcrypt for passwords"}

	story := &def.UserStories[0]

	prompt := generateRunPrompt(cfg, featureDir, def, state, story, "", "", "")

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

	def := &PRDDefinition{
		Project:     "Test",
		Description: "Test feature",
		BranchName:  "ralph/test",
		UserStories: []StoryDefinition{
			{ID: "US-001", Title: "Database setup"},
			{ID: "US-002", Title: "API endpoint", AcceptanceCriteria: []string{"Returns 200"}},
			{ID: "US-003", Title: "UI component"},
			{ID: "US-004", Title: "Broken feature"},
		},
	}

	state := NewRunState()
	state.MarkPassed("US-001")
	state.MarkSkipped("US-004", "Missing dependency")

	story := &def.UserStories[1] // US-002 is current

	prompt := generateRunPrompt(cfg, featureDir, def, state, story, "", "", "")

	// Completed story
	if !strings.Contains(prompt, "✓ US-001: Database setup") {
		t.Error("prompt should show completed story with checkmark")
	}

	// Current story
	if !strings.Contains(prompt, "→ US-002: API endpoint [CURRENT]") {
		t.Error("prompt should mark current story")
	}

	// Pending story
	if !strings.Contains(prompt, "○ US-003: UI component") {
		t.Error("prompt should show pending story")
	}

	// Skipped story
	if !strings.Contains(prompt, "✗ US-004: Broken feature (skipped: Missing dependency)") {
		t.Error("prompt should show skipped story with reason")
	}

	// Progress
	if !strings.Contains(prompt, "1/4 stories complete (1 skipped)") {
		t.Error("prompt should show progress with skipped count")
	}
}

func TestBuildProgress(t *testing.T) {
	tests := []struct {
		name   string
		def    *PRDDefinition
		state  *RunState
		expect string
	}{
		{
			"no stories",
			&PRDDefinition{UserStories: []StoryDefinition{}},
			NewRunState(),
			"0/0 stories complete",
		},
		{
			"some complete",
			&PRDDefinition{UserStories: []StoryDefinition{
				{ID: "US-001"}, {ID: "US-002"}, {ID: "US-003"},
			}},
			func() *RunState {
				s := NewRunState()
				s.MarkPassed("US-001")
				s.MarkPassed("US-003")
				return s
			}(),
			"2/3 stories complete",
		},
		{
			"with skipped",
			&PRDDefinition{UserStories: []StoryDefinition{
				{ID: "US-001"}, {ID: "US-002"}, {ID: "US-003"},
			}},
			func() *RunState {
				s := NewRunState()
				s.MarkPassed("US-001")
				s.MarkSkipped("US-002", "too hard")
				return s
			}(),
			"1/3 stories complete (1 skipped)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildProgress(tt.def, tt.state)
			if result != tt.expect {
				t.Errorf("expected %q, got %q", tt.expect, result)
			}
		})
	}
}

func TestBuildStoryMap(t *testing.T) {
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-001", Title: "Done"},
			{ID: "US-002", Title: "Current"},
			{ID: "US-003", Title: "Pending"},
			{ID: "US-004", Title: "Stuck"},
		},
	}

	state := NewRunState()
	state.MarkPassed("US-001")
	state.MarkSkipped("US-004", "Need API")

	current := &StoryDefinition{ID: "US-002"}
	result := buildStoryMap(def, state, current)

	if !strings.Contains(result, "✓ US-001: Done") {
		t.Error("should show completed story")
	}
	if !strings.Contains(result, "→ US-002: Current [CURRENT]") {
		t.Error("should mark current story")
	}
	if !strings.Contains(result, "○ US-003: Pending") {
		t.Error("should show pending story")
	}
	if !strings.Contains(result, "✗ US-004: Stuck (skipped: Need API)") {
		t.Error("should show skipped story with reason")
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
		"codebaseContext":                   "",
		"diffSummary":                       "",
		"resourceGuidance":resourceInstr,
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
		"codebaseContext":                   "",
		"diffSummary":                       "",
		"resourceGuidance":webSearchInstr,
	})

	if !strings.Contains(prompt, "Documentation Verification") {
		t.Error("prompt should contain documentation verification section")
	}
	if !strings.Contains(prompt, "web search") {
		t.Error("prompt should contain web search fallback instructions")
	}
}

func TestGenerateRefineSessionPrompt(t *testing.T) {
	dir := t.TempDir()
	featureDir := &FeatureDir{
		Feature: "auth",
		Path:    dir,
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

	summaryContent := "## auth (2026-02-25)\n\nBuilt login and logout. Used bcrypt for passwords."

	prompt := generateRefineSessionPrompt(cfg, featureDir, summaryContent, "", "")

	if !strings.Contains(prompt, "auth") {
		t.Error("prompt should contain feature name")
	}
	if !strings.Contains(prompt, "Built login and logout") {
		t.Error("prompt should contain summary content")
	}
	if !strings.Contains(prompt, "ralph/auth") {
		t.Error("prompt should contain branch name")
	}
	if !strings.Contains(prompt, "CLAUDE.md") {
		t.Error("prompt should contain knowledge file")
	}
	if !strings.Contains(prompt, "bun run typecheck") {
		t.Error("prompt should contain verify commands")
	}
	if !strings.Contains(prompt, "localhost:3000") {
		t.Error("prompt should contain service URLs")
	}
}

func TestBuildRefinementStoryDetails(t *testing.T) {
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-001", Title: "Login"},
			{ID: "US-002", Title: "Logout"},
			{ID: "US-003", Title: "Register"},
		},
	}

	state := NewRunState()
	state.MarkPassed("US-001")
	state.MarkSkipped("US-002", "Cannot implement")
	state.Retries = map[string]int{"US-002": 3}

	result := buildRefinementStoryDetails(def, state)

	if !strings.Contains(result, "Story Execution Status") {
		t.Error("expected heading")
	}
	if !strings.Contains(result, "US-001: Login** — PASSED") {
		t.Error("expected PASSED status for US-001")
	}
	if !strings.Contains(result, "US-002: Logout** — SKIPPED (3 retries)") {
		t.Error("expected SKIPPED with retries for US-002")
	}
	if !strings.Contains(result, "Notes: Cannot implement") {
		t.Error("expected notes for US-002")
	}
	if !strings.Contains(result, "US-003: Register** — PENDING") {
		t.Error("expected PENDING for US-003")
	}
}

func TestBuildRefinementStoryDetails_Empty(t *testing.T) {
	def := &PRDDefinition{UserStories: []StoryDefinition{}}
	state := NewRunState()
	result := buildRefinementStoryDetails(def, state)
	if result != "" {
		t.Errorf("expected empty string for no stories, got %q", result)
	}
}

func TestBuildCriteriaChecklist(t *testing.T) {
	def := &PRDDefinition{
		UserStories: []StoryDefinition{
			{ID: "US-001", Title: "Login", AcceptanceCriteria: []string{"Form renders", "Auth works"}},
			{ID: "US-002", Title: "Logout", AcceptanceCriteria: []string{"Session cleared"}},
			{ID: "US-003", Title: "Register", AcceptanceCriteria: []string{"Account created"}},
		},
	}

	state := NewRunState()
	state.MarkPassed("US-001")
	state.MarkSkipped("US-003", "blocked")

	result := buildCriteriaChecklist(def, state)

	if !strings.Contains(result, "### US-001: Login (PASSED)") {
		t.Error("expected US-001 with PASSED status")
	}
	if !strings.Contains(result, "### US-002: Logout (PENDING)") {
		t.Error("expected US-002 with PENDING status")
	}
	if !strings.Contains(result, "### US-003: Register (SKIPPED)") {
		t.Error("expected US-003 with SKIPPED status")
	}
	if !strings.Contains(result, "- [ ] Form renders") {
		t.Error("expected criterion checkbox")
	}
	if !strings.Contains(result, "- [ ] Session cleared") {
		t.Error("expected criterion checkbox")
	}
}

func TestBuildCriteriaChecklist_Empty(t *testing.T) {
	def := &PRDDefinition{UserStories: []StoryDefinition{}}
	state := NewRunState()
	result := buildCriteriaChecklist(def, state)
	if result != "" {
		t.Errorf("expected empty string for no stories, got %q", result)
	}
}

func TestGenerateVerifyAnalyzePrompt(t *testing.T) {
	cfg := &ResolvedConfig{
		ProjectRoot: "/project",
		Config: RalphConfig{
			Provider: ProviderConfig{
				Command:       "claude",
				KnowledgeFile: "CLAUDE.md",
			},
			Verify: VerifyConfig{
				Default: []string{"go vet ./...", "go test ./..."},
			},
		},
	}

	featureDir := &FeatureDir{
		Feature: "auth",
		Path:    "/project/.ralph/2024-01-15-auth",
	}

	def := &PRDDefinition{
		Project:     "MyApp",
		Description: "Auth feature",
		BranchName:  "ralph/auth",
		UserStories: []StoryDefinition{
			{ID: "US-001", Title: "Login", AcceptanceCriteria: []string{"Form renders"}},
			{ID: "US-002", Title: "Logout", AcceptanceCriteria: []string{"Session cleared"}},
		},
	}

	state := NewRunState()
	state.MarkPassed("US-001")

	report := &VerifyReport{}
	report.AddPass("go vet ./...")
	report.AddFail("go test ./...", "exit code 1")

	prompt := generateVerifyAnalyzePrompt(cfg, featureDir, def, state, report, "")

	if !strings.Contains(prompt, "MyApp") {
		t.Error("prompt should contain project name")
	}
	if !strings.Contains(prompt, "Auth feature") {
		t.Error("prompt should contain feature description")
	}
	if !strings.Contains(prompt, "ralph/auth") {
		t.Error("prompt should contain branch name")
	}
	if !strings.Contains(prompt, "US-001: Login (PASSED)") {
		t.Error("prompt should contain criteria checklist")
	}
	if !strings.Contains(prompt, "VERIFY_PASS") {
		t.Error("prompt should contain VERIFY_PASS marker instructions")
	}
	if !strings.Contains(prompt, "VERIFY_FAIL") {
		t.Error("prompt should contain VERIFY_FAIL marker instructions")
	}
}

func TestGenerateVerifyFixPrompt(t *testing.T) {
	dir := t.TempDir()
	cfg := &ResolvedConfig{
		ProjectRoot: dir,
		Config: RalphConfig{
			Provider: ProviderConfig{Command: "claude", KnowledgeFile: "CLAUDE.md"},
			Verify: VerifyConfig{
				Default: []string{"bun run typecheck", "bun run test"},
				UI:      []string{"bun run test:e2e"},
			},
			Services: []ServiceConfig{{Name: "dev", Ready: "http://localhost:3000"}},
		},
	}
	featureDir := &FeatureDir{Feature: "auth", Path: filepath.Join(dir, ".ralph", "2024-01-15-auth")}
	def := &PRDDefinition{
		Project: "MyApp", Description: "Auth feature", BranchName: "ralph/auth",
		UserStories: []StoryDefinition{
			{ID: "US-001", Title: "Login", AcceptanceCriteria: []string{"Form renders"}},
			{ID: "US-002", Title: "Logout", AcceptanceCriteria: []string{"Session cleared"}},
		},
	}
	state := NewRunState()
	state.MarkPassed("US-001")
	state.Learnings = []string{"Use bcrypt for passwords"}

	report := &VerifyReport{}
	report.AddPass("bun run typecheck")
	report.AddFail("bun run test", "FAIL login_test.ts\nexit code 1")
	report.Finalize()

	prompt := generateVerifyFixPrompt(cfg, featureDir, def, state, report, "")

	// Project metadata
	if !strings.Contains(prompt, "MyApp") {
		t.Error("prompt should contain project name")
	}
	if !strings.Contains(prompt, "Auth feature") {
		t.Error("prompt should contain description")
	}
	if !strings.Contains(prompt, "ralph/auth") {
		t.Error("prompt should contain branch name")
	}

	// Verify results from report.FormatForPrompt()
	if !strings.Contains(prompt, "FAIL login_test.ts") {
		t.Error("prompt should contain verify failure output")
	}

	// Story details from buildRefinementStoryDetails()
	if !strings.Contains(prompt, "US-001: Login** — PASSED") {
		t.Error("prompt should contain story status")
	}

	// Verify commands
	if !strings.Contains(prompt, "bun run typecheck") {
		t.Error("prompt should contain verify commands")
	}
	if !strings.Contains(prompt, "bun run test:e2e") {
		t.Error("prompt should contain UI verify commands")
	}

	// Knowledge file
	if !strings.Contains(prompt, "CLAUDE.md") {
		t.Error("prompt should contain knowledge file")
	}

	// Feature dir
	if !strings.Contains(prompt, featureDir.Path) {
		t.Error("prompt should contain feature directory path")
	}

	// Learnings
	if !strings.Contains(prompt, "bcrypt") {
		t.Error("prompt should contain learnings")
	}

	// Service URLs
	if !strings.Contains(prompt, "localhost:3000") {
		t.Error("prompt should contain service URLs")
	}
}

func TestGenerateVerifyFixPrompt_WithResourceGuidance(t *testing.T) {
	dir := t.TempDir()
	cfg := &ResolvedConfig{
		ProjectRoot: dir,
		Config: RalphConfig{
			Provider: ProviderConfig{Command: "claude", KnowledgeFile: "CLAUDE.md"},
			Verify:   VerifyConfig{Default: []string{"echo ok"}},
		},
	}
	featureDir := &FeatureDir{Feature: "auth", Path: filepath.Join(dir, ".ralph", "auth")}
	def := &PRDDefinition{
		Project: "MyApp", Description: "Auth", BranchName: "ralph/auth",
		UserStories: []StoryDefinition{{ID: "US-001", Title: "Login"}},
	}
	state := NewRunState()
	report := &VerifyReport{}
	report.AddFail("echo ok", "failed")
	report.Finalize()

	guidance := "## Framework Guidance\n\n### next\nUse app router.\n\nSource: src/server.ts"
	prompt := generateVerifyFixPrompt(cfg, featureDir, def, state, report, guidance)

	if !strings.Contains(prompt, "Framework Guidance") {
		t.Error("prompt should contain resource guidance heading")
	}
	if !strings.Contains(prompt, "app router") {
		t.Error("prompt should contain guidance content")
	}
	if !strings.Contains(prompt, "Source: src/server.ts") {
		t.Error("prompt should contain source citation")
	}
}

func TestGenerateRefineSessionPrompt_WithResourceGuidance(t *testing.T) {
	dir := t.TempDir()
	featureDir := &FeatureDir{
		Feature: "auth",
		Path:    dir,
	}

	cfg := &ResolvedConfig{
		ProjectRoot: dir,
		Config: RalphConfig{
			Provider: ProviderConfig{Command: "claude", KnowledgeFile: "CLAUDE.md"},
			Verify:   VerifyConfig{Default: []string{"echo ok"}},
		},
	}

	guidance := "## Framework Guidance\n\n### react\nUse hooks for state.\n\nSource: packages/react/src/hooks.ts"
	prompt := generateRefineSessionPrompt(cfg, featureDir, "Previous summary content.", "", guidance)

	if !strings.Contains(prompt, "Framework Guidance") {
		t.Error("prompt should contain resource guidance heading")
	}
	if !strings.Contains(prompt, "hooks") {
		t.Error("prompt should contain guidance content")
	}
	if !strings.Contains(prompt, "Source: packages/react/src/hooks.ts") {
		t.Error("prompt should contain source citation")
	}
	if !strings.Contains(prompt, "Previous summary content") {
		t.Error("prompt should still contain summary content")
	}
}

func TestGenerateRefineSummarizePrompt(t *testing.T) {
	prompt := generateRefineSummarizePrompt(
		"auth",
		"abc123 feat: added OAuth\ndef456 fix: token refresh",
		"2 files changed, 50 insertions(+), 10 deletions(-)",
		"## auth (2026-02-25)\n\nBuilt login and logout.",
		"2026-02-28",
	)

	if !strings.Contains(prompt, "auth") {
		t.Error("prompt should contain feature name")
	}
	if !strings.Contains(prompt, "added OAuth") {
		t.Error("prompt should contain git log")
	}
	if !strings.Contains(prompt, "50 insertions") {
		t.Error("prompt should contain diff stat")
	}
	if !strings.Contains(prompt, "Built login and logout") {
		t.Error("prompt should contain previous summary")
	}
	if !strings.Contains(prompt, "2026-02-28") {
		t.Error("prompt should contain timestamp")
	}
	if !strings.Contains(prompt, "SUMMARY_START") {
		t.Error("prompt should contain SUMMARY_START marker")
	}
}

func TestGenerateSummaryPrompt(t *testing.T) {
	dir := t.TempDir()
	featureDir := &FeatureDir{
		Feature:    "auth",
		Path:       dir,
		HasPrdMd:   true,
		HasPrdJson: true,
	}

	os.WriteFile(featureDir.PrdMdPath(), []byte("# Auth Feature\n\nLogin and logout."), 0644)

	def := &PRDDefinition{
		SchemaVersion: 3, Project: "MyApp", BranchName: "ralph/auth", Description: "Auth feature",
		UserStories: []StoryDefinition{
			{ID: "US-001", Title: "Login", AcceptanceCriteria: []string{"Works"}},
			{ID: "US-002", Title: "Logout", AcceptanceCriteria: []string{"Works"}},
		},
	}

	state := NewRunState()
	state.MarkPassed("US-001")
	state.MarkPassed("US-002")
	state.Learnings = []string{"Use bcrypt for passwords"}

	cfg := &ResolvedConfig{
		ProjectRoot: dir,
		Config: RalphConfig{
			Provider: ProviderConfig{Command: "claude", KnowledgeFile: "CLAUDE.md"},
			Verify:   VerifyConfig{Default: []string{"echo ok"}},
		},
	}

	prompt := generateSummaryPrompt(cfg, featureDir, def, state)

	if !strings.Contains(prompt, "MyApp") {
		t.Error("prompt should contain project name")
	}
	if !strings.Contains(prompt, "Auth Feature") {
		t.Error("prompt should contain prd.md content")
	}
	if !strings.Contains(prompt, "SUMMARY_START") {
		t.Error("prompt should contain SUMMARY_START marker")
	}
	if !strings.Contains(prompt, "SUMMARY_END") {
		t.Error("prompt should contain SUMMARY_END marker")
	}
	if !strings.Contains(prompt, "bcrypt") {
		t.Error("prompt should contain learnings")
	}
}

func TestGenerateVerifyAnalyzePrompt_WithResourceGuidance(t *testing.T) {
	dir := t.TempDir()
	cfg := &ResolvedConfig{
		ProjectRoot: dir,
		Config: RalphConfig{
			Provider: ProviderConfig{Command: "claude", KnowledgeFile: "CLAUDE.md"},
			Verify:   VerifyConfig{Default: []string{"echo ok"}},
		},
	}
	featureDir := &FeatureDir{Feature: "auth", Path: filepath.Join(dir, ".ralph", "auth")}
	def := &PRDDefinition{
		Project: "MyApp", Description: "Auth", BranchName: "ralph/auth",
		UserStories: []StoryDefinition{{ID: "US-001", Title: "Login", AcceptanceCriteria: []string{"Works"}}},
	}
	state := NewRunState()
	report := &VerifyReport{}
	report.AddPass("echo ok")
	report.Finalize()

	guidance := "## Framework Guidance\n\n### react\nUse hooks.\n\nSource: packages/react/src/hooks.ts"
	prompt := generateVerifyAnalyzePrompt(cfg, featureDir, def, state, report, guidance)

	if !strings.Contains(prompt, "Framework Guidance") {
		t.Error("prompt should contain resource guidance heading")
	}
	if !strings.Contains(prompt, "hooks") {
		t.Error("prompt should contain guidance content")
	}
	if !strings.Contains(prompt, "Source: packages/react/src/hooks.ts") {
		t.Error("prompt should contain source citation")
	}

	// Baseline still present
	if !strings.Contains(prompt, "VERIFY_PASS") {
		t.Error("prompt should still contain VERIFY_PASS marker instructions")
	}
}

func TestGeneratePrdRefinePrompt(t *testing.T) {
	dir := t.TempDir()
	featureDir := &FeatureDir{
		Feature:  "auth",
		Path:     dir,
		HasPrdMd: true,
	}

	// Write a prd.md for the function to read
	os.WriteFile(featureDir.PrdMdPath(), []byte("# Auth Feature\n\n## User Stories\n\n### US-001: Login\nAs a user, I want to log in."), 0644)

	cfg := &ResolvedConfig{
		ProjectRoot: dir,
		Config: RalphConfig{
			Provider: ProviderConfig{Command: "claude", KnowledgeFile: "CLAUDE.md"},
			Verify:   VerifyConfig{Default: []string{"bun run test"}},
		},
	}

	codebaseCtx := &CodebaseContext{
		TechStack:      "TypeScript/JavaScript",
		PackageManager: "bun",
	}

	prompt := generatePrdRefinePrompt(cfg, featureDir, codebaseCtx, "## Guidance\nUse app router.")

	if !strings.Contains(prompt, "auth") {
		t.Error("prompt should contain feature name")
	}
	if !strings.Contains(prompt, "Auth Feature") {
		t.Error("prompt should contain prd.md content")
	}
	if !strings.Contains(prompt, "US-001: Login") {
		t.Error("prompt should contain story from prd.md")
	}
	if !strings.Contains(prompt, "app router") {
		t.Error("prompt should contain resource guidance")
	}
	if !strings.Contains(prompt, "Do NOT start implementing") {
		t.Error("prompt should prohibit implementation")
	}
	if !strings.Contains(prompt, "US-XXX") {
		t.Error("prompt should contain story format template")
	}
}

func TestGeneratePrdRefinePrompt_MissingPrdMd(t *testing.T) {
	dir := t.TempDir()
	featureDir := &FeatureDir{
		Feature: "auth",
		Path:    dir,
	}

	cfg := &ResolvedConfig{
		ProjectRoot: dir,
		Config: RalphConfig{
			Provider: ProviderConfig{Command: "claude"},
			Verify:   VerifyConfig{Default: []string{"echo ok"}},
		},
	}

	prompt := generatePrdRefinePrompt(cfg, featureDir, nil, "")

	if !strings.Contains(prompt, "prd.md not found") {
		t.Error("prompt should contain fallback text when prd.md missing")
	}
}
