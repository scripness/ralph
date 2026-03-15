package main

import (
	"testing"
)

func TestMergeWithExisting(t *testing.T) {
	tests := []struct {
		name     string
		detected *ScripConfig
		existing *ScripConfig
		want     *ScripConfig
	}{
		{
			name: "empty existing config — detected values win",
			detected: &ScripConfig{
				Verify: ScripVerifyConfig{
					Typecheck: "npx tsc --noEmit",
					Lint:      "npx eslint .",
					Test:      "npx vitest run",
				},
			},
			existing: &ScripConfig{},
			want: &ScripConfig{
				Verify: ScripVerifyConfig{
					Typecheck: "npx tsc --noEmit",
					Lint:      "npx eslint .",
					Test:      "npx vitest run",
				},
			},
		},
		{
			name: "existing verify commands preserved over detection",
			detected: &ScripConfig{
				Verify: ScripVerifyConfig{
					Typecheck: "npx tsc --noEmit",
					Lint:      "npx eslint .",
					Test:      "npx vitest run",
				},
			},
			existing: &ScripConfig{
				Verify: ScripVerifyConfig{
					Typecheck: "npx tsc --noEmit --strict",
					Lint:      "npx eslint --fix .",
					Test:      "npx jest",
				},
			},
			want: &ScripConfig{
				Verify: ScripVerifyConfig{
					Typecheck: "npx tsc --noEmit --strict",
					Lint:      "npx eslint --fix .",
					Test:      "npx jest",
				},
			},
		},
		{
			name: "existing services preserved",
			detected: &ScripConfig{
				Verify: ScripVerifyConfig{Test: "go test ./..."},
			},
			existing: &ScripConfig{
				Services: []ScripServiceConfig{
					{Name: "api", Command: "go run ./cmd/api", Ready: "http://localhost:8080/health"},
				},
			},
			want: &ScripConfig{
				Verify:   ScripVerifyConfig{Test: "go test ./..."},
				Services: []ScripServiceConfig{{Name: "api", Command: "go run ./cmd/api", Ready: "http://localhost:8080/health"}},
			},
		},
		{
			name: "non-default provider timeouts preserved",
			detected: &ScripConfig{
				Verify: ScripVerifyConfig{Test: "go test ./..."},
			},
			existing: &ScripConfig{
				Provider: ScripProviderConfig{
					Timeout:      3600,
					StallTimeout: 600,
				},
			},
			want: &ScripConfig{
				Provider: ScripProviderConfig{
					Timeout:      3600,
					StallTimeout: 600,
				},
				Verify: ScripVerifyConfig{Test: "go test ./..."},
			},
		},
		{
			name: "default provider timeouts overwritten by detection",
			detected: &ScripConfig{
				Verify: ScripVerifyConfig{Test: "go test ./..."},
			},
			existing: &ScripConfig{
				Provider: ScripProviderConfig{
					Timeout:      1800, // default
					StallTimeout: 300,  // default
				},
			},
			want: &ScripConfig{
				Verify: ScripVerifyConfig{Test: "go test ./..."},
			},
		},
		{
			name: "partial existing — only set fields preserved",
			detected: &ScripConfig{
				Verify: ScripVerifyConfig{
					Typecheck: "npx tsc",
					Test:      "npx vitest run",
				},
			},
			existing: &ScripConfig{
				Verify: ScripVerifyConfig{
					Test: "npm test",
					// Typecheck and Lint empty — detection wins for Typecheck
				},
			},
			want: &ScripConfig{
				Verify: ScripVerifyConfig{
					Typecheck: "npx tsc",
					Test:      "npm test",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mergeWithExisting(tt.detected, tt.existing)

			// Check verify commands
			if tt.detected.Verify.Typecheck != tt.want.Verify.Typecheck {
				t.Errorf("Verify.Typecheck = %q, want %q", tt.detected.Verify.Typecheck, tt.want.Verify.Typecheck)
			}
			if tt.detected.Verify.Lint != tt.want.Verify.Lint {
				t.Errorf("Verify.Lint = %q, want %q", tt.detected.Verify.Lint, tt.want.Verify.Lint)
			}
			if tt.detected.Verify.Test != tt.want.Verify.Test {
				t.Errorf("Verify.Test = %q, want %q", tt.detected.Verify.Test, tt.want.Verify.Test)
			}

			// Check services
			if len(tt.detected.Services) != len(tt.want.Services) {
				t.Errorf("Services length = %d, want %d", len(tt.detected.Services), len(tt.want.Services))
			} else {
				for i := range tt.detected.Services {
					if tt.detected.Services[i] != tt.want.Services[i] {
						t.Errorf("Services[%d] = %+v, want %+v", i, tt.detected.Services[i], tt.want.Services[i])
					}
				}
			}

			// Check provider timeouts
			if tt.detected.Provider.Timeout != tt.want.Provider.Timeout {
				t.Errorf("Provider.Timeout = %d, want %d", tt.detected.Provider.Timeout, tt.want.Provider.Timeout)
			}
			if tt.detected.Provider.StallTimeout != tt.want.Provider.StallTimeout {
				t.Errorf("Provider.StallTimeout = %d, want %d", tt.detected.Provider.StallTimeout, tt.want.Provider.StallTimeout)
			}
		})
	}
}
