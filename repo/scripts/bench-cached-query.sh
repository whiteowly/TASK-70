#!/usr/bin/env bash
# bench-cached-query.sh — Verify cached-query 300ms SLA
#
# Runs the TestCachedQueryPerformance test which:
# 1. Warms the search cache with a query
# 2. Measures 10 iterations of the same cached query
# 3. Asserts average response time < 300ms
#
# Usage:
#   docker compose up -d
#   ./scripts/bench-cached-query.sh
#
# The test runs inside the Go test suite against the real PostgreSQL
# database and the real LRU search cache layer.

set -euo pipefail
cd "$(dirname "$0")/.."

echo "Running cached-query SLA benchmark..."
docker compose exec -T api go test -v -run TestCachedQueryPerformance -count=1 ./cmd/api/ 2>&1
echo ""
echo "If the test passed, the cached-query avg latency is under 300ms."
