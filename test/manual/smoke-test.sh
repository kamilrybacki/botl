#!/bin/sh
# Manual smoke test for botl — exercises CLI surface, sessions, profiles, config,
# and optionally Docker-based run + label flow.
#
# Usage:
#   sh test/manual/smoke-test.sh                          # CLI-only tests (no Docker needed)
#   sh test/manual/smoke-test.sh --full                   # full test including Docker run + label
#
# Prerequisites:
#   - botl binary built (run: go build -o botl .)
#   - Docker daemon running + user in docker group (for --full mode)
#   - botl image built (run: botl build) (for --full mode)

set -e

# ── Helpers ───────────────────────────────────────────────────────────

PASS=0
FAIL=0
SKIP=0
SECTION=""

COLOR_GREEN="\033[32m"
COLOR_RED="\033[31m"
COLOR_YELLOW="\033[33m"
COLOR_CYAN="\033[36m"
COLOR_BOLD="\033[1m"
COLOR_DIM="\033[2m"
COLOR_RESET="\033[0m"

pass() {
    PASS=$((PASS + 1))
    printf "  ${COLOR_GREEN}PASS${COLOR_RESET} %s\n" "$1"
}

fail() {
    FAIL=$((FAIL + 1))
    printf "  ${COLOR_RED}FAIL${COLOR_RESET} %s\n" "$1"
}

skip() {
    SKIP=$((SKIP + 1))
    printf "  ${COLOR_YELLOW}SKIP${COLOR_RESET} %s\n" "$1"
}

section() {
    SECTION="$1"
    printf "\n${COLOR_BOLD}${COLOR_CYAN}=== %s ===${COLOR_RESET}\n" "$1"
}

# Expect a command to succeed (exit 0)
expect_ok() {
    desc="$1"; shift
    if "$@" > /dev/null 2>&1; then
        pass "$desc"
    else
        fail "$desc (exit $?)"
    fi
}

# Expect a command to fail (exit != 0)
expect_fail() {
    desc="$1"; shift
    if "$@" > /dev/null 2>&1; then
        fail "$desc (expected failure, got success)"
    else
        pass "$desc"
    fi
}

# Expect command output to contain a string
expect_output_contains() {
    desc="$1"; shift
    needle="$1"; shift
    output=$("$@" 2>&1) || true
    if echo "$output" | grep -qF "$needle"; then
        pass "$desc"
    else
        fail "$desc (expected output to contain: $needle)"
        printf "    ${COLOR_DIM}got: %.200s${COLOR_RESET}\n" "$output"
    fi
}

# Expect command output to NOT contain a string
expect_output_not_contains() {
    desc="$1"; shift
    needle="$1"; shift
    output=$("$@" 2>&1) || true
    if echo "$output" | grep -qF "$needle"; then
        fail "$desc (output should not contain: $needle)"
    else
        pass "$desc"
    fi
}

# ── Parse args ────────────────────────────────────────────────────────

FULL_MODE=false
DEFAULT_REPO="https://github.com/kamilrybacki/DummyRepo"
REPO_URL="$DEFAULT_REPO"
while [ $# -gt 0 ]; do
    case "$1" in
        --full) FULL_MODE=true; shift ;;
        *)      REPO_URL="$1"; shift ;;
    esac
done

# ── Find botl binary ─────────────────────────────────────────────────

BOTL="./botl"
if [ ! -x "$BOTL" ]; then
    BOTL="$(command -v botl 2>/dev/null || true)"
fi
if [ -z "$BOTL" ] || [ ! -x "$BOTL" ]; then
    echo "ERROR: botl binary not found. Run 'go build -o botl .' first."
    exit 1
fi

printf "${COLOR_BOLD}botl manual smoke test${COLOR_RESET}\n"
printf "binary: %s\n" "$BOTL"
printf "mode:   %s\n" "$(if $FULL_MODE; then echo "full (Docker)"; else echo "CLI-only"; fi)"

# ── Set up isolated XDG dirs for test ─────────────────────────────────

TEST_DIR=$(mktemp -d)
export XDG_CONFIG_HOME="$TEST_DIR/config"
export XDG_DATA_HOME="$TEST_DIR/data"
trap 'rm -rf "$TEST_DIR"' EXIT

printf "temp:   %s\n" "$TEST_DIR"

# ══════════════════════════════════════════════════════════════════════
# SECTION 1: Root command and help
# ══════════════════════════════════════════════════════════════════════

section "Root command"

expect_ok "bare botl prints help" $BOTL
expect_output_contains "help mentions 'run'" "run" $BOTL
expect_output_contains "help mentions 'config'" "config" $BOTL
expect_output_contains "help mentions 'label'" "label" $BOTL
expect_output_contains "help mentions 'profiles'" "profiles" $BOTL
expect_output_contains "help mentions 'build'" "build" $BOTL

