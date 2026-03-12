package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGeneratePlanCreatePrompt(t *testing.T) {
	fd := &FeatureDir{Feature: "auth", Path: "/tmp/test/.scrip/2026-03-12-auth"}
	prompt := generatePlanCreatePrompt(fd, "add oauth login", "codebase info", "framework guidance", "progress info")

	for _, want := range []string{"auth", "add oauth login", "codebase info", "framework guidance", "progress info"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestGeneratePlanRoundPrompt(t *testing.T) {
	fd := &FeatureDir{Feature: "auth", Path: "/tmp/test/.scrip/2026-03-12-auth"}
	prompt := generatePlanRoundPrompt(fd, "split item 3", "codebase", "consult", "history", "progress")

	for _, want := range []string{"auth", "split item 3", "codebase", "consult", "history", "progress"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestGeneratePlanVerifyPrompt(t *testing.T) {
	prompt := generatePlanVerifyPrompt("plan content here", "codebase ctx")

	if !strings.Contains(prompt, "plan content here") {
		t.Error("prompt missing plan content")
	}
	if !strings.Contains(prompt, "codebase ctx") {
		t.Error("prompt missing codebase context")
	}
	if !strings.Contains(prompt, "VERIFY_PASS") {
		t.Error("prompt should reference VERIFY_PASS marker")
	}
}

func TestContainsPlanDraft(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name: "valid plan draft",
			input: `Here's the plan:

---
feature: auth
created: 2026-03-12T14:00:00Z
item_count: 2
---

# Auth System

## Items

1. **Add OAuth2 flow**
   - Acceptance: Login returns JWT`,
			want: true,
		},
		{
			name:  "no plan content",
			input: "Let me think about this more...",
			want:  false,
		},
		{
			name:  "missing items section",
			input: "---\nfeature: auth\nitem_count: 2\n---\n# Auth\nSome text",
			want:  false,
		},
		{
			name:  "missing item_count",
			input: "feature: auth\n## Items\n1. foo",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsPlanDraft(tt.input); got != tt.want {
				t.Errorf("containsPlanDraft() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractPlanContent(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantPrefix string
	}{
		{
			name: "extracts from mixed response",
			input: `Based on my analysis, here's the plan:

---
feature: auth
item_count: 2
---

# Auth System`,
			wantPrefix: "---\nfeature: auth",
		},
		{
			name:       "returns whole response when no frontmatter",
			input:      "just some text without a plan",
			wantPrefix: "just some text",
		},
		{
			name: "skips non-plan frontmatter",
			input: `---
title: not a plan
---

Some markdown content.

---
feature: auth
item_count: 3
---

# Auth`,
			wantPrefix: "---\nfeature: auth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPlanContent(tt.input)
			if !strings.HasPrefix(got, tt.wantPrefix) {
				preview := got
				if len(preview) > 60 {
					preview = preview[:60] + "..."
				}
				t.Errorf("extractPlanContent() = %q, want prefix %q", preview, tt.wantPrefix)
			}
		})
	}
}

func TestBuildLandFailureContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.jsonl")

	// No file — empty context
	ctx := buildLandFailureContext(path)
	if ctx != "" {
		t.Error("expected empty context for missing file")
	}

	// Add non-failure event — still empty
	_ = AppendProgressEvent(path, &ProgressEvent{
		Event:   ProgressExecStart,
		Feature: "auth",
	})
	ctx = buildLandFailureContext(path)
	if ctx != "" {
		t.Error("expected empty context when no land_failed event")
	}

	// Add land_failed event
	_ = AppendProgressEvent(path, &ProgressEvent{
		Event:    ProgressLandFailed,
		Findings: []string{"test: 2 failures", "security: missing CSRF"},
		Analysis: "Login endpoint lacks CSRF protection",
	})

	ctx = buildLandFailureContext(path)
	if !strings.Contains(ctx, "Land Failure") {
		t.Error("expected land failure heading")
	}
	if !strings.Contains(ctx, "test: 2 failures") {
		t.Error("expected first finding")
	}
	if !strings.Contains(ctx, "security: missing CSRF") {
		t.Error("expected second finding")
	}
	if !strings.Contains(ctx, "CSRF protection") {
		t.Error("expected analysis text")
	}
}

func TestParsePlanVerifyOutput(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		wantWarnings int
		wantFirst    string
	}{
		{
			name:         "pass with no failures",
			output:       "Analysis complete.\n<scrip>VERIFY_PASS</scrip>\n",
			wantWarnings: 0,
		},
		{
			name: "two failures",
			output: `Item 1 has issues.
<scrip>VERIFY_FAIL:Missing database migration for users table</scrip>
Item 3 too large.
<scrip>VERIFY_FAIL:Item 3 spans UI + backend, should be split</scrip>`,
			wantWarnings: 2,
			wantFirst:    "Missing database migration for users table",
		},
		{
			name:         "no markers at all",
			output:       "Some analysis without any markers",
			wantWarnings: 0,
		},
		{
			name:         "pass and fail together",
			output:       "<scrip>VERIFY_FAIL:vague criterion in item 2</scrip>\n<scrip>VERIFY_PASS</scrip>\n",
			wantWarnings: 1,
			wantFirst:    "vague criterion in item 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := parsePlanVerifyOutput(tt.output)
			if len(v.Warnings) != tt.wantWarnings {
				t.Errorf("got %d warnings, want %d: %v", len(v.Warnings), tt.wantWarnings, v.Warnings)
			}
			if tt.wantFirst != "" && len(v.Warnings) > 0 && v.Warnings[0] != tt.wantFirst {
				t.Errorf("first warning = %q, want %q", v.Warnings[0], tt.wantFirst)
			}
		})
	}
}

func TestPlanResumeContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.jsonl")

	// Append several rounds simulating a planning session
	for i := 1; i <= 3; i++ {
		round := &PlanRound{
			Round:     i,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			UserInput: fmt.Sprintf("feedback round %d", i),
			AIResponse: fmt.Sprintf("response for round %d with some analysis", i),
		}
		if err := AppendPlanRound(path, round); err != nil {
			t.Fatal(err)
		}
	}

	// Load and verify all rounds preserved
	rounds, err := LoadPlanRounds(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rounds) != 3 {
		t.Fatalf("got %d rounds, want 3", len(rounds))
	}

	// Build history — all 3 should be in verbatim range (< 5 recent threshold)
	history := BuildPlanHistory(rounds)
	if history == "" {
		t.Fatal("expected non-empty plan history")
	}
	for i := 1; i <= 3; i++ {
		if !strings.Contains(history, fmt.Sprintf("feedback round %d", i)) {
			t.Errorf("history missing round %d user input", i)
		}
		if !strings.Contains(history, fmt.Sprintf("response for round %d", i)) {
			t.Errorf("history missing round %d response", i)
		}
	}
}

