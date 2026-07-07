#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${ROOT_DIR}/scripts/lib/local_env.sh"

MODE="${BIN_EVAL_SYSTEMD_MODE:-$(bin_eval_systemd_mode)}"
export BIN_EVAL_ENV_FILE
BIN_EVAL_ENV_FILE="$(bin_eval_default_env_file_for_mode "$ROOT_DIR" "$MODE")"

bin_eval_load_local_env "$ROOT_DIR"

"${ROOT_DIR}/scripts/validate_litellm_responses.sh"

bin_eval_systemctl "$MODE" start bin-eval-deps.service
"${ROOT_DIR}/scripts/wait-for-tcp.sh" 127.0.0.1 "${BIN_EVAL_POSTGRES_PORT:-55432}" 120
"${ROOT_DIR}/scripts/wait-for-tcp.sh" 127.0.0.1 7233 180
"${ROOT_DIR}/scripts/wait-for-tcp.sh" 127.0.0.1 3900 120

bin_eval_systemctl "$MODE" start bin-eval-worker.service
bin_eval_systemctl "$MODE" start bin-eval-api.service

for _ in $(seq 1 90); do
  code="$(curl -sS -o /dev/null -w '%{http_code}' "${BIN_EVAL_URL}/checklists/00000000-0000-0000-0000-000000000000" 2>/dev/null || true)"
  if [[ "$code" != "000" ]]; then
    echo "bin-eval local service started mode=${MODE} url=${BIN_EVAL_URL}"
    exit 0
  fi
  sleep 1
done

echo "timed out waiting for bin-eval API at ${BIN_EVAL_URL}" >&2
exit 1
