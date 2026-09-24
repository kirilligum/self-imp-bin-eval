#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

timeout_seconds="${BIN_EVAL_TEMPORAL_HEALTH_TIMEOUT_SECONDS:-180}"
deadline=$((SECONDS + timeout_seconds))
while ((SECONDS < deadline)); do
  if scripts/docker-compose-local.sh exec -T temporal temporal operator cluster health \
    --address 127.0.0.1:7233 >/dev/null 2>&1; then
    exit 0
  fi
  sleep 2
done

echo "Temporal did not become healthy within ${timeout_seconds}s" >&2
exit 1
