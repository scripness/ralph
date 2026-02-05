package main

import (
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// CleanupCoordinator manages graceful cleanup of resources during signal handling.
// Resources register themselves when created, and the coordinator ensures they
// are cleaned up properly when signals are received, even when os.Exit() is called.
type CleanupCoordinator struct {
	mu       sync.Mutex
	svcMgr   *ServiceManager
	provider *exec.Cmd
	logger   *RunLogger
	lock     *LockFile
	done     bool
}

// NewCleanupCoordinator creates a new cleanup coordinator.
func NewCleanupCoordinator() *CleanupCoordinator {
	return &CleanupCoordinator{}
}

// SetServiceManager registers the service manager for cleanup.
func (c *CleanupCoordinator) SetServiceManager(sm *ServiceManager) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.svcMgr = sm
}

// SetProvider registers the current provider process for cleanup.
func (c *CleanupCoordinator) SetProvider(cmd *exec.Cmd) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.provider = cmd
}

// ClearProvider unregisters the provider process after it completes.
func (c *CleanupCoordinator) ClearProvider() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.provider = nil
}

// SetLogger registers the run logger for cleanup.
func (c *CleanupCoordinator) SetLogger(l *RunLogger) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logger = l
}

// SetLock registers the lock file for cleanup.
func (c *CleanupCoordinator) SetLock(lf *LockFile) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lock = lf
}

// Cleanup performs graceful cleanup of all registered resources.
// Safe to call multiple times (idempotent).
func (c *CleanupCoordinator) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.done {
		return
	}
	c.done = true

	// Kill provider process group first (fast, prevents more output)
	if c.provider != nil && c.provider.Process != nil {
		syscall.Kill(-c.provider.Process.Pid, syscall.SIGTERM)
		time.Sleep(500 * time.Millisecond)
		syscall.Kill(-c.provider.Process.Pid, syscall.SIGKILL)
	}

	// Stop services (may take up to 5 seconds due to SIGTERM+wait)
	if c.svcMgr != nil {
		c.svcMgr.StopAll()
		c.svcMgr = nil // Prevent double-stop from defer
	}

	// Log and close
	if c.logger != nil {
		c.logger.RunEnd(false, "interrupted by signal")
		c.logger.Close()
	}

	// Release lock last
	if c.lock != nil {
		c.lock.Release()
	}
}
