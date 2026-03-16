package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendProgressEvent(t *testing.T) {
	t.Run("appends event to new file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "progress.jsonl")

		event := &ProgressEvent{
			Timestamp: "2026-03-11T15:00:00Z",
			Event:     ProgressExecStart,
			Feature:   "auth",
			PlanItems: 3,
		}
		if err := AppendProgressEvent(path, event); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		events, err := LoadProgressEvents(path)
		if err != nil {
			t.Fatalf("load failed: %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Event != ProgressExecStart {
			t.Errorf("expected exec_start, got %s", events[0].Event)
		}
		if events[0].Feature != "auth" {
			t.Errorf("expected feature 'auth', got %q", events[0].Feature)
		}
		if events[0].PlanItems != 3 {
			t.Errorf("expected plan_items 3, got %d", events[0].PlanItems)
		}
	})

	t.Run("appends multiple events", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "progress.jsonl")

		events := []*ProgressEvent{
			{Event: ProgressExecStart, Feature: "auth", PlanItems: 2},
			{Event: ProgressItemStart, Item: "OAuth2 setup", Criteria: []string{"OAuth2 works"}},
			{Event: ProgressItemDone, Item: "OAuth2 setup", Status: "passed", Commit: "abc123"},
			{Event: ProgressExecEnd, Passed: 1},
		}
		for _, e := range events {
			if err := AppendProgressEvent(path, e); err != nil {
				t.Fatalf("append failed: %v", err)
			}
		}

		loaded, err := LoadProgressEvents(path)
		if err != nil {
			t.Fatalf("load failed: %v", err)
		}
		if len(loaded) != 4 {
			t.Fatalf("expected 4 events, got %d", len(loaded))
		}
		// All should have auto-set timestamps
		for i, e := range loaded {
			if e.Timestamp == "" {
				t.Errorf("event %d has empty timestamp", i)
			}
		}
	})

	t.Run("auto-sets timestamp when empty", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "progress.jsonl")
		event := &ProgressEvent{Event: ProgressPlanPurged}

		if err := AppendProgressEvent(path, event); err != nil {
			t.Fatalf("append failed: %v", err)
		}
		if event.Timestamp == "" {
			t.Error("timestamp should be auto-set")
		}
	})

	t.Run("preserves explicit timestamp", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "progress.jsonl")
		event := &ProgressEvent{
			Timestamp: "2026-01-01T00:00:00Z",
			Event:     ProgressPlanPurged,
		}

		if err := AppendProgressEvent(path, event); err != nil {
			t.Fatalf("append failed: %v", err)
		}

		loaded, _ := LoadProgressEvents(path)
		if loaded[0].Timestamp != "2026-01-01T00:00:00Z" {
			t.Errorf("expected explicit timestamp, got %q", loaded[0].Timestamp)
		}
	})

	t.Run("creates parent directories", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nested", "dir", "progress.jsonl")
		event := &ProgressEvent{Event: ProgressPlanPurged}

		if err := AppendProgressEvent(path, event); err != nil {
			t.Fatalf("append to nested path failed: %v", err)
		}

		loaded, err := LoadProgressEvents(path)
		if err != nil {
			t.Fatalf("load failed: %v", err)
		}
		if len(loaded) != 1 {
			t.Fatalf("expected 1 event, got %d", len(loaded))
		}
	})

	t.Run("omits zero-value optional fields", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "progress.jsonl")
		event := &ProgressEvent{
			Timestamp: "2026-03-11T15:00:00Z",
			Event:     ProgressPlanPurged,
		}
		if err := AppendProgressEvent(path, event); err != nil {
			t.Fatalf("append failed: %v", err)
		}

		data, _ := os.ReadFile(path)
		line := strings.TrimSpace(string(data))
		// Should only have ts and event
		var raw map[string]interface{}
		json.Unmarshal([]byte(line), &raw)
		if len(raw) != 2 {
			t.Errorf("expected 2 fields (ts, event), got %d: %v", len(raw), raw)
		}
	})
}

