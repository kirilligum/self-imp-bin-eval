#!/usr/bin/env bash
set -euo pipefail

# TEST-004

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${ROOT_DIR}/scripts/lib/local_env.sh"

JSON=false
if [[ "${1:-}" == "--json" ]]; then
  JSON=true
fi

MODE="${BIN_EVAL_SYSTEMD_MODE:-$(bin_eval_systemd_mode)}"
export BIN_EVAL_ENV_FILE
BIN_EVAL_ENV_FILE="$(bin_eval_default_env_file_for_mode "$ROOT_DIR" "$MODE")"
LITELLM_ENV_FILE="$(bin_eval_litellm_env_file)"

bin_eval_load_local_env "$ROOT_DIR"

unit_state() {
  local unit="$1"
  local state enabled
  state="$(bin_eval_systemctl "$MODE" is-active "$unit" 2>/dev/null || true)"
  enabled="$(bin_eval_systemctl "$MODE" is-enabled "$unit" 2>/dev/null || true)"
  jq -n --arg name "$unit" --arg active "$state" --arg enabled "$enabled" \
    '{name:$name, active:$active, enabled:$enabled}'
}

api_code="$(curl -sS -o /dev/null -w '%{http_code}' "${BIN_EVAL_URL:-http://127.0.0.1:8080}/checklists/00000000-0000-0000-0000-000000000000" 2>/dev/null || printf '000')"

compose_raw="$(docker compose --env-file "$BIN_EVAL_ENV_FILE" -f "${ROOT_DIR}/deploy/compose/docker-compose.yml" ps --format json 2>/dev/null || true)"
if [[ -n "$compose_raw" ]]; then
  compose_json="$(
    printf '%s\n' "$compose_raw" |
      jq -s 'if length == 1 and (.[0] | type) == "array" then .[0] else . end
      | map({
          name: (.Name // .Names),
          service: .Service,
          state: .State,
          status: .Status,
          ports: .Ports,
          project: .Project
        })'
  )"
else
  compose_json="[]"
fi

summary_json="$(jq -n \
  --arg mode "$MODE" \
  --arg env_file "$BIN_EVAL_ENV_FILE" \
  --arg litellm_env_file "$LITELLM_ENV_FILE" \
  --arg api_url "${BIN_EVAL_URL:-http://127.0.0.1:8080}" \
  --arg api_http_code "$api_code" \
  --arg llm_base_url "${BIN_EVAL_LLM_BASE_URL:-}" \
  --arg model_profile "${BIN_EVAL_MODEL_PROFILE:-}" \
  --arg debug_summary "${ROOT_DIR}/debug/live-curl/summary.json" \
  --argjson deps "$(unit_state bin-eval-deps.service)" \
  --argjson api "$(unit_state bin-eval-api.service)" \
  --argjson worker "$(unit_state bin-eval-worker.service)" \
  --argjson compose "$compose_json" \
  --arg bin_eval_env_exists "$([[ -f "$BIN_EVAL_ENV_FILE" ]] && printf true || printf false)" \
  --arg litellm_env_exists "$([[ -f "$LITELLM_ENV_FILE" ]] && printf true || printf false)" \
  --arg llm_api_key_set "$([[ -n "${BIN_EVAL_LLM_API_KEY:-}" || -n "${LITELLM_MASTER_KEY:-}" ]] && printf true || printf false)" \
  --arg garage_secret_set "$([[ -n "${BIN_EVAL_GARAGE_SECRET_KEY:-}" ]] && printf true || printf false)" \
  '{
    systemd_mode: $mode,
    api_url: $api_url,
    api_http_code: $api_http_code,
    llm_base_url: $llm_base_url,
    model_profile: $model_profile,
    env: {
      bin_eval_env_file: $env_file,
      bin_eval_env_file_exists: ($bin_eval_env_exists == "true"),
      litellm_env_file: $litellm_env_file,
      litellm_env_file_exists: ($litellm_env_exists == "true"),
      secrets: {
        llm_api_key_set: ($llm_api_key_set == "true"),
        garage_secret_key_set: ($garage_secret_set == "true"),
        values: "redacted"
      }
    },
    systemd: {
      deps: $deps,
      api: $api,
      worker: $worker
    },
    compose: $compose,
    debug_summary_path: $debug_summary
  }')"

if [[ "$JSON" == "true" ]]; then
  printf '%s\n' "$summary_json"
else
  printf '%s\n' "$summary_json" | jq -r '
    "mode=\(.systemd_mode)",
    "api_url=\(.api_url)",
    "api_http_code=\(.api_http_code)",
    "model_profile=\(.model_profile)",
    "deps=\(.systemd.deps.active)",
    "api=\(.systemd.api.active)",
    "worker=\(.systemd.worker.active)",
    "secrets=redacted",
    "debug_summary_path=\(.debug_summary_path)"
  '
fi
