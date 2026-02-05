package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewRunLogger(t *testing.T) {
	dir := t.TempDir()

	logger, err := NewRunLogger(dir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer logger.Close()

	if logger.RunNumber() != 1 {
		t.Errorf("expected run number 1, got %d", logger.RunNumber())
	}

	logPath := logger.LogPath()
	if !strings.Contains(logPath, "run-001.jsonl") {
		t.Errorf("expected log path to contain 'run-001.jsonl', got %s", logPath)
	}

	// Verify logs directory was created
	logsDir := filepath.Join(dir, "logs")
	if _, err := os.Stat(logsDir); os.IsNotExist(err) {
		t.Error("logs directory was not created")
	}
}

func TestRunLogger_Disabled(t *testing.T) {
	dir := t.TempDir()

	config := &LoggingConfig{
		Enabled: false,
	}

	logger, err := NewRunLogger(dir, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer logger.Close()

	// Logging should be no-op when disabled
	logger.RunStart("test", "branch", 5)
	logger.RunEnd(true, "done")

	// No log file should be created
	if logger.LogPath() != "" {
		t.Errorf("expected no log path when disabled, got %s", logger.LogPath())
	}
}

func TestRunLogger_EventLogging(t *testing.T) {
	dir := t.TempDir()

	logger, err := NewRunLogger(dir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	logger.RunStart("auth", "ralph/auth", 3)
	logger.SetIteration(1)
	logger.SetCurrentStory("US-001")
	logger.IterationStart("US-001", "User login", 0)
	logger.ProviderStart()
	logger.MarkerDetected("DONE", "")
	logger.ProviderEnd(0, false, []string{"DONE"})
	logger.VerifyStart()
	logger.VerifyCmdStart("bun run test")
	logger.VerifyCmdEnd("bun run test", true, "output", 5000000000)
	logger.VerifyEnd(true)
	logger.StateChange("US-001", "pending", "passed", map[string]interface{}{"commit": "abc123"})
	logger.IterationEnd(true)
	logger.Learning("Always check types")
	logger.Warning("test warning")
	logger.Error("test error", nil)
	logger.RunEnd(true, "all verified")

	logger.Close()

	// Read and verify events
	events, err := ReadEvents(logger.LogPath(), nil)
	if err != nil {
		t.Fatalf("failed to read events: %v", err)
	}

	if len(events) < 10 {
		t.Errorf("expected at least 10 events, got %d", len(events))
	}

	// Check first event is run_start
	if events[0].Type != EventRunStart {
		t.Errorf("expected first event to be run_start, got %s", events[0].Type)
	}

	// Check last event is run_end
	if events[len(events)-1].Type != EventRunEnd {
		t.Errorf("expected last event to be run_end, got %s", events[len(events)-1].Type)
	}
}

func TestRunLogger_NextRunNumber(t *testing.T) {
	dir := t.TempDir()
	logsDir := filepath.Join(dir, "logs")
	os.MkdirAll(logsDir, 0755)

	// Create some existing run files
	os.WriteFile(filepath.Join(logsDir, "run-001.jsonl"), []byte{}, 0644)
	os.WriteFile(filepath.Join(logsDir, "run-002.jsonl"), []byte{}, 0644)
	os.WriteFile(filepath.Join(logsDir, "run-005.jsonl"), []byte{}, 0644)

	logger, err := NewRunLogger(dir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer logger.Close()

	if logger.RunNumber() != 6 {
		t.Errorf("expected run number 6, got %d", logger.RunNumber())
	}
}

func TestRunLogger_Rotation(t *testing.T) {
	dir := t.TempDir()
	logsDir := filepath.Join(dir, "logs")
	os.MkdirAll(logsDir, 0755)

	// Create 12 existing run files
	for i := 1; i <= 12; i++ {
		os.WriteFile(filepath.Join(logsDir, fmt.Sprintf("run-%03d.jsonl", i)), []byte("test"), 0644)
	}

	config := &LoggingConfig{
		Enabled: true,
		MaxRuns: 10,
	}

	logger, err := NewRunLogger(dir, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	logger.Close()

	// After rotation + new file, should have maxRuns files
	// Rotation happens BEFORE creating new file, so we delete to get to maxRuns-1
	// then create new file, ending up at maxRuns
	entries, _ := os.ReadDir(logsDir)
	// We started with 12 files, rotation deletes down to maxRuns-1=9, then we create 1 more = 10
	// But rotation happens before nextRunNumber is calculated, so:
	// - 12 files exist
	// - rotation keeps only maxRuns=10 files (deletes run-001, run-002)
	// - nextRunNumber finds max=12, returns 13
	// - creates run-013.jsonl
	// Total: 11 files (10 after rotation + 1 new)
	// Let's verify the old files were rotated away

	// run-001 and run-002 should be deleted (oldest)
	if _, err := os.Stat(filepath.Join(logsDir, "run-001.jsonl")); !os.IsNotExist(err) {
		t.Error("run-001.jsonl should have been deleted")
	}
	if _, err := os.Stat(filepath.Join(logsDir, "run-002.jsonl")); !os.IsNotExist(err) {
		t.Error("run-002.jsonl should have been deleted")
	}
	// run-003 should still exist
	if _, err := os.Stat(filepath.Join(logsDir, "run-003.jsonl")); os.IsNotExist(err) {
		t.Error("run-003.jsonl should still exist")
	}
	// New run-013 should be created
	if _, err := os.Stat(filepath.Join(logsDir, "run-013.jsonl")); os.IsNotExist(err) {
		t.Error("run-013.jsonl should have been created")
	}

	// Total should be 11 (10 original after rotation + 1 new)
	if len(entries) != 11 {
		t.Errorf("expected 11 files, got %d", len(entries))
	}
}

func TestEventFilter(t *testing.T) {
	filter := &EventFilter{
		EventType: EventError,
		StoryID:   "US-001",
	}

	// Should match
	e1 := &Event{Type: EventError, StoryID: "US-001"}
	if !filter.Match(e1) {
		t.Error("expected e1 to match filter")
	}

	// Wrong type
	e2 := &Event{Type: EventWarning, StoryID: "US-001"}
	if filter.Match(e2) {
		t.Error("expected e2 to not match filter (wrong type)")
	}

	// Wrong story
	e3 := &Event{Type: EventError, StoryID: "US-002"}
	if filter.Match(e3) {
		t.Error("expected e3 to not match filter (wrong story)")
	}

	// Empty filter matches all
	emptyFilter := &EventFilter{}
	if !emptyFilter.Match(e1) || !emptyFilter.Match(e2) || !emptyFilter.Match(e3) {
		t.Error("empty filter should match all events")
	}
}

func TestReadEvents(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.jsonl")

	// Write some events
	f, _ := os.Create(logPath)
	events := []Event{
		{Timestamp: time.Now(), Type: EventRunStart},
		{Timestamp: time.Now(), Type: EventError, StoryID: "US-001"},
		{Timestamp: time.Now(), Type: EventWarning, StoryID: "US-002"},
		{Timestamp: time.Now(), Type: EventRunEnd},
	}
	enc := json.NewEncoder(f)
	for _, e := range events {
		enc.Encode(e)
	}
	f.Close()

	// Read all
	all, err := ReadEvents(logPath, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("expected 4 events, got %d", len(all))
	}

	// Read filtered by type
	filter := &EventFilter{EventType: EventError}
	filtered, err := ReadEvents(logPath, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(filtered) != 1 {
		t.Errorf("expected 1 error event, got %d", len(filtered))
	}
}

func TestGetRunSummary(t *testing.T) {
	dir := t.TempDir()

	logger, err := NewRunLogger(dir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	logger.RunStart("auth", "ralph/auth", 2)
	logger.SetIteration(1)
	logger.IterationStart("US-001", "User login", 0)
	logger.IterationEnd(true)
	logger.Learning("Test learning")
	logger.Warning("Test warning")
	logger.RunEnd(true, "verified")
	logger.Close()

	summary, err := GetRunSummary(logger.LogPath())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.Feature != "auth" {
		t.Errorf("expected feature 'auth', got %s", summary.Feature)
	}
	if summary.Branch != "ralph/auth" {
		t.Errorf("expected branch 'ralph/auth', got %s", summary.Branch)
	}
	if summary.Success == nil || !*summary.Success {
		t.Error("expected success=true")
	}
	if len(summary.Learnings) != 1 {
		t.Errorf("expected 1 learning, got %d", len(summary.Learnings))
	}
	if summary.Warnings != 1 {
		t.Errorf("expected 1 warning, got %d", summary.Warnings)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d        time.Duration
		expected string
	}{
		{500 * time.Millisecond, "500ms"},
		{1500 * time.Millisecond, "1.5s"},
		{45 * time.Second, "45.0s"},
		{90 * time.Second, "1m30s"},
		{5 * time.Minute, "5m"},
		{5*time.Minute + 30*time.Second, "5m30s"},
	}

	for _, tt := range tests {
		got := FormatDuration(tt.d)
		if got != tt.expected {
			t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.expected)
		}
	}
}

func TestListRuns(t *testing.T) {
	dir := t.TempDir()
	logsDir := filepath.Join(dir, "logs")
	os.MkdirAll(logsDir, 0755)

	// Create some run files with valid JSONL content
	for i := 1; i <= 3; i++ {
		f, _ := os.Create(filepath.Join(logsDir, fmt.Sprintf("run-%03d.jsonl", i)))
		enc := json.NewEncoder(f)
		enc.Encode(Event{Timestamp: time.Now(), Type: EventRunStart})
		enc.Encode(Event{Timestamp: time.Now(), Type: EventRunEnd, Success: ptrBool(i%2 == 1)})
		f.Close()
	}

	runs, err := ListRuns(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(runs) != 3 {
		t.Errorf("expected 3 runs, got %d", len(runs))
	}

	// Should be sorted by run number descending
	if runs[0].RunNumber != 3 {
		t.Errorf("expected first run to be #3, got #%d", runs[0].RunNumber)
	}
}

func TestDefaultLoggingConfig(t *testing.T) {
	cfg := DefaultLoggingConfig()

	if !cfg.Enabled {
		t.Error("expected Enabled=true by default")
	}
	if cfg.MaxRuns != 10 {
		t.Errorf("expected MaxRuns=10, got %d", cfg.MaxRuns)
	}
	if !cfg.ConsoleTimestamps {
		t.Error("expected ConsoleTimestamps=true by default")
	}
	if !cfg.ConsoleDurations {
		t.Error("expected ConsoleDurations=true by default")
	}
}

// Helper to create bool pointer
func ptrBool(b bool) *bool {
	return &b
}