func TestLoadProgressEvents(t *testing.T) {
	t.Run("nonexistent file returns nil", func(t *testing.T) {
		events, err := LoadProgressEvents(filepath.Join(t.TempDir(), "nope.jsonl"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if events != nil {
			t.Fatal("expected nil for nonexistent file")
		}
	})

	t.Run("skips corrupt lines", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "progress.jsonl")
		content := `{"ts":"2026-01-01T00:00:00Z","event":"exec_start","feature":"auth"}
not json
{"ts":"2026-01-01T00:01:00Z","event":"exec_end"}
`
		os.WriteFile(path, []byte(content), 0644)

		events, err := LoadProgressEvents(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(events) != 2 {
			t.Fatalf("expected 2 events (skipping corrupt), got %d", len(events))
		}
	})

	t.Run("skips empty lines", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "progress.jsonl")
		content := `{"ts":"2026-01-01T00:00:00Z","event":"exec_start"}

{"ts":"2026-01-01T00:01:00Z","event":"exec_end"}
`
		os.WriteFile(path, []byte(content), 0644)

		events, err := LoadProgressEvents(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(events) != 2 {
			t.Fatalf("expected 2 events, got %d", len(events))
		}
	})

	t.Run("preserves all event fields", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "progress.jsonl")
		event := &ProgressEvent{
			Timestamp: "2026-03-11T15:10:00Z",
			Event:     ProgressItemDone,
			Item:      "OAuth2 setup",
			Status:    "passed",
			Commit:    "abc123",
			Learnings: []string{"callback URL must match exactly"},
		}
		AppendProgressEvent(path, event)

		loaded, _ := LoadProgressEvents(path)
		if len(loaded) != 1 {
			t.Fatalf("expected 1 event, got %d", len(loaded))
		}
		e := loaded[0]
		if e.Item != "OAuth2 setup" {
			t.Errorf("expected item 'OAuth2 setup', got %q", e.Item)
		}
		if e.Status != "passed" {
			t.Errorf("expected status 'passed', got %q", e.Status)
		}
		if e.Commit != "abc123" {
			t.Errorf("expected commit 'abc123', got %q", e.Commit)
		}
		if len(e.Learnings) != 1 || e.Learnings[0] != "callback URL must match exactly" {
			t.Errorf("unexpected learnings: %v", e.Learnings)
		}
	})
}

func TestLastEventByType(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressExecStart, Feature: "auth"},
		{Event: ProgressItemStart, Item: "item1"},
		{Event: ProgressItemDone, Item: "item1", Status: "passed"},
		{Event: ProgressItemStart, Item: "item2"},
		{Event: ProgressItemStuck, Item: "item2", Reason: "confused"},
		{Event: ProgressItemStart, Item: "item2", Attempt: 2},
		{Event: ProgressItemDone, Item: "item2", Status: "passed"},
	}

	t.Run("finds last item_start", func(t *testing.T) {
		last := LastEventByType(events, ProgressItemStart)
		if last == nil {
			t.Fatal("expected non-nil")
		}
		if last.Item != "item2" || last.Attempt != 2 {
			t.Errorf("expected item2 attempt 2, got %q attempt %d", last.Item, last.Attempt)
		}
	})

	t.Run("finds last item_done", func(t *testing.T) {
		last := LastEventByType(events, ProgressItemDone)
		if last == nil {
			t.Fatal("expected non-nil")
		}
		if last.Item != "item2" {
			t.Errorf("expected item2, got %q", last.Item)
		}
	})

	t.Run("returns nil for missing type", func(t *testing.T) {
		last := LastEventByType(events, ProgressLandPassed)
		if last != nil {
			t.Error("expected nil for missing type")
		}
	})

	t.Run("returns nil for empty slice", func(t *testing.T) {
		last := LastEventByType(nil, ProgressExecStart)
		if last != nil {
			t.Error("expected nil for empty slice")
		}
	})
}

