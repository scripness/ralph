package main

import (
	"testing"
)

func TestCleanupCoordinatorIdempotent(t *testing.T) {
	// CleanupCoordinator.Cleanup() should be safe to call multiple times
	c := NewCleanupCoordinator()

	// First call should work
	c.Cleanup()

	// Second call should not panic or cause issues
	c.Cleanup()

	// Third call for good measure
	c.Cleanup()
}

func TestCleanupCoordinatorWithNilResources(t *testing.T) {
	// CleanupCoordinator should handle nil resources gracefully
	c := NewCleanupCoordinator()

	// Should not panic with no resources registered
	c.Cleanup()
}

func TestCleanupCoordinatorSettersWithNil(t *testing.T) {
	c := NewCleanupCoordinator()

	// Setting nil values should not panic
	c.SetServiceManager(nil)
	c.SetProvider(nil)
	c.SetLogger(nil)
	c.SetLock(nil)

	// Cleanup with nil values should not panic
	c.Cleanup()
}

func TestCleanupCoordinatorClearProvider(t *testing.T) {
	c := NewCleanupCoordinator()

	// Set and clear provider should not panic
	c.SetProvider(nil)
	c.ClearProvider()

	// Cleanup after clearing should work
	c.Cleanup()
}

func TestNewCleanupCoordinator(t *testing.T) {
	c := NewCleanupCoordinator()
	if c == nil {
		t.Fatal("NewCleanupCoordinator returned nil")
	}
}
