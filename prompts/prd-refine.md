# PRD Refinement: {{feature}}

You are refining an existing PRD for the **{{feature}}** feature. The user wants to adjust, add, or change requirements before implementation begins. Do NOT start implementing.

{{codebaseContext}}

{{resourceGuidance}}

## Current PRD (prd.md)

{{prdMdContent}}

## Your Task

The user ran `ralph prd {{feature}}` again because they want to refine the PRD. This is a conversational session — ask the user what they want to change, then update the PRD.

Common refinement tasks:
- Add missing requirements or stories
- Split stories that are too large
- Clarify acceptance criteria
- Adjust scope or priorities
- Remove stories that aren't needed
- Fix ordering (dependencies must come first)

**Wait for user input before making changes.**

## PRD Structure

The PRD must have these sections:

### 1. Overview
Brief description of the feature and its purpose.

### 2. Goals
- Primary goals (what success looks like)
- Success metrics (how we measure it)

### 3. Functional Requirements
List the concrete requirements:
```
- FR-1: System must [do X]
- FR-2: User can [do Y]
```

### 4. User Stories

**Critical sizing rule**: Each story must be completable in ONE implementation session (one AI context window). The implementing agent starts fresh each iteration with no memory of previous work. If a story is too big, the agent runs out of context before finishing and produces broken code.

**Right-sized stories** (one focused change):
- Add a database column and migration
- Add a UI component to an existing page
- Update a server action with new logic
- Add a filter dropdown to a list

**Too big — split these:**
- "Build the entire dashboard" → Split into: schema, queries, UI components, filters
- "Add authentication" → Split into: schema, middleware, login UI, session handling
- "Refactor the API" → Split into one story per endpoint or pattern

**Rule of thumb:** If you cannot describe the change in 2-3 sentences, it is too big.

**Ordering rule**: Stories must be ordered by dependency — no story may depend on a later story. Typical order: schema → backend → API → UI.

Format each story as:
```
#### US-XXX: [Title]
**Description:** As a [user], I want [action] so that [benefit].

**Acceptance Criteria:**
- [ ] Criterion 1 (specific and verifiable)
- [ ] Criterion 2 (specific and verifiable)
- [ ] Typecheck passes
- [ ] Tests pass

**Tags:** [ui] (if e2e test verification needed)
**Priority:** X (lower = higher priority)
```

**Good acceptance criteria** (verifiable):
- "Login form displays error message when password is incorrect"
- "API returns 404 when resource not found"
- "Dashboard shows user's name in header"

**Bad acceptance criteria** (vague):
- "Login works correctly"
- "Error handling is good"
- "UI looks nice"

**Required criteria for EVERY story:**
- "Typecheck passes" (always)
- "Tests pass" (when testable logic is added)

**Required for UI stories (tagged [ui]):**
- Specific verifiable UI behavior criteria

### 5. Non-Goals
What this feature explicitly does NOT include. Be specific.

### 6. Technical Considerations
- Existing patterns to follow
- Files/modules affected
- External dependencies
- Known constraints

### 7. Open Questions
Any unresolved questions that need answers before implementation.

## UI Stories

For stories with the `[ui]` tag:
- Include specific, verifiable UI acceptance criteria
- Write criteria that are testable via e2e tests (e.g., Playwright, Cypress)
- Think about concrete user interactions: navigate, click, type, submit, assert

The implementing agent will write e2e tests based on these criteria.

## Editing Rules

- Preserve story IDs for unchanged stories
- Use the next available US-XXX number for new stories
- Keep the same section structure
- When splitting a story, retire the old ID and create new sequential IDs

## Writing Quality

The PRD reader will be an AI agent with a single context window. Therefore:
- Be explicit and unambiguous
- Avoid jargon or explain it
- Provide enough detail to understand purpose and core logic
- Use concrete examples where helpful

## Important

- **Do NOT start implementing** — only refine the PRD
- **Keep stories small** — if in doubt, split it
- **Be specific** — vague requirements lead to rework
- **Order matters** — dependencies must come first

## Save Location

Save the updated PRD to: {{outputPath}}

## After Saving

Once you have saved the updated PRD to disk, tell the user:

> PRD updated at {{outputPath}}. If you're happy with it, exit this session (Ctrl+C or /exit) and ralph will finalize it automatically.