func TestQueryEventsByType(t *testing.T) {
	events := []ProgressEvent{
		{Event: ProgressExecStart},
		{Event: ProgressItemStart, Item: "item1"},
		{Event: ProgressItemDone, Item: "item1"},
		{Event: ProgressItemStart, Item: "item2"},
		{Event: ProgressItemDone, Item: "item2"},
		{Event: ProgressExecEnd},
	}

	t.Run("returns all matching events", func(t *testing.T) {
		starts := QueryEventsByType(events, ProgressItemStart)
		if len(starts) != 2 {
			t.Fatalf("expected 2 item_start events, got %d", len(starts))
		}
		if starts[0].Item != "item1" || starts[1].Item != "item2" {
			t.Errorf("unexpected items: %q, %q", starts[0].Item, starts[1].Item)
		}
	})

	t.Run("returns nil for no matches", func(t *testing.T) {
		result := QueryEventsByType(events, ProgressLandFailed)
		if result != nil {
			t.Errorf("expected nil, got %d events", len(result))
		}
	})

	t.Run("returns nil for empty input", func(t *testing.T) {
		result := QueryEventsByType(nil, ProgressExecStart)
		if result != nil {
			t.Error("expected nil for nil input")
		}
	})
}

func TestRotateProgress(t *testing.T) {
	t.Run("rotates at threshold", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "progress.jsonl")

		// Write exactly threshold lines
		f, _ := os.Create(path)
		for i := 0; i < 5; i++ {
			fmt.Fprintf(f, `{"ts":"t","event":"learning","text":"line %d"}`+"\n", i)
		}
		f.Close()

		// Trigger rotation with threshold of 5
		err := rotateProgressIfNeeded(path, 5)
		if err != nil {
			t.Fatalf("rotation failed: %v", err)
		}

		// Original should be gone, .1 should exist
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Error("original file should be rotated away")
		}
		if _, err := os.Stat(path + ".1"); err != nil {
			t.Error("archive .1 should exist")
		}

		// Archive should have the original content
		events, _ := LoadProgressEvents(path + ".1")
		if len(events) != 5 {
			t.Errorf("expected 5 events in archive, got %d", len(events))
		}
	})

	t.Run("does not rotate below threshold", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "progress.jsonl")

		f, _ := os.Create(path)
		for i := 0; i < 3; i++ {
			fmt.Fprintf(f, `{"ts":"t","event":"learning","text":"line %d"}`+"\n", i)
		}
		f.Close()

		err := rotateProgressIfNeeded(path, 5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// File should still be there
		events, _ := LoadProgressEvents(path)
		if len(events) != 3 {
			t.Errorf("expected 3 events, got %d", len(events))
		}
	})

	t.Run("cascades existing archives", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "progress.jsonl")

		// Create existing archive .1
		os.WriteFile(path+".1", []byte(`{"ts":"t","event":"exec_start"}`+"\n"), 0644)

		// Write current file at threshold
		f, _ := os.Create(path)
		for i := 0; i < 5; i++ {
			fmt.Fprintf(f, `{"ts":"t","event":"learning","text":"new %d"}`+"\n", i)
		}
		f.Close()

		err := rotateProgressIfNeeded(path, 5)
		if err != nil {
			t.Fatalf("rotation failed: %v", err)
		}

		// Old .1 should be at .2
		events2, _ := LoadProgressEvents(path + ".2")
		if len(events2) != 1 || events2[0].Event != ProgressExecStart {
			t.Error("old archive .1 should be cascaded to .2")
		}

		// Current should be at .1
		events1, _ := LoadProgressEvents(path + ".1")
		if len(events1) != 5 {
			t.Errorf("current file should be at .1 with 5 events, got %d", len(events1))
		}
	})

	t.Run("no error for nonexistent file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nope.jsonl")
		err := rotateProgressIfNeeded(path, 5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestAppendProgressMd(t *testing.T) {
	t.Run("creates new file with section", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "progress.md")
		section := "## 2026-03-11 15:30 — Exec Session\n\n### Completed\n- OAuth2 setup (abc123)"

		if err := AppendProgressMd(path, section); err != nil {
			t.Fatalf("append failed: %v", err)
		}

		data, _ := os.ReadFile(path)
		content := string(data)
		if !strings.Contains(content, "## 2026-03-11 15:30") {
			t.Error("expected section header in output")
		}
		if !strings.Contains(content, "OAuth2 setup") {
			t.Error("expected section content in output")
		}
		// Should NOT start with separator
		if strings.HasPrefix(content, "\n---") {
			t.Error("first section should not have separator prefix")
		}
	})

	t.Run("appends with separator", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "progress.md")

		AppendProgressMd(path, "## Session 1\n\nFirst session")
		AppendProgressMd(path, "## Session 2\n\nSecond session")

		data, _ := os.ReadFile(path)
		content := string(data)
		if !strings.Contains(content, "\n---\n") {
			t.Error("expected --- separator between sections")
		}
		if strings.Count(content, "---") != 1 {
			t.Errorf("expected exactly 1 separator, got %d", strings.Count(content, "---"))
		}
	})

	t.Run("creates parent directories", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nested", "progress.md")
		if err := AppendProgressMd(path, "## Test"); err != nil {
			t.Fatalf("append to nested path failed: %v", err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Error("file should exist")
		}
	})

	t.Run("strips trailing newlines from section", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "progress.md")
		AppendProgressMd(path, "## Test\n\n\n\n")

		data, _ := os.ReadFile(path)
		// Should end with exactly one newline
		if !strings.HasSuffix(string(data), "## Test\n") {
			t.Errorf("unexpected content: %q", string(data))
		}
	})
}

