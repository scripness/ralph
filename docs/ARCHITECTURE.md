# Ralph CLI Architecture Plan

> Living document for the agent-agnostic Ralph CLI refactor.

## Vision

Ralph is a standalone CLI that orchestrates any AI provider (Amp, Claude Code, OpenCode, etc.) in an autonomous loop to implement PRD stories with rigorous verification.

**Core principles:**
- 100% provider-agnostic (works with any AI CLI)
- No magic detection (explicit configuration)
- Ralph owns state (prd.json mutations)
- Deterministic verification (Ralph runs all QA gates)
- Infinite loop until verified complete

---

## File Locations (Fixed)

| File | Purpose | Committed |
|------|---------|-----------|
| `ralph.config.json` | Project configuration | ✅ Yes |
| `.ralph/` | Feature work directory | ✅ Yes |

### Feature Directory Structure

Each feature gets its own timestamped directory:

```
ralph.config.json                    # Project config (root)
.ralph/
├── 2024-01-15-auth/
│   ├── prd.md                       # Human-readable PRD
│   └── prd.json                     # Finalized for execution
├── 2024-01-20-billing/
│   ├── prd.md
│   └── prd.json
├── screenshots/                     # Browser verification evidence
└── .gitignore                       # Ignores locks, screenshots
```

### Feature Matching

Commands require explicit feature name:

```bash
ralph prd auth       # Creates/uses .ralph/*-auth/
ralph run auth       # Runs .ralph/*-auth/prd.json
```

- Matches by suffix (e.g., `auth` matches `2024-01-15-auth`)
- Multiple matches → uses most recent (by datetime prefix)
- No match → creates new with today's date

---

## Architecture

### Three Hard Boundaries

```
┌─────────────────────────────────────────────────────────────┐
│                         RALPH CLI                           │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────┐ │
│  │  ORCHESTRATOR   │  │  AGENT RUNNER   │  │  VERIFIER   │ │
│  │                 │  │                 │  │             │ │
│  │ - State machine │  │ - Subprocess    │  │ - Commands  │ │
│  │ - Story select  │  │ - Prompt in     │  │ - Services  │ │
│  │ - prd.json ops  │  │ - Text out      │  │ - Gates     │ │
│  │ - Git ops       │  │ - Agent-agnostic│  │ - Enforce   │ │
│  │ - Crash recovery│  │                 │  │             │ │
│  └─────────────────┘  └─────────────────┘  └─────────────┘ │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

| Component | Responsibility |
|-----------|----------------|
| **Orchestrator** | State machine: story selection, iterations, crash recovery, prd.json mutations, git operations |
| **Agent Runner** | Dumb subprocess adapter: sends prompt via stdin, streams output, detects completion marker |
| **Verifier** | Deterministic enforcement: runs configured commands, manages services, gates story completion |

---

## The Loop

Ralph runs an infinite loop until all stories pass and final verification succeeds.
The loop is idempotent - stop anytime, run again to resume from current state.

```
┌─────────────────────────────────────────────────────────────┐
│                       RALPH RUN                             │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌────────────────────────────────────────────────────────┐│
│  │  STORY LOOP (infinite until complete)                  ││
│  │                                                        ││
│  │  1. Ralph picks next story (passes=false, not blocked) ││
│  │  2. Ralph sets currentStoryId in prd.json              ││
│  │  3. Ralph commits prd.json                             ││
│  │  4. Ralph sends prompt to provider subprocess          ││
│  │  5. Provider implements code + writes tests + commits  ││
│  │  6. Provider outputs: <ralph>DONE</ralph>              ││
│  │  7. Ralph runs verify.default commands                 ││
│  │  8. Ralph runs verify.ui commands (if story tagged)    ││
│  │  9. Ralph runs built-in browser checks (if UI story)   ││
│  │  10. ALL pass?                                         ││
│  │      → Ralph sets passes=true, lastResult={...}        ││
│  │      → Ralph clears currentStoryId                     ││
│  │      → Ralph commits prd.json                          ││
│  │  11. ANY fail?                                         ││
│  │      → Ralph sets retries++, notes="reason"            ││
│  │      → retries >= maxRetries? → blocked=true           ││
│  │      → Next iteration retries same story (if !blocked) ││
│  │  12. More incomplete stories? → continue loop          ││
│  │                                                        ││
│  └────────────────────────────────────────────────────────┘│
│                           ↓                                 │
│                 All stories passes=true                     │
│                           ↓                                 │
│  ┌────────────────────────────────────────────────────────┐│
│  │  FINAL VERIFICATION                                    ││
│  │                                                        ││
│  │  1. Ralph runs ALL verify commands (full suite)        ││
│  │  2. Ralph sends verification prompt to provider        ││
│  │  3. Provider reviews: coverage, quality, completeness  ││
│  │  4. Provider outputs VERIFIED or RESET markers         ││
│  │  5. <ralph>RESET:US-001,US-003</ralph> found?          ││
│  │     → Ralph resets those stories:                      ││
│  │       - passes=false                                   ││
│  │       - retries++ (may trigger blocked if >= max)      ││
│  │       - lastResult=null                                ││
│  │       - notes="reason from verification"               ││
│  │     → Loop back to STORY LOOP                          ││
│  │  6. <ralph>VERIFIED</ralph> found?                     ││
│  │     → ✅ COMPLETE - Ready to merge                     ││
│  │                                                        ││
│  └────────────────────────────────────────────────────────┘│
│                                                             │
│  IDEMPOTENT WORKFLOW:                                       │
│  - Stop anytime (Ctrl+C, crash, internet loss)              │
│  - State preserved in prd.json (currentStoryId, passes...)  │
│  - Run 'ralph run [feature]' again to resume                │
│  - Run 'ralph verify [feature]' to check without impl work  │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## Configuration

