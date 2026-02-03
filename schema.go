package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

// LastResult contains the result of a completed story
type LastResult struct {
	CompletedAt string `json:"completedAt"`
	Commit      string `json:"commit"`
	Summary     string `json:"summary"`
}

// UserStory represents a single user story in the PRD
type UserStory struct {
	ID                 string      `json:"id"`
	Title              string      `json:"title"`
	Description        string      `json:"description"`
	AcceptanceCriteria []string    `json:"acceptanceCriteria"`
	Tags               []string    `json:"tags,omitempty"`
	Priority           int         `json:"priority"`
	Passes             bool        `json:"passes"`
	Retries            int         `json:"retries"`
	Blocked            bool        `json:"blocked"`
	LastResult         *LastResult `json:"lastResult"`
	Notes              string      `json:"notes"`
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

// GetNextStory returns the next story to work on (not passed, not blocked, by priority)
func GetNextStory(prd *PRD) *UserStory {
	var incomplete []UserStory
	for _, s := range prd.UserStories {
		if !s.Passes && !s.Blocked {
			incomplete = append(incomplete, s)
		}
	}

	if len(incomplete) == 0 {
		return nil
	}

	// Sort by priority (lower = higher priority)
	sort.Slice(incomplete, func(i, j int) bool {
		return incomplete[i].Priority < incomplete[j].Priority
	})

	return &incomplete[0]
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

// AddLearning adds a learning to the PRD
func (prd *PRD) AddLearning(learning string) {
	prd.Run.Learnings = append(prd.Run.Learnings, learning)
}

// MarkStoryPassed marks a story as passed with result info
func (prd *PRD) MarkStoryPassed(storyID, commit, summary string) {
	for i := range prd.UserStories {
		if prd.UserStories[i].ID == storyID {
			prd.UserStories[i].Passes = true
			prd.UserStories[i].LastResult = &LastResult{
				CompletedAt: time.Now().Format(time.RFC3339),
				Commit:      commit,
				Summary:     summary,
			}
			break
		}
	}
}

// MarkStoryFailed marks a story as failed, incrementing retries
func (prd *PRD) MarkStoryFailed(storyID, notes string, maxRetries int) {
	for i := range prd.UserStories {
		if prd.UserStories[i].ID == storyID {
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
