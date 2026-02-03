# PRD Refinement: {{feature}}

## Current PRD

{{prdContent}}

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
Each story must be completable in ONE implementation session.
- **Too big**: "Build the entire dashboard" → Split into smaller pieces
- **Right size**: "Add user avatar to header", "Create login form validation"

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
