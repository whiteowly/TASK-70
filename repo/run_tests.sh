#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

echo "=== Initializing database ==="
./init_db.sh

echo ""
echo "=== Building test images ==="
docker compose --profile test build backend-test frontend-test

echo ""
echo "=== Running backend tests ==="
docker compose --profile test run --rm backend-test

echo ""
echo "=== Running frontend tests ==="
docker compose --profile test run --rm --no-deps frontend-test

echo ""
echo "=== All tests passed ==="
