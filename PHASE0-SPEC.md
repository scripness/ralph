# Phase 0 Implementation Specification

**Purpose:** Complete, self-contained specification for implementing the scrip v1 CLI. This is a new product — ralph as a CLI ceases to exist, replaced entirely by scrip. Each implementation session's agent reads this document + the relevant source files to work with full context. No need to read the entire ROADMAP.md.

**Scope:** Phase 0 only. Phases 1-4 of ROADMAP.md are superseded and will be re-evaluated after Phase 0 ships.

**Source of truth:** ROADMAP.md Phase 0 section (lines 471-1048) is the design document. This spec adds implementation detail — schemas, session boundaries, reuse guidance — without changing the design.

**Lineage:** Scrip v1 is built on the ralph codebase (reusing ~75% of its Go source — direct copies + adaptations). After this rewrite, the `ralph` binary, command set, and all ralph-specific references are gone. Only `scrip` exists.

---

## Table of Contents

- [Claude Code Invocation](#claude-code-invocation)
- [Architecture Summary](#architecture-summary)
- [State Schemas](#state-schemas)
- [Prompt Template Inventory](#prompt-template-inventory)
- [Codebase Reuse Map](#codebase-reuse-map)
- [Implementation Sessions](#implementation-sessions)
- [Cross-Session Contract](#cross-session-contract)
- [Testing Architecture](#testing-architecture)
- [Partially Specified Details](#partially-specified-details)
- [Post-Implementation Refinement](#post-implementation-refinement)

---

## Claude Code Invocation

Every `claude` CLI invocation by scrip includes `--model opus` and `--effort max`. No exceptions.

### Autonomous execution (scrip exec)

```
claude --print --dangerously-skip-permissions --model opus --effort max
```

Prompt delivered via stdin. Provider signals completion via markers. CLI runs verification after.

### Consultation subagents (scrip plan, scrip exec, scrip land)

```
claude --print --model opus --effort max
```

No `--dangerously-skip-permissions` — consultation subagents read cached framework source and produce guidance. They don't modify files.

### Verification subagents (scrip plan, scrip land)

```
claude --print --model opus --effort max
```

No `--dangerously-skip-permissions` — verification subagents analyze code and produce structured reports. They don't modify files.

### Planning rounds (scrip plan)

```
claude --print --model opus --effort max
```

No `--dangerously-skip-permissions` — planning produces analysis and plan drafts. File writes (plan.md) are done by the CLI after the agent returns.

### Land fix agent (scrip land)

```
claude --print --dangerously-skip-permissions --model opus --effort max
```

Same as autonomous execution — the fix agent needs to modify files to address verification failures.

### Summary

| Context | --print | --dangerously-skip-permissions | --model opus | --effort max |
|---------|---------|-------------------------------|-------------|-------------|
| `scrip exec` (build) | Yes | Yes | Yes | Yes |
| `scrip land` (fix) | Yes | Yes | Yes | Yes |
| Consultation | Yes | No | Yes | Yes |
| Verification | Yes | No | Yes | Yes |
| Planning | Yes | No | Yes | Yes |

---

## Architecture Summary

Four commands. Self-contained CLI. Claude Code-only. All state in `.scrip/` and `~/.scrip/`.

```
scrip prep    # project setup + harness audit
scrip plan    # CLI-mediated planning rounds with consultation
scrip exec    # autonomous item loop (Ralph technique)
scrip land    # comprehensive verification + finalize
```

### Design Principles (from ROADMAP)

1. **Self-contained CLI** — touches `.scrip/` and `~/.scrip/` only. No `.claude/`, CLAUDE.md, `.mcp.json`.
2. **Claude Code-only** — no provider abstraction. Hardcoded `claude` command.
3. **Four commands, clear boundaries** — each command has one job.
4. **Consultation and verification are CLI infrastructure** — separate `--print` subagent calls, not prompt instructions.
5. **Disposable plans, permanent progress** — plan.md purged after execution; progress files are durable.
6. **CLI orchestrates, provider implements** — CLI handles state/consultation/verification/retries. Provider signals DONE/STUCK/LEARNING.

### Core Ralph Technique Elements (all load-bearing)

1. Core loop: fresh `claude --print` per item
2. One item per loop
3. Fresh context every iteration (disk is the only bridge)
4. plan.md as shared state (CLI-managed, not agent-updated)
5. Backpressure via verification commands
6. Subagent control (250-500 for reads/searches, 1 for build/tests)
7. "Don't assume not implemented" (in exec-build.md)
8. "No placeholders or stubs" (in exec-build.md)
9. "Capture the why" (LEARNING markers + guidance)
10. Plans are disposable
11. Operational guide via CLI-injected context (replaces AGENTS.md)
12. Prompt structure: Orient -> Act -> Guardrails
13. Two-mode architecture: planning and building use same loop mechanism
14. Steering via patterns, not instructions

### Subagent Control Model

This is directly from the original Ralph technique. Every prompt template that instructs the spawned agent MUST include subagent usage instructions. The spawned `claude --print` instance is the **main agent/scheduler** — it delegates expensive work to its own internal subagents.

**Subagent tiers (by model and task):**

| Task | Model | Parallelism | Rationale |
|------|-------|-------------|-----------|
| Searching/reading codebase | Sonnet | Up to 500 parallel | Fast, cheap, embarrassingly parallel |
| Studying specs/docs | Sonnet | Up to 500 parallel | Read comprehension at scale |
| Build/test execution | Sonnet | **Only 1** | Multiple build agents = bad backpressure (step on each other) |
| Complex reasoning (debugging, architecture) | Opus | As needed | "Use Opus subagents when complex reasoning is needed" |
| Spec inconsistency resolution | Opus with ultrathink | 1 | High-stakes decisions need deep reasoning |

**Why 250-500 and not just "many":** The context window is ~176K usable tokens. The main agent acts as scheduler — it keeps ~40-60% for its own reasoning ("smart zone") and delegates the rest to subagents. Each subagent gets its own context window that's garbage collected when it returns. 250 is the floor for meaningful parallel coverage; 500 is the ceiling before diminishing returns. The prompt must specify a concrete number so the agent doesn't self-limit to 3-5 subagents.

**Critical constraint:** "Up to 500 parallel subagents for searches/reads and **only 1 subagent for build/tests.**" Multiple build agents stepping on each other's output is the primary source of bad backpressure. This single constraint prevents the most common autonomous execution failure mode.

---

## State Schemas

### .scrip/config.json

```json
{
  "$schema": "https://scrip.dev/config.schema.json",
  "project": {
    "name": "my-project",
    "type": "go",
    "root": "."
  },
  "provider": {
    "command": "claude",
    "timeout": 1800,
    "stallTimeout": 300
  },
  "verify": {
    "typecheck": "go vet ./...",
    "lint": "golangci-lint run",
    "test": "go test ./..."
  },
  "services": [
    {
      "name": "api",
      "command": "go run ./cmd/server",
      "ready": "http://localhost:8080/health",
      "timeout": 30
    }
  ]
}
```

Fields:
- `project.name` (string, required) — project identifier
- `project.type` (string, required) — detected project type (go, node, elixir, python, rust)
- `project.root` (string, default ".") — project root relative to .scrip/
- `provider.command` (string, default "claude") — CLI command
- `provider.timeout` (int, default 1800) — hard timeout in seconds per spawn
- `provider.stallTimeout` (int, default 300) — no-output timeout in seconds
- `verify.typecheck` (string, optional) — typecheck command
- `verify.lint` (string, optional) — lint command
- `verify.test` (string, required) — test command
- `services[]` (array, optional) — dev servers to manage

### plan.md

YAML frontmatter + markdown body. AI writes naturally; CLI parses frontmatter.

```markdown
---
feature: auth-system
created: 2026-03-11T14:32:00Z
item_count: 3
---

# Auth System

## Items

1. **Set up OAuth2 dependencies**
   - Acceptance: OAuth2 client instantiates, no hardcoded secrets

2. **Google login flow**
   - Acceptance: End-to-end Google auth works, session persists

3. **Session management**
   - Acceptance: Sessions expire after 24h, refresh token works
   - Depends on: item 2
```

CLI parses:
- Frontmatter: `feature`, `created`, `item_count`
- Body: numbered items with `**bold title**` and `- Acceptance:` lines
- Optional: `- Depends on:` lines for ordering

Parsing strategy: regex on markdown conventions, not full YAML-in-body. If frontmatter is malformed, treat entire file as markdown body (degrade gracefully).

> **Note (pending refinement):** The exact plan.md format needs finalization — whether items should also be structured in YAML frontmatter (with `id`, `depends_on` fields for machine parsing) or kept as pure markdown. This spec uses markdown body only. A hybrid approach (YAML for machine fields, markdown for context) may be needed for dependency enforcement and item tracking. To be refined after initial implementation proves out the parsing approach.

### plan.jsonl

One JSON object per line. Each line is one planning round.

```jsonl
{"round":1,"ts":"2026-03-11T14:30:00Z","user_input":"add google oauth login","consultation":["Phoenix auth: ueberauth is standard..."],"ai_response":"Based on research, here are 3 approaches...","has_plan_draft":false}
{"round":2,"ts":"2026-03-11T14:35:00Z","user_input":"go with ueberauth","consultation":["ueberauth strategy pattern..."],"ai_response":"Updated approach...","has_plan_draft":true}
{"round":3,"ts":"2026-03-11T14:40:00Z","user_input":"write the plan","ai_response":"[plan written to plan.md]","finalized":true,"verification":{"items":5,"warnings":["missing CSRF"]}}
```

Fields per round:
- `round` (int) — round number within this planning session
- `ts` (string, RFC3339) — timestamp
- `user_input` (string) — what the user said
- `consultation` ([]string, optional) — consultation results injected
- `ai_response` (string) — AI's response text
- `has_plan_draft` (bool) — whether this round produced a draft
- `finalized` (bool, optional) — true if plan.md was written
- `verification` (object, optional) — plan verification results

Progressive compression for context injection (deterministic, no AI summarization):

**Algorithm:** Given N total rounds, the CLI builds `{{planHistory}}` as follows:
1. **Recent 5 rounds** (N-4 through N): include verbatim — all fields
2. **Middle rounds** (round 6 through N-5): decision-only — include `round`, `ts`, `user_input`, first 200 chars of `ai_response` (truncated with "…"), drop `consultation` array entirely
3. **Old rounds** (1 through 5, if N > 10): one-line digest — `"Round {n}: {user_input} → {first 80 chars of ai_response}…"`
4. **Maximum context budget:** 8,000 tokens (~32KB). If compressed history exceeds this, drop oldest digests first, then oldest middle rounds.

This is a fixed algorithm — no AI summarization, no lossy compression. The raw plan.jsonl is always preserved on disk.

### progress.jsonl

Append-only event log. One JSON object per line.

```jsonl
{"ts":"2026-03-11T15:00:00Z","event":"exec_start","feature":"auth","plan_items":3}
{"ts":"2026-03-11T15:01:00Z","event":"item_start","item":"Set up OAuth2","criteria":["OAuth2 client instantiates","no hardcoded secrets"]}
{"ts":"2026-03-11T15:10:00Z","event":"item_done","item":"Set up OAuth2","status":"passed","commit":"abc123","learnings":["callback URL must be exact match"]}
{"ts":"2026-03-11T15:11:00Z","event":"item_start","item":"Google login flow","criteria":["End-to-end Google auth works","session persists"]}
{"ts":"2026-03-11T15:15:00Z","event":"item_stuck","item":"Google login flow","attempt":1,"reason":"Guardian config unclear"}
{"ts":"2026-03-11T15:15:01Z","event":"learning","text":"Guardian requires serializer module, not just config"}
{"ts":"2026-03-11T15:16:00Z","event":"item_start","item":"Google login flow","attempt":2}
{"ts":"2026-03-11T15:25:00Z","event":"item_done","item":"Google login flow","status":"passed","commit":"def456"}
{"ts":"2026-03-11T15:30:00Z","event":"exec_end","passed":2,"skipped":0,"failed":0}
{"ts":"2026-03-11T16:00:00Z","event":"plan_purged"}
{"ts":"2026-03-12T10:00:00Z","event":"land_failed","findings":["test: 2 failures","security: missing CSRF"],"analysis":"..."}
{"ts":"2026-03-12T11:00:00Z","event":"land_passed","summary_appended":true}
```

Event types:
- `exec_start` — execution session begins (feature, plan_items)
- `item_start` — item attempt begins (item, criteria, attempt number)
- `item_done` — item completed (item, status: passed/skipped, commit, learnings)
- `item_stuck` — item stuck (item, attempt, reason)
- `learning` — standalone learning (text)
- `exec_end` — execution session ends (passed, skipped, failed counts)
- `plan_purged` — plan.md deleted after execution
- `plan_created` — new plan.md written (item_count, context)
- `land_failed` — land verification failed (findings[], analysis)
- `land_passed` — land verification passed (summary_appended)

Rotation: archive to `progress.jsonl.1` at 10,000 lines. Only current file loaded into prompts.

### progress.md

Append-only markdown narrative. Written by CLI after each exec session and after land.

```markdown
## 2026-03-11 15:30 — Exec Session

### Completed
- **Set up OAuth2 dependencies** (abc123) — OAuth2 client configured with env-based secrets
- **Google login flow** (def456) — End-to-end auth working, retry needed due to Guardian config

### Learnings
- Callback URL must be exact match (no trailing slash)
- Guardian requires serializer module, not just config

### Next
- Session management (1 item remaining)

---

## 2026-03-12 11:00 — Land Passed

Feature landed successfully. All verification passed.
Summary appended to feature record.
```

Written by CLI, not AI. Structured sections. Machine-readable enough for context injection.

**`{{progressContext}}` injection format:** The CLI reads the most recent 3 sections from progress.md and injects them verbatim as the `{{progressContext}}` template variable. If progress.md exceeds 4,000 tokens (~16KB), only the last 2 sections are included. If no progress.md exists, `{{progressContext}}` is replaced with "No previous execution history for this feature."

### state.json

Runtime recovery only. Deleted on clean exit.

```json
{
  "version": 1,
  "current_item": "Google login flow",
  "current_attempt": 2,
  "provider_pid": 12345,
  "provider_started_at": 1741700000,
  "started_at": "2026-03-11T15:16:00Z",
  "lock_holder": "scrip-exec"
}
```

Fields:
- `version` (int) — checkpoint version, incremented on each write
- `current_item` (string) — item being worked on
- `current_attempt` (int) — attempt number for current item
- `provider_pid` (int) — PID of spawned claude process
- `provider_started_at` (int64) — Unix timestamp when provider started (prevents PID reuse confusion)
- `started_at` (string, RFC3339) — when this execution started
- `lock_holder` (string) — which command holds the lock

Recovery logic (progress.jsonl is the source of truth, state.json is a hint):
```
1. Load progress.jsonl — find last event by type:
   - Last item_done → that item is complete, next item follows
   - Last item_start without matching item_done → that item was interrupted
   - Last item_stuck → that item's attempt failed, retry or skip
2. If state.json exists, use as optimization hint:
   - Check provider_pid alive AND provider_started_at matches
   - If match → provider still running, attach or kill+resume
   - If no match → stale state, ignore
3. If state.json and progress.jsonl disagree → progress.jsonl wins
   (state.json may have been written but progress.jsonl append failed — the reverse is safe because progress.jsonl is append-only)
4. Delete stale state.json, rebuild runtime state from progress.jsonl
```

### .scrip/scrip.lock

Lock file for concurrency control. One global lock per project.

- `scrip exec` and `scrip land` acquire the lock
- `scrip prep` and `scrip plan` do NOT acquire (safe to run concurrently)
- Lock contains: PID, start time (Unix), 24h max age
- PID alive check + start time match prevents stale lock issues

---

## Prompt Template Inventory

Scrip embeds all prompts at compile time via `//go:embed prompts/*`. Templates use `{{variable}}` substitution.

### Templates (11 total)

| Template | Command | Purpose | Key Variables |
|----------|---------|---------|---------------|
| `consult-item.md` | exec | Per-item framework consultation | `{{framework}}`, `{{item}}`, `{{criteria}}` |
| `consult-feature.md` | plan, land | Feature-level consultation | `{{feature}}`, `{{techStack}}`, `{{frameworks}}` |
| `plan-round.md` | plan | Planning round with research | `{{userInput}}`, `{{consultation}}`, `{{planHistory}}`, `{{codebaseContext}}`, `{{progressContext}}` |
| `plan-verify.md` | plan | Adversarial plan verification | `{{planContent}}`, `{{codebaseContext}}` |
| `exec-build.md` | exec | Item implementation (core build prompt) | `{{item}}`, `{{criteria}}`, `{{consultation}}`, `{{learnings}}`, `{{retryContext}}`, `{{codebaseContext}}`, `{{progressContext}}` |
| `exec-verify.md` | exec | AI deep analysis after DONE | `{{item}}`, `{{criteria}}`, `{{diff}}`, `{{testOutput}}` |
| `land-analyze.md` | land | Comprehensive analysis | `{{allCriteria}}`, `{{fullDiff}}`, `{{verifyResults}}`, `{{consultation}}` |
| `land-fix.md` | land | Fix prompt on land failure | `{{findings}}`, `{{verifyResults}}`, `{{diff}}`, `{{consultation}}` |
| `summary.md` | land | Machine-optimized feature summary | `{{feature}}`, `{{progressEvents}}`, `{{diff}}`, `{{learnings}}` |
| `progress-narrative.md` | exec | Session narrative for progress.md | `{{completedItems}}`, `{{learnings}}`, `{{remainingItems}}` |
| `plan-create.md` | plan | Initial plan creation (first round) | `{{feature}}`, `{{description}}`, `{{consultation}}`, `{{codebaseContext}}` |

### exec-build.md Behavioral Requirements

These instructions MUST appear in exec-build.md. The structure follows the original Ralph PROMPT_build.md: Orient (0a-0c) → Act (1-4) → Guardrails (escalating 9s).

**Phase 0a-0c: Orient**

```
0a. Study the application source code using up to 500 parallel Sonnet subagents to understand the codebase structure, patterns, and conventions.
0b. Study the item description, acceptance criteria, and consultation results provided below.
0c. Study the learnings from previous items provided below. These were captured by earlier iterations — use them to avoid repeating mistakes.
```

Injected context:
- `{{codebaseContext}}` — project structure, tech stack, frameworks
- `{{item}}` + `{{criteria}}` — what to implement and how to verify
- `{{consultation}}` — framework-specific guidance from consultation subagents
- `{{learnings}}` — learnings from previous items in this feature
- `{{retryContext}}` — if retrying: "You are retrying because: [reason]. Do NOT re-implement from scratch. Focus on the specific failure and try a different approach."
- `{{progressContext}}` — narrative context from progress.md

**Phase 1-4: Act**

```
1. Your task is to implement functionality per the item description and acceptance criteria using parallel subagents. Before making changes, search the codebase (don't assume not implemented) using up to 500 parallel Sonnet subagents for searches/reads. You may use only 1 Sonnet subagent for build/tests. Use Opus subagents when complex reasoning is needed (debugging, architectural decisions). Ultrathink.
2. After implementing functionality, run the tests for that unit of code. If functionality is missing then it's your job to add it per the acceptance criteria.
3. Every new function needs at least one test. Cover happy path AND error/edge cases. For items with UI changes: write e2e tests using the project's existing framework.
4. When the tests pass, `git add` the relevant files then `git commit` with message: `feat: <item-title>`.
```

**Guardrails (escalating "9s" — each level is more critical):**

```
99999. Important: Your item only passes if ALL verification commands succeed. Services must remain responsive — a crashed service is a verification failure.
999999. Important: Signal honestly. Use STUCK if you cannot complete. Don't hope DONE works.
9999999. Important: Capture the why — learnings should explain patterns, gotchas, integration points, and non-obvious behaviors. Good learnings are: key files created/modified, codebase patterns discovered, how components connect, non-obvious requirements. Do NOT emit trivial learnings like "I implemented X".
99999999. Important: Implement functionality completely. Placeholders and stubs waste efforts and time redoing the same work.
999999999. Important: Do NOT modify files outside the scope of this item.
9999999999. Important: Single sources of truth, no migrations/adapters. If tests unrelated to your work fail, resolve them as part of the increment.
99999999999. For any bugs you notice, document them via LEARNING markers even if unrelated to the current item.
```

**Markers:**
```
<scrip>DONE</scrip>
<scrip>STUCK:reason</scrip>
<scrip>LEARNING:text</scrip>
```

### plan-round.md Behavioral Requirements

Planning prompts follow the original Ralph PROMPT_plan.md structure.

**Phase 0a-0c: Orient**

```
0a. Study the project source code using up to 500 parallel Sonnet subagents to learn the current codebase structure, existing implementations, patterns, and test coverage.
0b. Study the consultation results provided below — framework-specific guidance and codebase analysis.
0c. Study the planning history below — previous rounds in this planning session (progressively compressed by the CLI from plan.jsonl).
0d. Study the progress history below — what was built before, what failed, what was learned (from progress.md).
```

**Phase 1: Plan**

```
1. Compare existing code against the user's feature request using up to 500 parallel Sonnet subagents. Use an Opus subagent for synthesis and prioritization. Search for incomplete work: TODOs, minimal implementations, placeholders, failing tests. Before assuming functionality is missing, search the codebase to confirm using Sonnet subagents. Plan only. Do NOT implement anything.
```

**Guardrails:**

```
99999. Important: When authoring the plan, capture the why — acceptance criteria must explain importance, not just state facts.
999999. Important: Confirm missing functionality through code search before assuming gaps. Don't assume not implemented.
9999999. If you find inconsistencies in the user's requirements, use an Opus subagent with ultrathink to resolve them and note the resolution.
```

### plan-verify.md Behavioral Requirements

Adversarial verification of the plan draft.

```
Study the plan using up to 500 parallel Sonnet subagents to verify every claim against the actual codebase. For each item and acceptance criterion:
- Does the claimed gap actually exist? (search codebase to confirm)
- Is the acceptance criterion specific and testable? (not "works correctly" but "returns 401 on invalid credentials")
- Does the plan contradict existing codebase patterns?
- Are there missing items the plan doesn't cover?
- Are there security gaps?
Use Opus subagents for architectural analysis and complex trade-off evaluation.
```

### consult-item.md and consult-feature.md Behavioral Requirements

Consultation subagents (spawned by the CLI) instruct the agent to research using its own subagents.

```
Study the cached framework source code using up to 500 parallel Sonnet subagents. Read ACTUAL source files — do NOT rely on training data or memory. Use Opus subagents when evaluating architectural patterns or resolving conflicting approaches. Must cite actual source code (Source: file:line) — citations validated by CLI, uncited guidance treated as hallucination.
```

### exec-verify.md Behavioral Requirements

AI deep analysis after the provider signals DONE. Runs as a non-autonomous subagent (no `--dangerously-skip-permissions`).

**Phase 0a-0c: Orient**

```
0a. Study the diff produced by this item using up to 500 parallel Sonnet subagents.
0b. Study the acceptance criteria and test output provided below.
0c. Study the item's context — what was requested and what the provider claimed to have implemented.
```

**Phase 1: Verify**

```
1. For each acceptance criterion, trace the implementation through the diff. Verify:
   - The criterion is addressed by actual code changes (not just test assertions)
   - Edge cases are handled (nil, empty, error, boundary values)
   - No regressions to existing functionality
   - Tests cover the claimed behavior (not just "test exists" but "test validates criterion")
   Use up to 500 parallel Sonnet subagents for code analysis. Use Opus subagents for complex logic verification.
```

**Guardrails:**

```
99999. Important: Base your verdict on code reality, not provider claims. The provider may say DONE when work is incomplete.
999999. Important: A passing test suite alone does NOT mean criteria are met. Verify the logic, not just the test results.
```

**Markers:**
```
<scrip>VERIFY_PASS</scrip>
<scrip>VERIFY_FAIL:specific reason — which criterion failed and why</scrip>
```

Multiple VERIFY_FAIL markers supported (one per failed criterion).

### land-analyze.md Behavioral Requirements

Comprehensive analysis for landing. This is the final quality gate before the feature is accepted.

**Phase 0a-0c: Orient**

```
0a. Study the full diff (all commits for this feature) using up to 500 parallel Sonnet subagents.
0b. Study all acceptance criteria across all plan items.
0c. Study the verification command output (typecheck, lint, test results) provided below.
```

**Phase 1: Analyze**

```
1. For each plan item's acceptance criteria, trace the implementation through the diff. Verify criteria are met by actual code, not just test assertions. Check edge cases: nil, empty, error, timeout, boundary values, concurrent access.
2. Architecture review using Opus subagents: cross-item consistency, no conflicting patterns, no dead code, no orphaned imports.
3. Security audit using Opus subagents: OWASP top 10, input validation at boundaries, no hardcoded secrets, no command injection.
4. Only 1 subagent for running any build/test verification.
```

**Guardrails:**

```
99999. Important: Base verdicts on code reality. Trace every criterion to actual implementation.
999999. Important: "Could be improved" is not a failure. Only report concrete, demonstrable issues.
```

**Markers:**
```
<scrip>VERIFY_PASS</scrip>
<scrip>VERIFY_FAIL:specific finding — which criterion or quality gate failed</scrip>
```

Multiple VERIFY_FAIL markers supported. Each must cite a specific file:line and explain the issue.

### land-fix.md Behavioral Requirements

Fix prompt when landing fails.

```
Study the verification failures using up to 500 parallel Sonnet subagents to understand root causes. Use Opus subagents for debugging complex failures. Implement fixes using only 1 Sonnet subagent for build/tests. Do NOT re-implement from scratch — focus on the specific failures identified.
```

### summary.md Behavioral Requirements

This template generates the permanent record of a completed feature. Machine-optimized, not narrative prose.

Required sections:
- Implementation map (key files with purpose)
- Data models and APIs (schemas, endpoints, tables)
- Patterns and conventions (architectural decisions constraining future work)
- Integration points (imports, shared state, dependencies)
- Gotchas (non-obvious behaviors, workarounds, constraints)
- Skipped items (what and why)

Guidance: "Every sentence must contain a file path, function name, or concrete technical detail. Do NOT write narrative prose."

### plan-create.md Behavioral Requirements

First-round planning prompt. Used when no plan.md exists yet. Produces the initial plan draft.

**Phase 0a-0c: Orient**

```
0a. Study the project source code using up to 500 parallel Sonnet subagents to understand architecture, patterns, existing implementations, and test coverage.
0b. Study the feature description and consultation results provided below.
0c. Study the progress context below — any prior execution history, learnings, and land failure findings for this feature.
```

**Phase 1: Plan**

```
1. Design a plan for the requested feature. For each item:
   - Title: short, imperative ("Add OAuth2 login flow", not "OAuth2")
   - Acceptance criteria: specific, testable conditions (not "works correctly" but "returns 401 on invalid token")
   - Size: each item must be completable in one autonomous execution (~15-30 min). If larger, split.
   - Dependencies: note if an item depends on another being completed first
   Search the codebase using up to 500 parallel Sonnet subagents before assuming gaps. Use Opus subagents for architectural decisions. If the user's description is ambiguous, note assumptions clearly rather than guessing.
```

**Guardrails:**

```
99999. Important: Items must be sized for one exec loop iteration. An item that requires multiple files across multiple subsystems should be split.
999999. Important: Acceptance criteria must be verifiable by automated tests. "Looks good" is not a criterion.
9999999. Important: Don't assume not implemented — search the codebase to confirm gaps before including items.
```

---

## Codebase Reuse Map

The existing ralph codebase provides the foundation. ~75% of source code copies directly or adapts with minor modifications into scrip v1. After this rewrite, ralph ceases to exist — only scrip remains.

### Direct Copy (no changes needed)

| File | LOC | Purpose |
|------|-----|---------|
| `atomic.go` | 54 | AtomicWriteJSON, AtomicWriteFile |
| `git.go` | 251 | Git operations (branch, commit, diff) |
| `services.go` | 266 | Service management (start/stop/health) |
| `consultation.go` | 619 | Parallel subagent consultation |
| `resolve.go` | 906 | Dependency resolution (npm, Go, PyPI, crates, hex) |
| `external_git.go` | 184 | Git clone operations |
| `prompts.go` | 435 | Template rendering with {{var}} substitution |
| `refine.go` | 58 | Refine session logic |
| **Total** | **~2,773** | |

### Adapt (minor to moderate modifications)

| File | LOC | Changes |
|------|-----|---------|
| `lock.go` | 169 | Rename ralph→scrip in lock path and error messages |
| `schema.go` | 350 | Rename ralph→scrip types (PRDDefinition→PlanDefinition, etc.), adapt for plan.md model |
| `feature.go` | 211 | Rename `.ralph/`→`.scrip/` directory paths, update feature dir format |
| `resources.go` | 307 | Rename `~/.ralph/`→`~/.scrip/` cache paths |
| `resourcereg.go` | 9 | Rename `~/.ralph/`→`~/.scrip/` registry path |
| `resourceregistry.go` | 160 | Rename `~/.ralph/`→`~/.scrip/` cache paths |
| `discovery.go` | 751 | Rename `.ralph/`→`.scrip/` references in project detection |
| `logger.go` | 849 | Rename `ralph`→`scrip` in log paths and messages |
| `config.go` | 405 | Remove provider abstraction (knownProviders map), hardcode Claude Code, add --model/--effort, rename ralph→scrip paths |
| `loop.go` | 1,413 | Rename ralph→scrip markers, remove multi-provider support, adapt for plan.md item selection + progress.jsonl, add --model/--effort to spawn args |
| `cleanup.go` | 96 | Rename lock path, adjust signal handling |
| `prd.go` | 270 | Adapt for CLI-mediated rounds (remove interactive sessions, add plan.jsonl round tracking) |
| **Total** | **~4,990** | |

### Rewrite / New

| File | LOC (est) | Purpose |
|------|-----------|---------|
| `main.go` | ~80 | New CLI dispatcher (prep/plan/exec/land) |
| `cmd_prep.go` | ~150 | Extract from commands.go, add harness audit |
| `cmd_plan.go` | ~250 | CLI-mediated rounds, plan.jsonl management |
| `cmd_land.go` | ~200 | Land flow (verify + summary + purge + push) |
| `plan.go` | ~150 | plan.md parsing/writing, plan.jsonl compression |
| `progress.go` | ~120 | progress.jsonl append/query, progress.md append |
| `state.go` | ~80 | state.json checkpoint, recovery logic |
| **Total** | **~1,030** | |

### Delete (not needed in scrip v1)

| File | Reason |
|------|--------|
| `upgrade.go` | Different upgrade mechanism |
| `update_check.go` | Different update check |
| `utils.go` | Consolidated elsewhere |
| `commands.go` | Split into cmd_*.go files |

### Test Files

All `*_test.go` files for copied modules copy directly. New modules need new tests (~500-800 LOC estimated). Total test codebase: ~9,000-10,000 LOC.

---

## Implementation Sessions

### Session 0: Foundation (must complete first)

**Goal:** New CLI entry point + adapted config + state/progress infrastructure.

**Files to create:**
- `main.go` — CLI dispatcher for prep/plan/exec/land
- `state.go` — state.json checkpoint read/write/recovery
- `progress.go` — progress.jsonl append/query, progress.md append, rotation
- `plan.go` — plan.md parse/write, plan.jsonl append/query, compression

**Files to adapt:**
- `config.go` — remove knownProviders/promptMode/promptFlag/knowledgeFile, hardcode claude with `--print --model opus --effort max`, rename paths (ralph→scrip), update `WriteDefaultConfig`

**Files to adapt:**
- `lock.go` — rename ralph→scrip paths
- `schema.go` — rename types for plan.md model

**Files to copy as-is:**
- `atomic.go`

**Tests:** Unit tests for all new functions. Table-driven for plan.md parsing, progress event types.

**Exit criteria:** `go build` produces `scrip` binary. `go test ./...` passes. Config generates `.scrip/config.json`. State/progress/plan read/write works.

**Estimated LOC:** ~600 new + ~920 adapted + ~54 copied

---

### Session 1: scrip prep (parallel with Session 2)

**Goal:** Project setup command.

**Files to create:**
- `cmd_prep.go` — project detection, config generation, dependency resolution, harness audit

**Files to adapt:**
- `discovery.go` — rename `.ralph/`→`.scrip/` references
- `resources.go` — rename `~/.ralph/`→`~/.scrip/` cache paths
- `resourcereg.go` — rename `~/.ralph/`→`~/.scrip/` registry path
- `resourceregistry.go` — rename `~/.ralph/`→`~/.scrip/` cache paths

**Files to copy as-is:**
- `resolve.go`, `external_git.go`

**Reuse from ralph:** `cmdInit()` logic from commands.go (prompt for verify commands, services).

**Key behaviors:**
- Detect project type, package manager, test framework, linter
- Generate `.scrip/config.json` with detected verify commands
- Create `.scrip/.gitignore` (ignore: scrip.lock, */logs/, state.json)
- Resolve dependencies → cache to `~/.scrip/resources/`
- Report harness gaps (test coverage, linter config, SAST tools)
- Safe to re-run anytime (regenerates, preserves user customizations)

**Tests:** `TestDetectProject`, `TestGenerateConfig`, `TestResolveDeps`, `TestPrepIdempotent`

**Exit criteria:** `scrip prep` on a Go/Node/Elixir project produces correct config.

**Estimated LOC:** ~150 new + ~1,227 adapted + ~1,090 copied

---

### Session 2: scrip plan (parallel with Session 1)

**Goal:** CLI-mediated planning rounds.

**Files to create:**
- `cmd_plan.go` — round orchestration, user input loop, plan.md finalization

**Files to adapt:**
- `feature.go` — rename `.ralph/`→`.scrip/` directory paths

**Files to copy as-is:**
- `consultation.go`, `git.go`

**New prompt templates:**
- `plan-round.md`, `plan-verify.md`, `plan-create.md`

**Key behaviors:**
- `scrip plan <feature> "description"` — create feature dir + branch, start planning
- Each round: pre-compute consultation → build prompt → spawn `claude --print --model opus --effort max` → show response → get user feedback
- Round history: append to plan.jsonl with progressive compression
- Finalize: write plan.md, run adversarial verification (plan-verify.md), show warnings, emit `plan_created` event to progress.jsonl
- Resume: reads plan.jsonl + progress.jsonl + progress.md for context
- Land failure context: detect `land_failed` events in progress.jsonl, inject findings

**Tests:** `TestPlanRound`, `TestPlanResume`, `TestPlanFinalize`, `TestPlanCompression`, `TestPlanLandFailureContext`

**Exit criteria:** Full planning cycle works: create → rounds → finalize → resume.

**Estimated LOC:** ~250 new + ~211 adapted + ~870 copied

---

### Session 3: scrip exec (requires Session 0)

**Goal:** Autonomous execution loop — the core Ralph technique.

**Files to adapt:**
- `loop.go` — rename markers (ralph→scrip), hardcode claude args with --model opus --effort max, adapt for plan.md item selection + progress.jsonl tracking (significant changes — PRD→Plan model)
- `logger.go` — rename ralph→scrip in log paths and messages

**Files to copy as-is:**
- `services.go`, `prompts.go`

**New prompt templates:**
- `exec-build.md`, `exec-verify.md`

**Key behaviors:**
- Read plan.md, iterate items
- Per item: consult → build prompt → spawn `claude --print --dangerously-skip-permissions --model opus --effort max` → detect markers → verify → advance/retry
- Verify-at-top: before each item, re-run verify commands for previously attempted items
- Retry: failure classification + diff from failed attempt + structured retry context
- STUCK: log reason, retry up to threshold, then skip
- LEARNING: persist to progress.jsonl, inject into next spawn
- Resume from crash: read progress.jsonl for last completed item
- Quick fix: `scrip exec "fix X"` auto-generates 1-item plan
- After all items: append session narrative to progress.md

**Tests:** `TestExecLoop`, `TestExecResume`, `TestExecRetry`, `TestExecQuickFix`, `TestExecVerifyAtTop`, `TestExecMarkerDetection`

**Exit criteria:** Full exec cycle: items pass, progress tracked, retry works, resume works.

**Estimated LOC:** ~400-500 new adaptation (loop.go is 1,413 LOC with significant PRD→Plan model changes) + ~700 copied

---

### Session 4: scrip land (requires Sessions 2 + 3)

**Goal:** Final verification, summary, and artifact push.

**Files to create:**
- `cmd_land.go` — verification pipeline, fix loop, summary generation, plan purge

**New prompt templates:**
- `land-analyze.md`, `land-fix.md`, `summary.md`, `progress-narrative.md`

**Key behaviors:**
- Run all verify commands (typecheck, lint, test)
- Run AI deep analysis via `claude --print --model opus --effort max` (land-analyze.md)
- If pass:
  - Generate feature summary (summary.md template via `claude --print --model opus --effort max`)
  - Append final narrative to progress.md
  - Purge plan.md + state.json
  - Commit and push artifacts
- If fail:
  - Spawn fix agent (`claude --print --dangerously-skip-permissions --model opus --effort max`)
  - Re-verify after fix
  - Max 3 fix attempts
  - On 3rd failure: write `land_failed` to progress.jsonl with findings, exit with instructions
- Land failure → plan loop: "Run `scrip plan` to rethink, or `scrip exec` to fix."

**Tests:** `TestLandPass`, `TestLandFail`, `TestLandFixLoop`, `TestLandFailFindings`, `TestLandSummaryGeneration`

**Exit criteria:** Land pass flow works (verify → summary → purge → push). Land fail flow works (findings → progress.jsonl → plan picks them up).

**Estimated LOC:** ~200 new

---

### Session 5: Integration + Polish (requires all above)

**Goal:** End-to-end integration, rename completion, cleanup.

**Tasks:**
- Rename all internal references: ralph → scrip (paths, error messages, comments)
- Rename markers: `<ralph>` → `<scrip>`
- Wire all commands into main.go dispatcher
- Integration tests: full prep → plan → exec → land cycle with mock provider
- E2E test skeleton (tagged `e2e`, real claude, 60min timeout)
- `.scrip/.gitignore` template
- Update go.mod module path if needed
- Remove deleted files (upgrade.go, update_check.go, utils.go, commands.go)

**Tests:** `TestPrepPlanExecLand` (integration), `TestResumeFromCrash` (integration), `TestLandFailPlanLoop` (integration)

**Exit criteria:** `go test ./...` passes. `go vet ./...` clean. `go build` produces working `scrip` binary. Integration tests pass with mock provider.

**Estimated LOC:** ~250 modified across codebase

---

## Cross-Session Contract

Each session's agent needs:

1. **This document** (PHASE0-SPEC.md) — always loaded, provides all schemas, templates, and session scope
2. **ROADMAP.md Phase 0 section** (lines 471-1048) — the design document for context
3. **Session-specific source files** — listed in each session's "Files to copy/adapt/create"
4. **Completed dependency sessions' output** — read actual implementations, not just types

Each session produces:
- Working code that compiles (`go build`)
- Unit tests that pass (`go test ./...`)
- No lint warnings (`go vet ./...`)

### What agents should NOT do:
- Modify files outside their session scope
- Add features not in this spec
- Create documentation files (README, etc.)
- Refactor code they didn't write
- Add dependencies not already in go.mod

### Prompt for spawning implementation agents:

```
You are implementing Session N of the scrip v1 CLI — a new CLI that replaces ralph entirely. Read PHASE0-SPEC.md for your session scope, schemas, and exit criteria. Read ROADMAP.md lines 471-1048 for design context. Read the source files listed in your session using up to 500 parallel Sonnet subagents. Use Opus subagents for complex architectural decisions. Use only 1 subagent for build/tests. Implement, test, and verify your session's exit criteria.
```

---

## Testing Architecture

Every exported function has at least one test. Changed behavior has tests covering the new behavior.

### Module test expectations

| Module | Key tests | What they cover |
|--------|----------|----------------|
| `cmd_prep.go` | `TestDetectProject`, `TestGenerateConfig`, `TestResolveDeps` | Project detection, config generation, dependency caching |
| `cmd_plan.go` | `TestPlanRound`, `TestPlanResume`, `TestPlanFinalize` | Round orchestration, resume from plan.jsonl, plan.md generation |
| `cmd_exec.go` | `TestExecLoop`, `TestExecResume`, `TestExecRetry`, `TestExecQuickFix` | Item loop, crash recovery, retry with failure classification, quick fix shortcut |
| `cmd_land.go` | `TestLandPass`, `TestLandFail`, `TestLandFailFindings` | Pass flow (purge + push), fail flow (findings → progress.jsonl) |
| `consultation.go` | `TestConsultParallel`, `TestConsultCitationValidation`, `TestConsultCaching` | Parallel subagent dispatch, citation validation, result caching |
| `plan.go` | `TestPlanParse`, `TestPlanWrite`, `TestPlanJsonlAppend`, `TestContextReconstruction` | YAML frontmatter, plan.md write, plan.jsonl append/query, progressive context compression |
| `progress.go` | `TestProgressAppend`, `TestProgressQuery`, `TestProgressMdAppend` | Event append, event query by type, progress.md narrative append |
| `state.go` | `TestAtomicWrite`, `TestCrashRecovery`, `TestLockAcquire` | Atomic JSON writes, resume from state.json, lock file management |
| `prompts.go` | `TestTemplateRender`, `TestStrictRendering`, `TestAllVariables` | Variable injection, no leftover `{{...}}` patterns, all templates render cleanly |

### Integration tests

Test command-level flows with a mock `claude --print` (fake provider binary that reads prompt from stdin, returns canned responses with markers):

- `TestPrepIntegration` — `scrip prep` on a real project directory, verify config generated correctly
- `TestPlanIntegration` — full plan round: consultation subagents → planning → verification
- `TestExecIntegration` — full item loop: consult → spawn → DONE marker → verify → advance
- `TestLandIntegration` — pass and fail paths with mock deep analysis
- `TestResumeFromCrash` — kill mid-exec, restart, verify resume from progress.jsonl
- `TestLandFailPlanLoop` — land fails → plan with injected findings → exec → land passes

Mock provider: a test binary (or `testProvider(responses map[string]string)` helper) that maps prompt substrings to canned responses with markers. Same `--print` interface as real Claude Code.

### E2E test

Full end-to-end test with real `claude --print` (requires API key, tagged `e2e`, 60min timeout):

```bash
make test-e2e  # go test -tags e2e -timeout 60m -v -run TestE2E ./...
```

E2E test project: a minimal but real project in `testdata/e2e-project/` (e.g., Go HTTP server with 2-3 endpoints). Has intentional gaps that scrip should detect and fix: missing tests for one endpoint, a TODO placeholder in one handler, linter warnings. Validates the full prep → plan → exec → land cycle.

### Test patterns

- **Temp directories** for tests touching `.scrip/` or `.git/` — `t.TempDir()` with setup helpers
- **Table-driven tests** for classification logic (failure classification, project detection, marker parsing)
- **`go test ./...` and `go vet ./...`** must pass — enforced in CI
- **Build tags**: `e2e` for real-provider tests, no tag for unit/integration (fast, no API key needed)
- **Test helpers**: `setupTestProject(t)` creates temp dir with `.git/` + `.scrip/`, `setupTestConfig(t)` writes valid config

---

## Partially Specified Details

These items have design intent in ROADMAP.md but need implementation decisions. Each session's agent should handle them as described.

### PID reuse in state.json

state.json stores `provider_pid` and `provider_started_at`. On recovery, check BOTH:
- Is the PID alive? (`os.FindProcess` + signal 0)
- Does the start time match? (compare `provider_started_at` against process creation time)
- If PID alive but start time differs → stale PID, ignore it
- If PID dead → stale, resume from progress.jsonl

### Lock scope

- `scrip.lock` at `.scrip/scrip.lock` (project-level, not per-feature)
- `scrip exec` and `scrip land` acquire before starting, release on exit
- `scrip prep` and `scrip plan` do NOT acquire (concurrent-safe)
- Lock contains: PID, start time (Unix), 24h max age
- Stale lock detection: same as ralph's lock.go

### File growth

- progress.jsonl: rotate at 10,000 lines to progress.jsonl.1, .2, etc.
- Only current file loaded into prompts
- plan.jsonl: no rotation (per-feature, bounded by planning rounds)

### Consultation timeout

- 30 seconds per consultation subagent call
- 120 seconds total consultation budget per exec iteration
- If exceeded, proceed without consultation (log warning)
- Consultation enhances but never blocks execution

### Quick fix flow

`scrip exec "fix the login button"`:
1. CLI generates a 1-item plan.md with the description as the item
2. No planning rounds, no plan.jsonl entry
3. Same exec loop, same verification, same progress tracking
4. Useful for targeted fixes without full planning cycle

### Cross-feature learnings

Deferred to post-Phase 0. Each feature's progress is self-contained. No mechanism for sharing learnings across features in Phase 0.

### Container isolation

Deferred to post-Phase 0. Phase 0 provides filesystem isolation (`.scrip/` only). Container support (Docker, E2B) is a future enhancement.

---

## Marker Protocol

Renamed from ralph's markers:

```
<scrip>DONE</scrip>
<scrip>STUCK:reason text here</scrip>
<scrip>LEARNING:insight text here</scrip>
```

Verification markers (exec-verify.md, land-analyze.md):
```
<scrip>VERIFY_PASS</scrip>
<scrip>VERIFY_FAIL:specific reason</scrip>
```

Detection: whole-line matching (not substring). DONE uses exact `==`. STUCK/LEARNING/VERIFY_FAIL use anchored regex. Multiple VERIFY_FAIL markers supported in one response.

### Marker-driven state machine

| Marker | CLI Action |
|--------|-----------|
| DONE | Run verification commands. If pass → advance. If fail → retry. |
| STUCK | Log reason. Increment attempt. If at threshold → skip item. |
| LEARNING | Persist to progress.jsonl. Inject into next spawn's prompt. |
| VERIFY_PASS | Accept item as verified. |
| VERIFY_FAIL | Report failure. In exec: retry. In land: attempt fix or fail. |
| No marker + timeout | Kill process group. Log timeout. Retry as stall. |
| No marker + exit | Treat as stuck. Log exit code. Retry. |

---

## Post-Implementation Refinement

After Phase 0 implementation is complete and all sessions pass their exit criteria, each command will need further refinement based on real-world usage:

- **scrip prep** — harness audit heuristics, additional project type detectors, config migration for format changes
- **scrip plan** — planning round UX polish, consultation quality tuning, plan.md format finalization (see pending refinement note in State Schemas)
- **scrip exec** — retry heuristics, failure classification accuracy, stall detection tuning, verify-at-top performance
- **scrip land** — fix agent prompt tuning, summary quality, land failure message clarity, push workflow options

This refinement work happens iteratively after the core implementation is functional. Each command's refinement is scoped by real usage patterns — not speculated in advance.
