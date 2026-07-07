#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${ROOT_DIR}/scripts/lib/local_env.sh"

MODE="${BIN_EVAL_SYSTEMD_MODE:-$(bin_eval_systemd_mode)}"
export BIN_EVAL_ENV_FILE
BIN_EVAL_ENV_FILE="$(bin_eval_default_env_file_for_mode "$ROOT_DIR" "$MODE")"

if [[ "${BIN_EVAL_DOCKER_COMPOSE_IN_SG:-}" != "1" ]] &&
  ! docker ps >/dev/null 2>&1 &&
  command -v sg >/dev/null 2>&1 &&
  id -nG "${USER:-$(id -un)}" | tr ' ' '\n' | grep -qx docker; then
  export BIN_EVAL_DOCKER_COMPOSE_IN_SG=1
  command_line="$(printf '%q ' "$0" "$@")"
  exec sg docker -c "$command_line"
fi

exec docker compose --env-file "$BIN_EVAL_ENV_FILE" -f "${ROOT_DIR}/deploy/compose/docker-compose.yml" "$@"
