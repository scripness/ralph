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
	os.MkdirAll(filepath.Join(dir, ".ralph"), 0755)

	lf := NewLockFile(dir)

	err := lf.Acquire("auth", "ralph/auth")
	if err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}

	// Lock file should exist
	lockPath := filepath.Join(dir, ".ralph", "ralph.lock")
	if !fileExists(lockPath) {
		t.Error("lock file should exist after acquire")
	}

	// Release
	err = lf.Release()
	if err != nil {
		t.Fatalf("failed to release lock: %v", err)
	}

	// Lock file should be gone
	if fileExists(lockPath) {
		t.Error("lock file should not exist after release")
	}
}

func TestLockFile_DoubleAcquireFails(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ralph"), 0755)

	lf1 := NewLockFile(dir)
	lf2 := NewLockFile(dir)

	err := lf1.Acquire("auth", "ralph/auth")
	if err != nil {
		t.Fatalf("failed to acquire first lock: %v", err)
	}
	defer lf1.Release()

	err = lf2.Acquire("billing", "ralph/billing")
	if err == nil {
		t.Error("expected error when acquiring second lock")
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
	os.MkdirAll(filepath.Join(dir, ".ralph"), 0755)

	lf := NewLockFile(dir)
	lf.Acquire("auth", "ralph/auth")
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
	if info.Branch != "ralph/auth" {
		t.Errorf("expected branch='ralph/auth', got '%s'", info.Branch)
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
		PID:       os.Getpid(), // current process, alive
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

// --- Scrip lock tests (coexist with ralph lock tests during transition) ---

func TestScripLockFile_AcquireRelease(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".scrip"), 0755)

	lf := NewScripLockFile(dir)

	err := lf.Acquire("auth", "scrip/auth")
	if err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}

	// Lock file should exist at .scrip/scrip.lock
	lockPath := filepath.Join(dir, ".scrip", "scrip.lock")
	if !fileExists(lockPath) {
		t.Error("scrip lock file should exist after acquire")
	}

	// Ralph lock should NOT exist
	ralphLockPath := filepath.Join(dir, ".ralph", "ralph.lock")
	if fileExists(ralphLockPath) {
		t.Error("ralph lock file should not exist when using scrip lock")
	}

	// Release
	err = lf.Release()
	if err != nil {
		t.Fatalf("failed to release lock: %v", err)
	}

	if fileExists(lockPath) {
		t.Error("scrip lock file should not exist after release")
	}
}

func TestScripLockFile_DoubleAcquireFails(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".scrip"), 0755)

	lf1 := NewScripLockFile(dir)
	lf2 := NewScripLockFile(dir)

	err := lf1.Acquire("auth", "scrip/auth")
	if err != nil {
		t.Fatalf("failed to acquire first lock: %v", err)
	}
	defer lf1.Release()

	err = lf2.Acquire("billing", "scrip/billing")
	if err == nil {
		t.Error("expected error when acquiring second scrip lock")
	}
}

func TestScripLockFile_ErrorMessagesSayScrip(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".scrip"), 0755)

	lf1 := NewScripLockFile(dir)
	lf2 := NewScripLockFile(dir)

	err := lf1.Acquire("auth", "scrip/auth")
	if err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}
	defer lf1.Release()

	err = lf2.Acquire("billing", "scrip/billing")
	if err == nil {
		t.Fatal("expected error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "scrip is already running") {
		t.Errorf("error message should contain 'scrip is already running', got: %s", errMsg)
	}
	if strings.Contains(errMsg, "ralph") {
		t.Errorf("error message should not contain 'ralph', got: %s", errMsg)
	}
}

func TestRalphLockFile_ErrorMessagesSayRalph(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ralph"), 0755)

	lf1 := NewLockFile(dir)
	lf2 := NewLockFile(dir)

	err := lf1.Acquire("auth", "ralph/auth")
	if err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}
	defer lf1.Release()

	err = lf2.Acquire("billing", "ralph/billing")
	if err == nil {
		t.Fatal("expected error")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "ralph is already running") {
		t.Errorf("error message should contain 'ralph is already running', got: %s", errMsg)
	}
}

func TestScripAndRalphLocksIndependent(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ralph"), 0755)
	os.MkdirAll(filepath.Join(dir, ".scrip"), 0755)

	ralphLock := NewLockFile(dir)
	scripLock := NewScripLockFile(dir)

	// Both should acquire independently
	err := ralphLock.Acquire("auth", "ralph/auth")
	if err != nil {
		t.Fatalf("failed to acquire ralph lock: %v", err)
	}
	defer ralphLock.Release()

	err = scripLock.Acquire("auth", "scrip/auth")
	if err != nil {
		t.Fatalf("failed to acquire scrip lock while ralph lock held: %v", err)
	}
	defer scripLock.Release()
}

func TestReadScripLockStatus_NoLock(t *testing.T) {
	dir := t.TempDir()

	info, err := ReadScripLockStatus(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Error("expected nil for no scrip lock")
	}
}

func TestReadScripLockStatus_WithLock(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".scrip"), 0755)

	lf := NewScripLockFile(dir)
	lf.Acquire("auth", "scrip/auth")
	defer lf.Release()

	info, err := ReadScripLockStatus(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("expected scrip lock info")
	}
	if info.Feature != "auth" {
		t.Errorf("expected feature='auth', got '%s'", info.Feature)
	}
	if info.Branch != "scrip/auth" {
		t.Errorf("expected branch='scrip/auth', got '%s'", info.Branch)
	}
	if info.PID != os.Getpid() {
		t.Errorf("expected PID=%d, got %d", os.Getpid(), info.PID)
	}
}

func TestScripLockFile_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	// Don't pre-create .scrip/ — Acquire should create it
	lf := NewScripLockFile(dir)

	err := lf.Acquire("auth", "scrip/auth")
	if err != nil {
		t.Fatalf("failed to acquire lock (should auto-create .scrip/): %v", err)
	}
	defer lf.Release()

	lockPath := filepath.Join(dir, ".scrip", "scrip.lock")
	if !fileExists(lockPath) {
		t.Error("scrip lock file should exist after acquire")
	}
}
