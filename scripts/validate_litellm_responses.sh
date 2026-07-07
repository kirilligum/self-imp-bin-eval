#!/usr/bin/env bash
set -euo pipefail

# TEST-002

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${ROOT_DIR}/scripts/lib/local_env.sh"

MODE="${BIN_EVAL_SYSTEMD_MODE:-$(bin_eval_systemd_mode)}"
export BIN_EVAL_ENV_FILE
BIN_EVAL_ENV_FILE="$(bin_eval_default_env_file_for_mode "$ROOT_DIR" "$MODE")"
bin_eval_load_local_env "$ROOT_DIR"
bin_eval_require_tools curl jq

BASE_URL="${BIN_EVAL_LLM_BASE_URL:-http://127.0.0.1:4000}"
MODEL="${BIN_EVAL_MODEL_PROFILE:-gpt-5.4-mini}"
if [[ "$MODEL" == "checklist-evaluator" ]]; then
  MODEL="gpt-5.4-mini"
fi
API_KEY="${BIN_EVAL_LLM_API_KEY:-${LITELLM_MASTER_KEY:-}}"

if [[ -z "$API_KEY" || "$API_KEY" == "replace-with-local-llm-key" ]]; then
  echo "LiteLLM API key is not configured; set BIN_EVAL_LLM_API_KEY or LITELLM_MASTER_KEY" >&2
  exit 1
fi

models_tmp="$(mktemp)"
response_tmp="$(mktemp)"
payload_tmp="$(mktemp)"
trap 'rm -f "$models_tmp" "$response_tmp" "$payload_tmp"' EXIT

curl -fsS \
  -H "Authorization: Bearer ${API_KEY}" \
  "${BASE_URL}/v1/models" \
  -o "$models_tmp"

if ! jq -e --arg model "$MODEL" 'any(.data[]?; .id == $model)' "$models_tmp" >/dev/null; then
  echo "LiteLLM model ${MODEL} was not found at ${BASE_URL}/v1/models" >&2
  jq -r '.data[]?.id' "$models_tmp" >&2
  exit 1
fi

jq -n --arg model "$MODEL" '{
  model: $model,
  stream: true,
  input: [
    {
      role: "user",
      content: [
        {
          type: "input_text",
          text: "Return only this JSON object with no markdown: {\"ok\":true}"
        }
      ]
    }
  ],
  text: {
    format: {
      type: "json_schema",
      name: "bin_eval_litellm_contract",
      strict: true,
      schema: {
        type: "object",
        additionalProperties: false,
        required: ["ok"],
        properties: {
          ok: {type: "boolean", const: true}
        }
      }
    }
  }
}' > "$payload_tmp"

curl -fsS \
  -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/json" \
  -X POST \
  --data @"$payload_tmp" \
  "${BASE_URL}/v1/responses" \
  -o "$response_tmp"

output_text="$(
  awk '/^data: / { sub(/^data: /, ""); if ($0 != "[DONE]") print }' "$response_tmp" |
    jq -r 'select(type == "object" and .type == "response.output_text.done") | .text' |
    tail -n 1
)"

if [[ -z "$output_text" ]]; then
  echo "LiteLLM Responses stream did not include response.output_text.done" >&2
  exit 1
fi
if ! jq -e '.ok == true' <<<"$output_text" >/dev/null; then
  echo "LiteLLM Responses output failed JSON contract" >&2
  exit 1
fi

jq -n --arg base_url "$BASE_URL" --arg model "$MODEL" '{
  ok: true,
  base_url: $base_url,
  model: $model,
  endpoint: "/v1/responses",
  auth: "redacted"
}'
