package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePlan(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		feature   string
		created   string
		itemCount int
		items     []PlanItem
	}{
		{
			name: "full plan with frontmatter",
			content: `---
feature: auth-system
created: 2026-03-11T14:32:00Z
item_count: 3
---

# Auth System

## Items

1. **Set up OAuth2 dependencies**
   - Acceptance: OAuth2 client instantiates, no hardcoded secrets

2. **Google login flow**
   - Acceptance: End-to-end Google auth works, session persists

3. **Session management**
   - Acceptance: Sessions expire after 24h, refresh token works
   - Depends on: item 2
`,
			feature:   "auth-system",
			created:   "2026-03-11T14:32:00Z",
			itemCount: 3,
			items: []PlanItem{
				{Title: "Set up OAuth2 dependencies", Acceptance: []string{"OAuth2 client instantiates, no hardcoded secrets"}},
				{Title: "Google login flow", Acceptance: []string{"End-to-end Google auth works, session persists"}},
				{Title: "Session management", Acceptance: []string{"Sessions expire after 24h, refresh token works"}, DependsOn: []string{"item 2"}},
			},
		},
		{
			name: "no frontmatter degrades gracefully",
			content: `# My Feature

## Items

1. **First item**
   - Acceptance: it works
`,
			feature:   "",
			created:   "",
			itemCount: 1,
			items: []PlanItem{
				{Title: "First item", Acceptance: []string{"it works"}},
			},
		},
		{
			name:      "empty content",
			content:   "",
			feature:   "",
			created:   "",
			itemCount: 0,
			items:     nil,
		},
		{
			name: "malformed frontmatter treated as body",
			content: `---
this is not yaml
1. **An item**
   - Acceptance: criterion
`,
			feature:   "",
			created:   "",
			itemCount: 1,
			items: []PlanItem{
				{Title: "An item", Acceptance: []string{"criterion"}},
			},
		},
		{
			name: "multiple acceptance criteria per item",
			content: `---
feature: multi-criteria
created: 2026-03-12T00:00:00Z
item_count: 1
---

1. **Complex item**
   - Acceptance: first criterion
   - Acceptance: second criterion
   - Acceptance: third criterion
`,
			feature:   "multi-criteria",
			created:   "2026-03-12T00:00:00Z",
			itemCount: 1,
			items: []PlanItem{
				{Title: "Complex item", Acceptance: []string{"first criterion", "second criterion", "third criterion"}},
			},
		},
		{
			name: "multiple depends on",
			content: `---
feature: deps
created: 2026-03-12T00:00:00Z
item_count: 1
---

1. **Depends on many**
   - Acceptance: it works
   - Depends on: item 1
   - Depends on: item 2
`,
			feature:   "deps",
			created:   "2026-03-12T00:00:00Z",
			itemCount: 1,
			items: []PlanItem{
				{Title: "Depends on many", Acceptance: []string{"it works"}, DependsOn: []string{"item 1", "item 2"}},
			},
		},
		{
			name: "item_count in frontmatter overrides actual",
			content: `---
feature: mismatch
created: 2026-03-12T00:00:00Z
item_count: 5
---

1. **Only one item**
   - Acceptance: exists
`,
			feature:   "mismatch",
			created:   "2026-03-12T00:00:00Z",
			itemCount: 5,
			items: []PlanItem{
				{Title: "Only one item", Acceptance: []string{"exists"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := ParsePlan(tt.content)
			if err != nil {
				t.Fatalf("ParsePlan error: %v", err)
			}
			if plan.Feature != tt.feature {
				t.Errorf("Feature = %q, want %q", plan.Feature, tt.feature)
			}
			if plan.Created != tt.created {
				t.Errorf("Created = %q, want %q", plan.Created, tt.created)
			}
			if plan.ItemCount != tt.itemCount {
				t.Errorf("ItemCount = %d, want %d", plan.ItemCount, tt.itemCount)
			}
			if len(plan.Items) != len(tt.items) {
				t.Fatalf("len(Items) = %d, want %d", len(plan.Items), len(tt.items))
			}
			for i, want := range tt.items {
				got := plan.Items[i]
				if got.Title != want.Title {
					t.Errorf("Items[%d].Title = %q, want %q", i, got.Title, want.Title)
				}
				if len(got.Acceptance) != len(want.Acceptance) {
					t.Errorf("Items[%d].Acceptance len = %d, want %d", i, len(got.Acceptance), len(want.Acceptance))
				} else {
					for j := range want.Acceptance {
						if got.Acceptance[j] != want.Acceptance[j] {
							t.Errorf("Items[%d].Acceptance[%d] = %q, want %q", i, j, got.Acceptance[j], want.Acceptance[j])
						}
					}
				}
				if len(got.DependsOn) != len(want.DependsOn) {
					t.Errorf("Items[%d].DependsOn len = %d, want %d", i, len(got.DependsOn), len(want.DependsOn))
				} else {
					for j := range want.DependsOn {
						if got.DependsOn[j] != want.DependsOn[j] {
							t.Errorf("Items[%d].DependsOn[%d] = %q, want %q", i, j, got.DependsOn[j], want.DependsOn[j])
						}
					}
				}
			}
		})
	}
}

func TestWritePlanMd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.md")

	plan := &Plan{
		Feature: "auth-system",
		Created: "2026-03-11T14:32:00Z",
		Items: []PlanItem{
			{Title: "Set up OAuth2", Acceptance: []string{"OAuth2 client works"}},
			{Title: "Google login", Acceptance: []string{"Login works"}, DependsOn: []string{"item 1"}},
		},
	}

	if err := WritePlanMd(path, plan); err != nil {
		t.Fatalf("WritePlanMd error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	content := string(data)

	// Verify frontmatter
	if !strings.Contains(content, "feature: auth-system") {
		t.Error("missing feature in frontmatter")
	}
	if !strings.Contains(content, "item_count: 2") {
		t.Error("missing item_count in frontmatter")
	}

	// Verify items
	if !strings.Contains(content, "1. **Set up OAuth2**") {
		t.Error("missing first item")
	}
	if !strings.Contains(content, "2. **Google login**") {
		t.Error("missing second item")
	}
	if !strings.Contains(content, "- Acceptance: OAuth2 client works") {
		t.Error("missing acceptance criterion")
	}
	if !strings.Contains(content, "- Depends on: item 1") {
		t.Error("missing depends on")
	}

	// Verify roundtrip: parse what we wrote
	parsed, err := ParsePlan(content)
	if err != nil {
		t.Fatalf("ParsePlan roundtrip error: %v", err)
	}
	if parsed.Feature != plan.Feature {
		t.Errorf("roundtrip Feature = %q, want %q", parsed.Feature, plan.Feature)
	}
	if len(parsed.Items) != len(plan.Items) {
		t.Fatalf("roundtrip Items len = %d, want %d", len(parsed.Items), len(plan.Items))
	}
	for i, want := range plan.Items {
		got := parsed.Items[i]
		if got.Title != want.Title {
			t.Errorf("roundtrip Items[%d].Title = %q, want %q", i, got.Title, want.Title)
		}
	}
}

func TestPlanJsonlAppendAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.jsonl")

	rounds := []PlanRound{
		{Round: 1, Timestamp: "2026-03-11T14:30:00Z", UserInput: "add oauth", AIResponse: "Here are options..."},
		{Round: 2, Timestamp: "2026-03-11T14:35:00Z", UserInput: "go with option A", AIResponse: "Updated plan...", HasPlanDraft: true},
		{
			Round: 3, Timestamp: "2026-03-11T14:40:00Z", UserInput: "write the plan",
			AIResponse: "Plan written", Finalized: true,
			Verification: &PlanVerification{Items: 5, Warnings: []string{"missing CSRF"}},
		},
	}

	for _, r := range rounds {
		r := r
		if err := AppendPlanRound(path, &r); err != nil {
			t.Fatalf("AppendPlanRound round %d: %v", r.Round, err)
		}
	}

	loaded, err := LoadPlanRounds(path)
	if err != nil {
		t.Fatalf("LoadPlanRounds error: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("loaded %d rounds, want 3", len(loaded))
	}

	// Verify round 1
	if loaded[0].Round != 1 || loaded[0].UserInput != "add oauth" {
		t.Errorf("round 1 mismatch: %+v", loaded[0])
	}

	// Verify round 2 has plan draft
	if !loaded[1].HasPlanDraft {
		t.Error("round 2 should have HasPlanDraft=true")
	}

	// Verify round 3 has finalized + verification
	if !loaded[2].Finalized {
		t.Error("round 3 should be finalized")
	}
	if loaded[2].Verification == nil {
		t.Fatal("round 3 should have verification")
	}
	if loaded[2].Verification.Items != 5 {
		t.Errorf("verification items = %d, want 5", loaded[2].Verification.Items)
	}
	if len(loaded[2].Verification.Warnings) != 1 || loaded[2].Verification.Warnings[0] != "missing CSRF" {
		t.Errorf("verification warnings = %v, want [missing CSRF]", loaded[2].Verification.Warnings)
	}
}

func TestLoadPlanRoundsNonexistent(t *testing.T) {
	rounds, err := LoadPlanRounds("/nonexistent/plan.jsonl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rounds != nil {
		t.Errorf("expected nil for nonexistent file, got %v", rounds)
	}
}

func TestLoadPlanRoundsCorruptLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.jsonl")

	content := `{"round":1,"ts":"2026-03-11T00:00:00Z","user_input":"hello","ai_response":"hi"}
not valid json
{"round":2,"ts":"2026-03-11T00:01:00Z","user_input":"bye","ai_response":"goodbye"}
`
	os.WriteFile(path, []byte(content), 0644)

	rounds, err := LoadPlanRounds(path)
	if err != nil {
		t.Fatalf("LoadPlanRounds error: %v", err)
	}
	if len(rounds) != 2 {
		t.Fatalf("expected 2 valid rounds, got %d", len(rounds))
	}
	if rounds[0].Round != 1 {
		t.Errorf("first round = %d, want 1", rounds[0].Round)
	}
	if rounds[1].Round != 2 {
		t.Errorf("second round = %d, want 2", rounds[1].Round)
	}
}

func TestBuildPlanHistoryEmpty(t *testing.T) {
	result := BuildPlanHistory(nil)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestBuildPlanHistoryFewRounds(t *testing.T) {
	// 3 rounds: all should be verbatim (recent)
	rounds := makeTestRounds(3)
	result := BuildPlanHistory(rounds)

	if !strings.Contains(result, "### Round 1") {
		t.Error("missing round 1 (should be verbatim)")
	}
	if !strings.Contains(result, "### Round 3") {
		t.Error("missing round 3")
	}
	// Should contain full consultation
	if !strings.Contains(result, "**Consultation:**") {
		t.Error("missing consultation in verbatim round")
	}
	// Should NOT contain digest format
	if strings.Contains(result, "Early rounds") {
		t.Error("should not have old digests for 3 rounds")
	}
}

func TestBuildPlanHistoryMiddleCompression(t *testing.T) {
	// 8 rounds: rounds 1-3 are middle (decision-only), rounds 4-8 are recent
	rounds := makeTestRounds(8)
	result := BuildPlanHistory(rounds)

	// No old digests (N <= 10)
	if strings.Contains(result, "Early rounds") {
		t.Error("should not have old digests for 8 rounds")
	}

	// Middle rounds should NOT have consultation
	// Round 1 is middle: should have "### Round 1" but no "**Consultation:**" before "### Round 4"
	r1Idx := strings.Index(result, "### Round 1")
	r4Idx := strings.Index(result, "### Round 4")
	if r1Idx < 0 || r4Idx < 0 {
		t.Fatal("missing expected round headers")
	}
	middleSection := result[r1Idx:r4Idx]
	if strings.Contains(middleSection, "**Consultation:**") {
		t.Error("middle rounds should not contain consultation")
	}

	// Recent rounds (4-8) should have consultation
	recentSection := result[r4Idx:]
	if !strings.Contains(recentSection, "**Consultation:**") {
		t.Error("recent rounds should contain consultation")
	}
}

func TestBuildPlanHistoryFullCompression(t *testing.T) {
	// 15 rounds: 1-5 old (digest), 6-10 middle (decision), 11-15 recent (verbatim)
	rounds := makeTestRounds(15)
	result := BuildPlanHistory(rounds)

	// Should have old digests
	if !strings.Contains(result, "Early rounds") {
		t.Error("should have old digest section")
	}
	// Old rounds use digest format: "Round N: input → response..."
	if !strings.Contains(result, "Round 1: input-1 →") {
		t.Error("missing digest for round 1")
	}
	if !strings.Contains(result, "Round 5: input-5 →") {
		t.Error("missing digest for round 5")
	}

	// Middle section should exist
	if !strings.Contains(result, "### Round 6") {
		t.Error("missing middle round 6")
	}
	if !strings.Contains(result, "### Round 10") {
		t.Error("missing middle round 10")
	}

	// Recent section should have verbatim rounds 11-15
	if !strings.Contains(result, "### Round 11") {
		t.Error("missing recent round 11")
	}
	if !strings.Contains(result, "### Round 15") {
		t.Error("missing recent round 15")
	}
}

func TestBuildPlanHistoryBudgetEnforcement(t *testing.T) {
	// Create rounds with very large AI responses to exceed 32KB budget
	rounds := make([]PlanRound, 20)
	largeResponse := strings.Repeat("x", 5000)
	for i := range rounds {
		rounds[i] = PlanRound{
			Round:        i + 1,
			Timestamp:    "2026-03-12T00:00:00Z",
			UserInput:    "input",
			Consultation: []string{"consult"},
			AIResponse:   largeResponse,
		}
	}

	result := BuildPlanHistory(rounds)

	if len(result) > planHistoryMaxBytes {
		t.Errorf("result exceeds budget: %d > %d", len(result), planHistoryMaxBytes)
	}

	// Recent 5 rounds should always be preserved
	if !strings.Contains(result, "### Round 20") {
		t.Error("most recent round should be preserved even under budget pressure")
	}
}

func TestContextReconstruction(t *testing.T) {
	// Full roundtrip: write rounds → load → build history → verify structure
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.jsonl")

	for i := 1; i <= 12; i++ {
		r := &PlanRound{
			Round:        i,
			Timestamp:    "2026-03-12T00:00:00Z",
			UserInput:    "input for round",
			Consultation: []string{"consult result"},
			AIResponse:   "response for round",
		}
		if i == 12 {
			r.Finalized = true
			r.Verification = &PlanVerification{Items: 3}
		}
		if err := AppendPlanRound(path, r); err != nil {
			t.Fatalf("AppendPlanRound %d: %v", i, err)
		}
	}

	rounds, err := LoadPlanRounds(path)
	if err != nil {
		t.Fatalf("LoadPlanRounds: %v", err)
	}
	if len(rounds) != 12 {
		t.Fatalf("loaded %d rounds, want 12", len(rounds))
	}

	history := BuildPlanHistory(rounds)

	// N=12, N>10: old=1-5 (digest), middle=6-7 (decision), recent=8-12 (verbatim)
	if !strings.Contains(history, "Early rounds") {
		t.Error("should contain old digest section")
	}

	// Check the 3 tiers are present with separators
	parts := strings.Split(history, "\n\n---\n\n")
	if len(parts) != 3 {
		t.Errorf("expected 3 sections separated by ---, got %d", len(parts))
	}

	// Finalized round (12) should show in recent
	if !strings.Contains(history, "*(plan finalized)*") {
		t.Error("finalized marker missing from recent rounds")
	}
	if !strings.Contains(history, "**Verification:** 3 items") {
		t.Error("verification info missing from recent rounds")
	}
}

func TestPlanRoundJSON(t *testing.T) {
	// Verify JSON serialization matches spec field names
	r := PlanRound{
		Round:        1,
		Timestamp:    "2026-03-11T14:30:00Z",
		UserInput:    "add oauth",
		Consultation: []string{"use ueberauth"},
		AIResponse:   "Here are options",
		HasPlanDraft: true,
		Finalized:    true,
		Verification: &PlanVerification{Items: 3, Warnings: []string{"no CSRF"}},
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Check field names match spec
	s := string(data)
	for _, field := range []string{`"round"`, `"ts"`, `"user_input"`, `"consultation"`, `"ai_response"`, `"has_plan_draft"`, `"finalized"`, `"verification"`} {
		if !strings.Contains(s, field) {
			t.Errorf("missing JSON field %s in %s", field, s)
		}
	}

	// Roundtrip
	var decoded PlanRound
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Round != r.Round || decoded.UserInput != r.UserInput || decoded.Finalized != r.Finalized {
		t.Error("roundtrip mismatch")
	}
}

func TestPlanRoundOmitempty(t *testing.T) {
	// Optional fields should be omitted when zero
	r := PlanRound{
		Round:      1,
		Timestamp:  "2026-03-11T00:00:00Z",
		UserInput:  "hello",
		AIResponse: "hi",
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	s := string(data)
	for _, field := range []string{`"consultation"`, `"has_plan_draft"`, `"finalized"`, `"verification"`} {
		if strings.Contains(s, field) {
			t.Errorf("zero-value field %s should be omitted, got %s", field, s)
		}
	}
}

func TestWritePlanMdTitleCase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.md")

	plan := &Plan{
		Feature: "my-cool-feature",
		Created: "2026-03-12T00:00:00Z",
		Items:   []PlanItem{{Title: "Item one", Acceptance: []string{"works"}}},
	}

	if err := WritePlanMd(path, plan); err != nil {
		t.Fatalf("WritePlanMd: %v", err)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "# My Cool Feature") {
		t.Errorf("expected title-cased heading, got:\n%s", string(data))
	}
}

func TestAppendPlanRoundCreatesDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "plan.jsonl")

	r := &PlanRound{Round: 1, Timestamp: "2026-03-12T00:00:00Z", UserInput: "test", AIResponse: "ok"}
	if err := AppendPlanRound(path, r); err != nil {
		t.Fatalf("AppendPlanRound: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

// makeTestRounds creates n test rounds with predictable content.
func makeTestRounds(n int) []PlanRound {
	rounds := make([]PlanRound, n)
	for i := range rounds {
		num := fmt.Sprintf("%d", i+1)
		rounds[i] = PlanRound{
			Round:        i + 1,
			Timestamp:    "2026-03-12T00:00:00Z",
			UserInput:    "input-" + num,
			Consultation: []string{"consult-" + num},
			AIResponse:   "response-" + num,
		}
	}
	return rounds
}
