# Ralph

Autonomous AI agent loop for implementing PRD stories with any AI provider.

Ralph orchestrates AI coding agents (Amp, Claude Code, OpenCode, etc.) in an infinite loop until all PRD stories are complete and verified. Provider-agnostic design with deterministic verification.

## Install

```bash
# One-line install (Linux/macOS)
curl -fsSL https://raw.githubusercontent.com/scripness/ralph/main/install.sh | bash

# Or with Go
go install github.com/scripness/ralph@latest
```

## Quick Start

```bash
# Initialize Ralph in your project
ralph init

# Create a PRD for a feature
ralph prd auth

# Run the loop (runs forever until complete)
ralph run auth

# Check status anytime
ralph status auth
```

## Key Features

- **Provider-agnostic**: Works with any AI CLI (amp, claude, opencode)
- **Idempotent**: Stop anytime, run again to resume
- **Multi-feature**: Work on multiple features in parallel
- **Deterministic verification**: Ralph runs all QA gates
- **Service management**: Auto-starts dev servers for UI verification

## The Flow

```
ralph init              Create ralph.config.json + .ralph/
  ↓
ralph prd auth          Brainstorm → Draft prd.md → Finalize prd.json
  ↓
ralph run auth          Infinite loop: implement → verify → repeat
  ↓
  ├── Story complete? → Mark passes=true → Next story
  ├── Verification failed? → Retry (up to maxRetries)
  ├── Max retries? → Mark blocked → Skip to next
  └── All done? → Final verification → VERIFIED or RESET
```

## How It Works

1. Ralph loads `.ralph/*-[feature]/prd.json`
2. Creates/switches to `ralph/{feature}` branch
3. Picks next story (highest priority, not passed, not blocked)
4. Sets `currentStoryId` in prd.json (crash recovery)
5. Sends prompt to provider subprocess
6. Provider implements code, writes tests, commits
7. Provider outputs `<ralph>DONE</ralph>` when finished
8. Ralph runs `verify.default` commands
9. If UI story: restarts services, runs `verify.ui` commands
10. Pass → mark story complete, next story
11. Fail → increment retries, retry or block
12. All stories pass → final verification prompt
13. Provider outputs `<ralph>VERIFIED</ralph>` or `<ralph>RESET:US-001,US-003</ralph>`
14. Complete → "Ready to merge"

## Commands

```bash
# Core
ralph init                 # Initialize Ralph (creates config + .ralph/)
ralph prd <feature>        # Create/refine/finalize a PRD
ralph run <feature>        # Run the agent loop (infinite until done)
ralph verify <feature>     # Run verification only

# Status
ralph status               # Show all features
ralph status <feature>     # Show specific feature status
ralph next <feature>       # Show next story to work on
ralph validate <feature>   # Validate prd.json schema

# Utility
ralph doctor               # Check environment
ralph upgrade              # Update to latest version
```

## File Structure

```
project/
├── ralph.config.json          # Project configuration (required)
└── .ralph/
    ├── 2024-01-15-auth/
    │   ├── prd.md             # Human-readable PRD
    │   └── prd.json           # Finalized for execution
    ├── 2024-01-20-billing/
    │   ├── prd.md
    │   └── prd.json
    ├── screenshots/           # Browser verification evidence
    └── ralph.lock             # Prevents concurrent runs
```

## Configuration

`ralph.config.json`:

```json
{
  "maxRetries": 3,
  "provider": {
    "command": "amp",
    "args": ["--dangerously-allow-all"],
    "timeout": 1800
  },
  "services": [
    {
      "name": "dev",
      "start": "bun run dev",
      "ready": "http://localhost:3000",
      "readyTimeout": 30,
      "restartBeforeVerify": true
    }
  ],
  "verify": {
    "default": [
      "bun run typecheck",
      "bun run lint",
      "bun run test:unit"
    ],
    "ui": [
      "bun run test:e2e"
    ]
  }
}
```

## Provider Signals

Provider communicates with Ralph via markers in stdout:

| Signal | Purpose |
|--------|---------|
| `<ralph>DONE</ralph>` | Story implementation complete |
| `<ralph>LEARNING:text</ralph>` | Add learning for future context |
| `<ralph>VERIFIED</ralph>` | Final verification passed |
| `<ralph>RESET:US-001,US-003</ralph>` | Reset specific stories |
| `<ralph>REASON:explanation</ralph>` | Reason for reset |

## prd.json Schema (v2)

```json
{
  "schemaVersion": 2,
  "project": "ProjectName",
  "branchName": "ralph/feature-name",
  "description": "Feature description",
  "run": {
    "startedAt": "2024-01-15T10:00:00Z",
    "currentStoryId": null,
    "learnings": []
  },
  "userStories": [{
    "id": "US-001",
    "title": "Story title",
    "description": "As a user, I want...",
    "acceptanceCriteria": ["Criterion 1", "Criterion 2"],
    "tags": ["ui"],
    "priority": 1,
    "passes": false,
    "retries": 0,
    "blocked": false,
    "lastResult": null,
    "notes": ""
  }]
}
```

## Story Tags

| Tag | Effect |
|-----|--------|
| `ui` | Restarts services before verify, runs `verify.ui` commands |

## Idempotent Workflow

Ralph is designed for interruption:

```bash
ralph run auth          # Working on US-003...
^C                      # Stop (Ctrl+C)

# Later...
ralph run auth          # Resumes from US-003 automatically
```

State is preserved in prd.json:
- `currentStoryId`: Story being worked on
- `passes`: Whether story is complete
- `retries`: Number of failed attempts
- `blocked`: Story exceeded maxRetries

## Example Configs

### Amp (Sourcegraph)
```json
{
  "provider": {
    "command": "amp",
    "args": ["--dangerously-allow-all"]
  }
}
```

### Claude Code
```json
{
  "provider": {
    "command": "claude",
    "args": ["--print"]
  }
}
```

### OpenCode
```json
{
  "provider": {
    "command": "opencode",
    "args": []
  }
}
```

## Build from Source

```bash
git clone https://github.com/scripness/ralph
cd ralph
go build -ldflags="-s -w" -o ralph .
```

## Testing

```bash
go test ./...
```

## Credits

Inspired by [snarktank/ralph](https://github.com/snarktank/ralph). v2.0.0 is a complete Go rewrite with provider-agnostic architecture.

## License

MIT