### ralph.config.json

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
  },

  "commits": {
    "prdChanges": true,
    "message": "chore: update prd.json"
  }
}
```

### Configuration Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `maxRetries` | No | 3 | Max retries per story before marking blocked |
| `provider.command` | **Yes** | - | Provider CLI command (amp, claude, opencode) |
| `provider.args` | No | [] | Arguments to pass to provider |
| `provider.timeout` | No | 1800 | Max seconds per iteration |
| `services` | No | [] | Services to manage (dev server, etc.) |
| `services[].name` | Yes | - | Service identifier |
| `services[].start` | No | - | Command to start service (if missing, user manages) |
| `services[].ready` | Yes | - | URL to check for readiness |
| `services[].readyTimeout` | No | 30 | Seconds to wait for ready |
| `services[].restartBeforeVerify` | No | true | Restart service before verify.ui |
| `verify.default` | **Yes** | - | Commands run for ALL stories |
| `verify.ui` | No | [] | Additional commands for UI-tagged stories |
| `commits.prdChanges` | No | true | Ralph commits prd.json changes |
| `commits.message` | No | "chore: update prd.json" | Commit message for prd.json |

### Provider Signals

Provider communicates with Ralph via structured markers in stdout:

| Signal | Purpose |
|--------|---------|
| `<ralph>DONE</ralph>` | Story implementation complete, ready for verification |
| `<ralph>LEARNING:text</ralph>` | Add learning to prd.json for future context |
| `<ralph>VERIFIED</ralph>` | Final verification passed (all stories complete) |
| `<ralph>RESET:US-001,US-003</ralph>` | Reset specific stories (verification found issues) |
| `<ralph>REASON:explanation</ralph>` | Reason for reset (populates notes field) |

Ralph detects markers in streaming output, applies changes atomically to prd.json.

**Learnings**: Provider can add context discovered during implementation. Ralph appends to `run.learnings` array. These are included in future prompts and can be used to update AGENTS.md/claude.md files.

---

## prd.json Schema (v2)

**Location:** `.ralph/[datetime]-[feature]/prd.json` (per-feature)

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
  "userStories": [
    {
      "id": "US-001",
      "title": "Story title",
      "description": "As a user, I want...",
      "acceptanceCriteria": ["Criterion 1", "Criterion 2"],
      "tags": [],
      "priority": 1,
      "passes": false,
      "retries": 0,
      "blocked": false,
      "lastResult": null,
      "notes": ""
    }
  ]
}
```

