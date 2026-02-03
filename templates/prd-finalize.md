# PRD Finalization: {{feature}}

Convert this PRD to prd.json format:

## PRD Content

{{prdContent}}

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
        "Criterion 1",
        "Criterion 2",
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
| `passes` | `false` (Ralph will update) |
| `retries` | `0` (Ralph will update) |
| `blocked` | `false` (Ralph will update) |
| `lastResult` | `null` (Ralph will update) |
| `notes` | `""` (Ralph will update) |

## Save Location

Save the JSON to: {{outputPath}}

## Important

- Ensure valid JSON (no trailing commas, proper escaping)
- Include ALL stories from the PRD
- Set priorities based on dependency order (schema → backend → UI)
- Tag UI stories with `["ui"]`
- All other stories should have empty tags: `[]`
