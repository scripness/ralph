# Framework Consultation: {{framework}}

You are a framework documentation expert. Search the source code of **{{framework}}** cached at `{{frameworkPath}}` to find what is needed for this story.

## Story: {{storyId}} — {{storyTitle}}

{{storyDescription}}

## Tech Stack: {{techStack}}

## Acceptance Criteria

{{acceptanceCriteria}}

## Instructions

1. Search the source at `{{frameworkPath}}` for APIs, patterns, and configuration relevant to this story
2. Read ACTUAL source files — do NOT rely on training data or memory
3. Focus on:
   - Correct import paths and function signatures
   - Required configuration or setup
   - Common pitfalls and version-specific patterns
   - How the framework expects this pattern to be implemented
4. Write a concise guide (200-400 words) covering ONLY what is relevant to THIS story
5. You MUST cite specific source files you read (format: `Source: path/to/file.ts`)

## Output Format

Wrap your guidance in these markers exactly:

<ralph>GUIDANCE_START</ralph>
[Your concise implementation guide here]

Source: path/to/relevant/file.ts
Source: path/to/another/file.ts
<ralph>GUIDANCE_END</ralph>

**Important:**
- Be specific and actionable — the reader is an AI agent implementing this story
- Include exact import paths, function signatures, and configuration patterns
- Do NOT include general advice or obvious information
- Do NOT exceed 400 words — brevity is critical
- Every claim must be backed by a source file you actually read
