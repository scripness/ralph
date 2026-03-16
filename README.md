# Scrip

Autonomous AI agent loop that implements entire features from a plan — with Claude Code.

```bash
# Install
curl -fsSL https://raw.githubusercontent.com/scripness/ralph/main/install.sh | bash
# or: go install github.com/scripness/scrip@latest

# Use
scrip prep                # detect project, configure verification, cache dependencies
scrip plan auth           # iterative planning with AI consultation
scrip exec auth           # autonomous execution loop until every item passes
scrip land auth           # verify, summarize, and push
```

Scrip orchestrates Claude Code in a deterministic loop: pick the next plan item, spawn a fresh AI instance, verify the implementation with your project's own test suite, persist learnings, repeat. The CLI controls everything — item selection, state management, verification, service lifecycle — while the AI provider is a pure code implementer.

---

## Features

### Claude Code-Only Design

Scrip is built exclusively for Claude Code. All spawns use `claude --print --model opus --effort max`. Autonomous mode (exec, land fix) adds `--dangerously-skip-permissions`. Non-autonomous mode (consultation, verification, planning) omits it.

Providers communicate with Scrip through three markers detected on stdout/stderr:

| Marker | Meaning |
|--------|---------|
| `<scrip>DONE</scrip>` | Implementation complete, ready for verification |
| `<scrip>STUCK:reason</scrip>` | Cannot proceed — counts as a failed attempt |
| `<scrip>LEARNING:text</scrip>` | Insight saved for future iterations |

Markers are matched as whole lines (not substrings) to prevent spoofing.

### Planning Workflow

`scrip plan <feature>` creates and iterates on a structured plan:

**New feature** — AI brainstorms a plan draft with prioritized items, acceptance criteria, and dependencies. Multiple rounds of iteration before finalization.

**Existing plan** — Opens a new planning round to refine items, adjust priorities, or add items based on new context.

Plans are stored as `plan.jsonl` (append-only rounds) with items containing titles, acceptance criteria, and priorities.

### Deterministic Agent Loop

`scrip exec <feature>` enters an infinite loop until every item is passed or skipped:

1. **Load state** — reads `plan.jsonl` + `progress.jsonl`, acquires lock, creates/switches to `plan/<feature>` branch, starts services
2. **Pick next item** — highest priority, not passed, not skipped
3. **Verify-at-top** — runs verification *before* spawning the provider. If the item already passes, marks it done and moves on
4. **Resource consultation** — spawns lightweight subagents to search cached framework source and produce focused guidance
5. **Spawn provider** — sends prompt with item details, learnings, consultation guidance
6. **Detect markers** — scans provider output for DONE, STUCK, LEARNING
7. **Commit check** — provider must have created a new git commit (DONE without a commit = failed attempt)
8. **Verify** — runs typecheck, lint, test commands + service health checks
9. **Mark result** — pass → next item; fail → retry up to `maxRetries` (default 3), then auto-skip
10. **Repeat** until all items are passed or skipped

SIGINT/SIGTERM triggers graceful cleanup: kills provider process group, stops services, releases lock, exits 130.

### Verification and Landing

`scrip land <feature>` — comprehensive verification and landing:
- All verify commands (typecheck + lint + test)
- Service health checks
- AI deep analysis — subagent reads changed files, checks every acceptance criterion, outputs `VERIFY_PASS` or `VERIFY_FAIL:reason`
- On failure, spawns an AI fix session pre-loaded with the full failure context, then re-verifies
- On success, generates an AI summary of what was built, writes it to the feature's `summary.md`, and pushes

### Framework Source Consultation

Scrip auto-resolves every project dependency to its source repository, caches it locally, and spawns lightweight subagents to search the cached source before each item.

**Resolution flow:**
1. Extract dependencies from package.json / go.mod / pyproject.toml / Cargo.toml / mix.exs
2. Read exact versions from lock files (bun.lock, package-lock.json, yarn.lock v1/berry, pnpm-lock.yaml v6/v9, go.mod)
3. Resolve repo URLs from registries (npm, PyPI, crates.io, hex.pm, Go module paths)
4. Shallow-clone at the version tag to `~/.scrip/resources/name@version/`

**Consultation flow:**
- Before each item, Scrip picks up to 3 relevant frameworks using keyword matching and name-based matching
- Spawns parallel subagents that search cached source and return focused guidance (200-800 tokens each)
- Results are cached per item (SHA256 hash) — retries reuse cached consultations
- Falls back to web search instructions when no resources are available

Typical cache sizes: React ~280MB, Next.js ~450MB, smaller libs 10-30MB. Delete `~/.scrip/resources/` to free space.

### Service Management

Scrip manages dev servers across the entire lifecycle:

1. **Start** — spawns service process with process group isolation (`Setpgid`)
2. **Ready check** — polls the `ready` URL every 500ms until HTTP status < 500
3. **Restart** — optionally restarts before each verification (`restartBeforeVerify: true`)
4. **Health check** — verifies services still respond during verification
5. **Cleanup** — kills entire process group on exit, error, or signal

