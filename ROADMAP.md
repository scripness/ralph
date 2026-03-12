# Roadmap: Ralph → Scrip

**Last updated:** 2026-03-11

This document is the single source of truth for Ralph's evolution into scrip. Every item here is concrete and implementable — vague aspirations have been cut. Items are organized by priority based on convergent evidence from multiple analyses.

**Research sources distilled into this document:**
- Ralph codebase audit (20K LOC, 47 source files)
- Symphony SPEC.md pattern extraction (16 reusable patterns evaluated)
- Symphony comparative analysis (daemon orchestrator, Codex-only)
- Stripe Minions analysis (shift-left verification, structured retry)
- Tidewave analysis (MCP runtime intelligence, service log surfacing)
- T3 Code analysis (worktree isolation, visual diff review)
- JetBrains Air analysis (ACP protocol, execution isolation tiers)
- ROADMAP accuracy improvement research (32 verified code claims)
- Security audit system design (4-layer architecture)
- Claude Code integration research (skills, hooks, MCP, plugin system, `--print` mode behavior)
- Two-mode architecture design session (interactive brainstorming + autonomous execution coexistence)

---

## Table of Contents

- [Phase 0: CLI Redesign — Scrip v3 Architecture](#phase-0-cli-redesign--scrip-v3-architecture)
- [Phase 1: Accuracy](#phase-1-accuracy)
- [Phase 2: Security](#phase-2-security)
- [Phase 3: Extensibility & Integration](#phase-3-extensibility--integration)
- [Phase 4: Quality of Life](#phase-4-quality-of-life)
- [Deferred](#deferred)

---

## Phase 1: Accuracy

Three independent analyses (Symphony failure classification, Stripe Minions structured feedback, ROADMAP accuracy gaps) converge on the same conclusion: Ralph's retry loop works but wastes attempts because providers get insufficient context about *why* the previous attempt failed and *what* the current codebase state is. These improvements target convergence speed — fewer retries to reach passing.

**Estimated total: ~600 LOC new, ~200 LOC modified**

### 1.1 Structured Retry Context with Failure Classification

**Sources:** ROADMAP accuracy Phase 1, Symphony SPEC Section 14.1, Minions structured feedback
**Priority:** Highest — directly improves retry success rate

Ralph currently passes `lastFailure` (last 50 lines of failed command output) and `retryInfo` ("Attempt 2 of 3") on retries. Providers don't know whether the failure was a compile error, test failure, lint violation, or timeout — and can't see what code changed in the failed attempt.

**Implementation:**

Add to `schema.go`:
```go
// RunState additions
LastDiff        map[string]string  // story ID → git diff from failed attempt
LastTestOutput  map[string]string  // story ID → last test output (truncated)
FailureClass    map[string]string  // story ID → classified failure type
```

Add to `loop.go` (~80 LOC):
- `classifyFailure(output string) string` — returns one of: `"compile"`, `"test"`, `"lint"`, `"timeout"`, `"service"`, `"no-commit"`, `"stuck"`, `"unknown"`
- After failed verification: capture `git diff` from pre-run commit hash
- Store classification + diff + test output in RunState

Add to `prompts.go` (~40 LOC):
- New template variables: `{{retryDiff}}`, `{{retryTestOutput}}`, `{{retryClassification}}`
- Populate from RunState on retry iterations

Update `prompts/run.md` (~20 LOC):
- Add structured retry section: "This is attempt {{attempt}} of {{maxRetries}}. Previous attempt failed with: {{retryClassification}}. Changes from failed attempt: {{retryDiff}}. Error output: {{retryTestOutput}}"

**~170 LOC total.** Test: `TestClassifyFailure` (classification logic), `TestRunPrompt_RetryContext` (template renders correctly).

### 1.2 Pre-Flight Baseline Capture

**Sources:** Minions shift-left verification, ROADMAP accuracy Phase 3.2

Providers currently don't know the baseline state. If 42 tests pass before implementation, the provider should know that — and know which tests are relevant to their story. After DONE, comparing against baseline detects regressions the provider introduced.

**Implementation:**

Add to `schema.go` (~10 LOC):
```go
BaselineFailures []string  // verification failures captured before first story attempt
```

Add to `loop.go` (~50 LOC):
- Before main story loop (after services start): if `state.BaselineFailures == nil`, run verification commands with no story context
- Store any pre-existing failures in `BaselineFailures`
- On retry, include baseline in prompt: "These failures existed before your attempt — do not fix them"

Update `prompts/run.md` (~10 LOC):
- Add `{{baselineInfo}}` section when baseline failures exist

**~70 LOC total.** Test: `TestBaselineCapture` (pre-existing failures stored), `TestBaselineExcludedFromRetry` (baseline failures not blamed on provider).

### 1.3 Collect All Verification Failures

**Sources:** Minions parallel feedback, ROADMAP accuracy Phase 3.1

`runStoryVerification` currently returns on first failure. Provider only sees "test failed" but not that lint also failed. Full error picture enables targeted fixes.

**Implementation:**

Modify `loop.go` (~50 LOC):
- Change `StoryVerifyResult` to collect `failures []string` instead of single `reason string`
- Remove early returns in verification loop — run all commands, collect all results
- Format all failures into `lastFailure` for retry prompt

**~50 LOC modified.** Test: `TestStoryVerification_CollectsAllFailures`.

### 1.4 Service Log Injection

**Sources:** Tidewave `get_logs` pattern, ROADMAP accuracy Phase 2

Ralph's `ServiceManager.GetRecentOutput()` already captures service output (256KB buffer). This data is never shown to the AI. When a service crashes or logs errors during implementation, the provider should know.

**Implementation:**

Add to `prompts.go` (~30 LOC):
- `buildServiceLogs(svcMgr, services) string` — extract ERROR/PANIC/WARN lines from service output

Add `{{serviceLogs}}` variable to `prompts/run.md`, `prompts/verify-fix.md`, `prompts/verify-analyze.md` (~20 LOC each, ~50 LOC total plumbing in prompts.go)

**~80 LOC total.** Test: `TestBuildServiceLogs` (filters errors from output).

### 1.5 Service Health Check Before Provider Spawn

**Sources:** ROADMAP accuracy Phase 2.1

Services can crash between story iterations. Provider spawns into a broken environment, wastes an attempt on "connection refused" errors, gets classified as test failure.

**Implementation:**

Add to `services.go` (~20 LOC):
- `ServiceManager.EnsureHealthy()` — check ready URLs for all started services, restart any that are down

Add to `loop.go` (~20 LOC):
- Call `EnsureHealthy()` before `runProvider()` (line ~269)

**~40 LOC total.** Test: `TestEnsureHealthy_RestartsDown`.

### 1.6 Procedural Recipes in Prompts

**Sources:** Symphony SKILL.md pattern (12-step commit procedure vs Ralph's 4-line instruction)

Provider improvisation on mechanical tasks (staging, committing, test execution order) causes avoidable retries. Explicit step-by-step procedures reduce variance.

**Implementation:**

Update `prompts/run.md` (~50 lines):
- Explicit staging procedure: verify scope before `git add`, avoid `git add .`
- Verification sequence: run all verify commands, confirm pass, then signal DONE
- Test writing patterns: match existing test framework and naming conventions
- Commit message format enforcement

**0 Go code, ~50 lines of prompt text.** No test needed (behavioral, measured via retry rates).

### 1.7 Two-Timer Timeout Model

**Sources:** Symphony SPEC Section 5.2.3/10.4

Ralph has a single `provider.timeout` (30min hard limit). No detection for alive-but-idle providers (stuck in reasoning loop, waiting for input). A provider could produce no output for 29 minutes and only get killed at the hard limit.

**Implementation:**

Add to `config.go` (~10 LOC):
- `provider.stallTimeout` field (default 300s / 5min)

Add to `loop.go` (~40 LOC):
- Track `lastOutputTime` in `processLine()`
- Second timer: if `time.Since(lastOutputTime) > stallTimeout`, kill provider process group
- Log stall event with duration

**~50 LOC total.** Test: `TestStallDetection` (mock provider with no output triggers kill).

---

## Phase 2: Security

AI-generated code has disproportionately high rates of logic-level security flaws (missing authorization, workflow manipulation) rather than pattern-level bugs. Ralph currently has near-zero security coverage: two bullet points in `verify-analyze.md`, no SAST detection, no security metadata in PRD schema.

This is a layered design — each layer adds value independently.

**Estimated total: ~1,400 LOC new, ~200 LOC modified**

### 2.1 Layer 1: Security Awareness in PRD

Add conditional security questions to `prd-create.md` that trigger for features involving user data, authentication, external APIs, or financial transactions. Add `SecurityTags []string` field to `StoryDefinition` in `schema.go`.

`prd-finalize.md` validates that security-tagged stories have specific, testable security criteria (not just "be secure" — must be "Invalid credentials return 401", "Passwords stored as bcrypt hash").

**~100 LOC** (schema field, prompt additions, validation).

### 2.2 Layer 2: Security Guidance in Implementation

Add `{{securityFocus}}` template variable to `prompts/run.md`. For stories with SecurityTags, populate with framework-aware security patterns (CSRF middleware for Phoenix, parameterized queries for Ecto, bcrypt for password hashing).

Extend `consult.md` to search cached framework source for security APIs, auth patterns, input validation utilities.

**~200 LOC** (prompts.go population logic, consultation security search, template additions).

### 2.3 Layer 3: Security in Verification

Extend `discovery.go` to detect SAST tools: Semgrep (cross-language), gosec (Go), Sobelow (Elixir), Bandit (Python). Pipe SAST output to AI for triage (filter false positives).

Replace vague "are there security issues?" in `verify-analyze.md` with tag-specific checklists:
- **auth** stories: session management, password hashing, rate limiting, CSRF
- **data-access** stories: parameterized queries, authorization checks, IDOR prevention
- **api** stories: input validation, authentication, rate limiting, error information leakage
- **input-validation** stories: XSS prevention, injection, file upload restrictions

**~240 LOC** (discovery.go extension, verify-analyze.md checklists).

### 2.4 Layer 4: `scrip secure` Command

Standalone deep security audit with 4 components:

| Component | Duration | What It Does |
|-----------|----------|-------------|
| SAST baseline | ~2min | Run detected SAST tools, capture structured output |
| Dependency audit | ~1min | `mix deps.audit` / `npm audit` / `cargo audit` for known CVEs |
| AI deep analysis | ~15min | Architecture-level review: auth flow consistency, trust boundaries, middleware gaps |
| AI triage | ~5min | Review SAST results for false positives, prioritize real issues |

For Elixir/Phoenix specifically: Sobelow (`mix sobelow --format json`), `mix deps.audit`, `mix hex.audit`.

Output: `SecurityReport` type with findings, severity, recommendations. Formatted for console and optionally JSON.

**~800 LOC** (secure.go command handler, prompts/secure-analyze.md, SecurityReport type, discovery.go SAST detection).

### 2.5 Lightweight Security in Verify

Integration point: run SAST + dependency audit as part of normal `ralph verify` (always-on, fast). Deep AI audit remains `scrip secure` only (on-demand, slow).

**~60 LOC** (wire SAST commands into verify pipeline).

---

## Phase 3: Extensibility & Integration

These features make Ralph usable in more contexts without changing the core orchestration model.

**Estimated total: ~900 LOC new, ~150 LOC modified**

### 3.1 Lifecycle Hooks

**Sources:** Symphony SPEC Section 5.3.4/9.4

Ralph hardcodes all lifecycle operations in Go. Users can't inject dependency installation, DB migrations, cache warming, or cleanup between stories. This blocks real projects that need pre-flight setup.

**Implementation:**

Config addition:
```json
{
  "hooks": {
    "beforeStory": "npm ci && prisma migrate deploy",
    "afterStory": "rm -rf .next/.cache",
    "beforeVerify": "mix deps.compile",
    "timeout": 60
  }
}
```

New `hooks.go` (~80 LOC):
- `RunHook(name, command, timeout, failureMode)` — shell exec with timeout, process group kill, error classification
- `failureMode`: `"fatal"` (abort story attempt) or `"warn"` (log and continue)

Config extension (~40 LOC):
- `HooksConfig` struct with optional string fields + timeout

Loop integration (~40 LOC):
- 4 call sites: before provider spawn, after provider exit, before verification, before archive

**~160 LOC total.** Test: `TestRunHook_Timeout`, `TestRunHook_FatalFailure`, `TestRunHook_WarnContinues`.

### 3.2 MCP Server Management

**Sources:** Tidewave MCP architecture, ROADMAP accuracy Phase 4.1

Providers that support MCP (Claude Code, Codex) can connect to MCP servers for runtime intelligence (Tidewave, custom tools). Ralph should manage MCP configuration so users don't manually configure each provider.

**Implementation:**

Config addition:
```json
{
  "mcp": {
    "tidewave": {
      "url": "http://localhost:4000/tidewave/mcp"
    }
  }
}
```

New logic in services/mcp area (~150 LOC):
- `MCPConfig` and `MCPServerConfig` types in config.go
- `generateMCPJson(projectRoot, mcpCfg)` — write `.mcp.json` for Claude Code auto-discovery
- `cleanupMCPJson(projectRoot)` — remove after provider exits
- Add to `CleanupCoordinator` for crash cleanup

Integration (~50 LOC):
- After services ready + before provider spawn: write `.mcp.json`
- After provider exit: cleanup `.mcp.json`

**~200 LOC total.** Test: `TestGenerateMCPJson`, `TestCleanupMCPJson`, `TestMCPConfig_Validation`.

### 3.3 Linear Push-Only Integration

**Sources:** SCRIP-PLAN integration analysis

Lightweight dashboard sync — scrip pushes story status to Linear for visibility. Linear is a viewport, not the orchestrator. PRD remains source of truth.

**Implementation:**

New `linear.go` (~200 LOC):
- GraphQL client with `IssueCreate` and `IssueUpdate` mutations
- After `scrip prd` finalize: create Linear issues for each story
- After each story pass/skip in `scrip run`: update Linear issue status
- After `scrip verify` success: close all Linear issues

Config addition:
```json
{
  "linear": {
    "apiKey": "$LINEAR_API_KEY",
    "teamId": "TEAM-ID"
  }
}
```

Constraints:
- Push-only (no polling, no daemon, no state sync)
- API key from env var expansion (never stored in config)
- All failures logged and ignored (informational, not critical path)

**~250 LOC total.** Test: `TestLinearClient_IssueCreate`, `TestLinearClient_EnvVarExpansion`, `TestLinearClient_FailureIgnored`.

### 3.4 Machine-Readable Status

**Sources:** Symphony observability Section 13

`ralph status` is human-readable only. Machine-readable output enables CI/CD integration, Slack notifications, dashboards.

**Implementation:**

Add `--json` flag to `ralph status` (~40 LOC):
```json
{
  "feature": "user-auth",
  "branch": "ralph/user-auth",
  "status": "in-progress",
  "stories": {"passed": 3, "skipped": 1, "pending": 2},
  "retries": {"US-001": 0, "US-002": 1},
  "learnings": 5
}
```

**~40 LOC.** Test: `TestStatusJSON_Format`.

---

## Phase 4: Quality of Life

Smaller improvements that reduce friction. Each is independently implementable.

**Estimated total: ~300 LOC**

### 4.1 Dependency Enforcement at Runtime

`StoryDefinition` supports `depends_on` field but `GetNextStory()` ignores it. Stories blocked on unmet dependencies waste attempts.

Add dependency check in `GetNextStory()`: skip stories whose `depends_on` IDs are not all in `state.Passed`. Log when a story is blocked. (~30 LOC in story selection logic)

### 4.2 Enriched Refine Summaries

Replace `git diff --stat` with truncated full `git diff` (capped at ~8000 chars) in post-refine summarization input. Gives summarizer actual code changes, not just file names. (~10 LOC in refine.go)

### 4.3 Post-Session Notes Capture

After interactive refine session, prompt user for notes. If provided with zero commits, write notes directly to progress.md. If with commits, include in summarizer context. Captures decisions and rationale from interactive work. (~40 LOC in commands.go + refine.go)

### 4.4 Prompt Polish

- `prompts/run.md`: Add "Do NOT signal DONE if any verification command fails" (prevents false-positive DONE markers)
- `prompts/verify-analyze.md`: Add `{{changedFiles}}` variable (from `git diff --name-only`) so AI analyzes only relevant files
- `prompts/run.md`: Add "Capture the why" sentence in LEARNING instructions — emphasize reasoning over facts

(~40 LOC in prompts.go + template edits)

### 4.5 Strict Template Rendering

Ralph's `{{var}}` replacement silently produces empty strings for undefined variables. A silent consultation failure results in empty `{{resourceGuidance}}` with no error. Add post-render check for remaining `{{...}}` patterns — fail with clear error. (~30 LOC in prompts.go)

### 4.6 Workspace Path Validation

Validate feature directory paths are under `.scrip/`. Prevent path traversal via `../` in feature names. Check in `CreateFeatureDir()` and before provider launch. (~20 LOC in feature.go)

### 4.7 Stale Feature Directory Cleanup

On `scrip run` start, scan `.scrip/` for feature dirs with no progress.md, no active run-state.json, and older than 30 days. Log warnings (don't auto-delete). (~40 LOC in startup path)

### 4.8 Verification Report Artifacts

After `runVerifyChecks()`, write structured JSON to `featureDir/verify-report.json`:
```json
{
  "timestamp": "2026-03-05T14:30:00Z",
  "feature": "user-auth",
  "passed": true,
  "checks": [
    {"name": "typecheck", "passed": true, "duration_ms": 2300},
    {"name": "test", "passed": true, "duration_ms": 5100}
  ]
}
```

Enables CI/CD integration and audit trails. (~80 LOC — mostly serialization, `VerifyReport` type already exists)

---

## Deferred

Items evaluated and explicitly deferred — not forgotten, but lacking evidence of current need or requiring architectural changes beyond current scope.

| Item | Reason Deferred |
|------|----------------|
| **Dynamic config reload** | No pain point — `scrip run` exits between features. Reload only matters for daemon mode, which scrip doesn't have. |
| **Reconciliation (manual code detection)** | Verify-at-top already catches regressions. Detecting "someone else touched the code" adds complexity for a solo-developer tool. |
| **Non-code task model** | No concrete schema or workflow designed. "Deployment as a task" needs its own design session with real use cases. |
| **Token/cost tracking** | Requires provider cooperation (many don't emit usage stats). Implement when provider output parsing is standardized. |
| **Worktree isolation** | Architectural change enabling parallel execution. Ralph is sequential by design. Defer until parallelism is designed. |
| **ACP protocol** | Ralph's marker protocol (DONE/STUCK/LEARNING) works with any CLI agent. ACP adds JSON-RPC complexity for marginal gain in unattended context. |
| **Plan-then-build separation** | Vague — no clear output schema, trigger condition, or re-planning flow. Needs design session. |
| **AMEND markers** | Overlaps with structured retry context. Unclear how amendments persist or trigger re-planning. Needs design session. |
| **Session IDs** | JSONL events already include story ID and iteration count. UUID per invocation is nice-to-have for debugging but not blocking. |
| **Exponential backoff retries** | Immediate retry works for logic errors (most common failure). No evidence Ralph hits rate limits in practice. |
| **Provider-agnostic support** | Scrip v3 targets Claude Code only. Other providers (amp, aider, codex, opencode) have different agentic workflows and don't support skills/hooks. Re-evaluate when demand exists. |

---

## Rename: Ralph → Scrip

Part of Phase 0 — happens alongside the architectural redesign, not as a separate step:
- Binary: `ralph` → `scrip`
- Config: `ralph.config.json` → `.scrip/config.json`
- Feature dir: `.ralph/` → `.scrip/`
- Lock file: `.ralph/ralph.lock` → `.scrip/scrip.lock`
- Cache dir: `~/.ralph/` → `~/.scrip/`
- Update check: `~/.config/ralph/` → `~/.scrip/`
- All internal references, prompts, error messages
- Drop: provider selection, CLAUDE.md/AGENTS.md modifications, all `.claude/` references
- Hardcode Claude Code (`--print --dangerously-skip-permissions` for autonomous, interactive for planning)

**~250 LOC modified** (find/replace across codebase). Include migration logic to detect and rename `.ralph/` → `.scrip/` and `~/.ralph/` → `~/.scrip/` for existing projects.

---

## Total Effort Summary

| Phase | New LOC | Modified LOC | Items |
|-------|---------|-------------|-------|
| **1: Accuracy** | ~460 | ~140 | 7 items |
| **2: Security** | ~1,400 | ~200 | 5 items |
| **3: Extensibility** | ~650 | ~150 | 4 items |
| **4: Quality of Life** | ~250 | ~50 | 8 items |
| **Rename** | — | ~250 | 1 item |
| **Total** | **~2,760** | **~790** | **25 items** |

Phase 1 should be implemented first — it directly improves the core value proposition (accuracy and convergence speed) with the lowest risk and highest ROI per line of code.

---

## Phase 0: CLI Redesign — Scrip v3 Architecture

**Status:** Design synthesis, pending refinement. Supersedes Phases 1–4 and Rename section above — those items will be re-evaluated within this new architecture.

**Origin:** The current 4-command pipeline (`prd` → `run` → `verify` → `refine`) treats the PRD as immutable and the run as finite. In practice, the user's real iterative work happens in refine sessions (post-archive), which lack structure. The original Ralph technique ([ghuntley.com/ralph](https://ghuntley.com/ralph/), [how-to-ralph-wiggum playbook](https://github.com/ghuntley/how-to-ralph-wiggum)) used disposable plans, infinite loops, and a single repeating command — that's the model to return to.

**Key architectural decision:** Scrip is a **self-contained CLI that orchestrates Claude Code from the outside**. It does not install plugins, modify `.claude/`, touch CLAUDE.md, or create MCP servers. All state lives in `.scrip/` (project) and `~/.scrip/` (home). When you upgrade scrip, everything upgrades — no files to sync across codebases, no conflicts with existing Claude Code setups. The provider (Claude Code) is a raw instance — whatever the user has configured for themselves. Scrip controls what the provider sees via **prompt injection**, not by modifying the provider's environment.

### Design Principles

1. **Self-contained CLI.** Scrip touches exactly two directories: `.scrip/` in the project and `~/.scrip/` in the user's home. No `.claude/` modifications, no CLAUDE.md markers, no `.mcp.json`, no skills, no hooks. The user's Claude Code installation stays pristine.
2. **Claude Code-only.** No provider-agnostic abstractions — no promptMode, promptFlag, knowledgeFile juggling. Scrip hardcodes Claude Code. Other providers may be added later as separate integration work.
3. **Four commands, clear boundaries.** `scrip prep` (setup), `scrip plan` (think + plan), `scrip exec` (execute), `scrip land` (verify + finalize). Each command has one job. The human gate between planning and execution is the CLI boundary itself — user runs `scrip plan` until satisfied, then explicitly runs `scrip exec`.
4. **Consultation and verification are CLI infrastructure.** Not skills the agent invokes. Not hooks that enforce behavior. The CLI pre-computes consultation (via separate `claude --print` subagent calls) and injects results into every prompt. The CLI runs verification after every execution. The agent never needs to "remember" to consult or verify — the CLI makes it happen.
5. **Disposable plans, permanent progress.** Plans are regenerated frequently and purged after execution. Progress tracking and summaries are the durable state.
6. **CLI orchestrates, provider implements.** The CLI handles state, consultation, verification, retries, branch management. The provider signals DONE/STUCK/LEARNING. During execution, each item gets a fresh provider spawn — no accumulated context, no hallucination compounding.

### The Core Ralph Technique

Scrip's autonomous execution is a direct implementation of the Ralph technique ([ghuntley.com/ralph](https://ghuntley.com/ralph/), [how-to-ralph-wiggum playbook](https://github.com/ghuntley/how-to-ralph-wiggum)). **The technique is essential for successful execution** — every element below is load-bearing and must be preserved in scrip's architecture. Removing or weakening any one of them degrades the entire system.

**The loop:** `while :; do cat PROMPT.md | claude ; done` — the original invention. A bash loop that feeds a prompt to an AI agent, lets it complete one unit of work, exits, and restarts with fresh context. Scrip implements this as a Go loop spawning fresh `claude --print` instances per item. The loop IS the product.

**One item per loop.** Singular focus. Each spawn gets one item to implement, with the full context budget allocated to that item alone. This manages the ~176K usable token window and prevents hallucination compounding across items. "I need to repeat myself here — one item per loop."

**Fresh context every iteration.** No accumulated state in the AI's context. Each spawn reads the current plan, specs, and operational guide from disk — the only bridge between iterations is what's written to files. This is why scrip's markers (DONE/STUCK/LEARNING) and progress.jsonl exist: they persist what the AI discovered so the CLI can inject relevant context into the next spawn's prompt.

**IMPLEMENTATION_PLAN.md as shared state.** In original Ralph, the plan file persists on disk as the bridge between isolated loop executions. The agent reads it, picks the most important item, implements it, updates the plan, commits, and exits. The next iteration reads the updated plan. In scrip, `plan.md` serves this role, with the CLI managing item selection and progress tracking via `progress.jsonl` — the agent doesn't need to update the plan file itself.

**Backpressure.** Tests, typechecks, lints, and builds are downstream backpressure that forces quality. "The wheel has got to turn fast." Without backpressure, the agent produces plausible-looking but broken code. In scrip, verification commands from `.scrip/config.json` ARE the backpressure — they run after every DONE signal, and failure triggers retry with structured context. Backpressure is not optional verification — it is the mechanism that makes autonomous execution work at all.

**Subagent backpressure control.** Original Ralph uses up to 500 parallel subagents for search/read operations but restricts build/test to a single agent. This prevents failures where multiple agents step on each other's builds. Scrip inherits this naturally — each `claude --print` spawn is a single agent that can delegate reads internally but owns the build exclusively.

**Steering via patterns, not instructions.** The engineer moves outside the loop — observing failure patterns, tuning specs, adjusting prompts, adding utilities and code patterns that steer the agent toward correct implementations. The planning phase is where this happens in scrip: the human shapes the plan and acceptance criteria through CLI-mediated rounds, then steps away while the autonomous loop executes. "Tune it like a guitar."

**Plans are disposable.** "Regenerate when trajectory diverges." The plan is a cheap artifact — what matters is the code, tests, and learnings it produced. Scrip purges plan.md after execution and regenerates from progress.jsonl + progress.md context when new work begins. Any problem created by AI can be resolved through a different series of prompts and running more loops.

**"Don't assume not implemented."** The Achilles' heel of AI coding — agents re-implement existing functionality instead of finding and using it. The build prompt must include: "Before making changes, search the codebase first (don't assume not implemented)." This single instruction prevents the most common class of wasted iteration.

**"No placeholders or stubs."** Full implementations only. Placeholders and stubs waste an entire loop iteration because the next iteration must redo the work. The build prompt must enforce: "Implement completely. Stubs waste efforts and time redoing the same work."

**"Capture the why."** Tests and learnings must explain importance, not just state facts. This leaves "little notes for future iterations" that compound into institutional knowledge. Scrip's LEARNING markers carry this forward — but the instruction must be explicit in the build prompt.

**Self-updating operational guide.** AGENTS.md in original Ralph is the operational "how to build/test/run" guide, kept brief (~60 lines), updated by the agent when it discovers something new about the project. In scrip, this maps to `.scrip/config.json` (verify commands, services) — the CLI injects operational context into every prompt directly, no knowledge file needed.

**Prompt structure: Orient → Act → Guardrails.** Original Ralph prompts have three layers: Phase 0 (study specs, plan, source code — orientation), Phase 1+ (main task instructions), and escalating "9s" numbering for invariants/guardrails that must never be violated. Scrip's prompts follow this same layered structure — orient the agent with context, give the task, then enforce invariants.

**Planning is the same loop, different prompt.** In original Ralph, planning and building use the **identical loop mechanism** — they just flip one instruction: "Plan only. Do NOT implement anything." The planning prompt: study specs, study code, compare against specs, search for gaps (TODOs, placeholders, failing tests), create/update IMPLEMENTATION_PLAN.md. Planning is fully autonomous and typically completes in 1-2 iterations. In scrip, `scrip plan` applies this same principle — the Ralph loop with a planning prompt — but uses **CLI-mediated rounds** where the user provides direction between iterations and the CLI enriches every round with pre-computed consultation. No interactive Claude Code sessions — the user talks to scrip, scrip talks to Claude.

#### Mapping to Scrip Architecture

| Original Ralph | Scrip Equivalent | Notes |
|----------------|-----------------|-------|
| `while :; do cat PROMPT.md \| claude ; done` | Go loop in `cmd_exec.go` spawning `claude --print` per item | Same pattern, better error handling + crash recovery |
| `IMPLEMENTATION_PLAN.md` | `plan.md` (disposable) + `progress.jsonl` (permanent) | CLI manages state instead of agent self-updating |
| `AGENTS.md` | `.scrip/config.json` + CLI-injected context | Operational guide baked into every prompt |
| `specs/*` | Plan items with acceptance criteria | Acceptance criteria = spec per item |
| PLANNING mode (`PROMPT_plan.md`) | `scrip plan` (CLI-mediated rounds with consultation) | Ralph's planning loop with human direction between rounds |
| BUILDING mode (`PROMPT_build.md`) | `scrip exec` (autonomous, CLI-driven) | Same isolation model, same fresh-context-per-item |
| Backpressure (tests/typecheck/lint) | Verification commands from `.scrip/config.json` | Same concept, CLI-controlled timing |
| `git commit` after tests pass | DONE marker → CLI verifies → commit accepted | CLI gatekeeps instead of trusting agent |
| Agent updates `IMPLEMENTATION_PLAN.md` | CLI updates `progress.jsonl` + manages plan state | CLI controls state, not agent |
| Up to 500 subagents for reads, 1 for builds | Single `claude --print` spawn delegates internally | Natural backpressure — one build owner per item |

### Four Commands

```
scrip prep    # project setup + harness audit (safe to re-run anytime)
scrip plan    # think + plan with deep consultation (CLI-mediated rounds, loops until solid)
scrip exec    # execute plan items using Ralph loop (autonomous, fresh context per item)
scrip land    # final verification + security + summary + push artifacts
```

#### `scrip prep`

Project setup and harness audit. Creates `.scrip/` directory with all config:
- Detect project type, package manager, test framework, linter
- Generate `.scrip/config.json` with verify commands, services, project metadata
- Create `.scrip/.gitignore` (ignore lock, logs, state.json, plan.jsonl)
- Resolve dependencies → cache framework source to `~/.scrip/resources/`
- Audit downstream harness: types coverage, test patterns, linter rules, SAST tools
- Report harness gaps with actionable recommendations (not auto-fix)

**Designed to be run repeatedly.** Safe anytime: when the codebase changes, when scrip CLI updates, when dependencies upgrade. Regenerates from project detection, preserves user customizations. Does NOT touch `.claude/`, CLAUDE.md, or any file outside `.scrip/`.

#### `scrip plan`

CLI-mediated planning rounds with pre-computed consultation. No interactive Claude Code sessions — the user talks to scrip, scrip talks to Claude. Each round gets fresh context.

**Flow:**
1. User runs `scrip plan auth "add google oauth login"` (or `scrip plan auth` → CLI prompts for description)
2. CLI creates `.scrip/auth/` directory, creates branch `scrip/auth` (first time only)
3. CLI pre-computes consultation (parallel `claude --print` subagent calls, ~10-20s):
   - Read project structure (tech stack, frameworks, test patterns)
   - Read cached framework source from `~/.scrip/resources/`
   - Run parallel consultation subagents for framework-specific research
   - Run codebase analysis for the feature area
   - Read `progress.jsonl` + `progress.md` + `plan.jsonl` (if resuming or after land failure)
4. CLI builds planning prompt with: consultation results + codebase context + planning history + user input + planning instructions
5. CLI spawns `claude --print` — runs in background, shows progress
6. CLI displays AI response to user
7. User decides:
   a. Provide feedback → another round (back to step 3 with accumulated context)
   b. Approve → finalize round
8. Finalize round: CLI spawns `--print` with "write plan.md" instruction
9. CLI runs adversarial verification on plan.md (separate `--print` call):
   - Are acceptance criteria specific and testable?
   - Are there missing items, untestable claims, security gaps?
   - Does the plan contradict existing codebase patterns?
10. CLI shows plan + verification warnings
11. User approves → plan.md saved. Or provides feedback → another round.

**This is the Ralph planning loop made CLI-mediated.** In original Ralph, planning is autonomous: "Study specs, study code, compare, write IMPLEMENTATION_PLAN.md. Plan only. Do NOT implement anything." Scrip applies the same principle — gap analysis, research, plan creation — but with the user providing direction between rounds. Each round gets fresh context with accumulated planning history injected by the CLI.

**Why non-interactive instead of open Claude Code sessions:**
- **Zero config dependency** — factory-fresh Claude Code works. No skills, hooks, or CLAUDE.md needed.
- **Fresh context per round** — no degradation from accumulated conversation history. Each round gets the full ~176K token budget.
- **CLI controls consultation** — separate `--print` subagent calls, deterministic, cacheable, parallelizable. Not "hope the AI remembers to research."
- **CLI controls verification** — independent adversarial review after plan draft, not self-judging.
- **Reproducible** — same input → similar output. Debuggable, tunable.
- **Aligned with Ralph** — planning uses the identical loop mechanism as execution, just with a different prompt.

**Consultation and verification are separate `--print` calls, not prompt instructions.** The CLI orchestrates consultation as independent subagent spawns BEFORE each planning round. Results are injected as context. Verification is a separate adversarial `--print` call AFTER the finalize round. Neither is self-directed — the CLI makes them happen deterministically.

**Consultation scales with what exists:**
- First time, no cached resources → project structure + codebase analysis only.
- With cached resources → CLI runs parallel consultation subagents that read framework source, produce guidance about patterns/APIs/security. Injected into planning prompt.
- Resuming after previous work → progress.jsonl + progress.md + plan.jsonl provide full context.
- After land failure → `land_failed` event findings injected as "Previous land failed because: X. Address these issues."

**Planning state persists across rounds.** Each round's user input, consultation results, and AI response are appended to `plan.jsonl`. When starting a new round, the CLI reads this history and injects it as context — recent rounds verbatim, older rounds progressively summarized to manage the token budget. This means you can walk away mid-planning and resume tomorrow.

**State after `scrip plan`:** `plan.md` exists in `.scrip/<feature>/`. `plan.jsonl` contains the round history. The user can inspect the plan, edit it manually, or iterate with another `scrip plan` invocation.

#### `scrip exec`

Autonomous execution of the plan using the Ralph loop technique. No human in the loop.

**Flow:**
1. User runs `scrip exec <feature>`
2. CLI reads `plan.md`, validates items exist
3. CLI starts services (if configured)
4. For each item in the plan:
   a. Pre-compute item-level consultation (CLI runs `claude --print` subagents for framework research relevant to this specific item)
   b. Build execution prompt with: item description + acceptance criteria, consultation results, learnings from previous items, retry context (if retrying), codebase context
   c. Spawn fresh `claude --print --dangerously-skip-permissions` with enriched prompt
   d. Provider implements, commits, signals DONE/STUCK/LEARNING via markers
   e. CLI runs verification commands (typecheck, lint, test) — the backpressure
   f. If DONE + verification passes → advance to next item, log to progress.jsonl
   g. If DONE + verification fails → retry with failure classification + diff from failed attempt
   h. If STUCK → log reason, retry or skip at threshold
   i. LEARNING → persist to progress.jsonl, inject into next spawn's prompt
5. After all items pass or skip → "All items complete. Run `scrip land` to finalize."

**Resume:** If interrupted, `scrip exec` reads `progress.jsonl` to find last completed item and resumes from the next pending item. Crash recovery via `state.json` (current item, provider PID).

**Quick fix shortcut:** `scrip exec "fix the login button"` — single-line description, CLI auto-generates a 1-item plan and executes immediately. Same state tracking, same verification, same consultation.

This is the core Ralph technique: fresh `claude --print` per item, one item per loop, backpressure via verification, markers for communication, learnings persisted across iterations. The CLI owns the loop — the provider just implements.

#### `scrip land`

Final comprehensive verification — the deepest check in the system. Land does NOT merge or create PRs. It verifies, summarizes, and pushes artifacts.

1. Run all verification commands (typecheck, lint, test)
2. Run SAST tools + dependency audit (security layer)
3. AI deep analysis via `claude --print` — architecture review, cross-item consistency, security audit
4. If all pass:
   - Append final narrative to `progress.md` (what was built, decisions made, learnings)
   - Purge `plan.md` + `state.json` (runtime state)
   - Commit and push artifacts (`progress.md`, purged plan, `progress.jsonl`)
   - Feature is "landed" — ready for PR/merge via normal git workflow
5. If any check fails:
   - Write `land_failed` event to `progress.jsonl` with structured findings (which checks failed, AI analysis results, specific issues found)
   - Append failure narrative to `progress.md` ("Land failed: X, Y, Z")
   - Do NOT purge plan or state
   - Exit with clear message: "Land failed. Run `scrip plan` to rethink, or `scrip exec` to fix."

**Land failure → plan/exec loop:** When `scrip plan` detects a `land_failed` event in `progress.jsonl`, it injects the failure findings into the planning round as context. The user thinks about the issues, plans targeted fixes, runs `scrip exec`, and tries `scrip land` again. This closes the loop: plan → exec → land → (fail) → plan (with findings) → exec → land.

### State Model

**Plan: Markdown with YAML frontmatter.** Not JSON — the AI writes markdown naturally. JSON adds parsing friction for a disposable artifact.

**Planning history: JSONL for round-by-round context.** Each planning round (user input + consultation results + AI response) is appended as one entry to `plan.jsonl`. The CLI reads this to reconstruct context for the next round — recent rounds verbatim, older rounds progressively summarized. Survives interruption (resume planning tomorrow where you left off).

**Progress: JSONL for execution events.** Append-only, grep-queryable, line-safe on crash, git-friendly diffs. Plus a **progress.md** (narrative, appended after each session) for human context. The two complement each other:
- `progress.jsonl` answers "what happened?" (machine-queryable)
- `progress.md` answers "why did we do it that way?" (human-readable, carried forward as context for next planning round)

**progress.md is as important as progress.jsonl.** JSONL is machine-queryable but not great context for the next planning round. Progress.md (narrative, appended after each session) gives the AI the "why" alongside the "what." Both files are permanent; plan.md is the only disposable artifact.

Five files, clear lifecycle:

| File | Location | Lifecycle | Format | Purpose |
|------|----------|-----------|--------|---------|
| `plan.jsonl` | `.scrip/<feature>/` | Permanent — append-only | JSONL | Planning round history (user input, consultation, AI response per round) |
| `plan.md` | `.scrip/<feature>/` | Disposable — purged after execution or on rethink | Markdown + YAML frontmatter | Current work items with acceptance criteria |
| `progress.jsonl` | `.scrip/<feature>/` | Permanent — append-only | JSONL | Machine-queryable event log (attempts, passes, failures, learnings) |
| `progress.md` | `.scrip/<feature>/` | Permanent — appended after each exec session | Markdown | Human-readable narrative of all work done |
| `state.json` | `.scrip/<feature>/` | Temporary — runtime recovery only | JSON | Current item, provider PID, lock info. Deleted on clean exit. |

#### Plan lifecycle

Plans are ALWAYS purged after execution. The cycle:

```
scrip plan → rounds until satisfied → plan.md created → scrip exec → plan.md purged
                                                                           ↓
progress.jsonl appended ←─────────────────────────────────────────────────┘
progress.md appended ←─────────────────────────────────────────────────────┘

scrip plan (resume) → reads progress.jsonl + progress.md + plan.jsonl → rounds with context → NEW plan.md → scrip exec
```

This is exactly original Ralph's model: IMPLEMENTATION_PLAN.md is disposable, progress.txt is permanent.

#### plan.md format

Acceptance criteria live in the markdown body (simple, natural for AI to write). The YAML frontmatter is minimal metadata only:

```markdown
---
feature: auth-system
created: 2026-03-11T14:32:00Z
---

# Auth System

## Items

1. **Set up OAuth2 dependencies**
   - Acceptance: OAuth2 client instantiates, no hardcoded secrets

2. **Google login flow**
   - Acceptance: End-to-end Google auth works, session persists
```

**Acceptance criteria survive plan purges** because the CLI writes them into progress.jsonl when execution starts:

```jsonl
{"ts":"...","event":"item_start","item":"Set up OAuth2","criteria":["OAuth2 client instantiates","no hardcoded secrets"]}
{"ts":"...","event":"item_done","item":"Set up OAuth2","status":"passed","commit":"abc123","learnings":["callback URL must be exact match"]}
```

#### plan.jsonl events

```jsonl
{"round":1,"ts":"...","user_input":"add google oauth login","consultation":["Phoenix auth: ueberauth is standard...","Codebase: no auth infrastructure exists"],"ai_response":"Based on research, here are 3 approaches...","has_plan_draft":false}
{"round":2,"ts":"...","user_input":"go with ueberauth, also consider GitHub later","consultation":["ueberauth strategy pattern supports multi-provider..."],"ai_response":"Updated approach with multi-provider ueberauth...","has_plan_draft":true}
{"round":3,"ts":"...","user_input":"write the plan","ai_response":"[plan content written to plan.md]","finalized":true,"verification":{"items":5,"warnings":["missing CSRF on OAuth callback"]}}
```

Context injection for round N: CLI reads plan.jsonl, builds a summary of previous rounds (user decisions + key AI recommendations), injects into the fresh `--print` prompt alongside new consultation results. Progressive compression: recent rounds verbatim, older rounds summarized to manage the ~176K token budget.

#### progress.jsonl events

```jsonl
{"ts":"...","event":"item_start","item":"Set up OAuth2","criteria":["OAuth2 client instantiates","no hardcoded secrets"]}
{"ts":"...","event":"item_done","item":"Set up OAuth2","status":"passed","commit":"abc123","learnings":["callback URL must be exact match"]}
{"ts":"...","event":"item_start","item":"Google login flow","criteria":["End-to-end Google auth works","session persists"]}
{"ts":"...","event":"item_stuck","item":"Google login flow","attempt":1,"reason":"Guardian config unclear"}
{"ts":"...","event":"learning","text":"Guardian requires serializer module, not just config"}
{"ts":"...","event":"item_start","item":"Google login flow","attempt":2}
{"ts":"...","event":"item_done","item":"Google login flow","status":"passed","commit":"def456"}
{"ts":"...","event":"plan_purged"}
{"ts":"...","event":"plan_created","item_count":1,"context":"extending with password reset"}
{"ts":"...","event":"land_failed","findings":["test: 2 failures in auth_test.go","security: missing CSRF on /api/sessions"],"analysis":"OAuth flow passes but session management lacks CSRF protection"}
{"ts":"...","event":"land_passed","summary_appended":true,"plan_purged":true}
```

New plans are generated with full context from progress.jsonl + progress.md + plan.jsonl. Land failure findings in progress.jsonl flow directly into the next planning round as injected context.

> **Note (pending refinement):** The exact plan.md format needs finalization — whether items should also be structured in YAML frontmatter (with `id`, `depends_on` fields for machine parsing) or kept as pure markdown. The original synthesis used markdown body only. A hybrid approach (YAML for machine fields, markdown for context) may be needed for dependency enforcement and item tracking.

### Self-Contained Architecture

Scrip touches exactly **two directories**. Nothing else on the user's system is modified.

#### Project directory: `.scrip/`

```
.scrip/
├── config.json                         # Project config (verify commands, services, project metadata)
├── .gitignore                          # Ignore: scrip.lock, */logs/, state.json
├── scrip.lock                          # Lock file for concurrency control
└── <feature>/                          # One directory per feature
    ├── plan.jsonl                # Planning round history (permanent, append-only)
    ├── plan.md                         # Current plan (disposable)
    ├── progress.jsonl                  # Event log (permanent, append-only)
    ├── progress.md                      # Narrative history (permanent)
    ├── state.json                      # Runtime recovery (temporary, deleted on clean exit)
    └── logs/
        └── exec-NNN.jsonl             # JSONL logs per exec session
```

#### Home directory: `~/.scrip/`

```
~/.scrip/
├── resources/                          # Cached framework source code
│   ├── registry.json                   # Resolution cache (URLs, versions, TTL)
│   └── <name>@<version>/              # Cloned repo per dependency
└── update-check.json                   # CLI update check cache (24h TTL)
```

#### What scrip does NOT touch

- `.claude/` — no skills, no hooks, no settings.json
- `CLAUDE.md` — no markers, no managed sections
- `.mcp.json` — no MCP server config
- Project root — no config files outside `.scrip/`
- Global Claude Code config — nothing in `~/.claude/`

The provider (Claude Code) runs as a raw instance — factory settings work perfectly. The user's personal skills, hooks, CLAUDE.md, and MCP servers are untouched and irrelevant. Scrip controls what the provider sees entirely through **prompt injection** — the prompt piped to `claude --print` contains all context. No interactive sessions, no environment modification.

### Consultation Architecture

Consultation is **CLI infrastructure** — separate `claude --print` subagent calls orchestrated by the CLI, not instructions embedded in the main prompt. The agent never needs to "remember" to consult — it receives a prompt already enriched with research. Consultation is deterministic, cacheable, parallelizable, and independently verifiable.

#### How consultation works

Before every `scrip plan` round and before every `scrip exec` item, the CLI:

1. **Reads project context** — tech stack, frameworks, test patterns, directory structure (from `scrip prep` detection stored in `.scrip/config.json`)
2. **Reads cached framework source** — if `~/.scrip/resources/` has relevant frameworks, the CLI identifies which are relevant to the current feature/item
3. **Runs parallel consultation subagents** — spawns `claude --print` instances with consultation prompts that read the cached framework source and produce guidance about patterns, APIs, security. Each subagent is a separate process with its own context window: reads actual source code, produces guidance with `Source:` citations, validated by CLI (no citations = treated as hallucination, falls back to file paths)
4. **Reads progress history** — `progress.jsonl` + `progress.md` + `plan.jsonl` for context on what was done before, what failed, what was learned
5. **Packages everything into the prompt** — consultation results, progress context, planning history, codebase context, all injected as template variables

#### Why separate subagent calls, not prompt injection

Consultation and verification behaviors are NOT injected as self-directed instructions ("before answering, research the options..."). They are separate `claude --print` processes orchestrated by the CLI.

| Property | Separate --print calls (scrip's approach) | Self-directed prompt injection |
|----------|------------------------------------------|-------------------------------|
| Determinism | High — CLI controls when/what | Low — AI might skip steps |
| Cacheability | Yes — results saved, reused across rounds | No — embedded in output stream |
| Parallelization | Yes — multiple subagents concurrent | No — serialized with main task |
| Independent verification | Yes — separate adversarial call | No — self-judging |
| Token efficiency | Each subagent gets its own context window | Research competes with main task for tokens |
| Debuggability | Clear per-subagent failures | Silent degradation |

#### Consultation at each command

| Command | What CLI pre-computes | Injected as |
|---------|----------------------|-------------|
| `scrip plan` (each round) | Project structure + cached framework guidance + codebase analysis + planning history | Planning prompt context — agent starts informed |
| `scrip plan` (after land failure) | Same + `land_failed` findings from progress.jsonl | "Land failed because: X. Address these issues." |
| `scrip exec` (per item) | Item-specific framework guidance + learnings from previous items + retry context | Build prompt `{{resourceGuidance}}` + `{{learnings}}` + `{{retryContext}}` |
| `scrip land` | Comprehensive framework guidance for all touched areas | Deep analysis prompt context |

**When nothing is cached:** First-time consultation is limited to project structure analysis and codebase scanning. As the user works and resources get cached via `scrip prep`, subsequent rounds get richer pre-computed guidance.

**This is exactly what Ralph v2's `ConsultResources()` does** — run parallel subagents that read cached framework source, validate citations, cache results, inject into prompts. Scrip keeps this pattern and applies it to planning rounds too, not just execution.

### Verification Architecture

Verification is **CLI-driven at every stage**. The agent never runs its own verification — the CLI controls all checks. Verification is always a separate `--print` call or mechanical command, never self-directed.

| Stage | When | What | How |
|-------|------|------|-----|
| **Plan verification** | After `scrip plan` finalize round | Adversarial review — testability, completeness, gaps, security | CLI spawns `claude --print` with adversarial prompt + plan.md content |
| **Pre-item verification** | Before each `scrip exec` item attempt | Re-verify previously attempted items (regression detection) | CLI runs verify commands from config |
| **Post-item verification** | After build agent signals DONE | Mechanical checks (typecheck, lint, test) — the backpressure | CLI runs verify commands, captures output, retries on failure |
| **Landing verification** | `scrip land` | Comprehensive: mechanical + security + AI deep analysis | CLI runs all layers in sequence |

**5 irreducible pillars preserved from the Ralph technique:**

1. **Process spawning** — fresh `claude --print` per item (no accumulated context/hallucinations)
2. **Marker communication** — DONE/STUCK/LEARNING (whole-line matching, no interpretation)
3. **State persistence** — atomic JSON writes, crash recovery, resume from last checkpoint
4. **Verification gatekeeping** — CLI controls all verification (agent can't bypass or forget)
5. **Service management** — long-lived dev servers reused across items

### Terminology

- **Item** — a single unit of work within a plan (replaces "story" / "user story")
- **Plan** — the container of items for current work (replaces "PRD")
- **Feature** — the overall thing being built, identified by branch/directory name
- **Round** — one iteration of `scrip plan` (user input → consultation → AI response)
- **Session** — one invocation of `scrip plan` (may contain multiple rounds) or `scrip exec`

### What This Replaces

| Ralph v2 | Scrip v3 |
|----------|----------|
| `ralph prd` (brainstorm + finalize) | `scrip plan` (CLI-mediated planning rounds with consultation) |
| `ralph run` (story loop) | `scrip exec` (autonomous execution loop) |
| `ralph verify` + archive | `scrip land` (comprehensive verification + finalize) |
| `ralph refine` (post-archive interactive) | `scrip plan` on existing feature (iterate with progress context) |
| `ralph status`, `ralph logs` | Folded into `scrip exec` status display |
| `ralph doctor` | `scrip prep` harness audit |
| `ralph.config.json` in project root | `.scrip/config.json` (inside .scrip/) |
| `.ralph/` feature directories | `.scrip/<feature>/` |
| `~/.ralph/resources/` | `~/.scrip/resources/` |
| CLAUDE.md / AGENTS.md modifications | None — prompt injection only |
| Skills, hooks, MCP server, `.claude/`, `.mcp.json` | None — self-contained CLI |
| Interactive Claude Code sessions | None — all communication via `claude --print` |

### Code Architecture

Clean, non-bloated module structure:

```
cmd_prep.go     — project setup + harness audit + dependency resolution
cmd_plan.go     — CLI-mediated planning rounds (consult → plan → verify cycles)
cmd_exec.go     — autonomous item loop (spawn, verify, retry, advance)
cmd_land.go     — final verification + security + summary + push artifacts
consultation.go — parallel subagent consultation (framework source → guidance)
verification.go — mechanical checks + adversarial AI review
plan.go         — plan.md read/write + plan.jsonl round history + context reconstruction
progress.go     — progress.jsonl append/query + progress.md narrative append
state.go        — state.json runtime recovery
prompts.go      — prompt template rendering with {{variable}} injection
```

Each file handles one concern. No plugin.go, no mcp.go, no skill.go — scrip is a self-contained CLI that controls Claude Code via prompt injection and marker detection. All communication with Claude Code uses `claude --print` — no interactive sessions.

**Dropped from Ralph v2:** Provider abstraction layer (`knownProviders`, `promptMode`/`promptFlag`/`knowledgeFile`, `providerChoices`, `stripNonInteractiveArgs()`, multi-mode `buildProviderArgs()`). Claude Code is hardcoded — `claude --print --dangerously-skip-permissions` for autonomous execution, `claude --print` for consultation and verification. No interactive mode.

### Embedded Prompt Templates

Scrip embeds all prompts at compile time via `//go:embed prompts/*`. These prompts capture the behaviors that would otherwise require installed Claude Code skills — consultation, verification, planning, and execution are all controlled by the CLI through prompt injection. The user's Claude Code needs zero custom configuration.

#### Consultation prompts (replaces /consult skill)

The /consult skill's core behaviors — expert-first research with parallel subagent dispatch — are embedded in scrip's consultation subagent prompts.

**`consult-item.md`** — Per-item framework consultation (spawned before each `scrip exec` item):
- Expert-first approach: learn the domain by reading cached framework source, understand the architecture, think in systems (trace dependencies and ripple effects) — then produce guidance
- Must cite actual source code (`Source: file:line`) — citations validated by CLI, uncited guidance treated as hallucination
- Structured output: patterns, API signatures, configuration, version-specific gotchas

**`consult-feature.md`** — Feature-level consultation (spawned before `scrip plan` rounds and `scrip land`):
- Same expert-first approach, broader scope
- Research along 4 angles: how it works now, what are the options, what are the risks, what does the ecosystem do
- Synthesis format: Understanding → Options → Recommendation with concrete evidence
- Scale subagent dispatch to scope — explore broadly, synthesize concisely

**Key /consult behaviors preserved:**
- Agents are domain experts first, searchers second — understand architecture and history before exploring
- Concrete evidence with code paths and file references, not opinions
- Surface disagreements between subagents — don't hide conflicts, let the user decide
- Break complex questions into independent sub-questions for parallel exploration

#### Planning prompts (replaces interactive brainstorming)

**`plan-round.md`** — Planning round prompt for `scrip plan`:
- Orient: project context + consultation results + planning history + codebase analysis
- Research-first: "Before recommending an approach, explore at least 2 alternatives with evidence from the codebase and framework source"
- Structured analysis: present trade-offs as concrete comparisons (not "A is simpler" but "A changes 2 files, B changes 5 + needs migration")
- When user says "write the plan": produce plan.md with items + acceptance criteria

**`plan-verify.md`** — Adversarial plan verification (spawned after finalize round):
- Extract every claim from plan.md (each item, each acceptance criterion, each architectural decision)
- For each claim: verify against codebase reality (files exist? APIs are real? patterns match?)
- Structured report: Claim | Verdict | Evidence
- Flag: untestable criteria, missing items, security gaps, contradictions with existing code
- "No concrete failure scenario = imaginary" — reject vague concerns without proof

#### Execution prompts (replaces ralph run.md)

**`exec-build.md`** — Build prompt for `scrip exec` items:
- Orient → Act → Guardrails structure (from original Ralph PROMPT_build.md)
- Injected: item description + acceptance criteria + consultation results + learnings + retry context
- "Before making changes, search the codebase first (don't assume not implemented)"
- "Implement completely. Stubs waste efforts and time redoing the same work."
- "Capture the why — tests and learnings must explain importance"
- Markers: `<ralph>DONE</ralph>`, `<ralph>STUCK:reason</ralph>`, `<ralph>LEARNING:text</ralph>`

**`exec-verify.md`** — AI deep analysis after item DONE (replaces verify-analyze.md + /verify implementation mode):
- Acceptance criteria checklist with checkboxes (from Ralph's `buildCriteriaChecklist`)
- Extract claims from BOTH plan AND code (implicit correctness claims)
- Expert specialist approach: understand the architecture and intent before judging. Think like the author — check why it was written that way before calling it wrong
- Trace full execution paths, construct concrete test inputs, check edge cases (nil, empty, error, timeout, boundary)
- Distinguish "could be improved" (not a failure) from actual bugs (concrete failure scenario required)
- Test file heuristic: warn if no test file changes detected
- Markers: `<ralph>VERIFY_PASS</ralph>`, `<ralph>VERIFY_FAIL:reason</ralph>` (multiple VERIFY_FAIL supported)

#### Landing prompts (replaces ralph verify deep analysis)

**`land-analyze.md`** — Comprehensive analysis for `scrip land` (combines verify-analyze.md + /verify audit mode):
- Full diff across all items, all acceptance criteria
- Architecture review: cross-item consistency, dependency correctness
- Security audit: auth flows, input validation, data handling, CSRF, injection
- Verify test coverage: new functions must have tests
- Structured report: Mechanical Results + Expert Analysis (Claim | Verdict | Evidence) + Summary + Recommendation
- "Never trust claims — verify against source code"

**`land-fix.md`** — Fix prompt if `scrip land` fails (replaces verify-fix.md):
- Verification results injected with failure details
- Per-item execution history (passed/skipped/retries)
- Diff summary + service URLs + verify commands
- Instructions: investigate root cause, fix, confirm locally, commit

#### What's captured from each source

| Source | Key behavior | Captured in |
|--------|-------------|-------------|
| /consult | Expert-first (learn domain → history → systems → explore) | consult-item.md, consult-feature.md |
| /consult | 4 angles (how now, options, risks, ecosystem) | consult-feature.md |
| /consult | Synthesis (Understanding → Options → Recommendation) | plan-round.md |
| /consult | Concrete evidence, surface disagreements | All consultation prompts |
| /verify | Context detection (plan vs code vs audit) | Different prompts per stage |
| /verify | Mechanical checks FIRST | CLI runs mechanical, then spawns AI analysis |
| /verify | Extract claims from plan AND code | exec-verify.md, land-analyze.md |
| /verify | Specialist approach (architecture → intent → author → verify) | exec-verify.md, land-analyze.md |
| /verify | Structured report (Claims table + verdicts) | plan-verify.md, land-analyze.md |
| /verify | "No concrete failure scenario = imaginary" | plan-verify.md, exec-verify.md |
| ralph verify | VerifyReport (Pass/Fail/Warn items) | verification.go VerifyReport type |
| ralph verify | All checks collected before verdict | CLI collects all, multiple VERIFY_FAIL markers |
| ralph verify | Criteria checklist with checkboxes | exec-verify.md, land-analyze.md |
| ralph verify | Verify-at-top regression detection | Pre-item verification in `scrip exec` |
| ralph verify | Service health as hard blocker | Post-item verification in `scrip exec` |
| ralph verify | DONE requires commit | exec-build.md + CLI hash check |
| ralph verify | Test file heuristic (WARN) | exec-verify.md |

### Testing Architecture

Scrip v3 is organized by modules, each independently testable. Every exported function has at least one test. Changed behavior has tests covering the new behavior.

#### Module test expectations

| Module | Key tests | What they cover |
|--------|----------|----------------|
| `cmd_prep.go` | `TestDetectProject`, `TestGenerateConfig`, `TestResolveDeps` | Project detection, config generation, dependency caching |
| `cmd_plan.go` | `TestPlanRound`, `TestPlanResume`, `TestPlanFinalize` | Round orchestration, resume from plan.jsonl, plan.md generation |
| `cmd_exec.go` | `TestExecLoop`, `TestExecResume`, `TestExecRetry`, `TestExecQuickFix` | Item loop, crash recovery, retry with failure classification, quick fix shortcut |
| `cmd_land.go` | `TestLandPass`, `TestLandFail`, `TestLandFailFindings` | Pass flow (purge + push), fail flow (findings → progress.jsonl) |
| `consultation.go` | `TestConsultParallel`, `TestConsultCitationValidation`, `TestConsultCaching` | Parallel subagent dispatch, citation validation, result caching |
| `verification.go` | `TestVerifyReport`, `TestVerifyCollectsAll`, `TestVerifyMarkers` | Report structure (Pass/Fail/Warn), all checks collected, marker detection |
| `plan.go` | `TestPlanParse`, `TestPlanWrite`, `TestPlanJsonlAppend`, `TestContextReconstruction` | YAML frontmatter, plan.md write, plan.jsonl append/query, progressive context compression |
| `progress.go` | `TestProgressAppend`, `TestProgressQuery`, `TestProgressMdAppend` | Event append, event query by type, progress.md narrative append |
| `state.go` | `TestAtomicWrite`, `TestCrashRecovery`, `TestLockAcquire` | Atomic JSON writes, resume from state.json, lock file management |
| `prompts.go` | `TestTemplateRender`, `TestStrictRendering`, `TestAllVariables` | Variable injection, no leftover `{{...}}` patterns, all templates render cleanly |

#### Integration tests

Test command-level flows with a mock `claude --print` (fake provider binary that reads prompt from stdin, returns canned responses with markers):

- `TestPrepIntegration` — `scrip prep` on a real project directory, verify config generated correctly
- `TestPlanIntegration` — full plan round: consultation subagents → planning → verification
- `TestExecIntegration` — full item loop: consult → spawn → DONE marker → verify → advance
- `TestLandIntegration` — pass and fail paths with mock deep analysis
- `TestResumeFromCrash` — kill mid-exec, restart, verify resume from progress.jsonl
- `TestLandFailPlanLoop` — land fails → plan with injected findings → exec → land passes

Mock provider: a test binary (or `testProvider(responses map[string]string)` helper) that maps prompt substrings to canned responses with markers. Same `--print` interface as real Claude Code.

#### E2e test

Full end-to-end test with real `claude --print` (requires API key, tagged `e2e`, 60min timeout):

```bash
make test-e2e  # go test -tags e2e -timeout 60m -v -run TestE2E ./...
```

```go
func TestE2E(t *testing.T) {
    // 1. scrip prep on testdata/e2e-project/ (small Go HTTP server with intentional gaps)
    // 2. scrip plan with canned user input → verify plan.md + plan.jsonl produced
    // 3. scrip exec → verify items pass, progress.jsonl populated
    // 4. scrip land → verify mechanical + AI checks pass, progress.md generated, plan.md purged
    // 5. scrip plan on same feature → verify plan.jsonl + progress context injected
}
```

**E2e test project:** A minimal but real project in `testdata/e2e-project/` (e.g., Go HTTP server with 2-3 endpoints). Has intentional gaps that scrip should detect and fix: missing tests for one endpoint, a TODO placeholder in one handler, linter warnings. Validates the full prep → plan → exec → land cycle.

#### Test patterns

- **Temp directories** for tests touching `.scrip/` or `.git/` — `t.TempDir()` with setup helpers
- **Table-driven tests** for classification logic (failure classification, project detection, marker parsing)
- **`go test ./...` and `go vet ./...`** must pass — enforced in CI
- **Build tags**: `e2e` for real-provider tests, no tag for unit/integration (fast, no API key needed)
- **Test helpers**: `setupTestProject(t)` creates temp dir with `.git/` + `.scrip/`, `setupTestConfig(t)` writes valid config
