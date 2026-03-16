#!/bin/bash
# scrip v1 implementation loop
# Temporary — exists only to implement PHASE0-SPEC.md, then gets deleted.
#
# Usage:
#   ./loop.sh              # run until complete or max iterations
#   ./loop.sh 50           # run up to 50 iterations
#   ./loop.sh --dry-run    # show what would run without executing

set -euo pipefail

MAX_ITERATIONS="${1:-300}"
ITERATION=0
LEARNINGS_FILE="LEARNINGS.md"
COMPLETION_FILE="SCRIP_V1_COMPLETE"
LOG_DIR=".scrip-loop-logs"
BRANCH="scrip"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

if [ "${1:-}" = "--dry-run" ]; then
    echo "Would run: cat PROMPT.md | claude --print --dangerously-skip-permissions --model opus --effort max"
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

# Ensure PROMPT.md exists
if [ ! -f "PROMPT.md" ]; then
    echo -e "${RED}ERROR: PROMPT.md not found${NC}"
    exit 1
fi

# Ensure PHASE0-SPEC.md exists
if [ ! -f "PHASE0-SPEC.md" ]; then
    echo -e "${RED}ERROR: PHASE0-SPEC.md not found${NC}"
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
echo -e "${BLUE}║  scrip v1 implementation loop        ║${NC}"
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
        echo -e "${GREEN}✅ $COMPLETION_FILE found — implementation complete!${NC}"
        cat "$COMPLETION_FILE"
        echo ""
        echo -e "${GREEN}Total iterations: $((ITERATION - 1))${NC}"
        exit 0
    fi

    # Run the build prompt
    # Pipe PROMPT.md to claude, capture output, stream to both console and log
    if cat PROMPT.md | claude \
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
        echo "### Iteration $ITERATION ($TIMESTAMP)" >> "$LEARNINGS_FILE"
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

    # Quick mechanical check (informational, not blocking — the agent does its own)
    echo -n "  Build: "
    if go build ./... 2>/dev/null; then
        echo -e "${GREEN}PASS${NC}"
    else
        echo -e "${RED}FAIL${NC}"
    fi

    # Push after each iteration
    git push origin "$BRANCH" 2>/dev/null && echo -e "  ${BLUE}Pushed to origin/$BRANCH${NC}" || true

    # Check for completion signal AFTER running
    if [ -f "$COMPLETION_FILE" ]; then
        echo ""
        echo -e "${GREEN}✅ $COMPLETION_FILE found — implementation complete!${NC}"
        cat "$COMPLETION_FILE"
        echo ""
        echo -e "${GREEN}Total iterations: $ITERATION${NC}"
        exit 0
    fi

    echo ""
done

echo -e "${YELLOW}⚠ Hit max iterations ($MAX_ITERATIONS). Review progress and restart if needed.${NC}"
exit 1
