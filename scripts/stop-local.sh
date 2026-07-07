#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${ROOT_DIR}/scripts/lib/local_env.sh"

MODE="${BIN_EVAL_SYSTEMD_MODE:-$(bin_eval_systemd_mode)}"

bin_eval_systemctl "$MODE" stop bin-eval-api.service >/dev/null 2>&1 || true
bin_eval_systemctl "$MODE" stop bin-eval-worker.service >/dev/null 2>&1 || true
bin_eval_systemctl "$MODE" stop bin-eval-deps.service >/dev/null 2>&1 || true

echo "bin-eval local service stopped mode=${MODE}"
