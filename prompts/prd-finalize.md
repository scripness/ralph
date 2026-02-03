# PRD Finalization: {{feature}}

Convert this PRD to prd.json format for execution.

## PRD Content

{{prdContent}}

## Quality Gates (Check Before Converting)

### 1. Story Sizing
Each story must be completable in ONE implementation session.
- If a story is too large, **do not convert** — ask to split it first.
- Rule of thumb: if it takes more than 2-3 sentences to describe, it's too big.

### 2. Acceptance Criteria Verifiability
Every criterion must be specific and testable.
- **Reject** vague criteria like "works correctly" or "handles errors"
- **Require** specific observable outcomes

### 3. Dependency Order
Stories must be ordered so no story depends on a later story.
- Priority 1 stories cannot depend on Priority 2 stories
- Typical order: schema → backend → API → UI

## Output Format

Create a valid JSON file with this structure:

```json
{
  "schemaVersion": 2,
  "project": "[project name from PRD]",
  "branchName": "ralph/{{feature}}",
  "description": "[feature description from PRD]",
  "run": {
    "startedAt": null,
    "currentStoryId": null,
    "learnings": []
  },
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
      "priority": 1,
      "passes": false,
      "retries": 0,
      "blocked": false,
      "lastResult": null,
      "notes": ""
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
| `tags` | `["ui"]` for stories needing browser verification |
| `priority` | Integer, lower = higher priority (order of execution) |
| `passes` | `false` (CLI will update) |
| `retries` | `0` (CLI will update) |
| `blocked` | `false` (CLI will update) |
| `lastResult` | `null` (CLI will update) |
| `notes` | `""` (CLI will update) |
| `browserSteps` | Optional array of interactive browser verification steps |

## Browser Steps (for UI stories)

For UI stories, define interactive browser verification steps that the CLI will execute like a real user:

```json
"browserSteps": [
  {"action": "navigate", "url": "/login"},
  {"action": "type", "selector": "#email", "value": "test@example.com"},
  {"action": "type", "selector": "#password", "value": "password123"},
  {"action": "click", "selector": "button[type=submit]"},
  {"action": "waitFor", "selector": ".dashboard"},
  {"action": "assertText", "selector": "h1", "contains": "Welcome"},
  {"action": "screenshot"}
]
```

Available actions:
| Action | Fields | Description |
|--------|--------|-------------|
| `navigate` | `url` | Go to URL (relative or absolute) |
| `click` | `selector` | Click an element |
| `type` | `selector`, `value` | Type text into input |
| `waitFor` | `selector` | Wait for element visible |
| `assertVisible` | `selector` | Assert element exists |
| `assertText` | `selector`, `contains` | Assert element has text |
| `assertNotVisible` | `selector` | Assert element hidden |
| `submit` | `selector` | Click and wait for navigation |
| `screenshot` | - | Capture screenshot |
| `wait` | `timeout` | Wait N seconds |

All steps support optional `timeout` (seconds, default 10).

## Save Location

Save the JSON to: {{outputPath}}

## Validation Checklist

Before saving, verify:

- [ ] Valid JSON (no trailing commas, proper escaping)
- [ ] All stories from PRD are included
- [ ] Priorities match dependency order (schema → backend → UI)
- [ ] UI stories have `["ui"]` tag
- [ ] All acceptance criteria are specific and testable
- [ ] No story depends on a later story
- [ ] Stories are small enough for one implementation session
