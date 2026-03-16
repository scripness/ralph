# scrip v1 — Refinement Prompt (Round 02)

You are refining the scrip v1 CLI. This round fixes 3 CodeRabbit-verified bugs and 3 codebase organization issues. All tasks are verified — the problems are real and the fixes are described precisely.

This is an autonomous loop — each iteration you fix ONE task, verify it, commit, and exit.

## Phase 0a: Orient — Study the refinement spec

Study `PHASE0-SPEC-refine-02.md` using up to 500 parallel Sonnet subagents. It contains 6 tasks: 3 bug fixes and 3 code organization improvements. Each task has exact problem descriptions, fix strategies, and exit criteria.

Also study `PHASE0-SPEC.md` for design context when needed.

## Phase 0b: Orient — Study the current codebase

Study the relevant source files using up to 500 parallel Sonnet subagents. Focus on the files listed in each task's scope.

## Phase 0c: Orient — Determine what's next

Check which tasks from `PHASE0-SPEC-refine-02.md` are already complete. Pick the highest-priority incomplete task. **One task per iteration.**

## Phase 1: Implement

Before making changes, search the codebase — don't assume not implemented. Use up to 500 parallel Sonnet subagents for searches/reads. Use Opus subagents for complex reasoning. Ultrathink.

Follow the fix instructions in the spec closely. Each task was verified by adversarial agents.

## Phase 2: Test

Run verification using only 1 subagent for build/tests:

```bash
go build ./...
go vet ./...
go test ./...
```

If the task adds new behavior, add tests. The build must be green after every commit.

## Phase 3: Verify against exit criteria

After tests pass, verify your implementation matches the task's exit criteria from `PHASE0-SPEC-refine-02.md`.

## Phase 4: Commit

```bash
git add <specific files you changed>
git commit -m "scrip: <what you fixed/changed>"
```

## Guardrails

99999. Important: ONE task per iteration. Complete the entire task, verify, commit, exit.

999999. Important: Don't assume not implemented. Check if a previous iteration already applied the fix.

9999999. Important: Follow the fix instructions in PHASE0-SPEC-refine-02.md closely. The problem descriptions are verified against the actual source code.

99999999. Important: Do NOT modify PHASE0-SPEC.md, PHASE0-SPEC-refine-01.md, PHASE0-SPEC-refine-02.md, or ROADMAP.md.

999999999. Important: If tests unrelated to your current task fail, fix them. The build must be green after every commit.

9999999999. Important: Capture insights via LEARNING markers when you discover something non-obvious.

99999999999. Important: When you finish your task and commit, EXIT cleanly. Do not start the next task.

999999999999. Important: If ALL 6 tasks are complete — verified by build, tests, and each task's exit criteria — create `REFINE_02_COMPLETE` containing the current date and a one-line summary.

## Learnings

If `LEARNINGS.md` exists, study it. Emit `<scrip>LEARNING:text</scrip>` markers for future iterations.

## Completion signals

```text
<scrip>DONE</scrip>
```

```text
<scrip>STUCK:reason</scrip>
```

```text
<scrip>LEARNING:text</scrip>
```
