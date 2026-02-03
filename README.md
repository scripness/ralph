# Ralph v2

Autonomous AI agent loop for implementing PRD stories with any AI provider.

Ralph orchestrates AI coding agents (Amp, Claude Code, OpenCode, etc.) in an infinite loop until all PRD stories are complete and verified. Provider-agnostic design with deterministic verification.

> **Note**: This is Ralph v2, a complete Go rewrite of the original [snarktank/ralph](https://github.com/snarktank/ralph) bash-based loop. See [Background](#background) for the evolution from v1 to v2.

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

- **Provider-agnostic**: Works with any AI CLI (amp, claude, opencode, aider, etc.)
- **Readiness enforcement**: Hard-fails before run if QA commands are missing, placeholder, or not in PATH
- **Idempotent**: Stop anytime, resume exactly where you left off
- **Multi-feature**: Work on multiple features in parallel with date-prefixed directories
- **Deterministic verification**: CLI runs all QA gates, not the AI
- **Service management**: Auto-starts dev servers, captures output, checks health during verification
- **Interactive browser verification**: Real user simulation with rod (click, type, assert, auto-downloads Chromium); console errors are hard failures
- **Crash recovery**: Tracks `currentStoryId` to resume interrupted stories
- **Learning deduplication**: Cross-iteration memory preserved on all code paths (including timeouts and errors)
- **Atomic operations**: Lock file prevents concurrent runs, atomic JSON writes
- **Auto-update notifications**: Checks for new versions in the background, notifies on exit

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
2. **Readiness check**: refuses to run if verify commands are placeholder or binaries missing from PATH
3. Creates/switches to `ralph/{feature}` branch
4. Picks next story (highest priority, not passed, not blocked)
5. Sets `currentStoryId` in prd.json (crash recovery)
6. Sends prompt to provider subprocess (includes deduplicated learnings)
7. Provider implements code, writes tests, commits
8. Provider outputs `<ralph>DONE</ralph>` when finished (learnings saved even on timeout/error)
9. Ralph runs `verify.default` commands
10. If UI story: restarts services, runs browser verification (console errors = hard fail), runs `verify.ui` commands
11. Service health check: verifies services still respond
12. Pass → mark story complete, next story
13. Fail → increment retries, retry or block
14. All stories pass → final verification (verify commands + browser checks for all UI stories + service health) → provider reviews → `VERIFIED` or `RESET`
15. Complete → "Ready to merge"

## Commands

```bash
# Core
ralph init [--force]       # Initialize Ralph (creates config + .ralph/)
ralph prd <feature>        # Create/refine/finalize a PRD
ralph run <feature>        # Run the agent loop (infinite until done)
ralph verify <feature>     # Run verification only

# Status
ralph status               # Show all features
ralph status <feature>     # Show specific feature status
ralph next <feature>       # Show next story to work on
ralph validate <feature>   # Validate prd.json schema

# Utility
ralph doctor               # Check environment + readiness
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
    "command": "amp"
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

### Provider Settings

| Field | Description | Default |
|-------|-------------|---------|
| `command` | Provider CLI command | (required) |
| `args` | Arguments to pass | Auto-detected |
| `timeout` | Seconds per iteration | `1800` |
| `promptMode` | How to pass prompt: `stdin`, `arg`, `file` | Auto-detected |
| `promptFlag` | Flag before prompt in arg/file modes (e.g. `--message`) | Auto-detected |
| `knowledgeFile` | Knowledge file name: `AGENTS.md`, `CLAUDE.md` | Auto-detected |

Only `command` is required. Everything else is auto-detected from the provider name.

**Auto-detection by provider:**

| Provider | args | promptMode | promptFlag | knowledgeFile |
|----------|------|-----------|------------|---------------|
| `amp` | `--dangerously-allow-all` | stdin | | AGENTS.md |
| `claude` | `--print --dangerously-skip-permissions` | stdin | | CLAUDE.md |
| `opencode` | `run` | arg | | AGENTS.md |
| `aider` | `--yes-always` | arg | `--message` | AGENTS.md |
| `codex` | `exec --full-auto` | arg | | AGENTS.md |
| other | | stdin | | AGENTS.md |

Setting `"args": []` explicitly opts out of default args.

## Provider Signals

Provider communicates with Ralph via markers in stdout or stderr:

| Signal | Purpose |
|--------|---------|
| `<ralph>DONE</ralph>` | Story implementation complete (runs verification) |
| `<ralph>STUCK</ralph>` | Provider can't proceed (counts as failed attempt) |
| `<ralph>BLOCK:US-001</ralph>` | Mark story as blocked (comma-separated for multiple: `BLOCK:US-001,US-003`) |
| `<ralph>LEARNING:text</ralph>` | Add learning for future context |
| `<ralph>SUGGEST_NEXT:US-003</ralph>` | Advisory: suggest next story (optional) |
| `<ralph>VERIFIED</ralph>` | Final verification passed |
| `<ralph>RESET:US-001,US-003</ralph>` | Reset stories for reimplementation |
| `<ralph>REASON:explanation</ralph>` | Explanation for STUCK/BLOCK/RESET |

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
    "notes": "",
    "browserSteps": []
  }]
}
```

## Story Tags

| Tag | Effect |
|-----|--------|
| `ui` | Restarts services before verify, runs `verify.ui` commands, runs browser verification |

## Browser Verification

For UI stories, Ralph can run interactive browser verification like a real user. Define `browserSteps` in the story:

```json
{
  "id": "US-003",
  "title": "Login form works",
  "tags": ["ui"],
  "browserSteps": [
    {"action": "navigate", "url": "/login"},
    {"action": "type", "selector": "#email", "value": "test@example.com"},
    {"action": "type", "selector": "#password", "value": "secret"},
    {"action": "click", "selector": "button[type=submit]"},
    {"action": "waitFor", "selector": ".dashboard"},
    {"action": "assertText", "selector": "h1", "contains": "Welcome"}
  ]
}
```

**Available actions:**

| Action | Fields | Description |
|--------|--------|-------------|
| `navigate` | `url` | Go to URL (relative or absolute) |
| `click` | `selector` | Click an element |
| `type` | `selector`, `value` | Type text into input |
| `waitFor` | `selector` | Wait for element visible |
| `assertVisible` | `selector` | Assert element exists |
| `assertText` | `selector`, `contains` | Assert element has text |
| `assertNotVisible` | `selector` | Assert element hidden |
| `submit` | `selector` | Click and wait for navigation |
| `screenshot` | - | Capture screenshot |
| `wait` | `timeout` | Wait N seconds |

All steps support optional `timeout` (seconds, default 10).

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

Only `command` is needed — args, promptMode, promptFlag, and knowledgeFile are all auto-detected.

### Amp (Sourcegraph)
```json
{
  "provider": { "command": "amp" }
}
```

### Claude Code
```json
{
  "provider": { "command": "claude" }
}
```

### OpenCode
```json
{
  "provider": { "command": "opencode" }
}
```

### Aider
```json
{
  "provider": { "command": "aider" }
}
```

### Codex
```json
{
  "provider": { "command": "codex" }
}
```

## Updating

Ralph checks for updates in the background and notifies you when a new version is available:

```
A new version of ralph is available: v2.1.0 (current: v2.0.0)
Run 'ralph upgrade' to update.
```

To update:
```bash
ralph upgrade
```

## Build from Source

```bash
git clone https://github.com/scripness/ralph
cd ralph
make build
```

## Testing

```bash
make test
```

## Releasing

Releases are automated via [GoReleaser](https://goreleaser.com/) and GitHub Actions.

```bash
# Patch release from local (requires gh CLI)
make release

# Or from GitHub UI: Actions → Release → Run workflow → pick bump type
```

The workflow auto-bumps the version tag, runs tests, builds 4 binaries (linux/darwin x amd64/arm64), and creates a GitHub Release.

## Background

Ralph v2 is a complete rewrite of the original [snarktank/ralph](https://github.com/snarktank/ralph) (the "Ralph pattern" by Geoffrey Huntley).

### Original Ralph (v1)

The original Ralph was a bash script (`ralph.sh`) that:
- Spawned fresh AI instances (Amp or Claude Code) in a loop
- Used `prompt.md` / `CLAUDE.md` for agent instructions
- Stored memory in `progress.txt` (append-only log)
- Relied on the agent to manage story state in `prd.json`
- Had fixed iteration limits (default 10)
- Required provider-specific skills (dev-browser, etc.)

### Ralph v2 (this repo)

v2 is a Go CLI that improves on v1:

| Aspect | v1 (bash) | v2 (Go CLI) |
|--------|-----------|-------------|
| **Architecture** | Bash script | Compiled Go binary |
| **Provider support** | Amp or Claude only | Any AI CLI (provider-agnostic) |
| **Story selection** | Agent decides | CLI decides (deterministic) |
| **State management** | Agent updates prd.json | CLI manages all state |
| **Memory** | progress.txt file | Learnings in prd.json |
| **Iteration limit** | Fixed (default 10) | Infinite until verified |
| **Multi-feature** | Manual archive/switch | Built-in date-prefixed dirs |
| **Crash recovery** | None | currentStoryId tracking |
| **Verification** | Agent runs commands | CLI runs commands |
| **Browser testing** | Provider skill (dev-browser) | Built-in rod (auto-downloads Chromium) |
| **Service management** | None | Built-in start/ready/restart |
| **Concurrency** | None | Lock file prevents conflicts |

### The Ralph Pattern

The core idea remains the same:
1. Break work into small, atomic user stories
2. Run AI agents in a loop, one story at a time
3. Verify each story with automated tests
4. Persist learnings for future iterations
5. Continue until all stories pass verification

For more on the pattern, see [ghuntley.com/ralph/](https://ghuntley.com/ralph/).

## License

MIT
