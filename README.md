# @scripness/ralph

Autonomous AI agent loop for implementing PRD stories with [Amp](https://ampcode.com).

Ralph runs Amp in a loop until all PRD stories are complete. Each iteration runs with fresh context, using git and prd.json as the single source of truth.

## Quick Start

```bash
# Install globally
npm install -g @scripness/ralph

# Or use with bunx/npx
bunx @scripness/ralph init

# Initialize in your project
ralph init

# Create a PRD interactively
ralph prd

# Convert PRD markdown to prd.json
ralph convert tasks/prd-feature.md

# Start the agent loop
ralph run
```

## The Flow

```
ralph prd              Create tasks/prd-[feature].md interactively
  ↓                    (asks clarifying questions, writes user stories)
ralph convert          Convert to scripts/ralph/prd.json
  ↓                    (generates v2 schema with all fields)
ralph run              Run loop → auto-verify → fix → repeat until done
```

## How It Works

1. Ralph reads `scripts/ralph/prd.json` for user stories
2. Picks highest-priority story where `passes: false`
3. Sets `run.currentStoryId` (crash recovery)
4. Implements/verifies that story via Amp
5. Runs quality checks (auto-detected from project type)
6. Commits changes with prd.json updates
7. Clears `currentStoryId`, sets `passes: true`
8. Repeats until all stories pass → outputs `<promise>COMPLETE</promise>`
9. Auto-runs verification
10. If issues found, resets stories and loops back
11. Exits "Ready to merge" when fully verified

## Commands

```bash
ralph init              # Initialize Ralph in your project
ralph prd [name]        # Create a PRD document interactively
ralph convert <file>    # Convert PRD markdown to prd.json
ralph run [iterations]  # Run the main loop (default: 10)
ralph run --no-verify   # Skip auto-verification after completion
ralph verify            # Run verification only
ralph status            # Show PRD story status
ralph next              # Show next story to work on
ralph validate          # Validate prd.json schema
ralph doctor            # Check Ralph environment
```

## Project Detection

Ralph auto-detects your project type and configures quality commands:

| Project Type | Detection | Quality Commands |
|--------------|-----------|------------------|
| Bun | `bun.lock` | typecheck, lint, test from package.json |
| Node | `package.json` | typecheck, lint, test from package.json |
| Rust | `Cargo.toml` | cargo check, clippy, test |
| Go | `go.mod` | go build, vet, test |

## Configuration

Create `ralph.config.json` in your project root to override defaults:

```json
{
  "prdPath": "scripts/ralph/prd.json",
  "iterations": 20,
  "verify": true,
  "quality": [
    { "name": "typecheck", "cmd": "bun run typecheck" },
    { "name": "test", "cmd": "bun run test" },
    { "name": "lint", "cmd": "bun run lint" }
  ],
  "amp": {
    "command": "amp",
    "args": ["--dangerously-allow-all"]
  }
}
```

## prd.json Schema (v2)

```json
{
  "schemaVersion": 2,
  "project": "ProjectName",
  "branchName": "ralph/feature-name",
  "description": "Feature description",
  "run": {
    "startedAt": "ISO timestamp",
    "currentStoryId": null,
    "learnings": ["patterns discovered this run"]
  },
  "userStories": [{
    "id": "US-001",
    "title": "Story title",
    "description": "As a...",
    "acceptanceCriteria": ["..."],
    "priority": 1,
    "passes": false,
    "retries": 0,
    "lastResult": null,
    "notes": ""
  }]
}
```

### Story Fields

| Field | Purpose |
|-------|---------|
| `passes` | `true` when story is complete |
| `retries` | Incremented by verification when resetting |
| `lastResult` | `{completedAt, thread, commit, summary}` on completion |
| `notes` | Blocker notes (cleared on success) |

### Run Fields

| Field | Purpose |
|-------|---------|
| `currentStoryId` | Story in progress (crash recovery) |
| `learnings` | Patterns discovered this run |

## Crash Recovery

If Ralph crashes mid-story:
1. `run.currentStoryId` contains the story ID
2. Next run detects dirty git tree + currentStoryId set
3. Agent resumes that story, verifying partial work

## Architecture

Ralph is fully self-contained. It embeds all agent instructions directly in the prompts sent to Amp - no external skills installation required. This means:

- Single install: just `npm install -g @scripness/ralph`
- No pollution of Amp's skills directory
- Instructions versioned with the CLI
- Works immediately after install

## License

MIT
