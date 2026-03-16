package main

import (
	"testing"
)

func TestNormalizeLearning(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello world"},
		{"trailing period.", "trailing period"},
		{"  spaces  ", "spaces"},
		{"Multiple!!!!", "multiple"},
		{"keep:colons", "keep:colons"},
	}
	for _, tt := range tests {
		got := normalizeLearning(tt.input)
		if got != tt.want {
			t.Errorf("normalizeLearning(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- ItemState tests ---

func TestComputeItemState_Empty(t *testing.T) {
	s := ComputeItemState("Setup OAuth2", nil)
	if s.Passed || s.Skipped || s.Attempted {
		t.Error("expected all false for empty events")
	}
	if s.Attempts != 0 {
		t.Errorf("expected 0 attempts, got %d", s.Attempts)
	}
}

func TestComputeItemState_Passed(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressItemStart, Item: "Setup OAuth2", Attempt: 1},
		{Event: ProgressItemDone, Item: "Setup OAuth2", Status: "passed", Commit: "abc123", Learnings: []string{"callback URL matters"}},
	}
	s := ComputeItemState("Setup OAuth2", events)
	if !s.Passed {
		t.Error("expected passed=true")
	}
	if s.Skipped {
		t.Error("expected skipped=false")
	}
	if !s.Attempted {
		t.Error("expected attempted=true")
	}
	if s.Attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", s.Attempts)
	}
	if s.LastCommit != "abc123" {
		t.Errorf("expected commit abc123, got %s", s.LastCommit)
	}
	if len(s.Learnings) != 1 || s.Learnings[0] != "callback URL matters" {
		t.Errorf("unexpected learnings: %v", s.Learnings)
	}
}

func TestComputeItemState_Skipped(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressItemStart, Item: "Hard task", Attempt: 1},
		{Event: ProgressItemStuck, Item: "Hard task", Attempt: 1, Reason: "too complex"},
		{Event: ProgressItemStart, Item: "Hard task", Attempt: 2},
		{Event: ProgressItemStuck, Item: "Hard task", Attempt: 2, Reason: "still too complex"},
		{Event: ProgressItemDone, Item: "Hard task", Status: "skipped"},
	}
	s := ComputeItemState("Hard task", events)
	if s.Passed {
		t.Error("expected passed=false")
	}
	if !s.Skipped {
		t.Error("expected skipped=true")
	}
	if s.Attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", s.Attempts)
	}
	if s.LastFailure != "still too complex" {
		t.Errorf("expected last failure reason, got %q", s.LastFailure)
	}
}

func TestComputeItemState_Regression(t *testing.T) {
	// Item passes, then fails during verify-at-top (stuck), then passes again
	events := []ProgressEvent{
		{Event: ProgressItemStart, Item: "Login flow", Attempt: 1},
		{Event: ProgressItemDone, Item: "Login flow", Status: "passed", Commit: "abc"},
		// verify-at-top detects regression
		{Event: ProgressItemStuck, Item: "Login flow", Reason: "regression: tests failed"},
		{Event: ProgressItemStart, Item: "Login flow", Attempt: 2},
		{Event: ProgressItemDone, Item: "Login flow", Status: "passed", Commit: "def"},
	}
	s := ComputeItemState("Login flow", events)
	if !s.Passed {
		t.Error("expected passed=true after retry")
	}
	if s.LastCommit != "def" {
		t.Errorf("expected commit def, got %s", s.LastCommit)
	}
	if s.Attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", s.Attempts)
	}
}

func TestComputeItemState_IgnoresOtherItems(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressItemStart, Item: "Item A", Attempt: 1},
		{Event: ProgressItemDone, Item: "Item A", Status: "passed"},
		{Event: ProgressItemStart, Item: "Item B", Attempt: 1},
		{Event: ProgressItemStuck, Item: "Item B", Reason: "stuck"},
	}
	sA := ComputeItemState("Item A", events)
	if !sA.Passed {
		t.Error("Item A should be passed")
	}
	sB := ComputeItemState("Item B", events)
	if sB.Passed {
		t.Error("Item B should not be passed")
	}
	if sB.LastFailure != "stuck" {
		t.Errorf("Item B expected failure reason, got %q", sB.LastFailure)
	}
}

func TestComputeItemState_StuckClearsPassed(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressItemStart, Item: "X", Attempt: 1},
		{Event: ProgressItemDone, Item: "X", Status: "passed", Commit: "abc"},
		{Event: ProgressItemStuck, Item: "X", Reason: "regression"},
	}
	s := ComputeItemState("X", events)
	if s.Passed {
		t.Error("expected passed=false after stuck event (regression)")
	}
}

func TestGetNextItem_Basic(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "First"},
			{Title: "Second"},
			{Title: "Third"},
		},
	}
	next := GetNextItem(plan, nil)
	if next == nil || next.Title != "First" {
		t.Errorf("expected First, got %v", next)
	}
}

