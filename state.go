package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// SessionState represents the runtime checkpoint file (state.json).
// This is temporary — deleted on clean exit. Used only for crash recovery.
// progress.jsonl is the source of truth; state.json is a hint.
type SessionState struct {
	Version           int    `json:"version"`
	CurrentItem       string `json:"current_item"`
	CurrentAttempt    int    `json:"current_attempt"`
	ProviderPID       int    `json:"provider_pid"`
	ProviderStartedAt int64  `json:"provider_started_at"`
	StartedAt         string `json:"started_at"`
	LockHolder        string `json:"lock_holder"`
}

// NewSessionState creates a new SessionState for the given item and lock holder.
func NewSessionState(item string, attempt int, lockHolder string) *SessionState {
	return &SessionState{
		CurrentItem:    item,
		CurrentAttempt: attempt,
		StartedAt:      time.Now().UTC().Format(time.RFC3339),
		LockHolder:     lockHolder,
	}
}

// SetProvider records the provider PID and start time in the state.
func (s *SessionState) SetProvider(pid int) {
	s.ProviderPID = pid
	s.ProviderStartedAt = time.Now().Unix()
}

// ClearProvider resets provider tracking fields (called after provider exits).
func (s *SessionState) ClearProvider() {
	s.ProviderPID = 0
	s.ProviderStartedAt = 0
}

// LoadSessionState reads state.json from the given path.
// Returns (nil, nil) if the file does not exist.
func LoadSessionState(path string) (*SessionState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read state.json: %w", err)
	}

	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("corrupt state.json: %w", err)
	}

	return &state, nil
}

// SaveSessionState writes state.json atomically, incrementing the version.
func SaveSessionState(path string, state *SessionState) error {
	state.Version++
	return AtomicWriteJSON(path, state)
}

// DeleteSessionState removes state.json. No error if file doesn't exist.
func DeleteSessionState(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// IsProviderAlive checks if the provider process recorded in state is still running.
// Returns false if PID is dead or if the recorded start time is stale (>24h),
// which guards against PID reuse by the OS.
func IsProviderAlive(state *SessionState) bool {
	if state == nil || state.ProviderPID == 0 {
		return false
	}

	if !isProcessAlive(state.ProviderPID) {
		return false
	}

	// Guard against PID reuse: reject if start time is missing or stale
	if state.ProviderStartedAt == 0 {
		return false
	}
	age := time.Since(time.Unix(state.ProviderStartedAt, 0))
	return age < maxLockAge
}
