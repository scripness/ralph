package main

import (
	"reflect"
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

