#!/usr/bin/env bash
set -euo pipefail

# TEST-005

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${ROOT_DIR}/scripts/lib/local_env.sh"

MODE="${BIN_EVAL_SYSTEMD_MODE:-$(bin_eval_systemd_mode)}"
export BIN_EVAL_ENV_FILE
BIN_EVAL_ENV_FILE="$(bin_eval_default_env_file_for_mode "$ROOT_DIR" "$MODE")"
bin_eval_load_local_env "$ROOT_DIR"
bin_eval_require_tools curl jq

BIN_EVAL_URL="${BIN_EVAL_URL:-http://127.0.0.1:8080}"
DEBUG_DIR="${ROOT_DIR}/debug/live-curl"
CASE_DIR="${BIN_EVAL_CASE_DIR:-${ROOT_DIR}/fixtures/smoke/cases/release_notes}"
mkdir -p "$DEBUG_DIR"

post_json() {
  local path="$1"
  local payload="$2"
  local out="$3"
  curl -fsS \
    -H 'Content-Type: application/json' \
    -X POST \
    --data "$payload" \
    "${BIN_EVAL_URL}${path}" \
    -o "$out"
}

poll_entity() {
  local url="$1"
  local out="$2"
  local label="$3"
  for _ in $(seq 1 150); do
    curl -fsS "$url" -o "$out"
    status="$(jq -r '.status // empty' "$out")"
    case "$status" in
      succeeded)
        return 0
        ;;
      failed)
        echo "${label} failed" >&2
        jq . "$out" >&2
        return 1
        ;;
    esac
    sleep 2
  done
  echo "timed out polling ${label}: ${url}" >&2
  return 1
}

api_code="$(curl -sS -o /dev/null -w '%{http_code}' "${BIN_EVAL_URL}/checklists/00000000-0000-0000-0000-000000000000" || true)"
if [[ "$api_code" == "000" ]]; then
  echo "bin-eval API is unreachable at ${BIN_EVAL_URL}" >&2
  exit 1
fi

create_payload="$(jq -c '{task, context}' "${CASE_DIR}/task.json")"
post_json "/checklists" "$create_payload" "${DEBUG_DIR}/create_checklist.json"
checklist_id="$(jq -r '.checklist_id' "${DEBUG_DIR}/create_checklist.json")"
if [[ -z "$checklist_id" || "$checklist_id" == "null" ]]; then
  echo "create checklist response did not include checklist_id" >&2
  jq . "${DEBUG_DIR}/create_checklist.json" >&2
  exit 1
fi

poll_entity "${BIN_EVAL_URL}/checklists/${checklist_id}" "${DEBUG_DIR}/checklist.json" "checklist"

jq -e '
  . as $root |
  $root.status == "succeeded" and
  ($root.questions | type == "array" and length > 0) and
  ($root.weights | type == "array" and length == ($root.questions | length)) and
  all($root.weights[]; has("question_id") and has("weight"))
' "${DEBUG_DIR}/checklist.json" >/dev/null

evaluation_payload="$(jq -n --arg id "$checklist_id" --rawfile answer "${CASE_DIR}/model_answer_good.txt" '{checklist_id:$id, model_answer:$answer}')"
post_json "/evaluations" "$evaluation_payload" "${DEBUG_DIR}/create_evaluation.json"
evaluation_id="$(jq -r '.evaluation_id' "${DEBUG_DIR}/create_evaluation.json")"
if [[ -z "$evaluation_id" || "$evaluation_id" == "null" ]]; then
  echo "create evaluation response did not include evaluation_id" >&2
  jq . "${DEBUG_DIR}/create_evaluation.json" >&2
  exit 1
fi

poll_entity "${BIN_EVAL_URL}/evaluations/${evaluation_id}" "${DEBUG_DIR}/evaluation.json" "evaluation"

jq -e '
  .status == "succeeded" and
  (.satisfied_points | type == "number") and
  (.total_possible_points | type == "number") and
  (.checklist_pass_rate | type == "number") and
  (.failed_question_ids | type == "array") and
  (.judgments | type == "array" and length > 0)
' "${DEBUG_DIR}/evaluation.json" >/dev/null

jq -n \
  --arg api_url "$BIN_EVAL_URL" \
  --slurpfile checklist "${DEBUG_DIR}/checklist.json" \
  --slurpfile evaluation "${DEBUG_DIR}/evaluation.json" '{
    api_url: $api_url,
    checklist_id: $checklist[0].checklist_id,
    checklist_status: $checklist[0].status,
    question_count: ($checklist[0].questions | length),
    weights: $checklist[0].weights,
    evaluation_id: $evaluation[0].evaluation_id,
    evaluation_status: $evaluation[0].status,
    satisfied_points: $evaluation[0].satisfied_points,
    total_possible_points: $evaluation[0].total_possible_points,
    checklist_pass_rate: $evaluation[0].checklist_pass_rate,
    failed_question_ids: $evaluation[0].failed_question_ids,
    judgment_count: ($evaluation[0].judgments | length)
  }' | tee "${DEBUG_DIR}/summary.json"
