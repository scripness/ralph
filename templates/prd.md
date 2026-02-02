# PRD Generator

Create a Product Requirements Document for a new feature.

## Your Task

1. Ask the user 3-5 clarifying questions about their feature (with lettered options for quick responses)
2. Generate a structured PRD based on their answers
3. Save the PRD to `{{outputPath}}`

**Important:** Do NOT implement anything. Just create the PRD document.

## Clarifying Questions

Ask only critical questions. Focus on:

- **Problem/Goal:** What problem does this solve?
- **Core Functionality:** What are the key actions?
- **Scope/Boundaries:** What should it NOT do?
- **Success Criteria:** How do we know it's done?

Format questions with lettered options so users can respond with "1A, 2C, 3B":

```
1. What is the primary goal?
   A. Option one
   B. Option two
   C. Other: [please specify]
```

## PRD Structure

Generate the PRD with these sections:

### 1. Introduction/Overview
Brief description of the feature and problem it solves.

### 2. Goals
Specific, measurable objectives (bullet list).

### 3. User Stories
Each story needs:
- **Title:** Short descriptive name
- **Description:** "As a [user], I want [feature] so that [benefit]"
- **Acceptance Criteria:** Verifiable checklist

Format:
```markdown
### US-001: [Title]
**Description:** As a [user], I want [feature] so that [benefit].

**Acceptance Criteria:**
- [ ] Specific verifiable criterion
- [ ] Another criterion
- [ ] Typecheck passes
- [ ] **[UI stories only]** Verify in browser
```

**Critical:** Each story must be small enough to implement in ONE focused session. If a story is too big, split it.

### 4. Functional Requirements
Numbered list: "FR-1: The system must..."

### 5. Non-Goals (Out of Scope)
What this feature will NOT include.

### 6. Technical Considerations (Optional)
Known constraints, dependencies, performance requirements.

### 7. Open Questions
Remaining questions needing clarification.

## Story Sizing Rules

**Right-sized stories:**
- Add a database column and migration
- Add a UI component to an existing page
- Update a server action with new logic

**Too big (split these):**
- "Build the entire dashboard" → Split into: schema, queries, UI, filters
- "Add authentication" → Split into: schema, middleware, login UI, sessions

**Rule of thumb:** If you cannot describe the change in 2-3 sentences, it's too big.

## Output

Save the PRD to: `{{outputPath}}`

After saving, tell the user to run:
```
ralph convert {{outputPath}}
```
