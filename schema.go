package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// BrowserStep represents a single browser verification step
type BrowserStep struct {
	Action   string `json:"action"`             // navigate, click, type, waitFor, assertVisible, assertText, assertNotVisible, submit, screenshot, wait
	URL      string `json:"url,omitempty"`      // for navigate
	Selector string `json:"selector,omitempty"` // CSS selector for click, type, waitFor, assert*
	Value    string `json:"value,omitempty"`    // for type action
	Contains string `json:"contains,omitempty"` // for assertText
	Timeout  int    `json:"timeout,omitempty"`  // seconds to wait (default 10)
}

// --- v3 definition types (on-disk format, AI-authored, immutable during runs) ---

// PRDDefinition is the v3 on-disk format for prd.json (no runtime fields).
type PRDDefinition struct {
	SchemaVersion int               `json:"schemaVersion"` // 3
	Project       string            `json:"project"`
	BranchName    string            `json:"branchName"`
	Description   string            `json:"description"`
	UserStories   []StoryDefinition `json:"userStories"`
}

// StoryDefinition contains only AI-authored story fields.
type StoryDefinition struct {
	ID                 string        `json:"id"`
	Title              string        `json:"title"`
	Description        string        `json:"description"`
	AcceptanceCriteria []string      `json:"acceptanceCriteria"`
	Tags               []string      `json:"tags,omitempty"`
	Priority           int           `json:"priority"`
	BrowserSteps       []BrowserStep `json:"browserSteps,omitempty"`
}

// --- Flat execution state (on-disk, CLI-managed) ---

// RunState is the flat execution state file written exclusively by the CLI.
type RunState struct {
	Passed      []string          `json:"passed"`
	Skipped     []string          `json:"skipped"`
	Retries     map[string]int    `json:"retries,omitempty"`
	LastFailure map[string]string `json:"lastFailure,omitempty"`
	Learnings   []string          `json:"learnings,omitempty"`
}

// NewRunState creates an empty RunState.
func NewRunState() *RunState {
	return &RunState{
		Passed:      []string{},
		Skipped:     []string{},
		Retries:     make(map[string]int),
		LastFailure: make(map[string]string),
	}
}

// IsPassed returns true if the story has passed verification.
func (s *RunState) IsPassed(id string) bool {
	for _, p := range s.Passed {
		if p == id {
			return true
		}
	}
	return false
}

// IsSkipped returns true if the story was skipped (exceeded retries or provider blocked it).
func (s *RunState) IsSkipped(id string) bool {
	for _, sk := range s.Skipped {
		if sk == id {
			return true
		}
	}
	return false
}

// MarkPassed marks a story as passed. Removes from Skipped if present.
func (s *RunState) MarkPassed(id string) {
	if s.IsPassed(id) {
		return
	}
	s.Passed = append(s.Passed, id)
	// Remove from skipped if it was there
	s.removeFromSkipped(id)
}

// MarkFailed records a failed attempt. Increments retries and auto-skips at threshold.
func (s *RunState) MarkFailed(id, reason string, maxRetries int) {
	if s.Retries == nil {
		s.Retries = make(map[string]int)
	}
	if s.LastFailure == nil {
		s.LastFailure = make(map[string]string)
	}
	s.Retries[id]++
	if reason != "" {
		s.LastFailure[id] = reason
	}
	// Remove from passed if it was there (regression)
	s.removeFromPassed(id)
	// Auto-skip at threshold
	if s.Retries[id] >= maxRetries {
		if !s.IsSkipped(id) {
			s.Skipped = append(s.Skipped, id)
		}
	}
}

// MarkSkipped explicitly skips a story (e.g., exceeded maxRetries).
func (s *RunState) MarkSkipped(id, reason string) {
	if s.IsSkipped(id) {
		return
	}
	s.Skipped = append(s.Skipped, id)
	s.removeFromPassed(id)
	if reason != "" {
		if s.LastFailure == nil {
			s.LastFailure = make(map[string]string)
		}
		s.LastFailure[id] = reason
	}
}

// UnmarkPassed removes a story from passed (e.g., regression detected by verify-at-top).
// Does NOT increment retries.
func (s *RunState) UnmarkPassed(id string) {
	s.removeFromPassed(id)
}

// GetRetries returns the retry count for a story (0 if never retried).
func (s *RunState) GetRetries(id string) int {
	return s.Retries[id]
}

// GetLastFailure returns the last failure reason for a story ("" if none).
func (s *RunState) GetLastFailure(id string) string {
	return s.LastFailure[id]
}

// normalizeLearning normalizes a learning string for deduplication comparison.
func normalizeLearning(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, ".!,;:")
	return strings.ToLower(s)
}

// AddLearning adds a learning, deduplicating with normalization.
func (s *RunState) AddLearning(learning string) {
	normalized := normalizeLearning(learning)
	for _, existing := range s.Learnings {
		if normalizeLearning(existing) == normalized {
			return
		}
	}
	s.Learnings = append(s.Learnings, learning)
}

func (s *RunState) removeFromPassed(id string) {
	for i, p := range s.Passed {
		if p == id {
			s.Passed = append(s.Passed[:i], s.Passed[i+1:]...)
			return
		}
	}
}

func (s *RunState) removeFromSkipped(id string) {
	for i, sk := range s.Skipped {
		if sk == id {
			s.Skipped = append(s.Skipped[:i], s.Skipped[i+1:]...)
			return
		}
	}
}

// --- Query functions (take definition + state pair) ---