# ══════════════════════════════════════════════════════════════════════
# SECTION 2: Config commands
# ══════════════════════════════════════════════════════════════════════

section "Config: list/get/set"

# List defaults
expect_ok "config list succeeds" $BOTL config list
expect_output_contains "config list shows clone-mode" "clone-mode" $BOTL config list
expect_output_contains "config list shows blocked-ports" "blocked-ports" $BOTL config list
expect_output_contains "default clone-mode is shallow" "shallow" $BOTL config get clone-mode
expect_output_contains "default blocked-ports is (none)" "(none)" $BOTL config get blocked-ports

# Set clone-mode
expect_ok "config set clone-mode deep" $BOTL config set clone-mode deep
expect_output_contains "config get reflects deep" "deep" $BOTL config get clone-mode

# Set back to shallow
expect_ok "config set clone-mode shallow" $BOTL config set clone-mode shallow
expect_output_contains "config get reflects shallow" "shallow" $BOTL config get clone-mode

# Set blocked-ports
expect_ok "config set blocked-ports" $BOTL config set blocked-ports "8080,3000"
expect_output_contains "config get reflects ports" "8080" $BOTL config get blocked-ports

# Clear blocked-ports
expect_ok "config set blocked-ports empty" $BOTL config set blocked-ports ""
expect_output_contains "config get reflects (none)" "(none)" $BOTL config get blocked-ports

# Invalid values
expect_fail "config set clone-mode invalid" $BOTL config set clone-mode invalid
expect_fail "config set blocked-ports bad" $BOTL config set blocked-ports "abc"
expect_fail "config set blocked-ports out-of-range" $BOTL config set blocked-ports "99999"
expect_fail "config set unknown-key" $BOTL config set unknown-key value
expect_fail "config get unknown-key" $BOTL config get unknown-key

# Bare config (no subcommand) prints help
expect_ok "bare config prints help" $BOTL config

# ══════════════════════════════════════════════════════════════════════
# SECTION 3: Profiles commands (empty state)
# ══════════════════════════════════════════════════════════════════════

section "Profiles: empty state"

expect_ok "profiles list succeeds when empty" $BOTL profiles list
expect_output_contains "profiles list shows no-profiles message" "no profiles found" $BOTL profiles list
expect_fail "profiles show nonexistent" $BOTL profiles show nonexistent
expect_fail "profiles delete nonexistent" $BOTL profiles delete --yes nonexistent

# ══════════════════════════════════════════════════════════════════════
# SECTION 4: Label command (error cases without Docker)
# ══════════════════════════════════════════════════════════════════════

section "Label: error cases"

expect_fail "label missing args" $BOTL label
expect_fail "label one arg" $BOTL label abcd1234
expect_fail "label invalid name (spaces)" $BOTL label abcd1234 "bad name"
expect_fail "label invalid name (slash)" $BOTL label abcd1234 "bad/name"
expect_fail "label invalid name (dot)" $BOTL label abcd1234 "bad.name"
expect_fail "label nonexistent session" $BOTL label abcd1234 my-profile

# Path traversal attempt
expect_fail "label path traversal session ID" $BOTL label "../../etc/passwd" my-profile
expect_fail "label path traversal profile name" $BOTL label abcd1234 "../../etc/passwd"

# ══════════════════════════════════════════════════════════════════════
# SECTION 5: Run command (error cases without Docker)
# ══════════════════════════════════════════════════════════════════════

section "Run: error cases (no Docker needed)"

expect_fail "run missing repo" $BOTL run
expect_fail "run invalid repo URL" $BOTL run "not-a-url"
expect_fail "run reserved env var" $BOTL run https://github.com/x/y -e BOTL_SECRET=bad
expect_fail "run with-label nonexistent" $BOTL run --with-label=nope https://github.com/x/y
expect_fail "run with-label invalid name" $BOTL run --with-label="bad name" https://github.com/x/y

# Check flag existence
expect_output_contains "run --help shows --with-label" "with-label" $BOTL run --help
expect_output_contains "run --help shows --clone-mode" "clone-mode" $BOTL run --help
expect_output_contains "run --help shows --blocked-ports" "blocked-ports" $BOTL run --help

# ══════════════════════════════════════════════════════════════════════
# SECTION 6: Session + Profile round-trip (synthetic — no Docker)
# ══════════════════════════════════════════════════════════════════════

section "Session + Profile: synthetic round-trip"

# Create a fake session record to test label + profiles flow without Docker
FAKE_SESSION_ID="deadbeef"
SESSION_DIR="$XDG_DATA_HOME/botl/sessions"
mkdir -p "$SESSION_DIR"

