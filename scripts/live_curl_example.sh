#!/usr/bin/env bash
set -euo pipefail


ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${ROOT_DIR}/scripts/lib/local_env.sh"
source "${ROOT_DIR}/scripts/lib/http.sh"

MODE="${BIN_EVAL_SYSTEMD_MODE:-$(bin_eval_systemd_mode)}"
export BIN_EVAL_ENV_FILE
BIN_EVAL_ENV_FILE="$(bin_eval_default_env_file_for_mode "$ROOT_DIR" "$MODE")"
bin_eval_load_local_env "$ROOT_DIR"
bin_eval_require_tools curl jq
export BIN_EVAL_GIT_SHA="${BIN_EVAL_GIT_SHA:-$(git rev-parse HEAD)}"
export BIN_EVAL_ENDPOINT_CLASS="${BIN_EVAL_ENDPOINT_CLASS:-local-live}"
export BIN_EVAL_FIXTURE_VERSION="${BIN_EVAL_FIXTURE_VERSION:-not-applicable}"

BIN_EVAL_URL="${BIN_EVAL_URL:-http://127.0.0.1:8080}"
DEBUG_DIR="${ROOT_DIR}/debug/live-curl"
CASE_DIR="${BIN_EVAL_CASE_DIR:-${ROOT_DIR}/fixtures/smoke/cases/release_notes}"
mkdir -p "$DEBUG_DIR"

bin_eval_wait_for_api "$BIN_EVAL_URL"

evaluation_runs=3
create_payload="$(jq -c --argjson evaluation_runs "$evaluation_runs" '{task, context, evaluation_runs:$evaluation_runs}' "${CASE_DIR}/task.json")"
bin_eval_post_json "$BIN_EVAL_URL" "/checklists" "$create_payload" "${DEBUG_DIR}/create_checklist.json"
checklist_id="$(jq -r '.checklist_id' "${DEBUG_DIR}/create_checklist.json")"
if [[ -z "$checklist_id" || "$checklist_id" == "null" ]]; then
  echo "create checklist response did not include checklist_id" >&2
  jq . "${DEBUG_DIR}/create_checklist.json" >&2
  exit 1
fi

bin_eval_poll_entity "${BIN_EVAL_URL}/checklists/${checklist_id}" "${DEBUG_DIR}/checklist.json" "checklist ${checklist_id}"

jq -e --argjson evaluation_runs "$evaluation_runs" '
  . as $root |
  $root.status == "succeeded" and
  ($root.evaluation_runs == $evaluation_runs) and
  ($root.dimensions | type == "array" and length > 0) and
  ($root.candidate_questions | type == "array" and length > 0) and
  ($root.questions | type == "array" and length >= 8) and
  ($root.weights | type == "array" and length == ($root.candidate_questions | length)) and
  all($root.weights[]; has("candidate_question_id") and has("rationale") and has("weight")) and
  all($root.questions[]; has("id") and has("dimension_id") and has("source_candidate_id") and has("question"))
' "${DEBUG_DIR}/checklist.json" >/dev/null

evaluation_payload="$(jq -n --arg id "$checklist_id" --rawfile answer "${CASE_DIR}/model_answer_good.txt" '{checklist_id:$id, model_answer:$answer}')"
bin_eval_post_json "$BIN_EVAL_URL" "/evaluations" "$evaluation_payload" "${DEBUG_DIR}/create_evaluation.json"
evaluation_id="$(jq -r '.evaluation_id' "${DEBUG_DIR}/create_evaluation.json")"
if [[ -z "$evaluation_id" || "$evaluation_id" == "null" ]]; then
  echo "create evaluation response did not include evaluation_id" >&2
  jq . "${DEBUG_DIR}/create_evaluation.json" >&2
  exit 1
fi

bin_eval_poll_entity "${BIN_EVAL_URL}/evaluations/${evaluation_id}" "${DEBUG_DIR}/evaluation.json" "evaluation ${evaluation_id}"

jq -e --argjson evaluation_runs "$evaluation_runs" '
  .status == "succeeded" and
  (.satisfied_points | type == "number") and
  (.total_possible_points | type == "number") and
  (.checklist_pass_rate | type == "number") and
  (.failed_question_ids | type == "array") and
  (.judgments | type == "array" and length > 0) and
  all(.judgments[];
    (.runs | length == $evaluation_runs) and
    ((.runs | map(.run_index)) == [range(1; $evaluation_runs + 1)]) and
    all(.runs[]; (.answer == "yes" or .answer == "no") and (.evidence | length > 0)))
' "${DEBUG_DIR}/evaluation.json" >/dev/null

jq -n \
  --arg api_url "$BIN_EVAL_URL" \
  --arg git_sha "$BIN_EVAL_GIT_SHA" \
  --arg endpoint_class "$BIN_EVAL_ENDPOINT_CLASS" \
  --arg fixture_version "$BIN_EVAL_FIXTURE_VERSION" \
  --arg model_profile "${BIN_EVAL_MODEL_PROFILE:-unknown}" \
  --slurpfile checklist "${DEBUG_DIR}/checklist.json" \
  --slurpfile evaluation "${DEBUG_DIR}/evaluation.json" '{
    api_url: $api_url,
    checklist_id: $checklist[0].checklist_id,
    checklist_status: $checklist[0].status,
    dimension_count: ($checklist[0].dimensions | length),
    candidate_question_count: ($checklist[0].candidate_questions | length),
    final_question_count: ($checklist[0].questions | length),
    evaluation_runs: $checklist[0].evaluation_runs,
    weights: $checklist[0].weights,
    evaluation_id: $evaluation[0].evaluation_id,
    evaluation_status: $evaluation[0].status,
    satisfied_points: $evaluation[0].satisfied_points,
    total_possible_points: $evaluation[0].total_possible_points,
    checklist_pass_rate: $evaluation[0].checklist_pass_rate,
    failed_question_ids: $evaluation[0].failed_question_ids,
    judgment_count: ($evaluation[0].judgments | length),
    limit_hit_count: 0,
    evidence: {
      git_sha: $git_sha,
      endpoint_class: $endpoint_class,
      fixture_version: $fixture_version,
      model_profile: $model_profile
    },
    run_pass_rates: [range(1; $checklist[0].evaluation_runs + 1) as $run |
      ([$evaluation[0].judgments[].runs[] | select(.run_index == $run and .answer == "yes")] | length) /
      ($checklist[0].questions | length)]
  }' | tee "${DEBUG_DIR}/summary.json"

scripts/capture_artifacts.sh "$DEBUG_DIR"
