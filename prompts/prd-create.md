# PRD Creation: {{feature}}

Help create a Product Requirements Document for this feature. Your job is to gather requirements and produce a structured PRD — do NOT start implementing.

## Step 1: Clarifying Questions

First, ask 3-5 critical questions to understand the requirements:

1. **Problem/Goal**: What problem does this solve? What's the desired outcome?
2. **Core Functionality**: What are the key actions users will take?
3. **Scope**: What should it NOT do? What's explicitly out of scope?
4. **Success Criteria**: How do we know it's done?
5. **Technical Context**: Any existing code/patterns to follow?

Format with lettered options when helpful:
```
1. What is the primary goal?
   A. Option one
   B. Option two  
   C. Other: [specify]
```

**Wait for user answers before proceeding to Step 2.**

## Step 2: Generate PRD

After receiving answers, create a detailed PRD with these sections:

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

**Tags:** [ui] (if browser verification needed)
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
- Consider what browser verification steps would look like (navigate, click, type, assert)
- Ensure e2e test coverage expectations are clear

For UI stories, think about concrete user interactions:
- What page does the user navigate to?
- What do they click, type, or submit?
- What should they see as a result?

These interactions will become automated browser verification steps (browserSteps) during finalization.

## Save Location

Save the PRD to: {{outputPath}}

## Splitting Large Features

If a feature is large, split it into focused stories:

**Original:** "Add user notification system"

**Split into:**
1. US-001: Add notifications table to database
2. US-002: Create notification service for sending notifications
3. US-003: Add notification bell icon to header
4. US-004: Create notification dropdown panel
5. US-005: Add mark-as-read functionality

Each story is one focused change that can be completed and verified independently.

## Writing Quality

The PRD reader will be an AI agent with a single context window. Therefore:
- Be explicit and unambiguous
- Avoid jargon or explain it
- Provide enough detail to understand purpose and core logic
- Use concrete examples where helpful

## Important

- **Do NOT start implementing** — only create the PRD
- **Keep stories small** — if in doubt, split it
- **Be specific** — vague requirements lead to rework
- **Order matters** — dependencies must come first
