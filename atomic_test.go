package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	err := AtomicWriteFile(path, []byte("hello world"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(content) != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", string(content))
	}

	// Temp file should not exist
	tmpPath := path + ".tmp"
	if fileExists(tmpPath) {
		t.Error("temp file should not exist after atomic write")
	}
}

func TestAtomicWriteFile_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "deep", "test.txt")

	err := AtomicWriteFile(path, []byte("nested"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(content) != "nested" {
		t.Errorf("expected 'nested', got '%s'", string(content))
	}
}

func TestAtomicWriteJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	data := map[string]string{"key": "value"}
	err := AtomicWriteJSON(path, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	expected := "{\n  \"key\": \"value\"\n}\n"
	if string(content) != expected {
		t.Errorf("expected '%s', got '%s'", expected, string(content))
	}
}

func TestAtomicWriteFile_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	// Writing invalid JSON to a .json file should fail validation
	err := AtomicWriteFile(path, []byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}

	// File should not exist
	if fileExists(path) {
		t.Error("file should not exist after failed atomic write")
	}
}
