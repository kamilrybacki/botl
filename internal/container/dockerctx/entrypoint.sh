#!/bin/sh
set -e

REPO_URL="${BOTL_REPO_URL}"
BRANCH="${BOTL_BRANCH}"
DEPTH="${BOTL_DEPTH:-1}"
PROMPT="${BOTL_PROMPT}"
SANITIZE_GIT="${BOTL_SANITIZE_GIT}"
BLOCKED_PORTS="${BOTL_BLOCKED_PORTS}"

# --- Fix Claude credential permissions ---
# Host mounts arrive with host UID (0600), which the botl user can't read.
# Running as root: copy to a writable HOME owned by botl so Claude Code
# finds ~/.claude and ~/.claude.json at the expected paths.
BOTL_HOME="/tmp/botl-home"
mkdir -p "$BOTL_HOME"
if [ -d /home/botl/.claude ]; then
    cp -a /home/botl/.claude "$BOTL_HOME/.claude"
fi
if [ -f /home/botl/.claude.json ]; then
    cp /home/botl/.claude.json "$BOTL_HOME/.claude.json"
fi
chown -R botl:botl "$BOTL_HOME"
export HOME="$BOTL_HOME"

if [ -z "$REPO_URL" ]; then
    echo "botl: error: BOTL_REPO_URL is not set" >&2
    exit 1
fi

# Validate REPO_URL looks like a git URL (https, ssh, or git protocol)
case "$REPO_URL" in
    https://*|git@*|ssh://*)
        ;;
    *)
        echo "botl: error: BOTL_REPO_URL must be an https://, git@, or ssh:// URL" >&2
        exit 1
        ;;
esac

# --- Port blocking (requires root, done before dropping privileges) ---
if [ -n "$BLOCKED_PORTS" ]; then
    blocked_count=0
    for port in $(echo "$BLOCKED_PORTS" | tr ',' ' '); do
        if echo "$port" | grep -qE '^[0-9]+$' && [ "$port" -ge 1 ] && [ "$port" -le 65535 ]; then
            if iptables -A INPUT -p tcp --dport "$port" -j REJECT 2>&1; then
                blocked_count=$((blocked_count + 1))
            else
                echo "botl: warning: failed to block port $port" >&2
            fi
        else
            echo "botl: warning: invalid port '$port', skipping" >&2
        fi
    done
    if [ "$blocked_count" -gt 0 ]; then
        echo "botl: blocked $blocked_count inbound port(s)" >&2
    fi
fi

# --- Clone repository (as botl user) ---
echo "botl: cloning ${REPO_URL} (depth=${DEPTH}, branch=${BRANCH:-default})..." >&2
if [ "$DEPTH" -eq 0 ] 2>/dev/null; then
    # Deep clone: omit --depth flag
    if [ -n "$BRANCH" ]; then
        gosu botl git clone --branch "${BRANCH}" "${REPO_URL}" /workspace/repo
    else
        gosu botl git clone "${REPO_URL}" /workspace/repo
    fi
else
    if [ -n "$BRANCH" ]; then
        gosu botl git clone --depth "${DEPTH}" --branch "${BRANCH}" "${REPO_URL}" /workspace/repo
    else
        gosu botl git clone --depth "${DEPTH}" "${REPO_URL}" /workspace/repo
    fi
fi
cd /workspace/repo

# Configure git for commits inside container
gosu botl git config user.email "botl@container"
gosu botl git config user.name "botl"

# Record the initial HEAD so postrun can detect new commits
INITIAL_HEAD="$(gosu botl git rev-parse HEAD 2>/dev/null || echo '')"

# --- Git sanitization (shallow mode) ---
if [ "$SANITIZE_GIT" = "true" ]; then
    echo "botl: sanitizing git history..." >&2
    gosu botl git commit --amend --allow-empty --no-gpg-sign -m "initial" 2>/dev/null || true
    gosu botl git reflog expire --expire=now --all 2>/dev/null || true
    gosu botl git gc --prune=now 2>/dev/null || true
    gosu botl git notes remove HEAD 2>/dev/null || true
    # Re-record HEAD after sanitization
    INITIAL_HEAD="$(gosu botl git rev-parse HEAD 2>/dev/null || echo '')"
fi

export BOTL_INITIAL_HEAD="$INITIAL_HEAD"

# --- Launch Claude Code (as botl user) ---
if [ -n "$PROMPT" ]; then
    gosu botl claude --dangerously-skip-permissions -p "$PROMPT"
else
    gosu botl claude --dangerously-skip-permissions
fi

# Run the post-session TUI to handle results
exec gosu botl botl-postrun
