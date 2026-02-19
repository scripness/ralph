# PRD Finalization: {{feature}}

Convert this PRD to prd.json format for execution.

{{resourceGuidance}}

## PRD Content

{{prdContent}}

## Quality Gates (Check Before Converting)

### 1. Story Sizing
Each story must be completable in ONE implementation session (one AI context window). The implementing agent starts fresh each iteration with no memory. If a story is too big, the agent runs out of context and produces broken code.
- If a story is too large, **do not convert** — ask to split it first.
- Rule of thumb: if you cannot describe the change in 2-3 sentences, it is too big.

**Right-sized:** "Add priority column and migration", "Display status badge on task cards"
**Too big:** "Build the entire dashboard" → split into schema, queries, UI components, filters

### 2. Acceptance Criteria Verifiability
Every criterion must be specific and testable.
- **Reject** vague criteria like "works correctly" or "handles errors"
- **Require** specific observable outcomes

### 3. Dependency Order
Stories must be ordered so no story depends on a later story.
- Priority 1 stories cannot depend on Priority 2 stories
- Typical order: schema → backend → API → UI

## Output Format

Create a valid JSON file with this structure. **Only include definition fields — no runtime state.** The CLI manages execution state separately in run-state.json.

```json
{
  "schemaVersion": 3,
  "project": "[project name from PRD]",
  "branchName": "ralph/{{feature}}",
  "description": "[feature description from PRD]",
  "userStories": [
    {
      "id": "US-001",
      "title": "Story title",
      "description": "As a user, I want...",
      "acceptanceCriteria": [
        "Specific criterion 1",
        "Specific criterion 2",
        "Typecheck passes",
        "Tests pass"
      ],
      "tags": [],
      "priority": 1
    }
  ]
}
```

## Story Fields

| Field | Description |
|-------|-------------|
| `id` | US-001, US-002, etc. |
| `title` | Short story title |
| `description` | Full user story description |
| `acceptanceCriteria` | Array of specific, testable criteria |
| `tags` | `["ui"]` for stories needing e2e test verification |
| `priority` | Integer, lower = higher priority (order of execution) |

## UI Stories and E2E Tests

For UI stories (tagged `["ui"]`), write acceptance criteria that are testable via e2e tests. The implementing agent will write e2e tests based on these criteria using the project's existing e2e testing framework (e.g., Playwright, Cypress).

**Good acceptance criteria for UI stories:**
- "Search results page shows matching certificates when a valid certificate number is entered"
- "Login form displays error message when password is incorrect"
- "Dashboard shows user's name in the header after login"

**Bad acceptance criteria:**
- "UI looks correct" (too vague for automated testing)

## Save Location

Save the JSON to: {{outputPath}}

## Validation Checklist

Before saving, verify:

- [ ] Valid JSON (no trailing commas, proper escaping)
- [ ] All stories from PRD are included
- [ ] Priorities match dependency order (schema → backend → UI)
- [ ] UI stories have `["ui"]` tag
- [ ] All acceptance criteria are specific and testable
- [ ] Every story has "Typecheck passes" as a criterion
- [ ] Stories with testable logic have "Tests pass" as a criterion
- [ ] No story depends on a later story
- [ ] Stories are small enough for one implementation session
- [ ] **No runtime fields** (passes, retries, blocked, lastResult, notes, run) — these belong in run-state.json

## After Saving

Once you have saved prd.json to disk, tell the user:

> prd.json saved to {{outputPath}}. If you're happy with it, exit this session (Ctrl+C or /exit) and ralph will validate the JSON and guide you to the next step.
