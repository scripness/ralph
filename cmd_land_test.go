package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLandBuildCriteria(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{
				Title:      "Add login form",
				Acceptance: []string{"Form validates email", "Form submits via POST /api/auth"},
			},
			{
				Title:      "Add logout button",
				Acceptance: []string{"Button visible when authenticated"},
			},
		},
	}

	result := landBuildCriteria(plan)

	if !strings.Contains(result, "### Item 1: Add login form") {
		t.Error("expected item 1 header")
	}
	if !strings.Contains(result, "### Item 2: Add logout button") {
		t.Error("expected item 2 header")
	}
	if !strings.Contains(result, "- Form validates email") {
		t.Error("expected acceptance criterion")
	}
	if !strings.Contains(result, "- Button visible when authenticated") {
		t.Error("expected acceptance criterion for item 2")
	}
}

func TestLandBuildCriteria_Empty(t *testing.T) {
	plan := &Plan{Items: []PlanItem{}}
	result := landBuildCriteria(plan)
	if result != "" {
		t.Errorf("expected empty string for empty plan, got %q", result)
	}
}

func TestLandParseAnalysis_Pass(t *testing.T) {
	result := &ProviderResult{
		Output: `Analysis complete.
All criteria verified.
<scrip>VERIFY_PASS</scrip>
`,
	}

	passed, failures := landParseAnalysis(result)
	if !passed {
		t.Error("expected passed=true")
	}
	if len(failures) != 0 {
		t.Errorf("expected no failures, got %v", failures)
	}
}

func TestLandParseAnalysis_Fail(t *testing.T) {
	result := &ProviderResult{
		Output: `Analysis found issues.
<scrip>VERIFY_FAIL:missing null check in auth.go:42</scrip>
<scrip>VERIFY_FAIL:no test for error path in handler.go:88</scrip>
`,
	}

	passed, failures := landParseAnalysis(result)
	if passed {
		t.Error("expected passed=false")
	}
	if len(failures) != 2 {
		t.Fatalf("expected 2 failures, got %d: %v", len(failures), failures)
	}
	if !strings.Contains(failures[0], "auth.go:42") {
		t.Errorf("expected first failure to mention auth.go:42, got %q", failures[0])
	}
	if !strings.Contains(failures[1], "handler.go:88") {
		t.Errorf("expected second failure to mention handler.go:88, got %q", failures[1])
	}
}

func TestLandParseAnalysis_FailOverridesPass(t *testing.T) {
	// If both VERIFY_PASS and VERIFY_FAIL appear, failures win
	result := &ProviderResult{
		Output: `<scrip>VERIFY_PASS</scrip>
<scrip>VERIFY_FAIL:edge case not handled</scrip>
`,
	}

	passed, failures := landParseAnalysis(result)
	if passed {
		t.Error("expected passed=false when failures present even with VERIFY_PASS")
	}
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
}

func TestLandParseAnalysis_NilResult(t *testing.T) {
	passed, failures := landParseAnalysis(nil)
	if passed {
		t.Error("expected passed=false for nil result")
	}
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure message, got %d", len(failures))
	}
}

func TestLandParseAnalysis_EmptyOutput(t *testing.T) {
	result := &ProviderResult{Output: ""}
	passed, failures := landParseAnalysis(result)
	if passed {
		t.Error("expected passed=false for empty output")
	}
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure message, got %d", len(failures))
	}
}

func TestLandParseAnalysis_MarkerlessOutput(t *testing.T) {
	// Non-empty output with no VERIFY_PASS/VERIFY_FAIL markers should produce a synthetic failure
	result := &ProviderResult{Output: "The code looks fine overall. I reviewed all the changes."}
	passed, failures := landParseAnalysis(result)
	if passed {
		t.Error("expected passed=false for markerless output")
	}
	if len(failures) != 1 {
		t.Fatalf("expected 1 synthetic failure, got %d: %v", len(failures), failures)
	}
	if !strings.Contains(failures[0], "no VERIFY_PASS/VERIFY_FAIL markers") {
		t.Errorf("expected synthetic failure message about missing markers, got %q", failures[0])
	}
}