func TestGetNextItem_SkipsPassed(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "First"},
			{Title: "Second"},
		},
	}
	events := []ProgressEvent{
		{Event: ProgressItemStart, Item: "First", Attempt: 1},
		{Event: ProgressItemDone, Item: "First", Status: "passed"},
	}
	next := GetNextItem(plan, events)
	if next == nil || next.Title != "Second" {
		t.Errorf("expected Second, got %v", next)
	}
}

func TestGetNextItem_SkipsSkipped(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "First"},
			{Title: "Second"},
			{Title: "Third"},
		},
	}
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "First", Status: "skipped"},
	}
	next := GetNextItem(plan, events)
	if next == nil || next.Title != "Second" {
		t.Errorf("expected Second, got %v", next)
	}
}

func TestGetNextItem_AllComplete(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "Only"},
		},
	}
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "Only", Status: "passed"},
	}
	next := GetNextItem(plan, events)
	if next != nil {
		t.Errorf("expected nil when all complete, got %v", next)
	}
}

func TestGetNextItem_RespectsDependencies(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "Setup DB"},
			{Title: "Auth flow", DependsOn: []string{"item 1"}},
			{Title: "Dashboard"},
		},
	}
	// Nothing done yet — Auth flow blocked by Setup DB, but Dashboard is free
	// Actually, GetNextItem returns the first available in plan order.
	// Setup DB has no deps, so it should be returned first.
	next := GetNextItem(plan, nil)
	if next == nil || next.Title != "Setup DB" {
		t.Errorf("expected Setup DB (no deps), got %v", next)
	}

	// After Setup DB passes, Auth flow is unblocked
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "Setup DB", Status: "passed"},
	}
	next = GetNextItem(plan, events)
	if next == nil || next.Title != "Auth flow" {
		t.Errorf("expected Auth flow (dep satisfied), got %v", next)
	}
}

func TestGetNextItem_BlockedByDep(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "First", DependsOn: []string{"item 2"}},
			{Title: "Second", DependsOn: []string{"item 1"}},
		},
	}
	// Circular dep — both blocked. Neither should be returned.
	// Actually, item 1 depends on item 2 and vice versa. Both are blocked.
	next := GetNextItem(plan, nil)
	if next != nil {
		t.Errorf("expected nil for circular deps, got %v", next)
	}
}

func TestGetPendingItems(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "A"},
			{Title: "B"},
			{Title: "C"},
		},
	}
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "A", Status: "passed"},
		{Event: ProgressItemDone, Item: "C", Status: "skipped"},
	}
	pending := GetPendingItems(plan, events)
	if len(pending) != 1 || pending[0].Title != "B" {
		t.Errorf("expected [B] pending, got %v", pending)
	}
}

func TestGetPendingItems_Empty(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{{Title: "Only"}},
	}
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "Only", Status: "passed"},
	}
	pending := GetPendingItems(plan, events)
	if len(pending) != 0 {
		t.Errorf("expected 0 pending, got %d", len(pending))
	}
}

func TestAllItemsComplete_True(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "A"},
			{Title: "B"},
		},
	}
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "A", Status: "passed"},
		{Event: ProgressItemDone, Item: "B", Status: "skipped"},
	}
	if !AllItemsComplete(plan, events) {
		t.Error("expected all items complete")
	}
}

func TestAllItemsComplete_False(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "A"},
			{Title: "B"},
		},
	}
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "A", Status: "passed"},
	}
	if AllItemsComplete(plan, events) {
		t.Error("expected not all items complete")
	}
}

func TestAllItemsComplete_EmptyPlan(t *testing.T) {
	plan := &Plan{}
	if !AllItemsComplete(plan, nil) {
		t.Error("expected empty plan to be complete")
	}
}

func TestCountItemsPassed(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "A", Status: "passed"},
		{Event: ProgressItemDone, Item: "B", Status: "skipped"},
		{Event: ProgressItemDone, Item: "C", Status: "passed"},
	}
	if n := CountItemsPassed(events); n != 2 {
		t.Errorf("expected 2 passed, got %d", n)
	}
}

func TestCountItemsSkipped(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "A", Status: "passed"},
		{Event: ProgressItemDone, Item: "B", Status: "skipped"},
		{Event: ProgressItemDone, Item: "C", Status: "skipped"},
	}
	if n := CountItemsSkipped(events); n != 2 {
		t.Errorf("expected 2 skipped, got %d", n)
	}
}

func TestCountItemsPassed_Regression(t *testing.T) {
	// Item passes, then gets stuck (regression), counts as NOT passed
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "A", Status: "passed"},
		{Event: ProgressItemStuck, Item: "A", Reason: "regression"},
	}
	if n := CountItemsPassed(events); n != 0 {
		t.Errorf("expected 0 passed after regression, got %d", n)
	}
}

func TestCountItemsSkipped_OverriddenByPass(t *testing.T) {
	// Skipped then later passed (manually re-attempted)
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "A", Status: "skipped"},
		{Event: ProgressItemDone, Item: "A", Status: "passed"},
	}
	if n := CountItemsSkipped(events); n != 0 {
		t.Errorf("expected 0 skipped after pass override, got %d", n)
	}
	if n := CountItemsPassed(events); n != 1 {
		t.Errorf("expected 1 passed after pass override, got %d", n)
	}
}