### Story Tags

| Tag | Effect |
|-----|--------|
| `ui` | Runs verify.ui commands in addition to verify.default |

### Story States

| State | Condition |
|-------|-----------|
| Pending | `passes: false`, `blocked: false` |
| Passed | `passes: true` |
| Blocked | `blocked: true` (after maxRetries exceeded) |

### lastResult Object (set by Ralph)

```json
{
  "completedAt": "2024-01-15T10:30:00Z",
  "commit": "abc1234",
  "summary": "Extracted from git commit message"
}
```

Note: `summary` is extracted from the git commit message, not from provider output.

---

## Prompt Templates

### prd-create.md (New PRD Brainstorming)

Sent to provider when creating a new PRD.

```markdown
# PRD Creation: {{feature}}

Help create a Product Requirements Document for this feature.

## Clarifying Questions

Ask 3-5 critical questions:
- Problem/Goal: What problem does this solve?
- Core Functionality: What are the key actions?
- Scope: What should it NOT do?
- Success Criteria: How do we know it's done?

Format with lettered options:
```
1. What is the primary goal?
   A. Option one
   B. Option two
   C. Other: [specify]
```

Wait for user answers before proceeding.

## Generate PRD

After answers, create a detailed PRD with:
- Introduction/Overview
- Goals
- User Stories (small, one session each)
- Functional Requirements
- Non-Goals
- Technical Considerations

### Story Rules
- Each story completable in ONE session
- Priority order: schema → backend → UI
- Include "Typecheck passes" in all criteria
- Include "Verify in browser" for UI stories

Save to: .ralph/{{datetime}}-{{feature}}/prd.md
```

---

### prd-refine.md (Refine Existing PRD)

Sent to provider when refining an existing PRD.

```markdown
# PRD Refinement: {{feature}}

Current PRD:
{{prdContent}}

Help refine this PRD. What would you like to improve?
- Add/remove/modify stories?
- Clarify acceptance criteria?
- Adjust scope?
- Split large stories?

After refinement, save updated version to: tasks/prd-{{feature}}.md
```

---

### prd-finalize.md (Finalize to JSON)

Sent to provider when finalizing PRD to prd.json.

```markdown
# PRD Finalization: {{feature}}

Convert this PRD to prd.json:

{{prdContent}}

## Output Format

```json
{
  "schemaVersion": 2,
  "project": "[from directory or PRD]",
  "branchName": "ralph/{{feature}}",
  "description": "[from PRD intro]",
  "run": {
    "startedAt": null,
    "currentStoryId": null,
    "learnings": []
  },
  "userStories": [...]
}
```

### Story Fields
- `id`: US-001, US-002, etc.
- `tags`: ["ui"] for stories with browser verification
- `priority`: Based on dependency order
- `passes`: false
- `retries`: 0
- `blocked`: false
- `lastResult`: null
- `notes`: ""

Save to: .ralph/{{datetime}}-{{feature}}/prd.json
```

---

### run.md (Story Implementation)

Sent to provider for each story. Must be provider-agnostic:

```markdown
# Story Implementation

## Your Task

Implement the following story:

**ID:** {{storyId}}
**Title:** {{storyTitle}}
**Description:** {{storyDescription}}

**Acceptance Criteria:**
{{acceptanceCriteria}}

## Instructions

1. Read the codebase to understand context
2. Implement the story following existing patterns
3. Write tests for your implementation
4. If this is a UI story (tagged "ui"):
   - Write e2e tests that verify the UI works
   - Tests should cover the acceptance criteria
5. Commit your changes with message: `feat: {{storyId}} - {{storyTitle}}`
6. When complete, output: {{doneMarker}}

## Verification

After you signal done, these commands will be run:
{{verifyCommands}}

Your story only passes if ALL verification commands succeed.

## Project Context

{{learnings}}
```

