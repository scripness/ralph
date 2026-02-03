package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWriteJSON writes JSON data atomically using temp file + rename
func AtomicWriteJSON(path string, data any) error {
	// Marshal to JSON with indentation
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Add trailing newline
	jsonData = append(jsonData, '\n')

	return AtomicWriteFile(path, jsonData)
}

// AtomicWriteFile writes data atomically using temp file + rename
func AtomicWriteFile(path string, data []byte) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write to temp file in same directory (for atomic rename)
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Validate JSON if it's a .json file
	if filepath.Ext(path) == ".json" {
		var js json.RawMessage
		if err := json.Unmarshal(data, &js); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("invalid JSON: %w", err)
		}
	}

	// Atomic rename (on POSIX systems)
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}
