# scrip v1 — Build Prompt

You are implementing the scrip v1 CLI. This is an autonomous loop — each iteration you implement ONE unit of work, verify it, commit, and exit. The next iteration starts with fresh context.

## Phase 0a: Orient — Study the specification

Study `PHASE0-SPEC.md` using up to 500 parallel Sonnet subagents. This is your complete specification — schemas, templates, session structure, reuse map, contracts. Understand what scrip v1 IS before touching code.

Also study `ROADMAP.md` lines 471-1048 for design context behind the specification.

## Phase 0b: Orient — Study the current codebase

Study the current Go source files using up to 500 parallel Sonnet subagents. Understand what EXISTS right now. The ralph codebase is your foundation — ~75% of it copies directly or adapts into scrip v1.

Key files to study:
- All `*.go` files in the project root (the ralph codebase you're building from)
- Any files you created in previous iterations (they will exist on disk)
- `go.mod` for the module path and dependencies

## Phase 0c: Orient — Determine what's next

Compare what PHASE0-SPEC.md says should exist against what actually exists in the codebase. The specification defines 6 implementation sessions with dependencies:

```
Session 0: Foundation (main.go, state.go, progress.go, plan.go, config adaptation)
  ↓
Session 1: scrip prep     (parallel with Session 2)
Session 2: scrip plan     (parallel with Session 1)
  ↓
Session 3: scrip exec     (requires Session 0)
  ↓
Session 4: scrip land     (requires Sessions 2 + 3)
  ↓
Session 5: Integration    (requires all above)
```

Determine progress by checking:
1. Do the files listed in each session exist?
2. Do they compile? (`go build ./...`)
3. Do their tests pass? (`go test ./...`)
4. Do they implement the behaviors described in the spec?

Pick the MOST IMPORTANT next item — the first uncompleted piece of work in the earliest incomplete session whose dependencies are met. If a session is partially done, continue it. If all items in a session pass, move to the next session.

**One item per iteration.** An "item" is one of:
- Create one new file (e.g., `state.go` with its types and functions)
- Adapt one existing file (e.g., rename ralph→scrip in `lock.go`)
- Copy one file as-is (just copy, verify it compiles in the new context)
- Write tests for one module
- Fix a failing test or build error from a previous iteration

## Phase 1: Implement

Before making changes, search the codebase — don't assume not implemented. Use up to 500 parallel Sonnet subagents for searches/reads. Use Opus subagents when complex reasoning is needed (debugging, architectural decisions). Ultrathink.

Implement the selected item COMPLETELY. No placeholders. No stubs. No TODOs. Full implementation with real logic.

When adapting ralph files for scrip:
- Rename all `ralph` → `scrip` references (paths, error messages, types, markers)
- Rename `.ralph/` → `.scrip/` and `~/.ralph/` → `~/.scrip/`
- Rename `<ralph>` markers → `<scrip>` markers
- Follow the specific adaptation notes in PHASE0-SPEC.md's Codebase Reuse Map

When creating new files:
- Follow the schemas in PHASE0-SPEC.md exactly
- Follow the behavioral requirements for prompt templates exactly
- Use the Orient → Act → Guardrails structure with escalating 9s for all prompt templates

## Phase 2: Test

Run verification using only 1 subagent for build/tests (critical — multiple build agents = bad backpressure):

```bash
go build ./...
go vet ./...
go test ./...
```

Every new exported function needs at least one test. Tests must cover the actual behavior, not just exist.

If tests fail, fix them in this iteration. Do not leave failing tests for the next iteration.

## Phase 3: Verify against spec

After tests pass, use up to 500 parallel Sonnet subagents to verify your implementation matches PHASE0-SPEC.md:
- Do the types match the schemas in the spec?
- Do the functions implement the behaviors described?
- Are the contracts from the Cross-Session Contract section honored?
- Did you introduce any regressions to previously working code?

Use an Opus subagent for any complex architectural verification.

If verification reveals a mismatch with the spec, fix it before committing.

## Phase 4: Commit

When everything passes:

```bash
git add <specific files you changed>
git commit -m "scrip: <what you implemented>"
```

Commit message should describe WHAT was implemented, not the session number.

## Guardrails

99999. Important: ONE item per iteration. Do not try to implement an entire session in one pass. Implement one file, one adaptation, or one test suite, verify it compiles and passes, commit, and exit. The loop will restart you with fresh context for the next item.

999999. Important: Don't assume not implemented. Before creating or modifying any file, search the codebase to see if it already exists or if the functionality is already present from a previous iteration.

9999999. Important: No placeholders or stubs. Implement functionality completely. Placeholders and stubs waste efforts and time redoing the same work. If you can't implement something fully, use a LEARNING marker to explain why and move on to something you CAN implement fully.

99999999. Important: Capture the why. When you discover something non-obvious — a pattern in the ralph codebase, a gotcha with Go APIs, a design decision that affects future work — emit a LEARNING marker. Good learnings: "config.go's knownProviders map must be removed entirely, not just modified — the provider detection logic in loop.go depends on it." Bad learnings: "I created state.go."

999999999. Important: If tests unrelated to your current work fail, fix them. Do not leave broken tests for the next iteration. The build must be green after every commit.

9999999999. Important: Do NOT modify PHASE0-SPEC.md or ROADMAP.md. These are your specification — read them, don't write them.

99999999999. Important: When you finish your item and commit, EXIT cleanly. Do not start the next item. The loop handles iteration.

999999999999. Important: If ALL sessions are complete — all files exist, all tests pass, `go build` produces a working `scrip` binary, and the implementation matches the spec — create a file called `SCRIP_V1_COMPLETE` containing the current date and a one-line summary. This signals the loop to stop.

## Learnings from previous iterations

If a file called `LEARNINGS.md` exists in the project root, study it. It contains insights from previous iterations that will help you avoid repeating mistakes.

When you emit `<scrip>LEARNING:text</scrip>` markers, the loop script appends them to `LEARNINGS.md` for future iterations.

## Completion signal

When done with your single item:
```
<scrip>DONE</scrip>
```

If stuck and cannot proceed:
```
<scrip>STUCK:reason</scrip>
```

To record an insight for future iterations:
```
<scrip>LEARNING:text</scrip>
```
