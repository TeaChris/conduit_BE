#!/usr/bin/env bash
# Helper script for database migrations.
#
# Usage:
#   ./scripts/migrate.sh up        Run all pending migrations
#   ./scripts/migrate.sh down       Rollback last migration
#   ./scripts/migrate.sh create NAME  Create new migration files

set -euo pipefail

MIGRATE_DSN="${DATABASE_URL:-postgres://conduit:conduit@localhost:5432/conduit?sslmode=disable}"
MIGRATIONS_DIR="$(dirname "$0")/../migrations"

case "${1:-help}" in
  up)
    migrate -path "$MIGRATIONS_DIR" -database "$MIGRATE_DSN" up
    ;;
  down)
    migrate -path "$MIGRATIONS_DIR" -database "$MIGRATE_DSN" down 1
    ;;
  create)
    if [ -z "${2:-}" ]; then
      echo "Usage: $0 create <migration_name>"
      exit 1
    fi
    migrate create -ext sql -dir "$MIGRATIONS_DIR" -seq "$2"
    ;;
  *)
    echo "Usage: $0 {up|down|create <name>}"
    exit 1
    ;;
esac