### verify.md (Final Verification)

Sent to provider after all stories pass:

```markdown
# Final Verification

All stories have been implemented. Perform a comprehensive review.

## Verification Commands

These have already passed:
{{verifyCommands}}

## Review Checklist

1. **Test Coverage**: Are all new functions/routes tested?
2. **Acceptance Criteria**: Does each story meet ALL criteria?
3. **Code Quality**: Are patterns consistent with codebase?
4. **Missing Pieces**: Anything incomplete or skipped?

## Stories Implemented

{{storySummaries}}

## Your Task

Review the implementation thoroughly. If you find issues:

Output the story IDs that need rework:
```
<ralph>RESET:US-001,US-003</ralph>
<ralph>REASON:US-001 missing test coverage, US-003 form validation incomplete</ralph>
```

If everything is complete and verified:
```
<ralph>VERIFIED</ralph>
```
```

---

## State Transitions

### Story States

```
┌─────────┐     agent done      ┌───────────┐     verify pass    ┌────────┐
│ PENDING │ ──────────────────→ │ VERIFYING │ ─────────────────→ │ PASSED │
│         │                     │           │                    │        │
└─────────┘                     └───────────┘                    └────────┘
     ↑                                │                               │
     │                                │ verify fail                   │
     │                                ↓                               │
     │                          ┌───────────┐                         │
     │                          │  FAILED   │                         │
     └────── retry ─────────────│ retries++ │                         │
                                └───────────┘                         │
                                      ↑                               │
                                      │ verification reset            │
                                      └───────────────────────────────┘
```

### Who Mutates What

| Field | Mutated By | When |
|-------|------------|------|
| `run.startedAt` | Ralph | First iteration start |
| `run.currentStoryId` | Ralph | Before sending prompt, cleared after pass |
| `run.learnings` | Provider | During implementation (patterns discovered) |
| `story.passes` | Ralph | After verification pass/fail |
| `story.retries` | Ralph | After verification fail |
| `story.blocked` | Ralph | After maxRetries exceeded |
| `story.lastResult` | Ralph | After verification pass |
| `story.notes` | Ralph | After verification fail (reason) |

Note: `run.learnings` is the exception - provider updates this during work to capture patterns for future iterations.

---

## Service Management

For UI stories, Ralph manages the dev server:

```
1. Check if service ready (HTTP GET to ready URL)
2. If not ready:
   a. Start service via start command
   b. Poll ready URL until success or timeout
   c. If timeout → fail story
3. Restart service before verify (fresh state)
4. Run browser verification (built-in)
5. Run verify.ui commands (e2e tests)
6. Keep service running for next iteration
7. On ralph exit → kill service
```

---

## Browser Verification (Built-in)

Ralph CLI includes browser automation for UI story verification. No provider MCP needed.

### How It Works

```
UI story completes
  ↓
Ralph starts/restarts dev server
  ↓
Ralph opens headless browser (chromedp/rod)
  ↓
Ralph runs checks:
  - Navigate to URLs from acceptance criteria
  - Check for console errors
  - Check for HTTP errors (no 4xx/5xx)
  - Take screenshot → .ralph/screenshots/
  ↓
Ralph runs verify.ui (e2e tests)
  ↓
All pass → story complete
```

### Config

```json
{
  "browser": {
    "enabled": true,
    "executablePath": "",
    "headless": true,
    "screenshotDir": ".ralph/screenshots"
  }
}
```

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | true | Enable built-in browser checks |
| `executablePath` | "" | Path to Chrome (auto-detect if empty) |
| `headless` | true | Run headless (no visible window) |
| `screenshotDir` | .ralph/screenshots | Where to save screenshots |

### What Ralph Checks

