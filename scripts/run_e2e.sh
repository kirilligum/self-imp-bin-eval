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
if [[ "${BIN_EVAL_LOAD_PUBLIC_ENV:-false}" == "true" ]]; then
  bin_eval_load_public_env "$ROOT_DIR"
fi

cleanup_test_stack() {
  if [[ "${BIN_EVAL_EXTERNAL_STACK:-false}" != "true" ]]; then
    scripts/docker-compose-local.sh down --volumes --remove-orphans >/dev/null 2>&1 || true
  fi
}
trap cleanup_test_stack EXIT

if [[ "${BIN_EVAL_EXTERNAL_STACK:-false}" != "true" ]]; then
  bin_eval_load_env_file deploy/compose/.env.example false
  export BIN_EVAL_COMPOSE_MODE=test
  export BIN_EVAL_ENV_FILE="$ROOT_DIR/deploy/compose/.env.example"
  export BIN_EVAL_TEST_PROJECT="${BIN_EVAL_TEST_PROJECT:-bin-eval-e2e-$$}"
  export BIN_EVAL_TEST_POSTGRES_PORT="${BIN_EVAL_TEST_POSTGRES_PORT:-55433}"
  export BIN_EVAL_TEST_TEMPORAL_PORT="${BIN_EVAL_TEST_TEMPORAL_PORT:-7234}"
  export BIN_EVAL_TEST_GARAGE_PORT="${BIN_EVAL_TEST_GARAGE_PORT:-23900}"
  export BIN_EVAL_TEST_LLM_PORT="${BIN_EVAL_TEST_LLM_PORT:-24000}"
  export BIN_EVAL_POSTGRES_PORT="$BIN_EVAL_TEST_POSTGRES_PORT"
  export BIN_EVAL_TEMPORAL_PORT="$BIN_EVAL_TEST_TEMPORAL_PORT"
  export BIN_EVAL_GARAGE_ENDPOINT="http://127.0.0.1:${BIN_EVAL_TEST_GARAGE_PORT}"
  export BIN_EVAL_DATABASE_URL="postgres://bin_eval:bin_eval@127.0.0.1:${BIN_EVAL_TEST_POSTGRES_PORT}/bin_eval?sslmode=disable"
  export BIN_EVAL_TEMPORAL_ADDRESS="127.0.0.1:${BIN_EVAL_TEST_TEMPORAL_PORT}"
  export BIN_EVAL_ENDPOINT_CLASS="${BIN_EVAL_ENDPOINT_CLASS:-deterministic}"
  export BIN_EVAL_FIXTURE_VERSION="${BIN_EVAL_FIXTURE_VERSION:-v2}"
  export BIN_EVAL_MODEL_PROFILE=deterministic-fixture
  export BIN_EVAL_E2E_PARENT=true
fi

DEBUG_DIR="${BIN_EVAL_DEBUG_DIR:-debug/smoke}"

scripts/smoke_curl.sh
scripts/validate_smoke_invariants.sh "$DEBUG_DIR"
scripts/capture_artifacts.sh "$DEBUG_DIR"
