package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestStripNonInteractiveArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "strips --print",
			args: []string{"--print", "--dangerously-skip-permissions"},
			want: []string{"--dangerously-skip-permissions"},
		},
		{
			name: "strips -p shorthand",
			args: []string{"-p", "--dangerously-skip-permissions"},
			want: []string{"--dangerously-skip-permissions"},
		},
		{
			name: "no non-interactive flags",
			args: []string{"--dangerously-allow-all"},
			want: []string{"--dangerously-allow-all"},
		},
		{
			name: "empty args",
			args: []string{},
			want: []string{},
		},
		{
			name: "nil args",
			args: nil,
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripNonInteractiveArgs(tt.args)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("stripNonInteractiveArgs(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestPrdEditManual_EditorNotFound(t *testing.T) {
	t.Setenv("EDITOR", "nonexistent-editor-xyz123")

	err := prdEditManual("/dev/null")
	if err == nil {
		t.Fatal("expected error when editor is not found")
	}
	if !strings.Contains(err.Error(), "not found in PATH") {
		t.Errorf("expected 'not found in PATH' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "nonexistent-editor-xyz123") {
		t.Errorf("expected editor name in error, got: %v", err)
	}
}
