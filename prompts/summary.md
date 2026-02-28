# Feature Summary Generation

You are generating a technical summary of a completed feature. This summary will be the ONLY context available to AI agents working on this project in the future — the PRD and run state files are deleted after archiving. The summary must preserve all information an agent needs to understand what was built and how.

## Feature Context

**Project:** {{project}}
**Feature:** {{feature}}
**Description:** {{description}}
**Branch:** {{branchName}}

## PRD (prd.md)

{{prdMdContent}}

## Execution State

{{storyDetails}}

- **Passed:** {{passedCount}} stories
- **Skipped:** {{skippedCount}} stories

{{retryDetails}}

{{learnings}}

## Changes on Branch

{{diffSummary}}

**Changed files:**
{{changedFiles}}

## Instructions

This summary is consumed by AI coding agents, not humans. Optimize for machine utility:

1. Use 2-4 subagents to review the PRD, implementation files, and git diff in parallel
2. Extract concrete technical details — file paths, function names, data models, API endpoints, database schemas, component hierarchies
3. Preserve the PRD's acceptance criteria and story specifications in condensed form
4. Document integration points: how this feature connects to existing code, what it imports/exports, what APIs it exposes or consumes
5. Record architectural patterns established by this feature that future work must follow

Write a dense technical summary (300-600 words) between the markers below.

The summary MUST include:
- **Specifications**: What each story required (condensed acceptance criteria). If stories were skipped, include what they specified and why they were skipped — a future agent may need to implement them.
- **Implementation map**: Key files created/modified with their purpose (e.g., `src/auth/login.ts — LoginForm component using react-hook-form, validates email+password, calls POST /api/auth/login`)
- **Data model & APIs**: Any schemas, endpoints, database tables, or state shapes introduced
- **Patterns & conventions**: Architectural decisions that constrain future work (e.g., "All API routes use middleware chain: auth → validate → handler → serialize", "State managed via Zustand store at src/stores/auth.ts")
- **Integration points**: How this feature connects to the rest of the codebase — imports, shared state, event buses, config dependencies
- **Gotchas**: Non-obvious behaviors, workarounds, or constraints discovered during implementation (from learnings and retry history)

Do NOT write narrative prose. Use terse technical notation. Every sentence should contain a file path, function name, or concrete technical detail.

## Output

Output your summary between these markers:

```
<ralph>SUMMARY_START</ralph>
[your summary here]
<ralph>SUMMARY_END</ralph>
```
