package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ProgressEventType identifies a progress event kind.
type ProgressEventType string

const (
	ProgressExecStart   ProgressEventType = "exec_start"
	ProgressItemStart   ProgressEventType = "item_start"
	ProgressItemDone    ProgressEventType = "item_done"
	ProgressItemStuck   ProgressEventType = "item_stuck"
	ProgressLearning    ProgressEventType = "learning"
	ProgressExecEnd     ProgressEventType = "exec_end"
	ProgressPlanPurged  ProgressEventType = "plan_purged"
	ProgressPlanCreated ProgressEventType = "plan_created"
	ProgressLandFailed  ProgressEventType = "land_failed"
	ProgressLandPassed  ProgressEventType = "land_passed"
)

const progressMaxLines = 10000

// ProgressEvent is a single entry in progress.jsonl.
// Fields are optional depending on Event type; omitempty suppresses zero values.
type ProgressEvent struct {
	Timestamp string            `json:"ts"`
	Event     ProgressEventType `json:"event"`

	// exec_start
	Feature   string `json:"feature,omitempty"`
	PlanItems int    `json:"plan_items,omitempty"`

	// item_start, item_done, item_stuck
	Item     string   `json:"item,omitempty"`
	Criteria []string `json:"criteria,omitempty"`
	Attempt  int      `json:"attempt,omitempty"`

	// item_done
	Status    string   `json:"status,omitempty"`
	Commit    string   `json:"commit,omitempty"`
	Learnings []string `json:"learnings,omitempty"`

	// item_stuck
	Reason string `json:"reason,omitempty"`

	// learning
	Text string `json:"text,omitempty"`

	// exec_end
	Passed  int `json:"passed,omitempty"`
	Skipped int `json:"skipped,omitempty"`
	Failed  int `json:"failed,omitempty"`

	// land_failed
	Findings []string `json:"findings,omitempty"`
	Analysis string   `json:"analysis,omitempty"`

	// land_passed
	SummaryAppended bool `json:"summary_appended,omitempty"`

	// plan_created
	ItemCount int    `json:"item_count,omitempty"`
	Context   string `json:"context,omitempty"`
}

// AppendProgressEvent appends a single event to progress.jsonl.
// Timestamp is auto-set to now (UTC) if empty. Rotates file at progressMaxLines.
func AppendProgressEvent(path string, event *ProgressEvent) error {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	if err := rotateProgressIfNeeded(path, progressMaxLines); err != nil {
		return fmt.Errorf("rotate progress: %w", err)
	}

	line, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal progress event: %w", err)
	}
	line = append(line, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create progress dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open progress file: %w", err)
	}
	defer f.Close()

	_, err = f.Write(line)
	return err
}

// LoadProgressEvents reads all events from progress.jsonl.
// Returns (nil, nil) if the file does not exist.
func LoadProgressEvents(path string) ([]ProgressEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open progress file: %w", err)
	}
	defer f.Close()

	var events []ProgressEvent
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var event ProgressEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue // skip corrupt lines
		}
		events = append(events, event)
	}
	return events, scanner.Err()
}

// LastEventByType returns the most recent event matching eventType, or nil.
func LastEventByType(events []ProgressEvent, eventType ProgressEventType) *ProgressEvent {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Event == eventType {
			return &events[i]
		}
	}
	return nil
}

// QueryEventsByType returns all events matching eventType.
func QueryEventsByType(events []ProgressEvent, eventType ProgressEventType) []ProgressEvent {
	var result []ProgressEvent
	for _, e := range events {
		if e.Event == eventType {
			result = append(result, e)
		}
	}
	return result
}

// rotateProgressIfNeeded renames progress.jsonl to .1 when it reaches maxLines.
// Cascades existing archives (.1 to .2, etc.) up to .10.
func rotateProgressIfNeeded(path string, maxLines int) error {
	count, err := countFileLines(path)
	if err != nil || count < maxLines {
		return nil
	}
	for i := 9; i >= 1; i-- {
		old := fmt.Sprintf("%s.%d", path, i)
		next := fmt.Sprintf("%s.%d", path, i+1)
		if _, err := os.Stat(old); err == nil {
			os.Rename(old, next)
		}
	}
	return os.Rename(path, path+".1")
}

// countFileLines counts newlines in a file. Returns 0, err if file doesn't exist.
func countFileLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}

// AppendProgressMd appends a narrative section to progress.md.
// Sections are separated by --- lines.
func AppendProgressMd(path string, section string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create progress dir: %w", err)
	}

	info, statErr := os.Stat(path)
	needsSeparator := statErr == nil && info.Size() > 0

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open progress.md: %w", err)
	}
	defer f.Close()

	var buf strings.Builder
	if needsSeparator {
		buf.WriteString("\n---\n\n")
	}
	buf.WriteString(strings.TrimRight(section, "\n"))
	buf.WriteString("\n")

	_, err = f.WriteString(buf.String())
	return err
}

const noProgressHistory = "No previous execution history for this feature."

// GetProgressContext reads the most recent sections from progress.md
// for injection as {{progressContext}}. Returns last 3 sections,
// or last 2 if total exceeds ~4000 tokens (16KB).
func GetProgressContext(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return noProgressHistory
	}

	sections := splitProgressSections(string(data))
	if len(sections) == 0 {
		return noProgressHistory
	}

	n := 3
	if len(sections) < n {
		n = len(sections)
	}
	recent := sections[len(sections)-n:]
	result := strings.Join(recent, "\n\n---\n\n")

	if len(result) > 16384 && n > 2 {
		recent = sections[len(sections)-2:]
		result = strings.Join(recent, "\n\n---\n\n")
	}

	return result
}

// splitProgressSections splits progress.md content by --- separators.
func splitProgressSections(content string) []string {
	parts := strings.Split(content, "\n---\n")
	var sections []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			sections = append(sections, p)
		}
	}
	return sections
}
