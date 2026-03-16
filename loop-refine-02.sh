#!/bin/bash
# scrip v1 refinement loop (round 02)
# Runs PHASE0-SPEC-refine-02.md tasks via PROMPT-refine-02.md
#
# Usage:
#   ./loop-refine-02.sh              # run until complete or max iterations
#   ./loop-refine-02.sh 20           # run up to 20 iterations
#   ./loop-refine-02.sh --dry-run    # show what would run

set -euo pipefail

MAX_ITERATIONS="${1:-30}"
ITERATION=0
LEARNINGS_FILE="LEARNINGS.md"
COMPLETION_FILE="REFINE_02_COMPLETE"
PROMPT_FILE="PROMPT-refine-02.md"
SPEC_FILE="PHASE0-SPEC-refine-02.md"
LOG_DIR=".scrip-loop-logs/refine-02"
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

if [ ! -f "$PROMPT_FILE" ]; then
    echo -e "${RED}ERROR: $PROMPT_FILE not found${NC}"
    exit 1
fi

if [ ! -f "$SPEC_FILE" ]; then
    echo -e "${RED}ERROR: $SPEC_FILE not found${NC}"
    exit 1
fi

mkdir -p "$LOG_DIR"

if [ ! -f "$LEARNINGS_FILE" ]; then
    echo "# Learnings" > "$LEARNINGS_FILE"
    echo "" >> "$LEARNINGS_FILE"
fi

echo -e "${BLUE}╔══════════════════════════════════════╗${NC}"
echo -e "${BLUE}║  scrip v1 refinement loop (round 02) ║${NC}"
echo -e "${BLUE}║  Tasks: 6  Max iter: $(printf '%-15s' "$MAX_ITERATIONS")║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════╝${NC}"
echo ""

while [ "$ITERATION" -lt "$MAX_ITERATIONS" ]; do
    ITERATION=$((ITERATION + 1))
    TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
    LOG_FILE="$LOG_DIR/iteration-$(printf '%03d' $ITERATION).log"

    echo -e "${BLUE}━━━ Iteration $ITERATION/$MAX_ITERATIONS [$TIMESTAMP] ━━━${NC}"

    if [ -f "$COMPLETION_FILE" ]; then
        echo -e "${GREEN}✅ $COMPLETION_FILE found — refinement complete!${NC}"
        cat "$COMPLETION_FILE"
        echo -e "${GREEN}Total iterations: $((ITERATION - 1))${NC}"
        exit 0
    fi

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

    if grep -q '<scrip>LEARNING:' "$LOG_FILE" 2>/dev/null; then
        echo "" >> "$LEARNINGS_FILE"
        echo "### Refine-02 Iteration $ITERATION ($TIMESTAMP)" >> "$LEARNINGS_FILE"
        grep '<scrip>LEARNING:' "$LOG_FILE" | sed 's/.*<scrip>LEARNING:\(.*\)<\/scrip>/- \1/' >> "$LEARNINGS_FILE"
        echo -e "${YELLOW}  📝 Learnings captured${NC}"
    fi

    if grep -q '<scrip>DONE</scrip>' "$LOG_FILE" 2>/dev/null; then
        echo -e "${GREEN}  ✓ DONE${NC}"
    elif grep -q '<scrip>STUCK:' "$LOG_FILE" 2>/dev/null; then
        REASON=$(grep '<scrip>STUCK:' "$LOG_FILE" | head -1 | sed 's/.*<scrip>STUCK:\(.*\)<\/scrip>/\1/')
        echo -e "${RED}  ✗ STUCK: $REASON${NC}"
    else
        echo -e "${YELLOW}  ⚠ No marker${NC}"
    fi

    echo -n "  Build: "
    if go build ./... 2>/dev/null; then echo -e "${GREEN}PASS${NC}"; else echo -e "${RED}FAIL${NC}"; fi

    echo -n "  Tests: "
    if go test ./... 2>/dev/null; then echo -e "${GREEN}PASS${NC}"; else echo -e "${RED}FAIL${NC}"; fi

    git push origin "$BRANCH" 2>/dev/null && echo -e "  ${BLUE}Pushed${NC}" || true

    if [ -f "$COMPLETION_FILE" ]; then
        echo ""
        echo -e "${GREEN}✅ $COMPLETION_FILE found — refinement complete!${NC}"
        cat "$COMPLETION_FILE"
        echo -e "${GREEN}Total iterations: $ITERATION${NC}"
        exit 0
    fi

    echo ""
done

echo -e "${YELLOW}⚠ Hit max iterations ($MAX_ITERATIONS).${NC}"
exit 1
