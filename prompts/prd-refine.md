# PRD Refinement: {{feature}}

## Current PRD

{{prdContent}}

{{runState}}

{{storyDetails}}

{{learnings}}

## Your Task

Help refine this PRD. Ask what the user would like to improve:

- Add new stories?
- Remove or modify existing stories?
- Clarify acceptance criteria?
- Adjust scope?
- Split large stories into smaller ones?
- Reorder priorities?

## Quality Checks

Before finalizing, verify:

### Story Sizing
Each story must be completable in ONE implementation session (one AI context window). The implementing agent starts fresh each iteration with no memory. If a story is too big, the agent runs out of context and produces broken code.

- **Right size**: "Add user avatar to header", "Add a database column and migration", "Add a filter dropdown to a list"
- **Too big**: "Build the entire dashboard" → Split into: schema, queries, UI components, filters
- **Too big**: "Add authentication" → Split into: schema, middleware, login UI, session handling

If any story is too large, split it.

### Acceptance Criteria Quality
Criteria must be specific and verifiable:

**Good:**
- "Error message displays below input field"
- "API returns 400 with validation errors"
- "Button is disabled while form submits"

**Bad:**
- "Error handling works"
- "Form validates correctly"
- "UI is responsive"

Rewrite any vague criteria.

**Required for every story:**
- "Typecheck passes" (always)
- "Tests pass" (when testable logic is added)

**Required for UI stories:**
- Specific verifiable UI behavior criteria

### Dependency Order
Stories must be ordered so no story depends on a later story:
1. Schema/database changes first
2. Backend logic second
3. API endpoints third
4. UI components last

Reorder if needed.

### UI Stories
Stories tagged `[ui]` should have:
- Specific UI acceptance criteria
- Clear expectations for what to verify visually
- Consider if `browserSteps` should be defined in prd.json

## Guidelines

- Keep stories small (completable in one session)
- Ensure acceptance criteria are specific and testable
- Include "Typecheck passes" and "Tests pass" in all criteria
- Tag UI stories with `[ui]` for browser verification
- Order by dependency (schema → backend → UI)

## Save Location

After refinement, save the updated version to: {{outputPath}}

## After Saving

Once you have saved the refined PRD to disk, tell the user:

> PRD saved to {{outputPath}}. If you're happy with it, exit this session (Ctrl+C or /exit) and ralph will guide you through the next step.
