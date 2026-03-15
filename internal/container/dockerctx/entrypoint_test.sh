#!/bin/sh
# Tests for entrypoint.sh logic — run with: sh entrypoint_test.sh
# These are shell-level tests that verify the script's behavior.

set -e

PASS=0
FAIL=0
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

pass() {
    PASS=$((PASS + 1))
    echo "  PASS: $1"
}

fail() {
    FAIL=$((FAIL + 1))
    echo "  FAIL: $1"
}

echo "=== entrypoint.sh tests ==="

# Test 1: Script exists and is readable
if [ -f "$SCRIPT_DIR/entrypoint.sh" ]; then
    pass "entrypoint.sh exists"
else
    fail "entrypoint.sh not found"
fi

# Test 2: Script starts with shebang
FIRST_LINE=$(head -1 "$SCRIPT_DIR/entrypoint.sh")
if echo "$FIRST_LINE" | grep -q '^#!/bin/sh'; then
    pass "has /bin/sh shebang"
else
    fail "missing /bin/sh shebang (got: $FIRST_LINE)"
fi

# Test 3: set -e is present (fail-fast)
if grep -q 'set -e' "$SCRIPT_DIR/entrypoint.sh"; then
    pass "has set -e"
else
    fail "missing set -e"
fi

# Test 4: Checks for BOTL_REPO_URL
if grep -q 'BOTL_REPO_URL' "$SCRIPT_DIR/entrypoint.sh"; then
    pass "references BOTL_REPO_URL"
else
    fail "does not reference BOTL_REPO_URL"
fi

# Test 5: Validates REPO_URL is non-empty
if grep -q '[ -z "$REPO_URL" ]' "$SCRIPT_DIR/entrypoint.sh" || grep -q '"\$REPO_URL"' "$SCRIPT_DIR/entrypoint.sh"; then
    pass "validates REPO_URL"
else
    fail "does not validate REPO_URL"
fi

# Test 6: Clones to /workspace/repo
if grep -q '/workspace/repo' "$SCRIPT_DIR/entrypoint.sh"; then
    pass "clones to /workspace/repo"
else
    fail "does not clone to /workspace/repo"
fi

# Test 7: Configures git user
if grep -q 'git config user' "$SCRIPT_DIR/entrypoint.sh"; then
    pass "configures git user"
else
    fail "does not configure git user"
fi

# Test 8: Records BOTL_INITIAL_HEAD
if grep -q 'BOTL_INITIAL_HEAD' "$SCRIPT_DIR/entrypoint.sh"; then
    pass "records BOTL_INITIAL_HEAD"
else
    fail "does not record BOTL_INITIAL_HEAD"
fi

# Test 9: Runs claude with --dangerously-skip-permissions
if grep -q 'claude --dangerously-skip-permissions' "$SCRIPT_DIR/entrypoint.sh"; then
    pass "runs claude with --dangerously-skip-permissions"
else
    fail "does not run claude with --dangerously-skip-permissions"
fi

# Test 10: Handles headless mode (PROMPT check)
if grep -q 'PROMPT' "$SCRIPT_DIR/entrypoint.sh"; then
    pass "handles PROMPT for headless mode"
else
    fail "does not handle PROMPT"
fi

# Test 11: Invokes botl-postrun at the end
if grep -q 'botl-postrun' "$SCRIPT_DIR/entrypoint.sh"; then
    pass "invokes botl-postrun"
else
    fail "does not invoke botl-postrun"
fi

# Test 12: Uses exec for postrun (replaces shell process)
if grep -q 'exec gosu botl botl-postrun' "$SCRIPT_DIR/entrypoint.sh"; then
    pass "uses exec for botl-postrun"
else
    fail "does not use exec for botl-postrun"
fi

# Test 13: Uses depth flag
if grep -q 'DEPTH' "$SCRIPT_DIR/entrypoint.sh"; then
    pass "uses DEPTH variable"
else
    fail "does not use DEPTH variable"
fi

# Test 14: Default depth is 1
if grep -q 'BOTL_DEPTH:-1' "$SCRIPT_DIR/entrypoint.sh"; then
    pass "defaults depth to 1"
else
    fail "does not default depth to 1"
fi

# Test 15: Validates REPO_URL protocol (https, git@, ssh only — no insecure git://)
if grep -q 'https://' "$SCRIPT_DIR/entrypoint.sh" && grep -q 'git@' "$SCRIPT_DIR/entrypoint.sh" && ! grep -q 'git://' "$SCRIPT_DIR/entrypoint.sh"; then
    pass "validates URL protocol"
else
    fail "does not validate URL protocol"
fi

# Test 16: No ANTHROPIC_API_KEY reference (removed for OAuth)
if grep -q 'ANTHROPIC_API_KEY' "$SCRIPT_DIR/entrypoint.sh"; then
    fail "still references ANTHROPIC_API_KEY (should use OAuth)"
else
    pass "no ANTHROPIC_API_KEY reference (uses OAuth)"
fi

# Test 17: References BOTL_SANITIZE_GIT
if grep -q 'BOTL_SANITIZE_GIT' "$SCRIPT_DIR/entrypoint.sh"; then
    pass "references BOTL_SANITIZE_GIT"
else
    fail "does not reference BOTL_SANITIZE_GIT"
fi

# Test 18: References BOTL_BLOCKED_PORTS
if grep -q 'BOTL_BLOCKED_PORTS' "$SCRIPT_DIR/entrypoint.sh"; then
    pass "references BOTL_BLOCKED_PORTS"
else
    fail "does not reference BOTL_BLOCKED_PORTS"
fi

# Test 19: Uses gosu for privilege dropping
if grep -q 'gosu' "$SCRIPT_DIR/entrypoint.sh"; then
    pass "uses gosu for privilege dropping"
else
    fail "does not use gosu"
fi

# Test 20: Handles deep clone (DEPTH -eq 0)
if grep -q 'DEPTH.*-eq 0' "$SCRIPT_DIR/entrypoint.sh"; then
    pass "handles deep clone when DEPTH is 0"
else
    fail "does not handle deep clone (DEPTH -eq 0)"
fi

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="

if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
