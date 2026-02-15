package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// LastResult contains the result of a completed story
type LastResult struct {
	CompletedAt string `json:"completedAt"`
	Commit      string `json:"commit"`
	Summary     string `json:"summary"`
}

// BrowserStep represents a single browser verification step
type BrowserStep struct {
	Action   string `json:"action"`             // navigate, click, type, waitFor, assertVisible, assertText, assertNotVisible, submit, screenshot, wait
	URL      string `json:"url,omitempty"`      // for navigate
	Selector string `json:"selector,omitempty"` // CSS selector for click, type, waitFor, assert*
	Value    string `json:"value,omitempty"`    // for type action
	Contains string `json:"contains,omitempty"` // for assertText
	Timeout  int    `json:"timeout,omitempty"`  // seconds to wait (default 10)
}

// UserStory represents a single user story in the PRD
type UserStory struct {
	ID                 string        `json:"id"`
	Title              string        `json:"title"`
	Description        string        `json:"description"`
	AcceptanceCriteria []string      `json:"acceptanceCriteria"`
	Tags               []string      `json:"tags,omitempty"`
	Priority           int           `json:"priority"`
	Passes             bool          `json:"passes"`
	Retries            int           `json:"retries"`
	Blocked            bool          `json:"blocked"`
	LastResult         *LastResult   `json:"lastResult"`
	Notes              string        `json:"notes"`
	BrowserSteps       []BrowserStep `json:"browserSteps,omitempty"` // Interactive browser verification
}

// Run contains runtime state for the PRD
type Run struct {
	StartedAt      *string  `json:"startedAt"`
	CurrentStoryID *string  `json:"currentStoryId"`
	Learnings      []string `json:"learnings"`
}

// PRD represents the full PRD document
type PRD struct {
	SchemaVersion int         `json:"schemaVersion"`
	Project       string      `json:"project"`
	BranchName    string      `json:"branchName"`
	Description   string      `json:"description"`
	Run           Run         `json:"run"`
	UserStories   []UserStory `json:"userStories"`
}

// LoadPRD loads and validates a PRD from a file
func LoadPRD(path string) (*PRD, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("PRD not found: %s", path)
	}

	var prd PRD
	if err := json.Unmarshal(data, &prd); err != nil {
		return nil, fmt.Errorf("invalid JSON in prd.json: %w", err)
	}

	if err := ValidatePRD(&prd); err != nil {
		return nil, err
	}

	return &prd, nil
}

// SavePRD saves a PRD atomically
func SavePRD(path string, prd *PRD) error {
	return AtomicWriteJSON(path, prd)
}

