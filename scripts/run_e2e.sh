#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
source scripts/lib/local_env.sh

if [[ "${BIN_EVAL_LOAD_LOCAL_ENV:-false}" == "true" ]]; then
  MODE="${BIN_EVAL_SYSTEMD_MODE:-$(bin_eval_systemd_mode)}"
  export BIN_EVAL_ENV_FILE
  BIN_EVAL_ENV_FILE="$(bin_eval_default_env_file_for_mode "$ROOT_DIR" "$MODE")"
  bin_eval_load_local_env "$ROOT_DIR"
fi

DEBUG_DIR="${BIN_EVAL_DEBUG_DIR:-debug/smoke}"

scripts/smoke_curl.sh
scripts/validate_smoke_invariants.sh "$DEBUG_DIR"
scripts/capture_artifacts.sh "$DEBUG_DIR"
