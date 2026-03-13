package main

import (
	"strings"
	"testing"
)

func TestScripProcessLine(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantDone  bool
		wantStuck bool
		wantNote  string
		wantLearn []string
	}{
		{
			name:     "DONE marker",
			line:     "<scrip>DONE</scrip>",
			wantDone: true,
		},
		{
			name:     "DONE marker with whitespace",
			line:     "  <scrip>DONE</scrip>  ",
			wantDone: true,
		},
		{
			name:      "STUCK marker without reason",
			line:      "<scrip>STUCK</scrip>",
			wantStuck: true,
		},
		{
			name:      "STUCK marker with reason",
			line:      "<scrip>STUCK:database connection failed</scrip>",
			wantStuck: true,
			wantNote:  "database connection failed",
		},
		{
			name:      "STUCK marker with reason and whitespace",
			line:      "  <scrip>STUCK:cannot find dependency</scrip>  ",
			wantStuck: true,
			wantNote:  "cannot find dependency",
		},
		{
			name:      "LEARNING marker",
			line:      "<scrip>LEARNING:Auth tokens stored in httpOnly cookies</scrip>",
			wantLearn: []string{"Auth tokens stored in httpOnly cookies"},
		},
		{
			name:      "LEARNING marker with whitespace",
			line:      "  <scrip>LEARNING:Use bun run test for tests</scrip>  ",
			wantLearn: []string{"Use bun run test for tests"},
		},
		{
			name: "no markers",
			line: "Just regular output from the provider",
		},
		{
			name: "partial marker not detected",
			line: "The <scrip>DONE</scrip> marker should only match full lines",
		},
		{
			name: "ralph markers not detected",
			line: "<ralph>DONE</ralph>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ProviderResult{}
			scripProcessLine(tt.line, result, nil)

			if result.Done != tt.wantDone {
				t.Errorf("Done = %v, want %v", result.Done, tt.wantDone)
			}
			if result.Stuck != tt.wantStuck {
				t.Errorf("Stuck = %v, want %v", result.Stuck, tt.wantStuck)
			}
			if result.StuckNote != tt.wantNote {
				t.Errorf("StuckNote = %q, want %q", result.StuckNote, tt.wantNote)
			}
			if len(tt.wantLearn) > 0 {
				if len(result.Learnings) != len(tt.wantLearn) {
					t.Errorf("Learnings count = %d, want %d", len(result.Learnings), len(tt.wantLearn))
				} else {
					for i, l := range tt.wantLearn {
						if result.Learnings[i] != l {
							t.Errorf("Learnings[%d] = %q, want %q", i, result.Learnings[i], l)
						}
					}
				}
			} else if len(result.Learnings) > 0 {
				t.Errorf("unexpected learnings: %v", result.Learnings)
			}
		})
	}
}

func TestScripProcessLineMultipleMarkers(t *testing.T) {
	// Process multiple lines and verify accumulation
	result := &ProviderResult{}

	scripProcessLine("<scrip>LEARNING:first insight</scrip>", result, nil)
	scripProcessLine("some output", result, nil)
	scripProcessLine("<scrip>LEARNING:second insight</scrip>", result, nil)
	scripProcessLine("<scrip>DONE</scrip>", result, nil)

	if !result.Done {
		t.Error("expected Done=true")
	}
	if len(result.Learnings) != 2 {
		t.Errorf("expected 2 learnings, got %d", len(result.Learnings))
	}
	if result.Learnings[0] != "first insight" {
		t.Errorf("learning[0] = %q, want %q", result.Learnings[0], "first insight")
	}
	if result.Learnings[1] != "second insight" {
		t.Errorf("learning[1] = %q, want %q", result.Learnings[1], "second insight")
	}
}

func TestScripToRalphServices(t *testing.T) {
	scrip := []ScripServiceConfig{
		{Name: "web", Command: "npm run dev", Ready: "http://localhost:3000", Timeout: 45},
		{Name: "api", Command: "go run .", Ready: "http://localhost:8080", Timeout: 30},
	}

	ralph := scripToRalphServices(scrip)

	if len(ralph) != 2 {
		t.Fatalf("expected 2 services, got %d", len(ralph))
	}

	if ralph[0].Name != "web" {
		t.Errorf("service[0].Name = %q, want %q", ralph[0].Name, "web")
	}
	if ralph[0].Start != "npm run dev" {
		t.Errorf("service[0].Start = %q, want %q", ralph[0].Start, "npm run dev")
	}
	if ralph[0].Ready != "http://localhost:3000" {
		t.Errorf("service[0].Ready = %q, want %q", ralph[0].Ready, "http://localhost:3000")
	}
	if ralph[0].ReadyTimeout != 45 {
		t.Errorf("service[0].ReadyTimeout = %d, want %d", ralph[0].ReadyTimeout, 45)
	}

	if ralph[1].Name != "api" {
		t.Errorf("service[1].Name = %q, want %q", ralph[1].Name, "api")
	}
}

func TestScripToRalphServicesEmpty(t *testing.T) {
	ralph := scripToRalphServices(nil)
	if ralph != nil {
		t.Errorf("expected nil for empty input, got %v", ralph)
	}
}

func TestScripItemIndex(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "Create database schema"},
			{Title: "Add API endpoints"},
			{Title: "Build frontend"},
		},
	}

	if idx := scripItemIndex(plan, &plan.Items[0]); idx != 0 {
		t.Errorf("index of first item = %d, want 0", idx)
	}
	if idx := scripItemIndex(plan, &plan.Items[1]); idx != 1 {
		t.Errorf("index of second item = %d, want 1", idx)
	}
	if idx := scripItemIndex(plan, &plan.Items[2]); idx != 2 {
		t.Errorf("index of third item = %d, want 2", idx)
	}

	// Unknown item returns 0
	unknown := &PlanItem{Title: "nonexistent"}
	if idx := scripItemIndex(plan, unknown); idx != 0 {
		t.Errorf("index of unknown item = %d, want 0", idx)
	}
}