// ValidatePRD validates a PRD structure
func ValidatePRD(prd *PRD) error {
	if prd.SchemaVersion != 2 {
		return fmt.Errorf("invalid schemaVersion: expected 2, got %d", prd.SchemaVersion)
	}
	if prd.Project == "" {
		return fmt.Errorf("missing required field: project")
	}
	if prd.BranchName == "" {
		return fmt.Errorf("missing required field: branchName")
	}
	if len(prd.UserStories) == 0 {
		return fmt.Errorf("userStories must have at least one story")
	}

	for i, story := range prd.UserStories {
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

// GetNextStory returns the next story to work on (not passed, not blocked, by priority).
// If CurrentStoryID is set (crash recovery), that story is returned first if still eligible.
// The returned pointer references the original element in prd.UserStories.
func GetNextStory(prd *PRD) *UserStory {
	// Crash recovery: prefer the story that was in progress when interrupted
	if prd.Run.CurrentStoryID != nil {
		for i := range prd.UserStories {
			s := &prd.UserStories[i]
			if s.ID == *prd.Run.CurrentStoryID && !s.Passes && !s.Blocked {
				return s
			}
		}
	}

	var indices []int
	for i, s := range prd.UserStories {
		if !s.Passes && !s.Blocked {
			indices = append(indices, i)
		}
	}

	if len(indices) == 0 {
		return nil
	}

	// Sort by priority (lower = higher priority)
	sort.Slice(indices, func(a, b int) bool {
		return prd.UserStories[indices[a]].Priority < prd.UserStories[indices[b]].Priority
	})

	return &prd.UserStories[indices[0]]
}

// GetPendingStories returns all stories that are neither passed nor blocked.
func GetPendingStories(prd *PRD) []UserStory {
	var pending []UserStory
	for _, s := range prd.UserStories {
		if !s.Passes && !s.Blocked {
			pending = append(pending, s)
		}
	}
	return pending
}

// GetStoryByID returns a story by its ID
func GetStoryByID(prd *PRD, id string) *UserStory {
	for i := range prd.UserStories {
		if prd.UserStories[i].ID == id {
			return &prd.UserStories[i]
		}
	}
	return nil
}

// AllStoriesComplete returns true if all non-blocked stories pass
func AllStoriesComplete(prd *PRD) bool {
	for _, s := range prd.UserStories {
		if !s.Passes && !s.Blocked {
			return false
		}
	}
	return true
}

// HasBlockedStories returns true if any stories are blocked
func HasBlockedStories(prd *PRD) bool {
	for _, s := range prd.UserStories {
		if s.Blocked {
			return true
		}
	}
	return false
}

// GetBlockedStories returns all blocked stories
func GetBlockedStories(prd *PRD) []UserStory {
	var blocked []UserStory
	for _, s := range prd.UserStories {
		if s.Blocked {
			blocked = append(blocked, s)
		}
	}
	return blocked
}

// IsUIStory returns true if the story has the "ui" tag
func IsUIStory(story *UserStory) bool {
	for _, tag := range story.Tags {
		if tag == "ui" {
			return true
		}
	}
	return false
}

// SetCurrentStory sets the current story being worked on
func (prd *PRD) SetCurrentStory(storyID string) {
	prd.Run.CurrentStoryID = &storyID
	if prd.Run.StartedAt == nil {
		now := time.Now().Format(time.RFC3339)
		prd.Run.StartedAt = &now
	}
}

// ClearCurrentStory clears the current story
func (prd *PRD) ClearCurrentStory() {
	prd.Run.CurrentStoryID = nil
}

// normalizeLearning normalizes a learning string for deduplication comparison.
// Trims whitespace and trailing punctuation so near-duplicates are caught.
func normalizeLearning(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, ".!,;:")
	return strings.ToLower(s)
}

// AddLearning adds a learning to the PRD, deduplicating with normalization.
// Comparison is case-insensitive and ignores trailing punctuation.
func (prd *PRD) AddLearning(learning string) {
	normalized := normalizeLearning(learning)
	for _, existing := range prd.Run.Learnings {
		if normalizeLearning(existing) == normalized {
			return
		}
	}
	prd.Run.Learnings = append(prd.Run.Learnings, learning)
}

// MarkStoryPassed marks a story as passed with result info.
// Clears Blocked flag to ensure no conflicting state.
func (prd *PRD) MarkStoryPassed(storyID, commit, summary string) {
	for i := range prd.UserStories {
		if prd.UserStories[i].ID == storyID {
			prd.UserStories[i].Passes = true
			prd.UserStories[i].Blocked = false
			prd.UserStories[i].LastResult = &LastResult{
				CompletedAt: time.Now().Format(time.RFC3339),
				Commit:      commit,
				Summary:     summary,
			}
			break
		}
	}
}

// MarkStoryFailed marks a story as failed, incrementing retries.
// Clears Passes and LastResult to ensure no conflicting state.
func (prd *PRD) MarkStoryFailed(storyID, notes string, maxRetries int) {
	for i := range prd.UserStories {
		if prd.UserStories[i].ID == storyID {
			prd.UserStories[i].Passes = false
			prd.UserStories[i].LastResult = nil
			prd.UserStories[i].Retries++
			prd.UserStories[i].Notes = notes
			if prd.UserStories[i].Retries >= maxRetries {
				prd.UserStories[i].Blocked = true
			}
			break
		}
	}
}

// ResetStory resets a story for re-implementation
func (prd *PRD) ResetStory(storyID, notes string, maxRetries int) {
	for i := range prd.UserStories {
		if prd.UserStories[i].ID == storyID {
			prd.UserStories[i].Passes = false
			prd.UserStories[i].LastResult = nil
			prd.UserStories[i].Retries++
			prd.UserStories[i].Notes = notes
			if prd.UserStories[i].Retries >= maxRetries {
				prd.UserStories[i].Blocked = true
			}
			break
		}
	}
}

// MarkStoryBlocked marks a story as blocked (provider explicitly blocked it).
// Clears Passes and LastResult to ensure no conflicting state.
func (prd *PRD) MarkStoryBlocked(storyID, notes string) {
	for i := range prd.UserStories {
		if prd.UserStories[i].ID == storyID {
			prd.UserStories[i].Blocked = true
			prd.UserStories[i].Passes = false
			prd.UserStories[i].LastResult = nil
			prd.UserStories[i].Notes = notes
			break
		}
	}
}

// ResetStoryForPreVerify resets a story to pending without incrementing retries.
// Used during pre-verification when a story no longer passes its checks.
// Unlike ResetStory (called from verify phase), this doesn't count as a failed attempt.
func (prd *PRD) ResetStoryForPreVerify(storyID, notes string) {
	for i := range prd.UserStories {
		if prd.UserStories[i].ID == storyID {
			prd.UserStories[i].Passes = false
			prd.UserStories[i].LastResult = nil
			prd.UserStories[i].Notes = notes
			break
		}
	}
}

// CountComplete returns the number of completed stories
func CountComplete(prd *PRD) int {
	count := 0
	for _, s := range prd.UserStories {
		if s.Passes {
			count++
		}
	}
	return count
}

// CountBlocked returns the number of blocked stories
func CountBlocked(prd *PRD) int {
	count := 0
	for _, s := range prd.UserStories {
		if s.Blocked {
			count++
		}
	}
	return count
}

// WarnPRDQuality returns warnings about PRD quality issues (not errors).
func WarnPRDQuality(prd *PRD) []string {
	var warnings []string
	for _, story := range prd.UserStories {
		hasTypecheck := false
		for _, criterion := range story.AcceptanceCriteria {
			if strings.Contains(strings.ToLower(criterion), "typecheck") {
				hasTypecheck = true
				break
			}
		}
		if !hasTypecheck {
			warnings = append(warnings, fmt.Sprintf("%s: missing 'Typecheck passes' criterion", story.ID))
		}
	}
	return warnings
}
