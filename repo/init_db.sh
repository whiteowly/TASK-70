#!/usr/bin/env bash
# ============================================================
# init_db.sh — Database initialization
#
# Applies SQL migrations from db/migrations/ with tracking.
# Skips already-applied migrations. Fails immediately on SQL
# errors and rolls back the failing migration.
#
# Usage:
#   ./init_db.sh           Apply pending migrations
#   ./init_db.sh --reset   Drop and recreate DB, then apply all
# ============================================================
set -euo pipefail
cd "$(dirname "$0")"

RESET=false
if [ "${1:-}" = "--reset" ]; then
  RESET=true
fi

echo "Starting postgres service..."
docker compose up -d bootstrap postgres

echo "Waiting for postgres to be healthy..."
until docker compose exec -T postgres pg_isready -U fieldserve -d fieldserve > /dev/null 2>&1; do
  sleep 1
done
echo "Postgres is ready."

# Helper: run psql with strict error handling
run_psql() {
  docker compose exec -T postgres psql -U fieldserve -d fieldserve \
    -v ON_ERROR_STOP=1 "$@"
}

# Reset path: drop and recreate the database
if [ "$RESET" = true ]; then
  echo "Resetting database..."
  docker compose exec -T postgres psql -U fieldserve -d postgres \
    -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS fieldserve"
  docker compose exec -T postgres psql -U fieldserve -d postgres \
    -v ON_ERROR_STOP=1 -c "CREATE DATABASE fieldserve"
  echo "Database recreated."
fi

# Create migration tracking table
run_psql <<'SQL'
CREATE TABLE IF NOT EXISTS schema_migrations (
    filename    VARCHAR(512) PRIMARY KEY,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
SQL

# Apply each migration in sorted order, skip if already applied
APPLIED_COUNT=0
SKIPPED_COUNT=0

for migration in db/migrations/*.up.sql; do
  BASENAME=$(basename "$migration")

  # Check if already applied
  ALREADY=$(run_psql -tAc \
    "SELECT 1 FROM schema_migrations WHERE filename = '${BASENAME}'" 2>/dev/null || true)

  if [ "$ALREADY" = "1" ]; then
    echo "  Skipping ${BASENAME} (already applied)"
    SKIPPED_COUNT=$((SKIPPED_COUNT + 1))
    continue
  fi

  echo "  Applying ${BASENAME}..."
  if run_psql --single-transaction < "$migration"; then
    # Record the successful migration
    run_psql -c "INSERT INTO schema_migrations (filename) VALUES ('${BASENAME}')"
    echo "  OK"
    APPLIED_COUNT=$((APPLIED_COUNT + 1))
  else
    echo ""
    echo "ERROR: Migration ${BASENAME} failed. Transaction rolled back."
    echo "Fix the migration SQL and rerun ./init_db.sh"
    echo "To start fresh: ./init_db.sh --reset"
    exit 1
  fi
done

echo ""
echo "Database initialized. Applied: ${APPLIED_COUNT}, Skipped: ${SKIPPED_COUNT}."
