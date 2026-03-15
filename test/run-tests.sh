#!/bin/sh
set -e

# ── Start Docker daemon (Docker-in-Docker) ─────────────────────────
echo "==> Starting Docker daemon..."
dockerd --host=unix:///var/run/docker.sock \
        --storage-driver=vfs \
        > /var/log/dockerd.log 2>&1 &

# Wait for daemon to be ready (up to 30s)
TRIES=0
until docker info > /dev/null 2>&1; do
    TRIES=$((TRIES + 1))
    if [ "$TRIES" -ge 30 ]; then
        echo "FATAL: Docker daemon failed to start within 30s"
        cat /var/log/dockerd.log
        exit 1
    fi
    sleep 1
done
echo "    Docker daemon ready."

# ── Determine which test suites to run ──────────────────────────────
# Accepts: "unit", "integration", "e2e", "entrypoint", or "all" (default)
SUITE="${1:-all}"

run_unit() {
    echo ""
    echo "==> Running unit tests..."
    go test -v -count=1 \
        ./internal/container/ \
        ./internal/detect/
}

run_integration() {
    echo ""
    echo "==> Running integration tests (cmd)..."
    go test -v -count=1 ./cmd/
}

run_e2e() {
    echo ""
    echo "==> Running E2E tests (requires Docker)..."
    go test -v -count=1 -tags e2e -timeout 10m ./e2e/
}

run_entrypoint() {
    echo ""
    echo "==> Running entrypoint shell tests..."
    sh ./internal/container/dockerctx/entrypoint_test.sh
}

case "$SUITE" in
    unit)         run_unit ;;
    integration)  run_integration ;;
    e2e)          run_e2e ;;
    entrypoint)   run_entrypoint ;;
    all)
        run_unit
        run_integration
        run_entrypoint
        run_e2e
        echo ""
        echo "==> All test suites passed."
        ;;
    *)
        echo "Unknown suite: $SUITE"
        echo "Usage: $0 [unit|integration|e2e|entrypoint|all]"
        exit 1
        ;;
esac
