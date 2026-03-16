package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadSessionState(t *testing.T) {
	t.Run("nonexistent file returns nil", func(t *testing.T) {
		state, err := LoadSessionState(filepath.Join(t.TempDir(), "state.json"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if state != nil {
			t.Fatal("expected nil state for nonexistent file")
		}
	})

	t.Run("valid state loads correctly", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "state.json")

		state := NewSessionState("Build auth", 2, "scrip-exec")
		state.SetProvider(os.Getpid())

		if err := SaveSessionState(path, state); err != nil {
			t.Fatalf("save failed: %v", err)
		}

		loaded, err := LoadSessionState(path)
		if err != nil {
			t.Fatalf("load failed: %v", err)
		}
		if loaded.CurrentItem != "Build auth" {
			t.Errorf("expected CurrentItem 'Build auth', got %q", loaded.CurrentItem)
		}
		if loaded.CurrentAttempt != 2 {
			t.Errorf("expected CurrentAttempt 2, got %d", loaded.CurrentAttempt)
		}
		if loaded.LockHolder != "scrip-exec" {
			t.Errorf("expected LockHolder 'scrip-exec', got %q", loaded.LockHolder)
		}
		if loaded.Version != 1 {
			t.Errorf("expected Version 1, got %d", loaded.Version)
		}
		if loaded.ProviderPID != os.Getpid() {
			t.Errorf("expected ProviderPID %d, got %d", os.Getpid(), loaded.ProviderPID)
		}
	})

	t.Run("corrupt file returns error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "state.json")
		// AtomicWriteJSON validates JSON, so write directly to bypass it
		os.WriteFile(path, []byte("not json"), 0644)

		_, err := LoadSessionState(path)
		if err == nil {
			t.Fatal("expected error for corrupt file")
		}
	})
}

func TestSaveSessionState(t *testing.T) {
	t.Run("increments version on each save", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "state.json")

		state := NewSessionState("item1", 1, "scrip-exec")
		if state.Version != 0 {
			t.Fatalf("expected initial version 0, got %d", state.Version)
		}

		if err := SaveSessionState(path, state); err != nil {
			t.Fatalf("first save failed: %v", err)
		}
		if state.Version != 1 {
			t.Errorf("expected version 1 after first save, got %d", state.Version)
		}

		if err := SaveSessionState(path, state); err != nil {
			t.Fatalf("second save failed: %v", err)
		}
		if state.Version != 2 {
			t.Errorf("expected version 2 after second save, got %d", state.Version)
		}
	})

	t.Run("creates parent directories", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "nested", "dir", "state.json")

		state := NewSessionState("item", 1, "scrip-exec")
		if err := SaveSessionState(path, state); err != nil {
			t.Fatalf("save to nested path failed: %v", err)
		}

		loaded, err := LoadSessionState(path)
		if err != nil {
			t.Fatalf("load from nested path failed: %v", err)
		}
		if loaded.CurrentItem != "item" {
			t.Errorf("expected CurrentItem 'item', got %q", loaded.CurrentItem)
		}
	})
}

func TestDeleteSessionState(t *testing.T) {
	t.Run("deletes existing file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "state.json")
		os.WriteFile(path, []byte("{}"), 0644)

		if err := DeleteSessionState(path); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("file should be deleted")
		}
	})

	t.Run("no error for nonexistent file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nonexistent.json")
		if err := DeleteSessionState(path); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestIsProviderAlive(t *testing.T) {
	t.Run("nil state returns false", func(t *testing.T) {
		if IsProviderAlive(nil) {
			t.Error("expected false for nil state")
		}
	})

	t.Run("zero PID returns false", func(t *testing.T) {
		state := &SessionState{ProviderPID: 0}
		if IsProviderAlive(state) {
			t.Error("expected false for zero PID")
		}
	})

	t.Run("current process is alive", func(t *testing.T) {
		state := &SessionState{
			ProviderPID:       os.Getpid(),
			ProviderStartedAt: time.Now().Unix(),
		}
		if !IsProviderAlive(state) {
			t.Error("expected current process to be alive")
		}
	})

	t.Run("dead PID returns false", func(t *testing.T) {
		// Use a very high PID that's almost certainly not running
		state := &SessionState{
			ProviderPID:       4194304, // max PID on most Linux systems
			ProviderStartedAt: time.Now().Unix(),
		}
		if IsProviderAlive(state) {
			t.Error("expected dead PID to return false")
		}
	})

	t.Run("stale start time returns false even if PID alive", func(t *testing.T) {
		state := &SessionState{
			ProviderPID:       os.Getpid(),
			ProviderStartedAt: time.Now().Add(-25 * time.Hour).Unix(),
		}
		if IsProviderAlive(state) {
			t.Error("expected stale start time to return false")
		}
	})

	t.Run("zero start time returns false", func(t *testing.T) {
		state := &SessionState{
			ProviderPID:       os.Getpid(),
			ProviderStartedAt: 0,
		}
		if IsProviderAlive(state) {
			t.Error("expected false for zero start time")
		}
	})
}

