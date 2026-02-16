# Fix Verification Failures

You are in an interactive session to investigate and fix verification failures for a feature.

## Project

**Name:** {{project}}
**Description:** {{description}}
**Branch:** {{branchName}}
**Progress:** {{progress}}
{{serviceURLs}}

## Verification Results

The following checks were run by the CLI. Fix the failures listed below.

```
{{verifyResults}}
```

{{storyDetails}}

{{learnings}}

{{diffSummary}}

## Verify Commands

{{verifyCommands}}

{{resourceVerificationInstructions}}

## Instructions

1. Read the failure output above carefully
2. Investigate the root cause of each failure
3. Fix the issue(s) in the implementation
4. Run the failing commands locally to confirm the fix:
   - Re-run each failed command and verify it passes
5. Commit your changes with a descriptive message (e.g., `fix: resolve failing tests`)

**Important:**
- Fix the implementation, not the tests (unless the tests themselves are wrong)
- All verify commands listed above should pass when you're done
- Update `{{knowledgeFile}}` if you discover new patterns or gotchas
- Feature directory: `{{featureDir}}`

After fixing, run `ralph verify` again to confirm all checks pass.
