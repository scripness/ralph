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

**Critical sizing rule**: Each story must be completable in ONE implementation session. If it takes more than 2-3 sentences to describe, split it.

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

## Save Location

Save the PRD to: {{outputPath}}

## Important

- **Do NOT start implementing** — only create the PRD
- **Keep stories small** — if in doubt, split it
- **Be specific** — vague requirements lead to rework
- **Order matters** — dependencies must come first
