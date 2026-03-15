#!/bin/sh
set -e

REPO_URL="${BOTL_REPO_URL}"
BRANCH="${BOTL_BRANCH}"
DEPTH="${BOTL_DEPTH:-1}"
PROMPT="${BOTL_PROMPT}"

if [ -z "$REPO_URL" ]; then
    echo "botl: error: BOTL_REPO_URL is not set" >&2
    exit 1
fi

# Build clone command
CLONE_ARGS="--depth ${DEPTH}"
if [ -n "$BRANCH" ]; then
    CLONE_ARGS="${CLONE_ARGS} --branch ${BRANCH}"
fi

echo "botl: cloning ${REPO_URL} (depth=${DEPTH}, branch=${BRANCH:-default})..." >&2
git clone ${CLONE_ARGS} "${REPO_URL}" /workspace/repo
cd /workspace/repo

# Configure git for commits inside container
git config user.email "botl@container"
git config user.name "botl"

# Record the initial HEAD so postrun can detect new commits
INITIAL_HEAD="$(git rev-parse HEAD 2>/dev/null || echo '')"
export BOTL_INITIAL_HEAD="$INITIAL_HEAD"

if [ -n "$PROMPT" ]; then
    # Headless mode: run with prompt, capture exit code
    claude --dangerously-skip-permissions -p "$PROMPT"
else
    # Interactive mode: launch Claude Code with TTY
    claude --dangerously-skip-permissions
fi

# Run the post-session TUI to handle results
exec botl-postrun
