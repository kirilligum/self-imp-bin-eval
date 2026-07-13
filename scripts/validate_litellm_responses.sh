#!/usr/bin/env bash
set -euo pipefail


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
auth_config="$(mktemp)"
chmod 0600 "$auth_config"
printf 'header = "Authorization: Bearer %s"\n' "$API_KEY" > "$auth_config"
trap 'rm -f "$models_tmp" "$response_tmp" "$payload_tmp" "$auth_config"' EXIT

curl -fsS \
  --config "$auth_config" \
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
          text: "Provide the successful structured contract result."
        }
      ]
    }
  ],
  tools: [
    {
      type: "function",
      name: "bin_eval_litellm_contract",
      description: "Report whether this structured-output contract call succeeded.",
      strict: true,
      parameters: {
        type: "object",
        additionalProperties: false,
        required: ["ok"],
        properties: {
          ok: {type: "boolean", const: true}
        }
      }
    }
  ],
  tool_choice: {
    type: "function",
    name: "bin_eval_litellm_contract"
  }
}' > "$payload_tmp"

curl -fsS \
  --config "$auth_config" \
  -H "Content-Type: application/json" \
  -X POST \
  --data @"$payload_tmp" \
  "${BASE_URL}/v1/responses" \
  -o "$response_tmp"

arguments="$(
  awk '/^data: / { sub(/^data: /, ""); if ($0 != "[DONE]") print }' "$response_tmp" |
    jq -r 'select(type == "object" and .type == "response.function_call_arguments.done") | .arguments' |
    tail -n 1
)"

if [[ -z "$arguments" ]]; then
  echo "LiteLLM Responses stream did not include forced function arguments" >&2
  exit 1
fi
if ! jq -e '.ok == true' <<<"$arguments" >/dev/null; then
  echo "LiteLLM Responses output failed strict function schema contract" >&2
  exit 1
fi

jq -n --arg base_url "$BASE_URL" --arg model "$MODEL" '{
  ok: true,
  base_url: $base_url,
  model: $model,
  endpoint: "/v1/responses",
  auth: "redacted"
}'