Service output is captured for diagnostics but not printed to the console.

### Summary-Based Memory

After a feature is verified and landed, Scrip generates a dense technical summary and writes it to the feature's `summary.md` (e.g., `.scrip/2024-01-15-auth/summary.md`). This summary is the permanent record of what was built.

### Safety and Reliability

- **Atomic writes** — all state files use temp + validate + rename to prevent corruption
- **Lock file** — prevents concurrent runs with stale detection (PID liveness + 24h age guard)
- **Append-only state** — `progress.jsonl` is the source of truth, recoverable after crashes
- **Branch management** — auto-creates `plan/<feature>` branch from the default branch (main/master)
- **Process group kills** — provider subprocesses and services use `Setpgid` so timeouts kill entire process trees

---

## Working with Scrip

### Setting Up

```bash
scrip prep
```

Scrip detects your project's tech stack, package manager, and suggests verify commands.

**Auto-detected tech stacks:**

| Stack | Detected from | Package managers | Frameworks detected |
|-------|--------------|-----------------|---------------------|
| Go | `go.mod` | go | Gin, Echo, Fiber, Chi, GORM, sqlx |
| TypeScript/JS | `package.json`, `tsconfig.json` | bun, npm, yarn, pnpm | React, Next.js, Vue, Nuxt, Svelte, SvelteKit, Express, Fastify, Hono, Prisma, Drizzle, Playwright, Vitest, Jest |
| Python | `pyproject.toml`, `requirements.txt` | pip | Django, Flask, FastAPI, pytest, SQLAlchemy |
| Rust | `Cargo.toml` | cargo | Actix, Axum, Rocket, Tokio, Diesel, sqlx |
| Elixir | `mix.exs` | mix | Phoenix, LiveView, Ecto, Plug, Absinthe, Oban |

**Verify command auto-detection:**
- JS/TS: reads `scripts` from package.json (prefers `test:unit` over `test`)
- Go: suggests `go vet ./...` and `go test ./...`; detects `golangci-lint run` if config exists
- Rust: suggests `cargo check` and `cargo test`
- Python: suggests `pytest` if in dependencies
- Elixir: suggests `mix compile --warnings-as-errors` and `mix test`; detects `mix credo`

### Creating a Plan

```bash
scrip plan auth
```

AI brainstorms a plan with prioritized items and acceptance criteria, then iterates with you through multiple rounds before finalization. Run it again to add new planning rounds.

### Running the Loop

```bash
scrip exec auth
```

Scrip picks items by priority and spawns fresh AI instances. The loop runs until every item is passed or skipped.

### Landing the Feature

```bash
scrip land auth
```

Runs all verification commands, spawns an AI deep analysis subagent, and on success generates a summary and pushes.

### Configuration Reference

`.scrip/config.json`:

```json
{
  "$schema": "https://scrip.dev/config.schema.json",
  "project": {
    "name": "my-app"
  },
  "provider": {
    "command": "claude",
    "timeout": 1800,
    "stallTimeout": 300
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
    "typecheck": "bun run typecheck",
    "lint": "bun run lint",
    "test": "bun run test:unit"
  }
}
```

| Section | Field | Default | Description |
|---------|-------|---------|-------------|
| project | `name` | auto (repo dir name) | Project name |
| provider | `command` | **required** | AI CLI command (claude) |
| provider | `timeout` | `1800` | Hard timeout per provider spawn in seconds |
| provider | `stallTimeout` | `300` | No-output timeout in seconds |
| services[] | `name` | **required** | Service identifier |
| services[] | `start` | — | Shell command to start the service |
| services[] | `ready` | **required** | URL to poll (must start with `http://` or `https://`) |
| services[] | `readyTimeout` | `30` | Seconds to wait for ready |
| services[] | `restartBeforeVerify` | `false` | Restart before each verification |
| verify | `typecheck` | — | Typecheck command |
| verify | `lint` | — | Lint command |
| verify | `test` | **required** | Test command |

### File Structure

```
project/
└── .scrip/
    ├── config.json
    ├── 2024-01-15-auth/
    │   ├── plan.jsonl                  # Planning rounds (append-only)
    │   ├── progress.jsonl              # Execution events (append-only)
    │   ├── summary.md                  # Feature summary (written on land)
    │   └── consultations/              # Cached framework consultation results
    │       ├── a1b2c3d4...sha.md
    │       └── e5f6g7h8...sha.md
    ├── 2024-01-20-billing/
    │   └── ...
    └── scrip.lock                      # Prevents concurrent runs
```

---

## Build from Source

```bash
git clone https://github.com/scripness/ralph
cd ralph
make build    # go build -ldflags="-s -w" -o scrip .
make test     # go test ./...
```

Releases are automated via [GoReleaser](https://goreleaser.com/) and GitHub Actions — `make release` triggers a workflow that bumps the version tag, runs tests, builds 4 binaries (linux/darwin x amd64/arm64), and creates a GitHub Release.

Based on the [Ralph pattern](https://ghuntley.com/ralph/) — originally [snarktank/ralph](https://github.com/snarktank/ralph).

## License

MIT