func TestCollectLearnings_FromItemDone(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "A", Status: "passed", Learnings: []string{"learning 1", "learning 2"}},
		{Event: ProgressItemDone, Item: "B", Status: "passed", Learnings: []string{"learning 3"}},
	}
	learnings := CollectLearnings(events)
	if len(learnings) != 3 {
		t.Errorf("expected 3 learnings, got %d: %v", len(learnings), learnings)
	}
}

func TestCollectLearnings_FromLearningEvents(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressLearning, Text: "standalone insight"},
		{Event: ProgressItemDone, Item: "A", Status: "passed", Learnings: []string{"item insight"}},
	}
	learnings := CollectLearnings(events)
	if len(learnings) != 2 {
		t.Errorf("expected 2 learnings, got %d", len(learnings))
	}
}

func TestCollectLearnings_Deduplication(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressItemDone, Item: "A", Learnings: []string{"callback URL must match exactly"}},
		{Event: ProgressLearning, Text: "Callback URL must match exactly."},
		{Event: ProgressItemDone, Item: "B", Learnings: []string{"CALLBACK URL MUST MATCH EXACTLY"}},
	}
	learnings := CollectLearnings(events)
	if len(learnings) != 1 {
		t.Errorf("expected 1 learning (deduplicated), got %d: %v", len(learnings), learnings)
	}
}

func TestCollectLearnings_Empty(t *testing.T) {
	learnings := CollectLearnings(nil)
	if len(learnings) != 0 {
		t.Errorf("expected 0 learnings from nil events, got %d", len(learnings))
	}
}

func TestResolveItemRef_ItemNumber(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "First"},
			{Title: "Second"},
			{Title: "Third"},
		},
	}
	tests := []struct {
		ref  string
		want string
	}{
		{"item 1", "First"},
		{"item 2", "Second"},
		{"Item 3", "Third"},
		{"item 0", ""},  // out of range
		{"item 4", ""},  // out of range
		{"item -1", ""}, // not a valid number
	}
	for _, tt := range tests {
		got := resolveItemRef(tt.ref, plan)
		if got != tt.want {
			t.Errorf("resolveItemRef(%q) = %q, want %q", tt.ref, got, tt.want)
		}
	}
}

func TestResolveItemRef_TitleMatch(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "Setup OAuth2 dependencies"},
			{Title: "Google login flow"},
		},
	}
	tests := []struct {
		ref  string
		want string
	}{
		{"Setup OAuth2 dependencies", "Setup OAuth2 dependencies"},
		{"setup oauth2 dependencies", "Setup OAuth2 dependencies"},
		{"Google", "Google login flow"}, // substring
		{"nonexistent", ""},
	}
	for _, tt := range tests {
		got := resolveItemRef(tt.ref, plan)
		if got != tt.want {
			t.Errorf("resolveItemRef(%q) = %q, want %q", tt.ref, got, tt.want)
		}
	}
}

func TestDepsResolved_NoDeps(t *testing.T) {
	item := &PlanItem{Title: "A"}
	plan := &Plan{Items: []PlanItem{{Title: "A"}}}
	states := map[string]ItemState{"A": {}}
	if !depsResolved(item, plan, states) {
		t.Error("expected true for item with no deps")
	}
}

func TestDepsResolved_DepPassed(t *testing.T) {
	item := &PlanItem{Title: "B", DependsOn: []string{"item 1"}}
	plan := &Plan{Items: []PlanItem{{Title: "A"}, {Title: "B"}}}
	states := map[string]ItemState{
		"A": {Passed: true},
		"B": {},
	}
	if !depsResolved(item, plan, states) {
		t.Error("expected true when dep is passed")
	}
}

func TestDepsResolved_DepNotPassed(t *testing.T) {
	item := &PlanItem{Title: "B", DependsOn: []string{"item 1"}}
	plan := &Plan{Items: []PlanItem{{Title: "A"}, {Title: "B"}}}
	states := map[string]ItemState{
		"A": {Attempted: true},
		"B": {},
	}
	if depsResolved(item, plan, states) {
		t.Error("expected false when dep is not passed")
	}
}

func TestComputeAllItemStates(t *testing.T) {
	plan := &Plan{
		Items: []PlanItem{
			{Title: "A"},
			{Title: "B"},
		},
	}
	events := []ProgressEvent{
		{Event: ProgressItemStart, Item: "A", Attempt: 1},
		{Event: ProgressItemDone, Item: "A", Status: "passed", Commit: "abc"},
		{Event: ProgressItemStart, Item: "B", Attempt: 1},
		{Event: ProgressItemStuck, Item: "B", Reason: "hard"},
	}
	states := ComputeAllItemStates(plan, events)
	if len(states) != 2 {
		t.Fatalf("expected 2 states, got %d", len(states))
	}
	if !states["A"].Passed {
		t.Error("expected A passed")
	}
	if states["B"].Passed {
		t.Error("expected B not passed")
	}
	if states["B"].LastFailure != "hard" {
		t.Errorf("expected B failure reason, got %q", states["B"].LastFailure)
	}
}
