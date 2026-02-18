package main

import (
	"testing"
)

func TestCapturedOutput_Write(t *testing.T) {
	co := &capturedOutput{maxBytes: 1024}

	n, err := co.Write([]byte("hello world\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 12 {
		t.Errorf("expected 12 bytes written, got %d", n)
	}
	if co.String() != "hello world\n" {
		t.Errorf("expected 'hello world\\n', got '%s'", co.String())
	}
}

func TestCapturedOutput_Truncation(t *testing.T) {
	co := &capturedOutput{maxBytes: 100}

	// Write more than maxBytes
	for i := 0; i < 20; i++ {
		co.Write([]byte("1234567890"))
	}

	output := co.String()
	if len(output) > 100 {
		t.Errorf("expected output <= 100 bytes, got %d", len(output))
	}
	if len(output) == 0 {
		t.Error("expected some output preserved, got empty")
	}
}

func TestServiceManager_GetRecentOutput(t *testing.T) {
	sm := NewServiceManager("/tmp", nil)

	// No output yet
	output := sm.GetRecentOutput("nonexistent", 10)
	if output != "" {
		t.Errorf("expected empty for nonexistent service, got '%s'", output)
	}

	// Add a captured output manually
	co := &capturedOutput{maxBytes: 1024}
	co.Write([]byte("line1\nline2\nline3\nline4\nline5\n"))
	sm.outputs["test"] = co

	// Get last 3 lines
	output = sm.GetRecentOutput("test", 3)
	lines := 0
	for _, c := range output {
		if c == '\n' {
			lines++
		}
	}
	// With trailing newline, split gives ["line3", "line4", "line5", ""]
	// so getting 3 lines should limit the output
	if len(output) == 0 {
		t.Error("expected some output, got empty")
	}
}

func TestServiceManager_HasServices(t *testing.T) {
	smWith := NewServiceManager("/tmp", []ServiceConfig{{Name: "dev"}})
	if !smWith.HasServices() {
		t.Error("expected HasServices=true with services")
	}

	smWithout := NewServiceManager("/tmp", nil)
	if smWithout.HasServices() {
		t.Error("expected HasServices=false without services")
	}
}

func TestServiceManager_HasUIServices(t *testing.T) {
	smUI := NewServiceManager("/tmp", []ServiceConfig{
		{Name: "dev", RestartBeforeVerify: true},
	})
	if !smUI.HasUIServices() {
		t.Error("expected HasUIServices=true with RestartBeforeVerify")
	}

	smNoUI := NewServiceManager("/tmp", []ServiceConfig{
		{Name: "dev", RestartBeforeVerify: false},
	})
	if smNoUI.HasUIServices() {
		t.Error("expected HasUIServices=false without RestartBeforeVerify")
	}
}

func TestServiceManager_CheckServiceHealth_NoServices(t *testing.T) {
	sm := NewServiceManager("/tmp", nil)
	issues := sm.CheckServiceHealth()
	if len(issues) != 0 {
		t.Errorf("expected no issues for empty service manager, got %v", issues)
	}
}