func TestLandExtractSummary(t *testing.T) {
	output := `Some preamble text.
<scrip>SUMMARY_START</scrip>
## Implementation Map

- src/auth/login.ts — LoginForm component
- src/api/auth.go — POST /api/auth/login handler

## Gotchas

- Session tokens expire after 24h
<scrip>SUMMARY_END</scrip>
More output after.`

	summary, ok := landExtractSummary(output)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(summary, "Implementation Map") {
		t.Error("expected summary to contain Implementation Map")
	}
	if !strings.Contains(summary, "Session tokens expire") {
		t.Error("expected summary to contain gotcha")
	}
	if strings.Contains(summary, "preamble") {
		t.Error("summary should not contain preamble")
	}
	if strings.Contains(summary, "More output") {
		t.Error("summary should not contain text after END marker")
	}
}

func TestLandExtractSummary_Empty(t *testing.T) {
	output := `<scrip>SUMMARY_START</scrip>
<scrip>SUMMARY_END</scrip>`

	_, ok := landExtractSummary(output)
	if ok {
		t.Error("expected ok=false for empty summary")
	}
}

func TestLandExtractSummary_NoMarkers(t *testing.T) {
	_, ok := landExtractSummary("Just some text without markers")
	if ok {
		t.Error("expected ok=false when no markers present")
	}
}

func TestLandExtractSummary_IndentedMarkers(t *testing.T) {
	output := `  <scrip>SUMMARY_START</scrip>
  Summary with indentation.
  <scrip>SUMMARY_END</scrip>`

	summary, ok := landExtractSummary(output)
	if !ok {
		t.Fatal("expected ok=true for indented markers")
	}
	if !strings.Contains(summary, "Summary with indentation") {
		t.Error("expected summary content")
	}
}

func TestLandRunVerifyCommands_NoCommands(t *testing.T) {
	verify := &ScripVerifyConfig{}
	formatted, allPassed := landRunVerifyCommands("/tmp", verify, 30)
	if !allPassed {
		t.Error("expected allPassed=true when no commands")
	}
	if !strings.Contains(formatted, "No verification commands") {
		t.Errorf("expected 'No verification commands' message, got %q", formatted)
	}
}

func TestLandRunVerifyCommands_PassingCommand(t *testing.T) {
	verify := &ScripVerifyConfig{
		Test: "true",
	}
	formatted, allPassed := landRunVerifyCommands("/tmp", verify, 30)
	if !allPassed {
		t.Error("expected allPassed=true for 'true' command")
	}
	if !strings.Contains(formatted, "PASSED") {
		t.Error("expected PASSED in output")
	}
}

func TestLandRunVerifyCommands_FailingCommand(t *testing.T) {
	verify := &ScripVerifyConfig{
		Test: "false",
	}
	formatted, allPassed := landRunVerifyCommands("/tmp", verify, 30)
	if allPassed {
		t.Error("expected allPassed=false for 'false' command")
	}
	if !strings.Contains(formatted, "FAILED") {
		t.Error("expected FAILED in output")
	}
}

func TestLandBuildNarrative(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "Add login"},
			{Title: "Add logout"},
		},
	}
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "Add login", Status: "passed", Commit: "abc1234567890"},
		{Event: ProgressItemDone, Item: "Add logout", Status: "passed", Commit: "def4567890123"},
	}

	narrative := landBuildNarrative(plan, events, []string{"bcrypt is slow in tests"})

	if !strings.Contains(narrative, "Landing") {
		t.Error("expected Landing header")
	}
	if !strings.Contains(narrative, "Add login") {
		t.Error("expected item listed")
	}
	if !strings.Contains(narrative, "abc1234") {
		t.Error("expected abbreviated commit hash")
	}
	if !strings.Contains(narrative, "bcrypt is slow") {
		t.Error("expected learning in narrative")
	}
}

func TestLandFormatProgressEvents(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressItemDone, Timestamp: "2026-03-12T10:00:00Z", Item: "Add login", Status: "passed", Commit: "abc1234567890"},
		{Event: ProgressItemStuck, Timestamp: "2026-03-12T10:30:00Z", Item: "Add OAuth", Reason: "API key missing"},
		{Event: ProgressLearning, Timestamp: "2026-03-12T11:00:00Z", Text: "bcrypt takes 2s per hash in tests"},
	}

	result := landFormatProgressEvents(events)

	if !strings.Contains(result, "Add login") {
		t.Error("expected item done event")
	}
	if !strings.Contains(result, "passed") {
		t.Error("expected passed status")
	}
	if !strings.Contains(result, "abc1234") {
		t.Error("expected abbreviated commit")
	}
	if !strings.Contains(result, "Add OAuth") {
		t.Error("expected stuck event")
	}
	if !strings.Contains(result, "API key missing") {
		t.Error("expected stuck reason")
	}
	if !strings.Contains(result, "bcrypt") {
		t.Error("expected learning event")
	}
}

