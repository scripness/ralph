# PRD Creation: {{feature}}

Help create a Product Requirements Document for this feature.

## Clarifying Questions

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

Wait for user answers before proceeding.

## Generate PRD

After receiving answers, create a detailed PRD with these sections:

### 1. Introduction/Overview
Brief description of the feature and its purpose.

### 2. Goals
- Primary goals
- Success metrics

### 3. User Stories
Each story should be:
- Small enough to complete in ONE implementation session
- Ordered by dependency (schema → backend → UI)
- Include specific acceptance criteria

Format each story as:
```
#### US-XXX: [Title]
**Description:** As a [user], I want [action] so that [benefit].

**Acceptance Criteria:**
- [ ] Criterion 1
- [ ] Criterion 2
- [ ] Typecheck passes
- [ ] Tests pass

**Tags:** [ui] (if browser verification needed)
**Priority:** X
```

### 4. Non-Goals
What this feature explicitly does NOT include.

### 5. Technical Considerations
- Patterns to follow
- Files/modules affected
- Dependencies

## Save Location

Save the PRD to: {{outputPath}}