| Check | Purpose |
|-------|---------|
| Page loads (200 OK) | Basic smoke test |
| No console errors | Catch JS errors |
| No visible error banners | UI not broken |
| Screenshot saved | Evidence for review |

### Relationship to e2e Tests

| Ralph Browser Checks | e2e Tests (verify.ui) |
|---------------------|----------------------|
| Quick smoke test | Full verification |
| Built into Ralph | Written by provider |
| Generic checks | Story-specific assertions |
| Runs first | Runs after |

Both are required for UI stories to pass.

---

## Commands

### Core

| Command | Description |
|---------|-------------|
| `ralph init` | Create ralph.config.json + .ralph/ directory |
| `ralph prd [feature]` | Smart PRD workflow (brainstorm, refine, finalize) |
| `ralph run [feature]` | Run the main loop for feature |
| `ralph status [feature]` | Show story status for feature |

### Utility

| Command | Description |
|---------|-------------|
| `ralph verify` | Run verification only |
| `ralph next` | Show next story |
| `ralph validate` | Validate config and prd.json |
| `ralph doctor` | Check environment |

### Deferred

| Command | Description |
|---------|-------------|
| `ralph upgrade` | Self-update |

---

## `ralph prd [feature]` State Machine

Smart command that detects current state and offers appropriate actions.

### Files

| File | Purpose |
|------|---------|
| `.ralph/[datetime]-[feature]/prd.md` | Human-readable PRD for review/refinement |
| `.ralph/[datetime]-[feature]/prd.json` | Finalized PRD for execution |

### State 1: New Feature

No markdown exists for this feature.

```
$ ralph prd auth

Starting PRD for "auth"...
[Provider asks clarifying questions]
[User answers]
[Provider generates PRD]

Saved to .ralph/2024-01-15-auth/prd.md

Ready to finalize? (y/n)
  → y: Saves prd.json → "Ready! Run 'ralph run'"
  → n: "Run 'ralph prd auth' to continue"
```

### State 2: Markdown Exists, Not Finalized

PRD drafted but prd.json doesn't exist or doesn't match.

```
$ ralph prd auth

PRD exists: .ralph/2024-01-15-auth/prd.md

What would you like to do?
  A) Keep refining
  B) Finalize for execution
  C) Edit manually ($EDITOR)
```

### State 3: Already Finalized

Both markdown and matching prd.json exist.

```
$ ralph prd auth

PRD is ready: .ralph/2024-01-15-auth/

What would you like to do?
  A) Refine further
  B) Update prd.json
  C) Edit markdown ($EDITOR)
  D) Edit prd.json ($EDITOR)
  E) Start execution (ralph run)
```

---

## Dependencies

| Dependency | Required | Purpose |
|------------|----------|---------|
| Git | Yes | Version control, commits |
| Agent CLI | Yes | AI agent (amp, claude, opencode, etc.) |
| Project tools | Yes | User's build/test/lint tools |

