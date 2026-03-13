package main

import (
	"strings"
)

// normalizeLearning normalizes a learning string for deduplication comparison.
func normalizeLearning(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, ".!,;:")
	return strings.ToLower(s)
}

// --- Item state derived from progress.jsonl events ---
//
// In scrip, item state is computed from the append-only progress event log
// rather than maintained as mutable state. No mutation methods — all state
// changes flow through AppendProgressEvent in progress.go.

// ItemState represents the computed state of a single plan item.
// Derived from progress.jsonl events — immutable snapshot.
type ItemState struct {
	Name        string
	Passed      bool
	Skipped     bool
	Attempted   bool
	Attempts    int
	LastFailure string
	LastCommit  string
	Learnings   []string
}

// ComputeItemState derives the current state of a named item from progress events.
// Processes events in order; the last relevant event for each field wins.
func ComputeItemState(item string, events []ProgressEvent) ItemState {
	state := ItemState{Name: item}
	for _, e := range events {
		if e.Item != item {
			continue
		}
		switch e.Event {
		case ProgressItemStart:
			state.Attempted = true
			state.Attempts++
		case ProgressItemDone:
			switch e.Status {
			case "passed":
				state.Passed = true
				state.Skipped = false
				if e.Commit != "" {
					state.LastCommit = e.Commit
				}
			case "skipped":
				state.Skipped = true
				state.Passed = false
			}
			state.Learnings = append(state.Learnings, e.Learnings...)
		case ProgressItemStuck:
			// Stuck clears passed (handles regression from verify-at-top)
			state.Passed = false
			if e.Reason != "" {
				state.LastFailure = e.Reason
			}
		}
	}
	return state
}

// ComputeAllItemStates computes state for every item in the plan.
func ComputeAllItemStates(plan *Plan, events []ProgressEvent) map[string]ItemState {
	states := make(map[string]ItemState, len(plan.Items))
	for _, item := range plan.Items {
		states[item.Title] = ComputeItemState(item.Title, events)
	}
	return states
}

// GetNextItem returns the first item that is not passed, not skipped, and has
// all dependencies satisfied. Items are evaluated in plan order (the plan
// already encodes priority). Returns nil if all items are complete or blocked.
func GetNextItem(plan *Plan, events []ProgressEvent) *PlanItem {
	states := ComputeAllItemStates(plan, events)
	for i := range plan.Items {
		item := &plan.Items[i]
		s := states[item.Title]
		if s.Passed || s.Skipped {
			continue
		}
		if !depsResolved(item, plan, states) {
			continue
		}
		return item
	}
	return nil
}

// GetPendingItems returns all items not yet passed or skipped.
func GetPendingItems(plan *Plan, events []ProgressEvent) []PlanItem {
	states := ComputeAllItemStates(plan, events)
	var pending []PlanItem
	for _, item := range plan.Items {
		s := states[item.Title]
		if !s.Passed && !s.Skipped {
			pending = append(pending, item)
		}
	}
	return pending
}

// AllItemsComplete returns true when every plan item is passed or skipped.
func AllItemsComplete(plan *Plan, events []ProgressEvent) bool {
	states := ComputeAllItemStates(plan, events)
	for _, item := range plan.Items {
		s := states[item.Title]
		if !s.Passed && !s.Skipped {
			return false
		}
	}
	return true
}

// CountItemsPassed counts items whose latest status is "passed" in progress events.
func CountItemsPassed(events []ProgressEvent) int {
	status := computeItemStatuses(events)
	count := 0
	for _, s := range status {
		if s == "passed" {
			count++
		}
	}
	return count
}

// CountItemsSkipped counts items whose latest status is "skipped" in progress events.
func CountItemsSkipped(events []ProgressEvent) int {
	status := computeItemStatuses(events)
	count := 0
	for _, s := range status {
		if s == "skipped" {
			count++
		}
	}
	return count
}

// computeItemStatuses returns the latest status string per item.
// "passed", "skipped", or "stuck" based on event ordering.
func computeItemStatuses(events []ProgressEvent) map[string]string {
	status := make(map[string]string)
	for _, e := range events {
		if e.Item == "" {
			continue
		}
		switch e.Event {
		case ProgressItemDone:
			status[e.Item] = e.Status
		case ProgressItemStuck:
			status[e.Item] = "stuck"
		}
	}
	return status
}

// CollectLearnings returns all unique learnings from progress events
// (both item_done learnings and standalone learning events). Deduplicates
// using normalizeLearning for deduplication.
func CollectLearnings(events []ProgressEvent) []string {
	var learnings []string
	seen := make(map[string]bool)
	for _, e := range events {
		switch e.Event {
		case ProgressItemDone:
			for _, l := range e.Learnings {
				norm := normalizeLearning(l)
				if !seen[norm] {
					seen[norm] = true
					learnings = append(learnings, l)
				}
			}
		case ProgressLearning:
			if e.Text != "" {
				norm := normalizeLearning(e.Text)
				if !seen[norm] {
					seen[norm] = true
					learnings = append(learnings, e.Text)
				}
			}
		}
	}
	return learnings
}

// depsResolved checks if all dependencies of an item are passed.
func depsResolved(item *PlanItem, plan *Plan, states map[string]ItemState) bool {
	for _, dep := range item.DependsOn {
		depTitle := resolveItemRef(dep, plan)
		if depTitle == "" {
			continue // unresolvable dep — don't block
		}
		s, ok := states[depTitle]
		if !ok || !s.Passed {
			return false
		}
	}
	return true
}

// resolveItemRef resolves a dependency reference to an item title.
// Supports "item N" (1-based index) and case-insensitive title matching.
func resolveItemRef(ref string, plan *Plan) string {
	ref = strings.TrimSpace(ref)

	// Try "item N" pattern (1-based index)
	lower := strings.ToLower(ref)
	if strings.HasPrefix(lower, "item ") {
		numStr := strings.TrimSpace(ref[5:])
		n := 0
		for _, c := range numStr {
			if c < '0' || c > '9' {
				n = -1
				break
			}
			n = n*10 + int(c-'0')
		}
		if n >= 1 && n <= len(plan.Items) {
			return plan.Items[n-1].Title
		}
	}

	// Try exact title match (case-insensitive)
	for _, item := range plan.Items {
		if strings.EqualFold(item.Title, ref) {
			return item.Title
		}
	}

	// Try title contains (case-insensitive)
	for _, item := range plan.Items {
		if strings.Contains(strings.ToLower(item.Title), lower) {
			return item.Title
		}
	}

	return ""
}