func TestFinalizePlanExtraction(t *testing.T) {
	planDraft := `---
feature: auth
created: 2026-03-12T14:00:00Z
item_count: 2
---

# Auth System

## Items

1. **Add user model**
   - Acceptance: User model with email and password_hash fields created
   - Acceptance: Migration runs successfully

2. **Add login endpoint**
   - Acceptance: POST /login returns 200 with JWT on valid credentials
   - Acceptance: Returns 401 on invalid credentials
   - Depends on: item 1`

	rounds := []PlanRound{
		{Round: 1, AIResponse: "Let me think about auth...", HasPlanDraft: false},
		{Round: 2, AIResponse: "Here's my revised plan:\n\n" + planDraft, HasPlanDraft: true},
	}

	// Find last draft
	var planContent string
	for i := len(rounds) - 1; i >= 0; i-- {
		if rounds[i].HasPlanDraft {
			planContent = extractPlanContent(rounds[i].AIResponse)
			break
		}
	}
	if planContent == "" {
		t.Fatal("no plan draft found")
	}

	// Parse and validate
	plan, err := ParsePlan(planContent)
	if err != nil {
		t.Fatalf("ParsePlan: %v", err)
	}
	if plan.Feature != "auth" {
		t.Errorf("feature = %q, want %q", plan.Feature, "auth")
	}
	if len(plan.Items) != 2 {
		t.Errorf("got %d items, want 2", len(plan.Items))
	}
	if plan.Items[0].Title != "Add user model" {
		t.Errorf("item 1 title = %q", plan.Items[0].Title)
	}
	if len(plan.Items[1].DependsOn) == 0 {
		t.Error("item 2 should have dependency on item 1")
	}

	// Write and re-parse plan.md
	dir := t.TempDir()
	planMdPath := filepath.Join(dir, "plan.md")
	if err := WritePlanMd(planMdPath, plan); err != nil {
		t.Fatalf("WritePlanMd: %v", err)
	}

	data, err := os.ReadFile(planMdPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	reparsed, err := ParsePlan(string(data))
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(reparsed.Items) != 2 {
		t.Errorf("re-parsed items = %d, want 2", len(reparsed.Items))
	}
}

func TestFinalizePlanProgressEvent(t *testing.T) {
	dir := t.TempDir()
	progressPath := filepath.Join(dir, "progress.jsonl")

	_ = AppendProgressEvent(progressPath, &ProgressEvent{
		Event:     ProgressPlanCreated,
		ItemCount: 3,
		Context:   "plan for auth",
	})

	events, err := LoadProgressEvents(progressPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].Event != ProgressPlanCreated {
		t.Errorf("event = %v, want %v", events[0].Event, ProgressPlanCreated)
	}
	if events[0].ItemCount != 3 {
		t.Errorf("item_count = %d, want 3", events[0].ItemCount)
	}
	if events[0].Context != "plan for auth" {
		t.Errorf("context = %q, want %q", events[0].Context, "plan for auth")
	}
}

func TestPlanPathHelpers(t *testing.T) {
	fd := &FeatureDir{Path: "/project/.scrip/2026-03-12-auth"}

	tests := []struct {
		name string
		fn   func(*FeatureDir) string
		want string
	}{
		{"planMd", planMdPathFor, "/project/.scrip/2026-03-12-auth/plan.md"},
		{"planJsonl", planJsonlPathFor, "/project/.scrip/2026-03-12-auth/plan.jsonl"},
		{"progressJsonl", progressJsonlPathFor, "/project/.scrip/2026-03-12-auth/progress.jsonl"},
		{"progressMd", progressMdPathFor, "/project/.scrip/2026-03-12-auth/progress.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fn(fd); got != tt.want {
				t.Errorf("%s() = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}
