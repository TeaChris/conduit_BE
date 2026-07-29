#!/usr/bin/env bash
# Run all code generation steps.

set -euo pipefail

SCRIPT_DIR="$(dirname "$0")"
ROOT_DIR="$SCRIPT_DIR/.."

echo "==> Generating sqlc code..."
sqlc generate -f "$ROOT_DIR/sqlc/sqlc.yaml"

echo "==> Code generation complete."
