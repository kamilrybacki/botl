#!/bin/sh
# Debug script to verify Claude Code works inside the botl container.
# Run from the botl repo root: sh test/manual/debug-claude.sh

IMAGE="botl:latest"
CLAUDE_DIR="$HOME/.claude"
CLAUDE_JSON="$HOME/.claude.json"
REPO_URL="https://github.com/kamilrybacki/DummyRepo"

echo "=== Test 1: Claude version ==="
docker run --rm \
  -v "$CLAUDE_DIR:/home/botl/.claude:ro" \
  -v "$CLAUDE_JSON:/home/botl/.claude.json:ro" \
  --entrypoint sh "$IMAGE" \
  -c "gosu botl claude --version 2>&1"

echo ""
echo "=== Test 2: Simulate credential copy (like entrypoint does) ==="
docker run --rm \
  -v "$CLAUDE_DIR:/home/botl/.claude:ro" \
  -v "$CLAUDE_JSON:/home/botl/.claude.json:ro" \
  --entrypoint sh "$IMAGE" \
  -c '
    # This mirrors the entrypoint credential copy logic
    BOTL_HOME="/tmp/botl-home"
    mkdir -p "$BOTL_HOME"
    cp -a /home/botl/.claude "$BOTL_HOME/.claude" 2>&1
    cp /home/botl/.claude.json "$BOTL_HOME/.claude.json" 2>&1
    chown -R botl:botl "$BOTL_HOME"
    export HOME="$BOTL_HOME"

    echo "--- HOME=$HOME ---"
    echo "--- .claude.json readable by botl? ---"
    gosu botl cat "$HOME/.claude.json" > /dev/null 2>&1 && echo "YES" || echo "NO"
    echo "--- .claude dir readable by botl? ---"
    gosu botl ls "$HOME/.claude/" > /dev/null 2>&1 && echo "YES" || echo "NO"
    echo "--- .claude.json owner ---"
    ls -la "$HOME/.claude.json"
    echo "--- Claude auth after copy ---"
    timeout 10 gosu botl sh -c "HOME=$HOME claude --dangerously-skip-permissions --output-format text -p \"say hello\"" 2>&1
    echo "EXIT=$?"
  '

echo ""
echo "=== Test 3: Full botl run through real entrypoint (60s hard timeout) ==="
echo "Streaming live..."
echo ""
timeout 60 ./botl run "$REPO_URL" -p "Print the word CANARY_OK to stdout" --timeout 30s 2>&1 | sed 's/^/  │ /' || true

echo ""
echo "=== Done ==="