func TestGetProgressContext(t *testing.T) {
	t.Run("returns default for nonexistent file", func(t *testing.T) {
		result := GetProgressContext(filepath.Join(t.TempDir(), "nope.md"))
		if result != noProgressHistory {
			t.Errorf("expected default message, got %q", result)
		}
	})

	t.Run("returns default for empty file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "progress.md")
		os.WriteFile(path, []byte(""), 0644)

		result := GetProgressContext(path)
		if result != noProgressHistory {
			t.Errorf("expected default message, got %q", result)
		}
	})

	t.Run("returns single section", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "progress.md")
		os.WriteFile(path, []byte("## Session 1\n\nCompleted stuff\n"), 0644)

		result := GetProgressContext(path)
		if !strings.Contains(result, "Session 1") {
			t.Error("expected section content")
		}
	})

	t.Run("returns last 3 sections", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "progress.md")
		AppendProgressMd(path, "## Session 1\n\nFirst")
		AppendProgressMd(path, "## Session 2\n\nSecond")
		AppendProgressMd(path, "## Session 3\n\nThird")
		AppendProgressMd(path, "## Session 4\n\nFourth")

		result := GetProgressContext(path)
		if strings.Contains(result, "Session 1") {
			t.Error("should not include oldest section")
		}
		if !strings.Contains(result, "Session 2") {
			t.Error("should include session 2")
		}
		if !strings.Contains(result, "Session 3") {
			t.Error("should include session 3")
		}
		if !strings.Contains(result, "Session 4") {
			t.Error("should include session 4")
		}
	})

	t.Run("reduces to 2 sections when over 16KB", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "progress.md")
		// Create 3 sections, each ~6KB (total ~18KB > 16KB threshold)
		bigContent := strings.Repeat("x", 6000)
		AppendProgressMd(path, "## Session 1\n\n"+bigContent)
		AppendProgressMd(path, "## Session 2\n\n"+bigContent)
		AppendProgressMd(path, "## Session 3\n\n"+bigContent)

		result := GetProgressContext(path)
		if strings.Contains(result, "Session 1") {
			t.Error("should drop oldest section when over 16KB")
		}
		if !strings.Contains(result, "Session 2") {
			t.Error("should include session 2")
		}
		if !strings.Contains(result, "Session 3") {
			t.Error("should include session 3")
		}
	})
}

