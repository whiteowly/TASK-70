#!/usr/bin/env bash
# Show the actual host URLs for running FieldServe services.
# Ports are assigned dynamically by the OS, so use this script
# (or `docker compose port <service> <port>`) to discover them.
set -euo pipefail
cd "$(dirname "$0")/.."

API=$(docker compose port api 8080 2>/dev/null) || true
FRONTEND=$(docker compose port frontend 80 2>/dev/null) || true

if [ -z "$API" ] && [ -z "$FRONTEND" ]; then
  echo "No running services found. Start with: docker compose up --build"
  exit 1
fi

echo "FieldServe URLs:"
[ -n "$FRONTEND" ] && echo "  Frontend:  http://${FRONTEND}"
[ -n "$API" ]      && echo "  API:       http://${API}"
[ -n "$API" ]      && echo "  Health:    http://${API}/api/v1/system/health"
