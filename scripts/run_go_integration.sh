#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

source scripts/lib/local_env.sh
bin_eval_load_env_file deploy/compose/.env.example true

export BIN_EVAL_COMPOSE_MODE=test
export BIN_EVAL_ENV_FILE="$ROOT_DIR/deploy/compose/.env.example"
export BIN_EVAL_TEST_PROJECT="${BIN_EVAL_TEST_PROJECT:-bin-eval-test}"
export BIN_EVAL_POSTGRES_PORT="${BIN_EVAL_TEST_POSTGRES_PORT:-55433}"
export BIN_EVAL_TEMPORAL_PORT="${BIN_EVAL_TEST_TEMPORAL_PORT:-7234}"
export BIN_EVAL_GARAGE_ENDPOINT="http://127.0.0.1:${BIN_EVAL_TEST_GARAGE_PORT:-23900}"
export BIN_EVAL_DATABASE_URL="postgres://bin_eval:bin_eval@127.0.0.1:${BIN_EVAL_POSTGRES_PORT}/bin_eval?sslmode=disable"
export BIN_EVAL_TEMPORAL_ADDRESS="127.0.0.1:${BIN_EVAL_TEMPORAL_PORT}"

COMPOSE=(scripts/docker-compose-local.sh)
"${COMPOSE[@]}" config >/dev/null
"${COMPOSE[@]}" up -d postgres temporal garage
scripts/wait-for-temporal.sh

cleanup() {
  "${COMPOSE[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

go test -tags integration "$@" -count=1 -timeout 10m