func TestNewSessionState(t *testing.T) {
	state := NewSessionState("Build auth", 3, "scrip-exec")

	if state.CurrentItem != "Build auth" {
		t.Errorf("expected CurrentItem 'Build auth', got %q", state.CurrentItem)
	}
	if state.CurrentAttempt != 3 {
		t.Errorf("expected CurrentAttempt 3, got %d", state.CurrentAttempt)
	}
	if state.LockHolder != "scrip-exec" {
		t.Errorf("expected LockHolder 'scrip-exec', got %q", state.LockHolder)
	}
	if state.StartedAt == "" {
		t.Error("expected StartedAt to be set")
	}
	// Verify RFC3339 format
	_, err := time.Parse(time.RFC3339, state.StartedAt)
	if err != nil {
		t.Errorf("StartedAt is not RFC3339: %v", err)
	}
	if state.Version != 0 {
		t.Errorf("expected initial Version 0, got %d", state.Version)
	}
	if state.ProviderPID != 0 {
		t.Errorf("expected initial ProviderPID 0, got %d", state.ProviderPID)
	}
}

func TestSetProvider(t *testing.T) {
	state := NewSessionState("item", 1, "scrip-exec")
	before := time.Now().Unix()
	state.SetProvider(12345)
	after := time.Now().Unix()

	if state.ProviderPID != 12345 {
		t.Errorf("expected PID 12345, got %d", state.ProviderPID)
	}
	if state.ProviderStartedAt < before || state.ProviderStartedAt > after {
		t.Errorf("expected ProviderStartedAt between %d and %d, got %d",
			before, after, state.ProviderStartedAt)
	}
}

func TestClearProvider(t *testing.T) {
	state := NewSessionState("item", 1, "scrip-exec")
	state.SetProvider(12345)

	state.ClearProvider()

	if state.ProviderPID != 0 {
		t.Errorf("expected PID 0 after clear, got %d", state.ProviderPID)
	}
	if state.ProviderStartedAt != 0 {
		t.Errorf("expected ProviderStartedAt 0 after clear, got %d", state.ProviderStartedAt)
	}
}

func TestSessionStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	original := NewSessionState("OAuth2 setup", 1, "scrip-exec")
	original.SetProvider(os.Getpid())

	if err := SaveSessionState(path, original); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := LoadSessionState(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.CurrentItem != original.CurrentItem {
		t.Errorf("CurrentItem mismatch: %q vs %q", loaded.CurrentItem, original.CurrentItem)
	}
	if loaded.CurrentAttempt != original.CurrentAttempt {
		t.Errorf("CurrentAttempt mismatch: %d vs %d", loaded.CurrentAttempt, original.CurrentAttempt)
	}
	if loaded.ProviderPID != original.ProviderPID {
		t.Errorf("ProviderPID mismatch: %d vs %d", loaded.ProviderPID, original.ProviderPID)
	}
	if loaded.ProviderStartedAt != original.ProviderStartedAt {
		t.Errorf("ProviderStartedAt mismatch: %d vs %d", loaded.ProviderStartedAt, original.ProviderStartedAt)
	}
	if loaded.StartedAt != original.StartedAt {
		t.Errorf("StartedAt mismatch: %q vs %q", loaded.StartedAt, original.StartedAt)
	}
	if loaded.LockHolder != original.LockHolder {
		t.Errorf("LockHolder mismatch: %q vs %q", loaded.LockHolder, original.LockHolder)
	}
	if loaded.Version != original.Version {
		t.Errorf("Version mismatch: %d vs %d", loaded.Version, original.Version)
	}
}