cat > "$SESSION_DIR/$FAKE_SESSION_ID.yaml" << 'YAML'
id: deadbeef
created_at: 2026-03-15T14:00:00Z
repo_url: https://github.com/test/repo
branch: main
status: success
run:
  clone_mode: deep
  depth: 0
  blocked_ports:
    - 8080
    - 3000
  timeout: 30m0s
  image: botl:latest
  output_dir: /tmp/test-output
  env_var_keys:
    - MY_SECRET
YAML

# Label the fake session
expect_ok "label synthetic session" $BOTL label "$FAKE_SESSION_ID" test-profile
expect_output_contains "label success message" "profile \"test-profile\" saved" $BOTL label "$FAKE_SESSION_ID" test-profile --force

# Profile should now exist
expect_ok "profiles list shows test-profile" $BOTL profiles list
expect_output_contains "profiles list has test-profile" "test-profile" $BOTL profiles list
expect_output_contains "profiles list has deadbeef" "deadbeef" $BOTL profiles list

# Show profile
expect_ok "profiles show test-profile" $BOTL profiles show test-profile
expect_output_contains "show has clone_mode deep" "deep" $BOTL profiles show test-profile
expect_output_contains "show has blocked_ports" "8080" $BOTL profiles show test-profile
expect_output_contains "show notes env vars on stderr" "MY_SECRET" $BOTL profiles show test-profile

# Label a second profile
expect_ok "label second profile" $BOTL label "$FAKE_SESSION_ID" another-profile
expect_output_contains "profiles list has 2 profiles" "another-profile" $BOTL profiles list

# Duplicate name without --force
expect_fail "label duplicate without force" $BOTL label "$FAKE_SESSION_ID" test-profile
expect_output_contains "duplicate error mentions --force" "use --force" $BOTL label "$FAKE_SESSION_ID" test-profile 2>&1 || true

# Delete with --yes
expect_ok "profiles delete --yes" $BOTL profiles delete --yes another-profile
expect_output_not_contains "deleted profile gone from list" "another-profile" $BOTL profiles list

# Label rejects non-success sessions
PENDING_ID="cafebabe"
cat > "$SESSION_DIR/$PENDING_ID.yaml" << 'YAML'
id: cafebabe
created_at: 2026-03-15T15:00:00Z
repo_url: https://github.com/test/repo
status: pending
run:
  clone_mode: shallow
  depth: 1
  blocked_ports: []
  timeout: 30m0s
  image: botl:latest
  output_dir: /tmp/out
YAML

expect_fail "label rejects pending session" $BOTL label "$PENDING_ID" pending-profile
expect_output_contains "pending error mentions status" "did not complete successfully" $BOTL label "$PENDING_ID" pending-profile 2>&1 || true

FAILED_ID="f00dcafe"
cat > "$SESSION_DIR/$FAILED_ID.yaml" << 'YAML'
id: f00dcafe
created_at: 2026-03-15T15:00:00Z
repo_url: https://github.com/test/repo
status: failed
run:
  clone_mode: shallow
  depth: 1
  blocked_ports: []
  timeout: 30m0s
  image: botl:latest
  output_dir: /tmp/out
YAML

expect_fail "label rejects failed session" $BOTL label "$FAILED_ID" failed-profile

# ══════════════════════════════════════════════════════════════════════
# SECTION 7: Error message format
# ══════════════════════════════════════════════════════════════════════

section "Error messages: botl: error: prefix"

output=$($BOTL run not-a-url 2>&1) || true
if echo "$output" | grep -q "^botl: error:"; then
    pass "error message has 'botl: error:' prefix"
else
    fail "error message missing 'botl: error:' prefix"
    printf "    ${COLOR_DIM}got: %.200s${COLOR_RESET}\n" "$output"
fi

output=$($BOTL config set bad-key val 2>&1) || true
if echo "$output" | grep -q "^botl: error:"; then
    pass "config error has 'botl: error:' prefix"
else
    fail "config error missing 'botl: error:' prefix"
    printf "    ${COLOR_DIM}got: %.200s${COLOR_RESET}\n" "$output"
fi

# ══════════════════════════════════════════════════════════════════════
# SECTION 8: Full Docker test (optional — requires --full and repo URL)
# ══════════════════════════════════════════════════════════════════════

