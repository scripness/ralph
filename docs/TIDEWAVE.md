# Tidewave vs Ralph CLI — Comparative Analysis

> Sources: [tidewave.ai](https://tidewave.ai/), [hexdocs installation](https://hexdocs.pm/tidewave/installation.html), [tidewave_phoenix](https://github.com/tidewave-ai/tidewave_phoenix) (README + all 32 docs pages + 6 MCP tool source files), [tidewave_js](https://github.com/tidewave-ai/tidewave_js), [tidewave_python](https://github.com/tidewave-ai/tidewave_python), [tidewave_rails](https://github.com/tidewave-ai/tidewave_rails), [tidewave_app](https://github.com/tidewave-ai/tidewave_app), [agent-client-protocol](https://github.com/tidewave-ai/agent-client-protocol), [ACP spec](https://agentclientprotocol.com).

## Overview

Tidewave is a browser-based coding agent platform by Dashbit (makers of Elixir and Livebook). It provides "runtime intelligence" — connecting AI coding agents to your RUNNING web application so they can see the UI, query the database, read logs, execute code, and access version-specific documentation. It supports Phoenix, Django, Flask, FastAPI, Next.js, Vite, TanStack Start, and Rails.

Ralph is a Go CLI that orchestrates AI coding agents per user story from a PRD. It spawns fresh AI instances per story, verifies with automated tests, persists learnings, and repeats until all stories pass.

**These are fundamentally different tools occupying different layers of the AI coding stack:**
- **Tidewave** = **Tool layer** — gives agents runtime eyes via MCP
- **Ralph** = **Orchestration layer** — manages agent lifecycle (stories, retries, verification, learnings)

They are **complementary, not competing**. The interesting question isn't "should Ralph become Tidewave" — it's "how can Ralph help providers access Tidewave-like tools?"

---

## Architecture Comparison

| Dimension | Tidewave | Ralph CLI |
|-----------|----------|-----------|
| **Layer** | Tool layer (runtime context for agents) | Orchestration layer (agent lifecycle management) |
| **Runtime** | Desktop app (Rust/Tauri) + framework plugin + MCP server | CLI tool, runs to completion |
| **Role** | Gives agents eyes into running app | Manages what agents work on and verifies results |
| **Agent relationship** | Tool provider (agents call Tidewave tools) | Agent spawner (Ralph creates and manages agent sessions) |
| **Agent support** | Claude Code, Codex, Copilot, OpenCode (via MCP) | Claude, Amp, Aider, Codex, OpenCode (as subprocesses) |
| **Protocol** | MCP (JSON-RPC 2.0 over HTTP/SSE) + ACP (editor↔agent) | Unidirectional: stdin prompt → stdout markers |
| **App connection** | Embedded in running dev server (framework plugin) | Manages services externally (start/stop/restart) |
| **State** | Stateless MCP queries against running app | File-based (run-state.json, atomic writes) |
| **Verification** | Not a verifier — provides runtime data to agents | Deterministic verify commands + AI deep analysis |
| **Context delivery** | MCP tools (agent queries on demand) | Prompt injection (all context upfront) |
| **Documentation** | Runtime introspection (exact loaded versions) | Cached repo clones + AI summarization |
| **Security model** | Localhost-only by default, optional container isolation | Process groups, timeouts, CLI/provider separation |
| **Learning** | None | Deduped, persisted, cap 50 in prompts |
| **Pricing** | Free OSS MCP + $10/mo Pro (browser agents) | Fully open source |

---

## Tidewave's Key Capabilities

### 1. MCP Tools (Runtime Intelligence)

Tidewave exposes 6-9 MCP tools per framework, all operating against the **running application**:

| Tool | What It Does | Frameworks |
|------|-------------|------------|
| `project_eval` | Execute code in running app context (30s timeout, process isolation, IO capture) | All |
| `get_docs` | Retrieve documentation for exact loaded library versions | All |
| `get_source_location` | Find source file + line number for any module/function/class | All |
| `get_logs` | Read server logs with tail/grep/level filtering | All |
| `get_models` | List all application modules with file locations | All |
| `search_package_docs` | Search package docs filtered to project dependencies | Phoenix, JS |
| `execute_sql_query` | Run SQL queries against app database (50-row limit, parameterized) | Phoenix, Rails, Django, Flask, FastAPI |
| `get_ecto_schemas` | List all Ecto schema modules | Phoenix |
| `get_ash_resources` | List Ash framework resources | Phoenix (with Ash) |

**Key technical details:**
- MCP protocol version 2025-03-26, JSON-RPC 2.0 over HTTP streamable at `/tidewave/mcp`
- Tools cached in ETS table for concurrent read access (Phoenix implementation)
- Callback arity detection (1-arg vs 2-arg) for stateful tools
- Errors return `isError: true` within HTTP 200 (MCP spec convention)
- `project_eval` uses process isolation with monitored spawning, timeout enforcement, captured IO
- `execute_sql_query` limits to 50 rows with LIMIT/OFFSET pagination
- `get_docs` uses `Code.fetch_docs/1` (Elixir), `inspect` module (Python), or equivalent runtime introspection

**Ralph's equivalent:** Resource consultation (`consultation.go`) auto-resolves deps from lock files, clones repos, spawns AI subagents to summarize docs, injects guidance into prompt. Works offline, AI-summarized, but no runtime access.

### 2. MCP Proxy (Transport Bridge)

Tidewave ships `mcp-proxy` — a binary that bridges stdio MCP clients to HTTP MCP servers:

- Converts stdio transport ↔ HTTP streamable transport
- Handles automatic reconnection on dev server restart
- Useful for editors/providers that only support stdio (not HTTP)

**Ralph's gap:** Ralph spawns providers as subprocesses. Some providers (Claude Code, Codex) support MCP natively, but the transport may not match. The proxy solves this without Ralph needing to implement MCP transport conversion.

### 3. Inspector (Point-and-Click Element Selection)

- Hover over UI elements → add them as prompt context
- Traces DOM elements back to source code (server-side templates AND client-side components)
- Right-click for depth-based selection (overlapping elements)
- Ctrl/Cmd+Click opens element in configured editor
- Works across React, Vue, Django templates, Phoenix LiveView

**Not applicable to Ralph.** Ralph is CLI-based, headless, unattended. No browser, no UI interaction.

### 4. Agent Client Protocol (ACP)

Standardizes editor↔agent communication (like LSP but for coding agents):
- JSON-RPC over stdio (local agents) or HTTP/WebSocket (remote agents)
- Reuses MCP JSON types but adds coding-specific UX (diffs, file operations)
- Still evolving — not all agents supported
- SDKs: Rust, Python, TypeScript, Kotlin

**Not applicable to Ralph.** Ralph's marker system (DONE/STUCK/LEARNING) is simpler and sufficient for its unidirectional subprocess model.

### 5. Task Board (Claude Code Only)

- Break larger efforts into smaller pieces, track progress across sessions
- Tasks stored on disk at `~/.claude/tasks`
- Must be assigned before first message in chat
- Agent retrieves task info upon first mention

**Weak mapping to Ralph.** Ralph's PRD story system is persistent, CLI-managed, and source-of-truth. Claude Code's Task Board is session-scoped and IDE-bound. Different lifecycles. Low value to integrate.

### 6. Container/Docker Isolation

- Containerize your app → Tidewave is automatically sandboxed
- `TIDEWAVE_HOST_PATH` env var maps container↔host file paths
- `socat` for port forwarding when dev servers listen on localhost only
- VS Code dev containers supported via `.devcontainer/devcontainer.json`
- CLI mode required for containers (not desktop app)

**Changes the security story.** `project_eval` in a container is significantly safer than on the host. This makes runtime code execution viable for unattended agent sessions if the project runs in containers.

### 7. Additional Features

| Feature | Description | Relevant to Ralph? |
|---------|------------|-------------------|
| **Accessibility diagnostics** | Checks rendered pages (not source), forwards reports to agents | No — Ralph's `verify.ui` already supports a11y test suites |
| **Viewport testing** | Agents test responsive breakpoints against real rendering | No — covered by e2e tests in `verify.ui` |
| **Mermaid diagrams** | Agents generate architecture/ER/sequence diagrams inline | Marginal — visual debugging niche |
| **Figma integration** | Attach Figma selections to prompts | No — code-first, not design-first |
| **Supabase integration** | Local Supabase as MCP server + performance/security advisors | No change needed — works as a Service config entry |
| **Notifications** | Browser notifications when agent finishes or needs input | No — Ralph is unattended |
| **Teams** | Org-wide settings, centralized billing, SSO | No — Ralph is OSS CLI |

### 8. Performance Claims

- 2x accuracy improvement for Claude Code on full-stack tasks
- 45% task duration reduction (158 seconds faster)
- 25,100 fewer tokens per task

These claims are for **interactive Tidewave Web usage**, not MCP-only. Relevance to Ralph is indirect — providers with MCP access to runtime tools would likely be more accurate, but the exact improvement in unattended orchestration is unknown.

---

## Documentation Access: Ralph vs Tidewave

A detailed comparison since both systems solve the "give agents framework knowledge" problem differently:

| Dimension | Ralph (Resource Consultation) | Tidewave (MCP Tools) |
|-----------|------------------------------|---------------------|
| **Version precision** | Lock file exact version → git tag lookup | Runtime introspection (whatever is loaded) |
| **Data source** | Full cloned repo source trees | Live loaded modules + registry APIs |
| **Availability** | Offline (cached locally at `~/.ralph/resources/`) | Online (requires running app or registry) |
| **Selection** | Tag/keyword matching → pick up to 3 frameworks | Agent queries directly by name |
| **Summarization** | AI subagent reads source, distills into 200-800 token guidance | Raw docs returned directly to agent |
| **Validation** | Mandatory source citations (anti-hallucination check) | Introspection-based (always accurate) |
| **Latency** | Zero on cache hit (disk reads) | Per-query roundtrip (network/subprocess) |
| **Accuracy risk** | AI may misread cached source | Minimal (runtime ground truth) |
| **Update lag** | Until re-sync (versioned = never) | None (always current) |
| **Setup required** | Initial clone/sync (~30s per framework) | Running application instance |
| **Storage** | Can be large (280MB for React repo) | Minimal (stateless queries) |
| **Cross-library** | Single consultation for integration patterns | Separate queries per package |

**Where Ralph excels:** Offline-first, AI-summarized (reduces noise), source-cited (anti-hallucination), cross-library patterns, reproducible across runs.

**Where Tidewave excels:** Exact version matching (runtime truth), zero caching lag, minimal storage, function-level precision (file + line), no stale cache risk.

**Key improvement from Tidewave:** Ralph could add **dependency-filtered framework selection** — only consult frameworks actually in the project's lock file dependencies, not just keyword-matched. Currently `relevantFrameworks()` uses tag + keyword matching; adding a dependency-presence boost would reduce noise.

---

## Security Analysis

### Tidewave's Model
- Development-only (explicit: "must not be enabled in production")
- Localhost-only by default (`--allow-remote-access` for override)
- Remote IP verification on MCP requests
- Origin header checks for browser requests
- Documentation scoped to project dependencies (prompt injection prevention)
- Container isolation recommended for sandboxing
- `project_eval` runs in isolated monitored process with timeout

### Ralph's Model
- CLI is orchestrator; provider is pure code implementer
- Provider cannot modify prd.json or run-state.json (CLI-only state)
- Process groups (`Setpgid: true`) for clean subprocess killing
- Timeout enforcement (30-min default)
- Provider output suppressed from console (JSONL logs only)
- Lock file prevents concurrent runs
- No runtime code execution — provider works with files only

### Security Implications for Integration

**Risk:** If Ralph enables MCP tools like `project_eval`, providers gain runtime code execution during unattended runs. An agent could:
- Read `.env` files and exfiltrate credentials
- Disable test assertions or fake passing tests
- Corrupt the development database
- Execute arbitrary shell commands via `project_eval`

**Mitigation (from Tidewave):** Container isolation. If the project runs in Docker, `project_eval` is sandboxed to the container. This makes runtime tools viable for unattended runs.

**Ralph's correct approach:** Make MCP server management available but **never require runtime tools**. Users opt in by configuring MCP servers. Ralph doesn't provide `project_eval` — Tidewave (or similar) does, at the user's risk.

---

## Concrete Improvements for Ralph

### Priority Ranking

| Change | Source | Impact | Effort |
|--------|--------|--------|--------|
| Service log grep in verification | Tidewave `get_logs` | High | 1 hour |
| MCP server management in config | Tidewave MCP architecture | High | 2 days |
| MCP proxy reference/bundling | Tidewave `mcp-proxy` | Medium | 0.5 day |
| Prompt size reduction via helper refs | Tidewave tips | Medium | 3 hours |
| Document runtime verify commands | Tidewave `project_eval` (safe version) | Medium | 0.5 hour |
| Service logs to AI verify subagent | Tidewave `get_logs` | Medium | 2 hours |
| Dependency-filtered consultation | Tidewave `search_package_docs` | Medium | 3 hours |
| Optional container isolation | Tidewave Docker model | Medium | 1 week |
| `verify.runtime` config section | Tidewave runtime tools | Low | 2 hours |

### Tier 1: Immediate (High ROI, Low Effort)

#### 1.1 Service Log Analysis in Verification

Ralph already captures service logs (256KB buffer per service via `ServiceManager`) but only exposes them on failure. Add automatic ERROR/PANIC/WARN grep to `runVerifyChecks()`.

**Current** (loop.go, service health check):
```go
// Only checks HTTP response code
if err := svcMgr.CheckServiceHealth(); err != nil {
    report.AddFail("service health", err.Error())
}
```

**Proposed** (~15 lines):
```go
// After health checks, grep service logs for errors
for _, svc := range cfg.Config.Services {
    output := svcMgr.GetRecentOutput(svc.Name, 100)
    if strings.Contains(output, "ERROR") || strings.Contains(output, "PANIC") {
        report.AddFail(svc.Name+" logs", "ERROR/PANIC found in recent output")
    }
}
```

**Also:** Pass recent service logs to the AI verification subagent via `{{serviceLogs}}` template variable. The subagent currently sees only code diffs and test results — service logs would reveal startup failures, migration errors, initialization issues.

**Files:** `loop.go` (add grep in `runVerifyChecks()`), `prompts/verify-analyze.md` (add `{{serviceLogs}}` variable), `prompts.go` (pass logs to template).

#### 1.2 Document Runtime Verify Commands

Users can already put runtime checks in `verify.ui`:
```json
{
  "verify": {
    "default": ["go test ./...", "go vet ./..."],
    "ui": [
      "curl -sf http://localhost:3000/health",
      "curl -s http://localhost:3000/api/status | jq -e '.initialized'"
    ]
  }
}
```

Ralph just doesn't document or encourage this pattern. Add examples to README showing runtime verification alongside static checks.

**Files:** `README.md` only. Zero code changes.

### Tier 2: Short-Term (This Month)

#### 2.1 MCP Server Management

Ralph already manages services via `ServiceManager`. MCP servers are just another service type. Ralph should:

1. Add MCP config section to `ralph.config.json`
2. Start MCP servers alongside services (reuse `ServiceManager`)
3. Generate `.mcp.json` for Claude Code (auto-discovered)
4. Clean up on exit (already handled by `CleanupCoordinator`)

**Config:**
```json
{
  "mcp": {
    "servers": [
      {
        "name": "tidewave",
        "command": "npx @tidewave/cli mcp",
        "ready": "http://localhost:9832/health",
        "port": 9832
      }
    ]
  }
}
```

**Generated `.mcp.json`** (for Claude Code):
```json
{
  "mcpServers": {
    "tidewave": {
      "command": "npx",
      "args": ["@tidewave/cli", "mcp", "--port", "9832"]
    }
  }
}
```

**Provider-specific MCP delivery:**

| Provider | MCP Delivery | Method |
|----------|-------------|--------|
| Claude Code | `.mcp.json` in project root | Auto-discovered |
| Codex | `codex mcp add` or CLI args | Generated before spawn |
| OpenCode | `opencode.json` config | Generated before spawn |
| Aider | Not supported | Skip |
| Amp | Not supported | Skip |

**Concrete scenario:** User has Phoenix app with `tidewave_phoenix` installed. Runs `ralph run`. Ralph starts Phoenix dev server AND Tidewave MCP, generates `.mcp.json`, spawns Claude Code. Claude Code discovers MCP, connects, gains access to `project_eval`, `get_logs`, `execute_sql_query`. All automatic, zero manual config.

**Implementation:** ~400-700 LOC. `config.go` (MCPConfig struct), `loop.go` (generate `.mcp.json` before provider spawn, cleanup after), `ralph.schema.json` (schema update).

#### 2.2 MCP Proxy Reference

Tidewave's `mcp-proxy` binary bridges stdio↔HTTP transport. Ralph should:
- Document how to use `mcp-proxy` with Ralph's providers
- Optionally reference it in generated MCP configs for providers that need stdio

**Why this matters:** Some MCP clients only support stdio. If Ralph generates an HTTP-based MCP config but the provider needs stdio, the proxy bridges the gap. Also handles automatic reconnection on dev server restart.

**Files:** Documentation + optional `mcp-proxy` download in `ralph init`.

#### 2.3 Prompt Size Reduction via Helper References

Tidewave's tip: "Extend by implementing functions in your codebase and telling the agent which ones to invoke."

Ralph's resource consultation currently pastes 1-3KB of guidance inline per story. Instead, generate a `.ralph/resource-helpers.md` with concise function signatures + paths:

**Current** (inline in prompt):
```
## React Implementation Guidance
When using useState hooks, prefer functional updates...
[800 tokens of framework patterns]
Source: react@19.1.0/src/ReactHooks.js:42
```

**Proposed** (reference + summary):
```
## Framework References Available
- React patterns: .ralph/consultations/react-guidance.md (useState, useEffect, hooks)
- Prisma patterns: .ralph/consultations/prisma-guidance.md (queries, migrations)
Read these files for detailed framework guidance before implementing.
```

Reduces prompt size ~30% without losing information. Provider reads files on demand.

**Files:** `consultation.go` (change `FormatGuidance()` to write file + return reference), `prompts/run.md` (reference instead of inline).

### Tier 3: Medium-Term (Next Quarter)

#### 3.1 Optional Container Isolation for Providers

Add optional `container` config to `ProviderConfig`:

```json
{
  "provider": {
    "command": "claude",
    "container": {
      "image": "myapp-dev:latest",
      "workdir": "/workspace",
      "volumes": [".:/workspace"]
    }
  }
}
```

When configured, `runProvider()` wraps the provider command in `docker run`. This makes `project_eval` safe for unattended runs (sandboxed to container).

**Implementation:** Detect container config → modify `exec.Command` to prefix with `docker run -v ... -w ... <image>` → map paths via `TIDEWAVE_HOST_PATH` pattern.

**Files:** `config.go` (ContainerConfig), `loop.go` (wrap `runProvider` command).

#### 3.2 Dependency-Filtered Resource Consultation

Currently `relevantFrameworks()` uses tag + keyword matching. Add a dependency-presence boost:

```go
// If framework is in project's actual dependencies, boost its score
for _, dep := range codebaseCtx.Dependencies {
    if strings.EqualFold(dep.Name, cached.Name) {
        score += 5 // strong signal
    }
}
```

Inspired by Tidewave's `search_package_docs` which filters by project dependencies only.

**Files:** `consultation.go` (`relevantFrameworks()`).

#### 3.3 Semantic Verify Section (`verify.runtime`)

Split verification into semantic sections:

```json
{
  "verify": {
    "default": ["go test ./...", "go vet ./..."],
    "runtime": ["curl -sf http://localhost:3000/health"],
    "ui": ["bun run test:e2e"]
  }
}
```

`runtime` commands signal intent: "these require the running app." Ralph could skip them when services aren't configured.

**Files:** `config.go` (add `Runtime []string` to `VerifyConfig`), `loop.go` (3 call sites).

---

## What Ralph Should NOT Adopt

| Tidewave Concept | Why Not | Risk if Adopted |
|-----------------|---------|-----------------|
| **Browser integration / Inspector** | Ralph is headless, unattended. No browser, no UI interaction. | Breaks unattended loops, requires human presence, non-deterministic |
| **Desktop app hub** | Ralph is stateless CLI, not a long-running service. | 1000+ LOC, entangles CLI with UI, incompatible with CI |
| **Agent Client Protocol (ACP)** | Ralph's unidirectional markers (DONE/STUCK/LEARNING) are sufficient. Bidirectional RPC risks deadlocks. | Breaks process isolation, provider becomes dependent on CLI state |
| **Framework plugins (embedded in app)** | Ralph orchestrates from outside. Embedding in the app couples to specific frameworks. | N frameworks × M versions maintenance nightmare |
| **`project_eval` as Ralph-provided tool** | Unattended agents with code execution = exfiltration, fake tests, DB corruption. | Violates CLI/provider separation, breaks verification integrity |
| **Direct database queries as Ralph feature** | Provider should write migration files, not execute SQL. Changes must be auditable in git. | Unauditable state mutation, can fake passing tests |
| **Figma integration** | Code-first, not design-first. Niche use case. | Over-engineering |
| **Viewport / accessibility testing** | Already covered by `verify.ui` commands (user's own e2e test suite). | Redundant |
| **Notifications** | Ralph is unattended — no user to notify during runs. | Irrelevant |
| **Task Board sync** | Ralph's PRD stories are persistent and CLI-managed. Claude Code tasks are session-scoped. Different lifecycles. | Bidirectional sync complexity for low value |

### Core Architectural Principle

Ralph's design assumes:
1. **Unattended operation** — loops run for hours without human oversight
2. **Determinism** — same input → same output, reproducible
3. **Auditability** — every change committed to git, reviewable
4. **Process isolation** — provider is a subprocess, not privileged
5. **Verification integrity** — provider cannot affect test outcomes

Tidewave assumes:
1. **Interactive development** — developer watches agent in real time
2. **Runtime introspection** — agents query live app state
3. **Hub coordination** — central service brokers communication
4. **Developer trust** — agents have broad system access

**These are incompatible worldviews for the core execution model.** But the MCP tool layer is framework-agnostic infrastructure that Ralph can **manage without adopting**. Ralph doesn't need to become Tidewave — it needs to make it easy for providers to use Tidewave when available.

---

## Where Ralph Already Wins

### Orchestration (Tidewave Has None)

Tidewave has no concept of:
- Story decomposition and sequencing
- Retry logic with failure context
- Cross-story learning accumulation
- Verify-at-top idempotent resume
- PRD-driven quality gates
- Provider-agnostic execution

Ralph IS the orchestration layer. Tidewave IS the tool layer. They compose naturally.

### Offline Documentation (Better Than Runtime Introspection for CI)

Ralph's resource consultation works in CI, private networks, air-gapped environments. Tidewave requires a running app instance. For unattended batch processing, offline > online.

### Provider Agnosticism

Tidewave's browser-based agents support Claude Code, Codex, Copilot, OpenCode. But MCP tools work with any MCP-compatible client. Ralph supports 5+ providers as subprocesses — complementary, not overlapping.

### Learning System (Unique)

Neither Tidewave nor any other system analyzed (Stripe Minions, Symphony) has cross-task learning persistence. Ralph's deduped, capped learning system remains unique.

### Atomic State Management

Ralph's `AtomicWriteJSON` (temp → validate → rename) provides crash recovery and state inspection. Tidewave is stateless (MCP queries). Different requirements, but Ralph's approach enables interrupt-and-resume workflows that Tidewave doesn't need.

---

## Summary: The 80/20

Ralph can get 80% of Tidewave's value from 3 changes:

1. **Service log analysis in verification** — Ralph already captures 256KB of service logs per service. Grep for ERROR/PANIC, pass to AI verify subagent. ~15 lines of code, zero new infrastructure. Captures the intent of Tidewave's `get_logs` without MCP.

2. **MCP server management** — Ralph already manages services. Add MCP servers as another service type, generate `.mcp.json` for Claude Code. Providers get Tidewave tools automatically when available. ~400-700 LOC, reuses existing `ServiceManager`.

3. **MCP proxy reference** — Document and optionally bundle the `mcp-proxy` binary so stdio-only providers can access HTTP MCP servers. Eliminates the transport mismatch problem.

The fundamental insight: **Ralph should not become a tool layer — it should make tool layers accessible.** Manage the MCP server lifecycle, generate provider configs, let Tidewave (or similar) provide the runtime intelligence. Ralph stays lean, orchestration-focused, and provider-agnostic.
