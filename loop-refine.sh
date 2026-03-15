#!/bin/bash
# scrip v1 refinement loop
# Runs PHASE0-SPEC-refine-01.md tasks via PROMPT-refine.md
#
# Usage:
#   ./loop-refine.sh              # run until complete or max iterations
#   ./loop-refine.sh 50           # run up to 50 iterations
#   ./loop-refine.sh --dry-run    # show what would run without executing

set -euo pipefail

MAX_ITERATIONS="${1:-100}"
ITERATION=0
LEARNINGS_FILE="LEARNINGS.md"
COMPLETION_FILE="REFINE_01_COMPLETE"
PROMPT_FILE="PROMPT-refine.md"
SPEC_FILE="PHASE0-SPEC-refine-01.md"
LOG_DIR=".scrip-loop-logs/refine-01"
BRANCH="scrip"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

if [ "${1:-}" = "--dry-run" ]; then
    echo "Would run: cat $PROMPT_FILE | claude --print --dangerously-skip-permissions --model opus --effort max"
    echo "Spec: $SPEC_FILE"
    echo "Max iterations: $MAX_ITERATIONS"
    echo "Learnings file: $LEARNINGS_FILE"
    echo "Completion signal: $COMPLETION_FILE"
    exit 0
fi

# Ensure we're on the right branch
CURRENT_BRANCH=$(git branch --show-current)
if [ "$CURRENT_BRANCH" != "$BRANCH" ]; then
    echo -e "${RED}ERROR: Not on branch '$BRANCH' (on '$CURRENT_BRANCH')${NC}"
    echo "Run: git checkout $BRANCH"
    exit 1
fi

# Ensure prompt and spec exist
if [ ! -f "$PROMPT_FILE" ]; then
    echo -e "${RED}ERROR: $PROMPT_FILE not found${NC}"
    exit 1
fi

if [ ! -f "$SPEC_FILE" ]; then
    echo -e "${RED}ERROR: $SPEC_FILE not found${NC}"
    exit 1
fi

# Create log directory
mkdir -p "$LOG_DIR"

# Initialize learnings file if it doesn't exist
if [ ! -f "$LEARNINGS_FILE" ]; then
    echo "# Learnings" > "$LEARNINGS_FILE"
    echo "" >> "$LEARNINGS_FILE"
    echo "Accumulated insights from implementation iterations." >> "$LEARNINGS_FILE"
    echo "" >> "$LEARNINGS_FILE"
fi

echo -e "${BLUE}╔══════════════════════════════════════╗${NC}"
echo -e "${BLUE}║  scrip v1 refinement loop (round 01) ║${NC}"
echo -e "${BLUE}║  Max iterations: $(printf '%-19s' "$MAX_ITERATIONS")║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════╝${NC}"
echo ""

while [ "$ITERATION" -lt "$MAX_ITERATIONS" ]; do
    ITERATION=$((ITERATION + 1))
    TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
    LOG_FILE="$LOG_DIR/iteration-$(printf '%03d' $ITERATION).log"

    echo -e "${BLUE}━━━ Iteration $ITERATION/$MAX_ITERATIONS [$TIMESTAMP] ━━━${NC}"

    # Check for completion signal BEFORE running
    if [ -f "$COMPLETION_FILE" ]; then
        echo -e "${GREEN}✅ $COMPLETION_FILE found — refinement complete!${NC}"
        cat "$COMPLETION_FILE"
        echo ""
        echo -e "${GREEN}Total iterations: $((ITERATION - 1))${NC}"
        exit 0
    fi

    # Run the refinement prompt
    if cat "$PROMPT_FILE" | claude \
        --print \
        --dangerously-skip-permissions \
        --model opus \
        --effort max \
        2>&1 | tee "$LOG_FILE"; then
        echo -e "${GREEN}  Agent exited cleanly${NC}"
    else
        EXIT_CODE=$?
        echo -e "${YELLOW}  Agent exited with code $EXIT_CODE${NC}"
    fi

    # Extract learnings from output and append to LEARNINGS.md
    if grep -q '<scrip>LEARNING:' "$LOG_FILE" 2>/dev/null; then
        echo "" >> "$LEARNINGS_FILE"
        echo "### Refine-01 Iteration $ITERATION ($TIMESTAMP)" >> "$LEARNINGS_FILE"
        grep '<scrip>LEARNING:' "$LOG_FILE" | sed 's/.*<scrip>LEARNING:\(.*\)<\/scrip>/- \1/' >> "$LEARNINGS_FILE"
        echo -e "${YELLOW}  📝 Learnings captured${NC}"
    fi

    # Check markers
    if grep -q '<scrip>DONE</scrip>' "$LOG_FILE" 2>/dev/null; then
        echo -e "${GREEN}  ✓ DONE marker detected${NC}"
    elif grep -q '<scrip>STUCK:' "$LOG_FILE" 2>/dev/null; then
        REASON=$(grep '<scrip>STUCK:' "$LOG_FILE" | head -1 | sed 's/.*<scrip>STUCK:\(.*\)<\/scrip>/\1/')
        echo -e "${RED}  ✗ STUCK: $REASON${NC}"
    else
        echo -e "${YELLOW}  ⚠ No marker detected${NC}"
    fi

    # Quick mechanical check
    echo -n "  Build: "
    if go build ./... 2>/dev/null; then
        echo -e "${GREEN}PASS${NC}"
    else
        echo -e "${RED}FAIL${NC}"
    fi

    echo -n "  Tests: "
    if go test ./... 2>/dev/null; then
        echo -e "${GREEN}PASS${NC}"
    else
        echo -e "${RED}FAIL${NC}"
    fi

    # Push after each iteration
    git push origin "$BRANCH" 2>/dev/null && echo -e "  ${BLUE}Pushed to origin/$BRANCH${NC}" || true

    # Check for completion signal AFTER running
    if [ -f "$COMPLETION_FILE" ]; then
        echo ""
        echo -e "${GREEN}✅ $COMPLETION_FILE found — refinement complete!${NC}"
        cat "$COMPLETION_FILE"
        echo ""
        echo -e "${GREEN}Total iterations: $ITERATION${NC}"
        exit 0
    fi

    echo ""
done

echo -e "${YELLOW}⚠ Hit max iterations ($MAX_ITERATIONS). Review progress and restart if needed.${NC}"
exit 1
