package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// LockInfo contains information about the current lock
type LockInfo struct {
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"startedAt"`
	Feature   string    `json:"feature"`
	Branch    string    `json:"branch"`
}

// LockFile manages the global ralph.lock file
type LockFile struct {
	path string
	info *LockInfo
}

// NewLockFile creates a new lock file manager
func NewLockFile(projectRoot string) *LockFile {
	return &LockFile{
		path: filepath.Join(projectRoot, ".ralph", "ralph.lock"),
	}
}

// Acquire attempts to acquire the lock atomically
func (lf *LockFile) Acquire(feature, branch string) error {
	// Ensure .ralph directory exists
	if err := os.MkdirAll(filepath.Dir(lf.path), 0755); err != nil {
		return fmt.Errorf("failed to create .ralph directory: %w", err)
	}

	// Check if lock exists and handle stale locks
	if lf.isHeld() {
		existing, err := lf.readLock()
		if err != nil {
			// Lock file exists but can't be read - try to remove it
			os.Remove(lf.path)
		} else if isLockStale(existing) {
			// Stale lock - remove it
			fmt.Printf("Removing stale lock (PID %d no longer running or lock too old)\n", existing.PID)
			if err := os.Remove(lf.path); err != nil {
				return fmt.Errorf("failed to remove stale lock: %w", err)
			}
		} else {
			return fmt.Errorf("ralph is already running (PID %d, feature: %s)\nStarted at: %s",
				existing.PID, existing.Feature, existing.StartedAt.Format(time.RFC3339))
		}
	}

	// Create lock atomically using O_CREATE|O_EXCL
	lf.info = &LockInfo{
		PID:       os.Getpid(),
		StartedAt: time.Now(),
		Feature:   feature,
		Branch:    branch,
	}

	data, err := json.MarshalIndent(lf.info, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal lock info: %w", err)
	}
	data = append(data, '\n')

	// O_CREATE|O_EXCL ensures atomic creation - fails if file already exists
	f, err := os.OpenFile(lf.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if os.IsExist(err) {
			// Another process created the lock between our check and create
			return fmt.Errorf("ralph is already running (lock acquired by another process)")
		}
		return fmt.Errorf("failed to create lock file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		os.Remove(lf.path)
		return fmt.Errorf("failed to write lock file: %w", err)
	}

	return nil
}

// Release releases the lock
func (lf *LockFile) Release() error {
	if lf.info == nil {
		return nil
	}

	// Only remove if we own it
	existing, err := lf.readLock()
	if err != nil {
		// Lock doesn't exist or can't be read - that's fine
		return nil
	}

	if existing.PID != os.Getpid() {
		// Someone else owns it now
		return nil
	}

	return os.Remove(lf.path)
}

// isHeld checks if the lock file exists
func (lf *LockFile) isHeld() bool {
	_, err := os.Stat(lf.path)
	return err == nil
}

// readLock reads the lock file
func (lf *LockFile) readLock() (*LockInfo, error) {
	data, err := os.ReadFile(lf.path)
	if err != nil {
		return nil, err
	}

	var info LockInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}

	return &info, nil
}

// isProcessAlive checks if a process with the given PID is still running
func isProcessAlive(pid int) bool {
	// Try to find the process
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds, so we need to send signal 0
	// to check if process exists
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// maxLockAge is the maximum age of a lock before it's considered stale,
// even if the process is still alive. Guards against PID reuse.
const maxLockAge = 24 * time.Hour

// isLockStale returns true if the lock should be considered stale.
// A lock is stale if the owning process is dead, or if the lock is older
// than maxLockAge (guards against PID reuse by the OS).
func isLockStale(info *LockInfo) bool {
	if !isProcessAlive(info.PID) {
		return true
	}
	return time.Since(info.StartedAt) > maxLockAge
}

// ReadLockStatus reads the current lock status without acquiring
func ReadLockStatus(projectRoot string) (*LockInfo, error) {
	lf := NewLockFile(projectRoot)
	if !lf.isHeld() {
		return nil, nil
	}
	return lf.readLock()
}