Ralph does NOT depend on:
- Specific agent features (skills, MCP, oracle)
- Specific test frameworks (user configures commands)
- Browser automation (user's e2e tests handle this)

---

## Migration from Local Ralph

### Removed

- Project detection (explicit config required)
- Agent-specific features in prompts (skills, MCP, oracle)
- Agent updating prd.json (Ralph owns state)

### Changed

- Prompts are capability-based, not tool-based
- Verification is enforced by Ralph, not prompt-based
- Story completion gated by command success, not agent claim

### Added

- `ralph.config.json` required
- `agent` config section for any agent CLI
- `services` for dev server management
- `verify.default` and `verify.ui` profiles
- `tags` field on stories
- Ralph-owned prd.json mutations

---

## Decisions Made

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Provider completion signal | `<ralph>DONE</ralph>` | Simple, reliable across providers |
| Max retries per story | 3 (configurable) | Prevents infinite loops |
| prd.json commits | Ralph commits separately | Ralph owns state |
| prd.json location | Repo root (fixed) | Simplicity, always committed |
| Service restart | Before verify.ui | Fresh state for browser tests |
| Summary extraction | From git commit message | Provider-agnostic |
| No progress.log | prd.json fields cover it | Single source of truth |
| File locking | Lock file during run | Prevent concurrent corruption |
| Atomic writes | Temp file + rename | Prevent partial writes |

---

## Safety Mechanisms

### File Locking

Ralph creates `prd.json.lock` during `ralph run` to prevent concurrent runs:

```
ralph run starts → create prd.json.lock (with PID)
ralph run ends → remove prd.json.lock
```

If lock exists and process alive → exit with error.
If lock exists but process dead → remove stale lock, continue.

### Atomic prd.json Writes

All prd.json updates use temp file + rename:

```
1. Write to prd.json.tmp
2. Validate JSON is valid
3. Rename prd.json.tmp → prd.json (atomic on POSIX)
```

### Provider Exit Handling

| Exit Code | Ralph Action |
|-----------|--------------|
| 0 | Check for DONE marker, proceed to verify |
| Non-zero | Mark iteration failed, increment retries |
| Timeout | Kill provider, mark iteration failed |

---

## Branch Management

Ralph handles branch creation automatically.

### Flow

```
User on: main (or any branch)
  ↓
ralph run
  ↓
Ralph checks: does ralph/{feature} branch exist?
  ↓
No → Ralph creates ralph/{feature} from current HEAD
Yes → Ralph switches to ralph/{feature}
  ↓
Ralph works on ralph/{feature}
  ↓
Ralph NEVER merges back
  ↓
User manually merges when ready
```

### Branch Name

Derived from `prd.json.branchName` (e.g., `ralph/auth-system`).

### Rules

- Ralph creates branch if it doesn't exist
- Ralph resumes on branch if it exists
- Ralph never merges or pushes
- User responsible for merge/PR when done

---

## Implementation Phases

### Phase 1: Foundation (config, paths, schema, file ops) ✅
- [x] Remove project detection (`detect.go`)
- [x] Require `ralph.config.json` at repo root
- [x] Implement feature directory resolution (`.ralph/*-[feature]/`)
- [x] Implement global lock `.ralph/ralph.lock`
- [x] Implement atomic writes (temp file + rename)
- [x] Update schema structs to v2 (add `tags`, `blocked`, update `lastResult`)
- [x] Update schema validation

### Phase 2: Provider Agnostic ✅
- [x] Rename `AmpConfig` → `ProviderConfig` throughout code
- [x] Implement streaming marker detection (`<ralph>DONE</ralph>`, `LEARNING`, etc.)
- [x] Implement per-iteration timeout + kill
- [x] Update prompt templates (remove provider-specific features)

### Phase 3: Orchestrator Loop + State ✅
- [x] Implement infinite loop (no iteration limit)
- [x] Ralph-owned state transitions (set currentStoryId, passes, retries, blocked)
- [x] Commit prd.json changes (path-only: `git commit -- path/to/prd.json`)
- [x] Skip blocked stories in selection
- [x] Crash recovery via currentStoryId detection
- [x] Branch management (create/switch to ralph/{feature} branch)

### Phase 4: Verification + Services ✅
- [x] Implement verification command runner (captures output, exit code)
- [x] Implement service manager (start, readiness check, restart-before-verify-ui, cleanup)
- [x] Record verify results into lastResult
- [x] Final verification prompt with VERIFIED/RESET markers
- [x] Story reset logic from RESET marker

### Phase 5: Browser Testing (chromedp) ✅
- [x] Add optional `browser` config block
- [x] Implement headless browser runner (chromedp)
- [x] Navigate to URLs extracted from acceptance criteria
- [x] Capture console errors
- [x] Screenshot capture to `.ralph/screenshots/`
- [x] Run browser checks before verify.ui commands for UI stories

### Phase 6: Polish ✅
- [x] Update README
- [x] Update/add tests (66 tests passing)
- [x] Example configs for amp, claude, opencode (in README)
- [x] Error messages and help text
- [x] `ralph prd [feature]` state machine (create/refine/finalize)
