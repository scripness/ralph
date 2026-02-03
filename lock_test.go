package main

import (
	"os"
	"path/filepath"
	"testing"
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