func TestAppendLearningDeduped(t *testing.T) {
	var learnings []string

	learnings = appendLearningDeduped(learnings, "Auth uses JWT tokens")
	if len(learnings) != 1 {
		t.Fatalf("expected 1 learning, got %d", len(learnings))
	}

	// Exact duplicate should be ignored
	learnings = appendLearningDeduped(learnings, "Auth uses JWT tokens")
	if len(learnings) != 1 {
		t.Errorf("duplicate not deduped: got %d learnings", len(learnings))
	}

	// Case-insensitive + trailing punctuation normalization
	learnings = appendLearningDeduped(learnings, "auth uses jwt tokens.")
	if len(learnings) != 1 {
		t.Errorf("normalized duplicate not deduped: got %d learnings", len(learnings))
	}

	// Different learning should be added
	learnings = appendLearningDeduped(learnings, "Database uses Postgres 16")
	if len(learnings) != 2 {
		t.Errorf("expected 2 learnings, got %d", len(learnings))
	}
}

func TestGenerateExecBuildPrompt(t *testing.T) {
	item := &PlanItem{
		Title:      "Add user login form",
		Acceptance: []string{"Login form renders at /login", "Invalid credentials show error"},
	}

	prompt := generateExecBuildPrompt(item, "Go project", "Check docs", "- previous insight", "", "No history")

	if !strings.Contains(prompt, "Add user login form") {
		t.Error("prompt should contain item title")
	}
	if !strings.Contains(prompt, "Login form renders at /login") {
		t.Error("prompt should contain acceptance criteria")
	}
	if !strings.Contains(prompt, "Check docs") {
		t.Error("prompt should contain consultation/resource guidance")
	}
	if !strings.Contains(prompt, "previous insight") {
		t.Error("prompt should contain learnings")
	}
	if !strings.Contains(prompt, "No history") {
		t.Error("prompt should contain progress context")
	}
}

func TestGenerateExecBuildPromptRetry(t *testing.T) {
	item := &PlanItem{
		Title:      "Fix auth flow",
		Acceptance: []string{"Tests pass"},
	}

	retryCtx := "## Retry Context\n\nYou are retrying. Previous failure: test timeout"
	prompt := generateExecBuildPrompt(item, "", "", "", retryCtx, "")

	if !strings.Contains(prompt, "Retry Context") {
		t.Error("prompt should contain retry context")
	}
	if !strings.Contains(prompt, "test timeout") {
		t.Error("prompt should contain failure reason")
	}
}

func TestGenerateExecVerifyPrompt(t *testing.T) {
	item := &PlanItem{
		Title:      "Add search endpoint",
		Acceptance: []string{"GET /search returns 200", "Empty query returns 400"},
	}

	prompt := generateExecVerifyPrompt(item, "diff --git a/search.go", "PASS: TestSearch")

	if !strings.Contains(prompt, "Add search endpoint") {
		t.Error("prompt should contain item title")
	}
	if !strings.Contains(prompt, "GET /search returns 200") {
		t.Error("prompt should contain acceptance criteria")
	}
	if !strings.Contains(prompt, "diff --git a/search.go") {
		t.Error("prompt should contain diff")
	}
	if !strings.Contains(prompt, "PASS: TestSearch") {
		t.Error("prompt should contain test output")
	}
}

func TestScripBuildSessionNarrative(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "Create schema"},
			{Title: "Add endpoints"},
			{Title: "Build UI"},
		},
	}

	events := []ProgressEvent{
		{Event: ProgressItemStart, Item: "Create schema"},
		{Event: ProgressItemDone, Item: "Create schema", Status: "passed"},
		{Event: ProgressItemStart, Item: "Add endpoints"},
		{Event: ProgressItemDone, Item: "Add endpoints", Status: "skipped"},
	}

	learnings := []string{"Schema uses UUID primary keys"}

	narrative := scripBuildSessionNarrative(plan, events, learnings)

	if !strings.Contains(narrative, "Execution Session") {
		t.Error("narrative should contain section header")
	}
	if !strings.Contains(narrative, "✓ Create schema") {
		t.Error("narrative should show passed items")
	}
	if !strings.Contains(narrative, "✗ Add endpoints") {
		t.Error("narrative should show skipped items")
	}
	if !strings.Contains(narrative, "Build UI") {
		t.Error("narrative should show remaining items")
	}
	if !strings.Contains(narrative, "Schema uses UUID primary keys") {
		t.Error("narrative should contain learnings")
	}
}

func TestScripRunVerifyEmptyConfig(t *testing.T) {
	// Verify with no commands should pass
	verify := &ScripVerifyConfig{}
	result := scripRunVerify("/tmp", verify, 10, nil)
	if !result.passed {
		t.Error("empty verify config should pass")
	}
}

func TestScripMarkerConstants(t *testing.T) {
	// Verify marker format matches the scrip convention
	if ScripDoneMarker != "<scrip>DONE</scrip>" {
		t.Errorf("ScripDoneMarker = %q, want %q", ScripDoneMarker, "<scrip>DONE</scrip>")
	}
	if ScripStuckMarker != "<scrip>STUCK</scrip>" {
		t.Errorf("ScripStuckMarker = %q, want %q", ScripStuckMarker, "<scrip>STUCK</scrip>")
	}

}