func TestLandFormatProgressEvents_Empty(t *testing.T) {
	result := landFormatProgressEvents(nil)
	if result != "No execution history." {
		t.Errorf("expected 'No execution history.' for nil events, got %q", result)
	}
}

func TestLandFormatProgressEvents_NoRelevantEvents(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressExecStart, Feature: "auth"},
		{Event: ProgressExecEnd, Passed: 2},
	}

	result := landFormatProgressEvents(events)
	if result != "No significant execution events." {
		t.Errorf("expected 'No significant execution events.' for non-relevant events, got %q", result)
	}
}

func TestGenerateLandAnalyzePrompt(t *testing.T) {
	prompt := generateLandAnalyzePrompt(
		"### Item 1: Login\n- validates email\n",
		"diff --git a/auth.go b/auth.go\n+func Login(){}",
		"### go test — PASSED\n",
		"Check docs.",
	)

	if !strings.Contains(prompt, "validates email") {
		t.Error("expected criteria in prompt")
	}
	if !strings.Contains(prompt, "func Login") {
		t.Error("expected diff in prompt")
	}
	if !strings.Contains(prompt, "go test") {
		t.Error("expected verify results in prompt")
	}
	if !strings.Contains(prompt, "VERIFY_PASS") {
		t.Error("expected VERIFY_PASS marker instruction in prompt")
	}
	if !strings.Contains(prompt, "VERIFY_FAIL") {
		t.Error("expected VERIFY_FAIL marker instruction in prompt")
	}
}

func TestGenerateLandFixPrompt(t *testing.T) {
	prompt := generateLandFixPrompt(
		[]string{"missing null check in auth.go:42", "no error test in handler.go:88"},
		"### go test — FAILED\n```\nFAIL auth_test.go\n```\n",
		"diff content",
		"Check docs.",
	)

	if !strings.Contains(prompt, "auth.go:42") {
		t.Error("expected finding in prompt")
	}
	if !strings.Contains(prompt, "handler.go:88") {
		t.Error("expected second finding in prompt")
	}
	if !strings.Contains(prompt, "FAIL auth_test.go") {
		t.Error("expected verify results in prompt")
	}
	if !strings.Contains(prompt, "DONE") {
		t.Error("expected DONE marker instruction in prompt")
	}
}

func TestGenerateLandSummaryPrompt(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressItemDone, Timestamp: "2026-03-12T10:00:00Z", Item: "Login", Status: "passed"},
	}

	prompt := generateLandSummaryPrompt("auth", events, "diff content", []string{"bcrypt is slow"})

	if !strings.Contains(prompt, "auth") {
		t.Error("expected feature name in prompt")
	}
	if !strings.Contains(prompt, "Login") {
		t.Error("expected progress events in prompt")
	}
	if !strings.Contains(prompt, "bcrypt") {
		t.Error("expected learnings in prompt")
	}
	if !strings.Contains(prompt, "SUMMARY_START") {
		t.Error("expected SUMMARY_START marker instruction in prompt")
	}
	if !strings.Contains(prompt, "SUMMARY_END") {
		t.Error("expected SUMMARY_END marker instruction in prompt")
	}
}

func TestGitOps_GetFullDiff(t *testing.T) {
	dir, git := initTestRepo(t)

	// Create a file and commit on a branch
	if err := git.CreateBranch("feature-test"); err != nil {
		t.Fatalf("create branch: %v", err)
	}

	testFile := filepath.Join(dir, "feature.txt")
	os.WriteFile(testFile, []byte("new feature code"), 0644)
	if err := git.CommitFile(testFile, "add feature"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	diff := git.GetFullDiff()
	if diff == "" {
		t.Error("expected non-empty diff from default branch")
	}
	if !strings.Contains(diff, "feature.txt") {
		t.Error("expected diff to mention feature.txt")
	}
	if !strings.Contains(diff, "new feature code") {
		t.Error("expected diff to contain file content")
	}
}

func TestGitOps_GetFullDiff_NoDiff(t *testing.T) {
	_, git := initTestRepo(t)

	// On main branch with no changes — diff should be empty
	diff := git.GetFullDiff()
	if diff != "" {
		t.Errorf("expected empty diff on default branch, got %q", diff)
	}
}