func TestSplitProgressSections(t *testing.T) {
	t.Run("splits on --- separator", func(t *testing.T) {
		content := "## Section 1\n\nContent 1\n\n---\n\n## Section 2\n\nContent 2"
		sections := splitProgressSections(content)
		if len(sections) != 2 {
			t.Fatalf("expected 2 sections, got %d", len(sections))
		}
		if !strings.HasPrefix(sections[0], "## Section 1") {
			t.Errorf("unexpected first section: %q", sections[0])
		}
		if !strings.HasPrefix(sections[1], "## Section 2") {
			t.Errorf("unexpected second section: %q", sections[1])
		}
	})

	t.Run("handles empty content", func(t *testing.T) {
		sections := splitProgressSections("")
		if len(sections) != 0 {
			t.Errorf("expected 0 sections for empty content, got %d", len(sections))
		}
	})

	t.Run("handles content with no separator", func(t *testing.T) {
		sections := splitProgressSections("## Single section\n\nContent")
		if len(sections) != 1 {
			t.Fatalf("expected 1 section, got %d", len(sections))
		}
	})
}

func TestProgressEventRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.jsonl")

	// Write diverse events covering all event types
	events := []*ProgressEvent{
		{Event: ProgressExecStart, Feature: "auth", PlanItems: 3},
		{Event: ProgressItemStart, Item: "OAuth2", Criteria: []string{"OAuth2 works", "no secrets"}},
		{Event: ProgressItemDone, Item: "OAuth2", Status: "passed", Commit: "abc", Learnings: []string{"URL must match"}},
		{Event: ProgressItemStart, Item: "Login", Attempt: 2},
		{Event: ProgressItemStuck, Item: "Login", Attempt: 1, Reason: "config unclear"},
		{Event: ProgressLearning, Text: "Guardian needs serializer"},
		{Event: ProgressExecEnd, Passed: 2, Skipped: 1, Failed: 1},
		{Event: ProgressPlanPurged},
		{Event: ProgressPlanCreated, ItemCount: 5, Context: "new plan"},
		{Event: ProgressLandFailed, Findings: []string{"2 test failures"}, Analysis: "missing CSRF"},
		{Event: ProgressLandPassed, SummaryAppended: true},
	}

	for _, e := range events {
		if err := AppendProgressEvent(path, e); err != nil {
			t.Fatalf("append failed for %s: %v", e.Event, err)
		}
	}

	loaded, err := LoadProgressEvents(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(loaded) != len(events) {
		t.Fatalf("expected %d events, got %d", len(events), len(loaded))
	}

	// Verify specific fields survived round-trip
	if loaded[0].Feature != "auth" || loaded[0].PlanItems != 3 {
		t.Error("exec_start fields lost")
	}
	if len(loaded[1].Criteria) != 2 {
		t.Error("item_start criteria lost")
	}
	if loaded[2].Commit != "abc" || len(loaded[2].Learnings) != 1 {
		t.Error("item_done fields lost")
	}
	if loaded[4].Reason != "config unclear" {
		t.Error("item_stuck reason lost")
	}
	if loaded[5].Text != "Guardian needs serializer" {
		t.Error("learning text lost")
	}
	if loaded[6].Passed != 2 || loaded[6].Skipped != 1 || loaded[6].Failed != 1 {
		t.Error("exec_end counts lost")
	}
	if loaded[8].ItemCount != 5 {
		t.Error("plan_created item_count lost")
	}
	if loaded[9].Analysis != "missing CSRF" {
		t.Error("land_failed analysis lost")
	}
	if !loaded[10].SummaryAppended {
		t.Error("land_passed summary_appended lost")
	}
}
