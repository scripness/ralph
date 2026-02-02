package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

type LastResult struct {
	CompletedAt string `json:"completedAt"`
	Thread      string `json:"thread"`
	Commit      string `json:"commit"`
	Summary     string `json:"summary"`
}

type UserStory struct {
	ID                 string      `json:"id"`
	Title              string      `json:"title"`
	Description        string      `json:"description"`
	AcceptanceCriteria []string    `json:"acceptanceCriteria"`
	Priority           int         `json:"priority"`
	Passes             bool        `json:"passes"`
	Retries            int         `json:"retries"`
	LastResult         *LastResult `json:"lastResult"`
	Notes              string      `json:"notes"`
}

type Run struct {
	StartedAt      *string  `json:"startedAt"`
	CurrentStoryID *string  `json:"currentStoryId"`
	Learnings      []string `json:"learnings"`
}

type PRD struct {
	SchemaVersion int         `json:"schemaVersion"`
	Project       string      `json:"project"`
	BranchName    string      `json:"branchName"`
	Description   string      `json:"description"`
	Run           Run         `json:"run"`
	UserStories   []UserStory `json:"userStories"`
}

func loadPRD(path string) (*PRD, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("PRD not found: %s\n\nRun 'ralph init' to create one", path)
	}

	var prd PRD
	if err := json.Unmarshal(data, &prd); err != nil {
		return nil, fmt.Errorf("invalid JSON in prd.json: %w", err)
	}

	if err := validatePRD(&prd); err != nil {
		return nil, err
	}

	return &prd, nil
}

func validatePRD(prd *PRD) error {
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

func getNextStory(prd *PRD) *UserStory {
	var incomplete []UserStory
	for _, s := range prd.UserStories {
		if !s.Passes {
			incomplete = append(incomplete, s)
		}
	}

	if len(incomplete) == 0 {
		return nil
	}

	sort.Slice(incomplete, func(i, j int) bool {
		return incomplete[i].Priority < incomplete[j].Priority
	})

	return &incomplete[0]
}

func allStoriesComplete(prd *PRD) bool {
	for _, s := range prd.UserStories {
		if !s.Passes {
			return false
		}
	}
	return true
}
