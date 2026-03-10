# OpenAI Symphony vs Ralph CLI — Comparative Analysis

> Sources: [Symphony repo](https://github.com/openai/symphony), [SPEC.md](https://github.com/openai/symphony/blob/main/SPEC.md), [Elixir reference implementation](https://github.com/openai/symphony/tree/main/elixir), skill definitions (commit, push, pull, land).

## Overview

Symphony is OpenAI's framework for issue-tracker-driven agent orchestration. It's a long-running Elixir/OTP service that polls Linear for candidate issues, creates isolated per-issue workspaces, spawns Codex agents via a bidirectional JSON-RPC protocol ("app-server mode"), and manages the full lifecycle from issue claim to PR merge.

Ralph is a Go CLI that orchestrates AI coding agents per user story from a PRD. It spawns fresh AI instances per story, verifies with automated tests, persists learnings, and repeats until all stories pass.

Symphony is **issue-tracker-out** (work comes from Linear). Ralph is **PRD-in** (work comes from the developer). They solve the same problem from opposite ends.

---

## Architecture Comparison

| Dimension | Symphony | Ralph CLI |
|-----------|----------|-----------|
| **Runtime** | Long-running Elixir/OTP daemon | CLI tool, runs to completion |
| **Work source** | Linear issues (polled every 5s) | Local PRD files (human-initiated) |
| **Execution model** | Turn-based JSON-RPC (max 20 turns/session) | Monolithic subprocess (30-min window) |
| **Agent protocol** | Bidirectional: ThreadStart → TurnStart → stream events → TurnComplete | Unidirectional: stdin/arg prompt → stdout markers (DONE/STUCK/LEARNING) |
| **Agent support** | Codex only (app-server mode required) | Any CLI agent (Claude, Amp, Aider, Codex, OpenCode) |
| **Isolation** | Separate workspace directory per issue | Single working directory, single branch |
| **Concurrency** | Max 10 simultaneous agents | Sequential (1 story at a time, lock file) |
| **Retry** | Exponential backoff (2^attempt x base, capped 300s) | Fixed count (default 3), immediate retry |
| **Failure model** | Classified: transient/permanent/partial/cascade | Unclassified: last 50 lines of output |
| **Lifecycle hooks** | 4 hooks: after_create, before_run, after_run, before_remove | None (all hardcoded in Go) |
| **Observability** | Phoenix LiveView dashboard + JSON API + structured logs + token metrics | JSONL logs + `ralph logs -f` + `ralph status` |
| **Skills** | 6 codified SKILL.md recipes (commit, push, pull, land, linear, debug) | Prompt templates with {{var}} substitution |
| **Context** | WORKFLOW.md template variables + Linear GraphQL tool | Resource consultation + learnings + knowledge file |
| **State** | In-memory OTP state + Linear as source of truth | File-based (run-state.json, atomic writes) |
| **Verification** | CI status + PR review before landing | Local verify commands (typecheck, lint, test) |
| **Learning** | None | Deduped, persisted, cap 50 in prompts |
| **Config** | WORKFLOW.md (YAML front matter + prompt template) | ralph.config.json + prompt templates in prompts/ |

---

## Symphony's Key Architectural Concepts

### 1. Turn-Based Agent Protocol

Symphony communicates with Codex via JSON-RPC over stdio:

```
Orchestrator                         Codex (app-server)
    |                                      |
    |--- ThreadStartRequest ------------->|  (initialize session)
    |<-- ThreadStartResponse -------------|
    |                                      |
    |--- TurnStartRequest (prompt) ------>|  (turn 1)
    |<-- CodexToolCall, CodexText --------|  (streaming events)
    |<-- CodexTokens --------------------|  (token counts)
    |<-- TurnCompleteEvent ---------------|
    |                                      |
    |  [orchestrator evaluates, runs       |
    |   checks, injects new context]       |
    |                                      |
    |--- TurnStartRequest (turn 2) ------>|  (next turn with new context)
    |...                                   |
```

- Max 20 turns per session, 1-hour turn timeout, 5s read timeout
- Between turns: orchestrator can check issue state, run verification, inject context
- Token-efficient: resumes session instead of restarting from scratch

**Ralph's model:** Single subprocess invocation. Provider gets full prompt once, runs to completion, signals markers on stdout. No mid-execution communication. After provider exits, Ralph verifies externally.

**Key insight:** Ralph's retry loop already functions as "pseudo-turns" — each retry is a new invocation with accumulated context (learnings, last failure). The gap isn't structural; it's the richness of inter-turn feedback.

### 2. Codified Skills System

Symphony encodes operational procedures as structured SKILL.md files in `.codex/skills/`:

**commit** (12 steps):
1. Read session history for intent/rationale
2. Inspect working tree and staged changes
3. Stage intended changes (`git add -A` after confirming scope)
4. Sanity-check newly added files (flag build artifacts, logs, temp files)
5. Fix index if staging is incomplete or includes unrelated files
6. Choose conventional type and optional scope
7. Write subject line (imperative mood, <= 72 chars, no trailing period)
8. Write body with summary, rationale, and test notes
9. Append Co-authored-by trailer
10. Wrap body lines at 72 chars
11. Create commit with `git commit -F <file>` (not `-m` with `\n`)
12. Verify staged diff matches commit message before committing

**push** (structured PR workflow):
- Run `make all` before pushing
- Create PR if none exists, update if open
- Fill PR body from `.github/pull_request_template.md`
- Validate body with `mix pr_body.check`
- Handle closed/merged PR → create new branch + PR

**pull** (merge-based sync):
- Enable `git rerere` for conflict memory
- Use `zdiff3` conflict style for better visibility
- Pull feature branch updates before merging main
- Minimize user prompts — only ask when product knowledge needed

**land** (full PR landing automation):
- Locate PR, confirm local tests pass
- Handle uncommitted changes (commit → push)
- Check mergeability, resolve conflicts via pull skill
- Address all review comments (Codex reviews + human reviews)
- Monitor CI with `gh pr checks --watch`
- On CI failure: inspect logs, fix, re-push
- Squash-merge using PR title/body as commit message
- Async `land_watch.py` helper for parallel monitoring

**Ralph's equivalent:** `prompts/run.md` says "commit your changes with message `feat: US-XXX - Title`" (one line). No procedural recipe for staging, validation, or error handling.

### 3. Workspace Isolation

Each issue gets a completely separate workspace directory:
```
<workspace.root>/<workspace_key>/
```

- `hooks.after_create` runs git clone + setup (once per issue)
- `hooks.before_run` runs before each agent session
- Workspaces cleaned up when issues reach terminal state
- Agents can't interfere with each other's working trees

**Ralph's model:** All stories modify the same git branch in the same working directory. Stories are sequential and build on each other's changes. Isolation would break this dependency model.

### 4. Lifecycle Hooks

Four user-configurable shell script hooks with timeout enforcement (default 60s):

| Hook | When | Use Case |
|------|------|----------|
| `after_create` | Once per issue (workspace init) | git clone, install deps |
| `before_run` | Before each agent session | pull latest, rebuild, migrate DB |
| `after_run` | After each agent session | cleanup temp files, report metrics |
| `before_remove` | Before workspace deletion | backup, archive logs |

All hooks run in workspace directory. Failure handling is phase-dependent:
- `after_create` / `before_run`: failure blocks execution (fatal)
- `after_run` / `before_remove`: failure logged but non-fatal (warn)

**Ralph's gap:** All lifecycle operations hardcoded in Go — service start, health check, restart, stop. No way for users to inject dependency installation, DB migrations, cache warming, or custom cleanup between stories.

### 5. Failure Model

Symphony classifies failures into four categories:

| Type | Behavior | Example |
|------|----------|---------|
| **Transient** | Exponential backoff retry (2^attempt × base, capped 300s) | API timeout, rate limit |
| **Permanent** | No retry, mark failed, move to terminal state | Invalid issue, missing repo |
| **Partial** | Log warning, continue with degraded state | Hook failure on after_run |
| **Cascade** | Pause polling, alert, wait for recovery | Linear API down, git auth expired |

**Ralph's model:** Unclassified failures. All failures get the same treatment: store last 50 lines, retry up to 3 times with immediate re-invocation. No distinction between transient (API timeout) and permanent (wrong architecture) failures.

### 6. Reconciliation Loop

Every 5 seconds, Symphony reconciles running agents against tracker state:
- Issue moved to terminal state (closed, cancelled) → stop agent, cleanup workspace
- Issue reassigned → stop agent
- Issue still active → continue
- Issue became eligible (new, unblocked) → dispatch

**Ralph's equivalent:** Verify-at-top checks if story already passes before spawning provider. But no external state reconciliation — Ralph doesn't check if the user modified the PRD or branch during a run.

### 7. WORKFLOW.md (Portable Config)

Single file with YAML front matter + Markdown prompt template:

```yaml
---
tracker:
  kind: linear
  project_slug: "..."
workspace:
  root: ~/code/workspaces
hooks:
  after_create: |
    git clone git@github.com:org/repo.git .
agent:
  max_concurrent_agents: 10
  max_turns: 20
codex:
  command: codex app-server
---

You are working on issue {{ issue.identifier }}.
Title: {{ issue.title }}
Body: {{ issue.description }}
```

Drop into any repo. Template variables rendered per-issue.

**Ralph's equivalent:** `ralph.config.json` + `prompts/*.md` (embedded at compile time). Config is JSON, prompts are Go-embedded templates with `{{var}}` substitution. Less portable (prompts compiled into binary) but more structured.

### 8. Observability & Token Tracking

- Phoenix LiveView dashboard at `/` — real-time agent status
- JSON API at `/api/v1/state`, `/api/v1/<issue_identifier>`, `/api/v1/refresh`
- Per-session metrics: input/output tokens, turn count, timing
- Structured JSON logs to configurable sinks
- Token cost tracking enables ROI analysis per issue

**Ralph's equivalent:** JSONL logs with 21+ event types, `ralph logs -f` for following, `ralph status` for progress. No dashboard, no API, no token tracking. No way to answer "how much did this feature cost in tokens?"

---

## Knowledge File Injection Gap

Ralph **names** the knowledge file in prompts but doesn't **embed** it:

```go
// config.go — specifies the filename
KnowledgeFile string `json:"knowledgeFile"` // e.g., "AGENTS.md"

// prompts.go — injects the name, not the content
map[string]string{
    "knowledgeFile": cfg.Config.Provider.KnowledgeFile,
}

// run.md — tells the agent to UPDATE the file
"Good additions to {{knowledgeFile}}: ..."
```

The prompt tells the agent to update the knowledge file but doesn't pre-load its content. The agent must locate, read, and parse it independently. This works for capable providers (Claude can read files) but is a gap for less capable ones.

Symphony's skill system codifies exactly where to find files and how to parse them. Ralph could add a `{{knowledgeFileContent}}` variable that reads and injects the file content into the prompt.

---

## Concrete Improvements for Ralph

### Priority Ranking

| Change | Source | Impact | Effort |
|--------|--------|--------|--------|
| Procedural recipes in `run.md` | Symphony skills | High | 1 day |
| Pre-flight baseline checks | Stripe shift-left | High | 1 day |
| Structured retry context (diff + classified errors) | Both systems | High | 1 day |
| Lifecycle hooks (`beforeRun`, `afterRun`) | Symphony hooks | High | 2 days |
| `ralph status --json` | Symphony API | Medium | 0.5 day |
| Verification report artifacts (JSON + Markdown) | Symphony proof-of-work | Medium | 1 day |
| Wall-clock metrics per story | Symphony observability | Medium | 1 day |
| PRD watcher (`ralph watch`) | Symphony polling | Medium | 1 day |
| Summary metadata JSON | Symphony proof-of-work | Low | 0.5 day |
| Optional exponential backoff | Symphony retry model | Low | 0.5 day |
| Token/cost tracking | Symphony metrics | Low | 2 days |

### Tier 1: Immediate (High ROI, Low Effort)

#### 1.1 Procedural Recipes in Prompts (from Skills System)

This is the single highest-impact change. Extend `prompts/run.md` with structured procedural sections that encode the operational knowledge Symphony puts in SKILL.md files:

```markdown
## Commit Procedure
1. Identify changed files: `git status --porcelain`
2. Stage only story-related files: `git add <file1> <file2> ...`
   (do NOT use `git add .` — avoid staging unrelated files)
3. Verify staging: `git diff --cached --stat`
4. Create commit: `git commit -m "feat: {{storyId}} - {{storyTitle}}"`
5. Verify commit created: `git log -1 --oneline` (must show your commit)
6. Signal DONE only after commit is confirmed

## Verification Procedure
Run these commands IN ORDER before signaling DONE:
{{verifyCommands}}

For each command:
  a. Run it
  b. If PASS: move to next
  c. If FAIL: read error output, fix code, re-run SAME command
Once all pass, re-run ALL commands once more to confirm.
Only signal DONE if final full pass succeeds.

## Test Writing Procedure
1. Find existing test files: look for `*_test.*`, `__tests__/`, `*.spec.*`
2. Read one existing test to understand patterns (framework, assertion style)
3. For each new function/component:
   - Create test file matching existing naming convention
   - Write happy-path test
   - Write at least one error/edge case test
4. Run tests before committing
```

**Why this matters:** Ralph's current `run.md` says "commit your changes" and "write tests" as bare instructions. Symphony's skills encode 12-step processes with specific conventions. Procedural recipes reduce agent improvisation on mechanical tasks where there's a clear right way.

**Files:** `prompts/run.md` only. Zero Go code changes.

#### 1.2 Structured Retry Context

On failure, inject richer diagnostic context into the retry prompt:

**Current** (`prompts.go:116-125`):
```
Previous Attempts: 2 of 3 (1 remaining before skipped)
Previous Issue: tests failed: error X
```

**Proposed:**
```
## Previous Failure Details

**Attempt 2 of 3** (1 remaining before skipped)
**Classification:** test failure
**Command:** bun run test:unit

**Error Output:**
  tests/auth.test.ts:42 — expected true, got false
  tests/auth.test.ts:89 — ReferenceError: getUserTasks is not defined

**Your Previous Changes (git diff summary):**
  Modified: auth.go (+32 -8), middleware.go (+8 -2)
  Added: auth_test.go (+45)

**Suggestion:** The test references `getUserTasks` which doesn't exist.
Check if you forgot to export it from the correct module.
```

**Files:** `loop.go` (capture git diff on failure), `prompts.go` (structured `{{retryInfo}}`), `schema.go` (extend state with failure details).

### Tier 2: Short-Term (This Month)

#### 2.1 Lifecycle Hooks

Add user-configurable hooks to `ralph.config.json`:

```json
{
  "hooks": {
    "beforeRun": {
      "commands": ["npm ci", "npx prisma migrate deploy"],
      "timeout": 120,
      "failureMode": "block"
    },
    "afterRun": {
      "commands": ["rm -rf .tmp-test-*"],
      "timeout": 30,
      "failureMode": "warn"
    },
    "beforeVerify": {
      "commands": ["npm run build"],
      "timeout": 60,
      "failureMode": "block"
    }
  }
}
```

- `beforeRun`: runs before each story provider spawn (install deps, migrate DB, warm cache)
- `afterRun`: runs after each story completes (cleanup temp files, reports)
- `beforeVerify`: runs before verification commands (build step, generate schemas)
- `failureMode`: `"block"` (fails the story), `"warn"` (logs warning, continues)
- `timeout`: seconds, default 60

**Implementation footprint:** ~100 LOC total — `HooksConfig` struct, `runHook()` function, 4 call sites in `loop.go`.

**Service restart policies:** Also consider extending service config with `restartPolicy`:
```json
{
  "services": [{
    "name": "dev",
    "start": "bun run dev",
    "ready": "http://localhost:3000",
    "restartPolicy": "verify-only"
  }]
}
```
Options: `"always"` (restart before every story), `"once"` (start once, never restart), `"verify-only"` (current behavior — restart before verification only).

**Tests needed:** `hooks_test.go` (TestRunHook_Success, TestRunHook_Timeout, TestRunHook_Failure), `config_test.go` (TestLoadConfig_WithHooks, TestValidateHooks).

**Files:** New `hooks.go` (~50 LOC), `config.go` (add `HooksConfig` struct), `loop.go` (4 call sites), `ralph.schema.json`.

#### 2.2 Verification Report Artifacts

Persist verification results as structured JSON + Markdown files on `ralph verify`:

```json
{
  "timestamp": "2026-03-05T14:30:00Z",
  "feature": "user-auth",
  "branch": "ralph/user-auth",
  "passed": true,
  "checks": [
    {"name": "typecheck", "passed": true, "duration_ms": 2300, "output": ""},
    {"name": "test:unit", "passed": true, "duration_ms": 5100, "output": "42 tests passed"},
    {"name": "AI analysis", "passed": true, "criteria_met": 8, "criteria_total": 8}
  ],
  "stories": {
    "US-001": {"status": "passed", "retries": 0},
    "US-002": {"status": "passed", "retries": 1},
    "US-003": {"status": "skipped", "retries": 3, "reason": "flaky external API"}
  }
}
```

Also generate `verification-report.md` with collapsible sections per check.

**Why this matters:** Symphony collects "proof-of-work" before landing PRs. Ralph runs verification and prints to console but doesn't persist structured reports. Reports enable audit trails, CI/CD integration, and team visibility.

**Files:** `loop.go` (write JSON/MD after `runVerifyChecks`).

#### 2.3 Machine-Readable Status (`ralph status --json`)

```bash
ralph status myfeature --json
# {"feature":"myfeature","status":"in-progress","passed":3,"skipped":1,"pending":2,"total":6,"branch":"ralph/myfeature"}
```

Enables GitHub Actions to check progress, Slack webhooks for notifications, dashboards for team visibility.

**Files:** `commands.go` (`cmdStatus` — add flag parsing + JSON output).

#### 2.4 Headless Task Mode (`ralph task`)

Non-interactive mode for CI/CD:

```bash
echo '{"feature":"auth-fix"}' | ralph task
# Runs prd → run → verify pipeline, outputs JSON result
```

**Files:** `main.go` (add case), `commands.go` (new `cmdTask`). Zero core loop changes.

#### 2.5 One-Shot Fix Command (`ralph fix`)

```bash
ralph fix "TestAuth fails with nil pointer"
# Auto-generates minimal fix PRD → runs → verifies
```

Symphony's model (triggered by issue state changes) enables automated fixes. Ralph can achieve similar with a dedicated command.

**Files:** `commands.go` (new `cmdFix`), reuses `prd.go` + `loop.go`.

#### 2.6 PRD Watcher (`ralph watch`)

File-watching mode that monitors PRD changes and auto-triggers runs:

```bash
ralph watch <feature>
# Watches .ralph/YYYY-MM-DD-<feature>/prd.json for changes
# On change: runs ralph run <feature> --verify
```

**Implementation:** ~80 LOC in `commands.go`. Use `time.Ticker` to poll prd.json mtime every 2s. On change (mtime or hash diff), invoke `runLoop()` directly. Respect lock file — if run is in progress, queue next run or skip.

**Why this matters:** Symphony polls Linear every 5s for changes. `ralph watch` provides equivalent reactivity for local PRD-driven workflows. Catches "user tweaked PRD" without manual re-run.

**Files:** `commands.go` (add `cmdWatch`). Reuses existing `loop.go` entirely.

### Tier 3: Medium-Term (Next Quarter)

#### 3.1 Execution Model Improvements

Five improvements to bridge the gap between Ralph's monolithic execution and Symphony's turn-based protocol, all provider-agnostic:

**1. CHECKPOINT markers:**
Add `<ralph>CHECKPOINT:description</ralph>` marker. Provider signals partial progress. Ralph logs checkpoint events, can run intermediate checks without resetting the session.

**2. Structured turn history:**
Formalize retry iterations into per-story turn logs:
```
.ralph/sessions/<feature>/<story>.turns.jsonl
{"turn":1, "start_time":"...", "result":"failed", "reason":"..."}
{"turn":2, "start_time":"...", "result":"done", "files":["a.go"]}
```
Enable `ralph logs --story US-001 --turns` to trace iteration history.

**3. Verification-informed retry guidance:**
Instead of just injecting last failure, track last 3 failures and identify patterns:
```
Attempt 1: go test failed with 'undefined: getUserTasks'.
Attempt 2: Same error.
Fix: Check schema.go — function not exported?
```

**4. Resource consultation feedback loop:**
Currently, resource consultation runs once per story before provider. On retry failure, re-consult with narrowed focus based on failed acceptance criteria:
```go
if verifyResult.failed && attempt < maxRetries {
    resourceGuidance = ConsultResources(..., story, verifyResult.failedACs)
}
```

**5. Multi-provider turn limits:**
Formalize per-provider iteration limits instead of one-size-fits-all 30-min window:

| Provider | Max turns | Turn timeout | Rationale |
|----------|-----------|-------------|-----------|
| codex | 20 | 1 hour | Matches Symphony's config |
| claude, amp | 5 | 30 min | Multi-invocation retry |
| aider | 3 | 30 min | Lighter agent |

| Feature | Effort | Impact | Provider-Agnostic? |
|---------|--------|--------|-------------------|
| Checkpoint markers + turn history | Low | Better debugging | Yes |
| Structured retry guidance | Medium | 10-20% faster convergence | Yes |
| Contextual resource consultation | Medium | Better framework guidance | Yes |
| Turn limits per provider | Low | Prevents infinite loops | Yes |
| Full turn-based (Codex-only) | High | Token efficiency for Codex | No |

**Files:** `loop.go` (checkpoint handler, retry guidance), `prompts.go` (turn history injection), `config.go` (per-provider turn limits).

#### 3.2 Wall-Clock Metrics Per Story

Track timing in RunState:

```go
StoryDurations map[string]int64     `json:"storyDurations,omitempty"`
FirstAttempt   map[string]time.Time `json:"firstAttempt,omitempty"`
LastAttempt    map[string]time.Time `json:"lastAttempt,omitempty"`
```

Enables: time estimates, identifying slow stories, cost analysis.

**Files:** `schema.go`, `loop.go` (capture timestamps at iteration start/end).

#### 3.3 Summary Metadata JSON

When archiving, write `summary.json` alongside `summary.md`:

```json
{
  "feature": "user-auth",
  "archiveDate": "2026-03-05",
  "totalDuration": 3600,
  "storyCount": 6,
  "passedStories": 5,
  "skippedStories": 1,
  "totalRetries": 4,
  "learningsCaptured": 12
}
```

Enables cross-feature analytics: success rates over time, average retries per story, learning effectiveness (do features with more learnings complete faster?).

**Files:** `loop.go` (`archiveFeature` — write additional JSON).

#### 3.4 Optional Exponential Backoff Retry

Symphony uses `2^attempt x base_interval`, capped at 300s. Ralph's immediate retry works for logic errors but not for rate limits or flaky providers.

Add to config:
```json
{
  "retryPolicy": "immediate",
  "retryBaseDelay": 10,
  "retryMaxDelay": 300
}
```

Helps with: rate limits (429), provider timeout, intermittent flakes. Does NOT help with: logic errors, missing packages, stuck loops.

**Files:** `config.go` (new fields), `loop.go` (add `time.Sleep` before retry `continue`).

#### 3.5 Token/Cost Tracking

Track provider input/output token counts when available:

**Phase 1:** Add wall-clock timing to RunState (enables forecasting). Per-story duration summaries in `ralph logs --summary`.

**Phase 2:** Token cost tracking (requires provider cooperation — parse token counts from provider output where available).

**Phase 3:** Cross-feature analytics — success rate trends, cost per feature, learning effectiveness over time.

**Files:** `schema.go` (timing fields), `loop.go` (capture timestamps), `commands.go` (summary output).

#### 3.6 Learning Frequency Ranking

Track how often each learning is re-discovered. Sort by frequency + recency when injecting into prompts.

**Files:** `schema.go` (change `Learnings []string` to structured type), `prompts.go` (sort before injection).

---

## What Ralph Should NOT Copy

| Symphony Feature | Why Skip |
|-----------------|----------|
| **Daemon/polling mode** | Adds 1000+ LOC for state management, recovery, distributed locking. Ralph is CLI-first; CI/cron handles scheduling. |
| **Linear integration** | Tight coupling to one tracker. Ralph's PRD model is tracker-agnostic. |
| **Codex app-server protocol** | Only works with Codex. Ralph must support Claude, Amp, Aider, OpenCode. Provider agnosticism is non-negotiable. |
| **Workspace isolation per story** | Would break sequential story dependencies. Stories build on each other's code. |
| **Elixir/OTP supervision** | Architecture-level choice. Go + subprocess is simpler for CLI. |
| **Phoenix LiveView dashboard** | Over-engineering. `--json` output is the right abstraction for CLI. |
| **Reconciliation loop** | No external event source to reconcile against. PRD is local and immutable during runs. `ralph watch` provides lighter alternative. |

---

## Where Ralph Already Wins

### Learning System (Unique to Ralph)

Symphony has no equivalent. Ralph's learning system (`schema.go:155-171`) deduplicates, persists, and caps at 50. Each story's discoveries feed into the next. This is genuine cross-task intelligence accumulation.

### Provider Agnosticism

Symphony is Codex-only (requires app-server mode). Ralph supports 5+ providers with auto-detection. Users choose their AI backend without lock-in.

### Verify-at-Top (Idempotent Resume)

`loop.go:208-227` — Check if story already passes before spawning provider. Interrupt `ralph run`, resume later, no wasted work. Symphony has no equivalent because it manages fresh sessions per dispatch.

### Resource Consultation

`consultation.go` — Auto-resolves dependencies from lock files, fetches upstream docs, injects framework-specific guidance. Open-source alternative to Symphony's Linear GraphQL tool.

### PRD-Driven Quality Gate

Structured stories with acceptance criteria, verified by AI deep analysis. Ensures requirements quality before implementation starts. Symphony relies on Linear issue descriptions (unstructured).

### Archive + Summary Lifecycle

After verification succeeds, AI generates summary, PRD files are deleted, committed. Clean feature completion. `ralph refine` uses summary as context for post-verification work. Symphony has no equivalent lifecycle.

---

## Summary: The 80/20

Ralph can get 80% of Symphony's architectural value from 3 changes:

1. **Procedural recipes in prompts** — Encode commit, verification, and test-writing procedures directly in `run.md`. This is what Symphony puts in SKILL.md files. Zero code changes, high impact on provider output quality.

2. **Lifecycle hooks** — Let users configure `beforeRun`/`afterRun`/`beforeVerify` shell commands. Covers dependency installation, DB migrations, custom cleanup. Unlocks use cases Ralph currently blocks.

3. **Verification report artifacts** — Persist JSON + Markdown reports on `ralph verify`. Provides audit trails, enables CI/CD integration, matches Symphony's proof-of-work pattern.

Everything else is either not applicable (daemon mode, Codex-only protocol, workspace isolation) or lower priority (metrics, backoff, analytics). Ralph's core architecture — provider-agnostic CLI with sequential stories, learnings, and verify-at-top — is sound. The improvements should make the feedback loop richer and the lifecycle more extensible, not change the fundamental model.
