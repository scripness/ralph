package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLockFile_AcquireRelease(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".scrip"), 0755)

	lf := NewLockFile(dir)

	err := lf.Acquire("auth", "plan/auth")
	if err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}

	// Lock file should exist at .scrip/scrip.lock
	lockPath := filepath.Join(dir, ".scrip", "scrip.lock")
	if !fileExists(lockPath) {
		t.Error("lock file should exist after acquire")
	}

	// Release
	err = lf.Release()
	if err != nil {
		t.Fatalf("failed to release lock: %v", err)
	}

	if fileExists(lockPath) {
		t.Error("lock file should not exist after release")
	}
}

func TestLockFile_DoubleAcquireFails(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".scrip"), 0755)

	lf1 := NewLockFile(dir)
	lf2 := NewLockFile(dir)

	err := lf1.Acquire("auth", "plan/auth")
	if err != nil {
		t.Fatalf("failed to acquire first lock: %v", err)
	}
	defer lf1.Release()

	err = lf2.Acquire("billing", "plan/billing")
	if err == nil {
		t.Error("expected error when acquiring second lock")
	}
}

func TestLockFile_ErrorMessagesSayScrip(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".scrip"), 0755)

	lf1 := NewLockFile(dir)
	lf2 := NewLockFile(dir)

	err := lf1.Acquire("auth", "plan/auth")
	if err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}
	defer lf1.Release()

	err = lf2.Acquire("billing", "plan/billing")
	if err == nil {
		t.Fatal("expected error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "scrip is already running") {
		t.Errorf("error message should contain 'scrip is already running', got: %s", errMsg)
	}
}

func TestReadLockStatus_NoLock(t *testing.T) {
	dir := t.TempDir()

	info, err := ReadLockStatus(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Error("expected nil for no lock")
	}
}

func TestReadLockStatus_WithLock(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".scrip"), 0755)

	lf := NewLockFile(dir)
	lf.Acquire("auth", "plan/auth")
	defer lf.Release()

	info, err := ReadLockStatus(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected lock info")
	}
	if info.Feature != "auth" {
		t.Errorf("expected feature='auth', got '%s'", info.Feature)
	}
	if info.Branch != "plan/auth" {
		t.Errorf("expected branch='plan/auth', got '%s'", info.Branch)
	}
	if info.PID != os.Getpid() {
		t.Errorf("expected PID=%d, got %d", os.Getpid(), info.PID)
	}
}

func TestIsLockStale_DeadProcess(t *testing.T) {
	info := &LockInfo{
		PID:       999999, // almost certainly not running
		StartedAt: time.Now(),
	}
	if !isLockStale(info) {
		t.Error("expected stale for dead process")
	}
}

func TestIsLockStale_AliveButOld(t *testing.T) {
	info := &LockInfo{
		PID:       os.Getpid(),                        // current process, alive
		StartedAt: time.Now().Add(-25 * time.Hour), // older than maxLockAge
	}
	if !isLockStale(info) {
		t.Error("expected stale for alive but old lock")
	}
}

func TestIsLockStale_AliveAndRecent(t *testing.T) {
	info := &LockInfo{
		PID:       os.Getpid(), // current process, alive
		StartedAt: time.Now(),  // recent
	}
	if isLockStale(info) {
		t.Error("expected not stale for alive and recent lock")
	}
}

func TestLockFile_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	// Don't pre-create .scrip/ — Acquire should create it
	lf := NewLockFile(dir)

	err := lf.Acquire("auth", "plan/auth")
	if err != nil {
		t.Fatalf("failed to acquire lock (should auto-create .scrip/): %v", err)
	}
	defer lf.Release()

	lockPath := filepath.Join(dir, ".scrip", "scrip.lock")
	if !fileExists(lockPath) {
		t.Error("lock file should exist after acquire")
	}
}
