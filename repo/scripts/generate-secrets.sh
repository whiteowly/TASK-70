#!/bin/sh
# ============================================================
# generate-secrets.sh — LOCAL DEVELOPMENT ONLY
#
# Runs inside the bootstrap container to generate ephemeral
# secrets into a shared Docker volume. No file is written to
# the repo tree. Reuses existing secrets if already generated
# (volume survives docker compose restart; cleared by
# docker compose down -v).
# ============================================================
set -eu

ENV_FILE="/run/secrets/env"

if [ -f "$ENV_FILE" ]; then
  echo "Dev secrets already exist, reusing."
  exit 0
fi

echo "Generating ephemeral dev secrets..."

POSTGRES_PASSWORD=$(cat /dev/urandom | tr -dc 'A-Za-z0-9' | head -c 40)
ENCRYPTION_KEY=$(cat /dev/urandom | tr -dc 'a-f0-9' | head -c 64)
SESSION_SECRET=$(cat /dev/urandom | tr -dc 'A-Za-z0-9' | head -c 64)

cat > "$ENV_FILE" <<EOF
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD}"
export DATABASE_URL="postgres://fieldserve:${POSTGRES_PASSWORD}@postgres:5432/fieldserve?sslmode=disable"
export ENCRYPTION_KEY="${ENCRYPTION_KEY}"
export SESSION_SECRET="${SESSION_SECRET}"
EOF

chmod 644 "$ENV_FILE"
echo "Dev secrets generated into container volume."