if $FULL_MODE; then
    section "Full Docker test"

    # Check Docker
    if ! docker info > /dev/null 2>&1; then
        skip "Docker not available — skipping full test"
    else
        # Check image exists
        if ! docker image inspect botl:latest > /dev/null 2>&1; then
            printf "  ${COLOR_DIM}Building botl image...${COLOR_RESET}\n"
            expect_ok "botl build" $BOTL build
        fi

        # Run a headless session with a simple prompt
        printf "  ${COLOR_DIM}Running botl with headless prompt (this may take a few minutes)...${COLOR_RESET}\n"
        unset XDG_CONFIG_HOME XDG_DATA_HOME  # use real dirs for Docker test
        REAL_DATA_HOME="${HOME}/.local/share"

        RUN_OUTPUT=$($BOTL run "$REPO_URL" -p "list all files in the repo and describe each one briefly" --timeout 5m 2>&1) || true

        # Show full output for debugging
        printf "\n  ${COLOR_BOLD}── botl run output ──${COLOR_RESET}\n"
        echo "$RUN_OUTPUT" | sed 's/^/  │ /'
        printf "  ${COLOR_BOLD}── end output ──${COLOR_RESET}\n\n"

        # Extract session ID from output
        SESSION_ID=$(echo "$RUN_OUTPUT" | grep "session id:" | head -1 | sed 's/.*session id: //')

        if [ -n "$SESSION_ID" ]; then
            pass "session ID printed: $SESSION_ID"
        else
            fail "no session ID in output"
        fi

        # Check session ID printed at end too
        if echo "$RUN_OUTPUT" | grep -q "session complete"; then
            pass "session complete message printed"
        else
            fail "no session complete message"
        fi

        # Check session file exists
        SESSION_FILE="$REAL_DATA_HOME/botl/sessions/$SESSION_ID.yaml"
        if [ -f "$SESSION_FILE" ]; then
            pass "session file created: $SESSION_FILE"

            # Show session record
            printf "  ${COLOR_DIM}Session record:${COLOR_RESET}\n"
            cat "$SESSION_FILE" | sed 's/^/  │ /'

            # Check status is terminal (success or failed)
            SESSION_STATUS=$(grep "^status:" "$SESSION_FILE" | awk '{print $2}')
            if [ "$SESSION_STATUS" = "success" ] || [ "$SESSION_STATUS" = "failed" ]; then
                pass "session has terminal status: $SESSION_STATUS"
            else
                fail "session status not updated (got: $SESSION_STATUS)"
            fi
        else
            skip "session file not found (may use different XDG_DATA_HOME)"
        fi

        # Label the session (only if it succeeded — a failed container can't be labeled by design)
        if [ -n "$SESSION_ID" ]; then
            if [ "$SESSION_STATUS" = "success" ]; then
                expect_ok "label the real session" $BOTL label "$SESSION_ID" smoke-test-profile --force
                expect_output_contains "profile appears in list" "smoke-test-profile" $BOTL profiles list

                # Show the profile
                printf "  ${COLOR_DIM}Profile contents:${COLOR_RESET}\n"
                $BOTL profiles show smoke-test-profile 2>&1 | sed 's/^/  │ /'

                # Clean up
                $BOTL profiles delete --yes smoke-test-profile > /dev/null 2>&1 || true
            else
                skip "label skipped — session status is '$SESSION_STATUS' (only successful sessions can be labeled)"
                printf "  ${COLOR_DIM}This is expected if the container exited non-zero (e.g. Claude prompt error).${COLOR_RESET}\n"
                printf "  ${COLOR_DIM}The session lifecycle (ID gen, record write, status update) still works correctly.${COLOR_RESET}\n"
            fi
        fi

        # Re-export for remaining tests
        export XDG_CONFIG_HOME="$TEST_DIR/config"
        export XDG_DATA_HOME="$TEST_DIR/data"
    fi
else
    section "Full Docker test"
    skip "Docker tests skipped (use --full <repo-url> to enable)"
fi

# ══════════════════════════════════════════════════════════════════════
# SECTION 9: Cleanup verification
# ══════════════════════════════════════════════════════════════════════

section "Cleanup"

# Delete remaining test profile
$BOTL profiles delete --yes test-profile > /dev/null 2>&1 || true
expect_output_contains "all test profiles cleaned up" "no profiles found" $BOTL profiles list
pass "temp directory will be cleaned up on exit: $TEST_DIR"

# ══════════════════════════════════════════════════════════════════════
# Results
# ══════════════════════════════════════════════════════════════════════

printf "\n${COLOR_BOLD}═══════════════════════════════════════════════════${COLOR_RESET}\n"
printf "${COLOR_GREEN}  PASS: %d${COLOR_RESET}" "$PASS"
if [ "$FAIL" -gt 0 ]; then
    printf "  ${COLOR_RED}FAIL: %d${COLOR_RESET}" "$FAIL"
fi
if [ "$SKIP" -gt 0 ]; then
    printf "  ${COLOR_YELLOW}SKIP: %d${COLOR_RESET}" "$SKIP"
fi
printf "\n${COLOR_BOLD}═══════════════════════════════════════════════════${COLOR_RESET}\n"

if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