// GetNextStory returns the next story to work on (not passed, not skipped, by priority).
func GetNextStory(def *PRDDefinition, state *RunState) *StoryDefinition {
	var indices []int
	for i, s := range def.UserStories {
		if !state.IsPassed(s.ID) && !state.IsSkipped(s.ID) {
			indices = append(indices, i)
		}
	}

	if len(indices) == 0 {
		return nil
	}

	// Sort by priority (lower = higher priority)
	sort.Slice(indices, func(a, b int) bool {
		return def.UserStories[indices[a]].Priority < def.UserStories[indices[b]].Priority
	})

	return &def.UserStories[indices[0]]
}

// GetPendingStories returns all stories that are neither passed nor skipped.
func GetPendingStories(def *PRDDefinition, state *RunState) []StoryDefinition {
	var pending []StoryDefinition
	for _, s := range def.UserStories {
		if !state.IsPassed(s.ID) && !state.IsSkipped(s.ID) {
			pending = append(pending, s)
		}
	}
	return pending
}

// AllComplete returns true if every story is either passed or skipped.
func AllComplete(def *PRDDefinition, state *RunState) bool {
	for _, s := range def.UserStories {
		if !state.IsPassed(s.ID) && !state.IsSkipped(s.ID) {
			return false
		}
	}
	return true
}

// CountPassed returns the number of passed stories.
func CountPassed(state *RunState) int {
	return len(state.Passed)
}

// CountSkipped returns the number of skipped stories.
func CountSkipped(state *RunState) int {
	return len(state.Skipped)
}

// IsUIStory returns true if the story has the "ui" tag.
func IsUIStory(story *StoryDefinition) bool {
	for _, tag := range story.Tags {
		if tag == "ui" {
			return true
		}
	}
	return false
}

// GetStoryByID returns a story by its ID from a definition.
func GetStoryByID(def *PRDDefinition, id string) *StoryDefinition {
	for i := range def.UserStories {
		if def.UserStories[i].ID == id {
			return &def.UserStories[i]
		}
	}
	return nil
}

// --- Load/Save/Validate helpers ---

// LoadPRDDefinition loads and validates a v3 PRD definition from disk.
func LoadPRDDefinition(path string) (*PRDDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("PRD not found: %s", path)
	}

	var def PRDDefinition
	if err := json.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("invalid JSON in prd.json: %w", err)
	}

	if err := ValidatePRDDefinition(&def); err != nil {
		return nil, err
	}

	return &def, nil
}

// ValidatePRDDefinition validates a v3 PRD definition.
func ValidatePRDDefinition(def *PRDDefinition) error {
	if def.SchemaVersion != 3 {
		return fmt.Errorf("invalid schemaVersion: expected 3, got %d", def.SchemaVersion)
	}
	if def.Project == "" {
		return fmt.Errorf("missing required field: project")
	}
	if def.BranchName == "" {
		return fmt.Errorf("missing required field: branchName")
	}
	if len(def.UserStories) == 0 {
		return fmt.Errorf("userStories must have at least one story")
	}
	for i, story := range def.UserStories {
		if story.ID == "" {
			return fmt.Errorf("userStories[%d]: missing id", i)
		}
		if story.Title == "" {
			return fmt.Errorf("userStories[%d]: missing title", i)
		}
		if len(story.AcceptanceCriteria) == 0 {
			return fmt.Errorf("userStories[%d]: missing acceptanceCriteria", i)
		}
	}
	return nil
}

// LoadRunState loads execution state from disk. Returns empty state if file doesn't exist.
// Auto-detects old v3 format (with "stories" key) and migrates.
func LoadRunState(path string) (*RunState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewRunState(), nil
		}
		return nil, fmt.Errorf("failed to read run-state.json: %w", err)
	}

	// Detect old format: has "stories" key with object value
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON in run-state.json: %w", err)
	}

	if _, hasStories := raw["stories"]; hasStories {
		return migrateOldRunState(data)
	}

	// New flat format
	var state RunState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("invalid JSON in run-state.json: %w", err)
	}
	if state.Passed == nil {
		state.Passed = []string{}
	}
	if state.Skipped == nil {
		state.Skipped = []string{}
	}
	if state.Retries == nil {
		state.Retries = make(map[string]int)
	}
	if state.LastFailure == nil {
		state.LastFailure = make(map[string]string)
	}
	return &state, nil
}

// migrateOldRunState converts old v3 run-state.json (with "stories" map) to new flat format.
func migrateOldRunState(data []byte) (*RunState, error) {
	var old struct {
		Learnings []string `json:"learnings"`
		Stories   map[string]struct {
			Passes  bool   `json:"passes"`
			Retries int    `json:"retries"`
			Blocked bool   `json:"blocked"`
			Notes   string `json:"notes"`
		} `json:"stories"`
	}
	if err := json.Unmarshal(data, &old); err != nil {
		return nil, fmt.Errorf("failed to parse old run-state.json: %w", err)
	}

	state := NewRunState()
	state.Learnings = old.Learnings
	for id, ss := range old.Stories {
		if ss.Passes {
			state.Passed = append(state.Passed, id)
		}
		if ss.Blocked {
			state.Skipped = append(state.Skipped, id)
		}
		if ss.Retries > 0 {
			state.Retries[id] = ss.Retries
		}
		if ss.Notes != "" {
			state.LastFailure[id] = ss.Notes
		}
	}
	// Sort for deterministic output
	sort.Strings(state.Passed)
	sort.Strings(state.Skipped)
	return state, nil
}

// SaveRunState writes execution state atomically.
func SaveRunState(path string, state *RunState) error {
	return AtomicWriteJSON(path, state)
}
