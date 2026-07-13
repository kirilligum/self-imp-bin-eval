#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
source scripts/lib/local_env.sh
source scripts/lib/http.sh

DEBUG_DIR="${BIN_EVAL_DEBUG_DIR:-$ROOT_DIR/debug/smoke}"
USER_LISTEN_ADDR="${BIN_EVAL_LISTEN_ADDR:-}"
USER_TEMPORAL_TASK_QUEUE="${BIN_EVAL_TEMPORAL_TASK_QUEUE:-}"
mkdir -p "$DEBUG_DIR"
find "$DEBUG_DIR" -mindepth 1 -maxdepth 1 -exec rm -rf {} +

configure_llm_defaults() {
  if [[ "${BIN_EVAL_LLM_API_KEY:-}" == "replace-with-local-llm-key" && -n "${LITELLM_MASTER_KEY:-}" ]]; then
    export BIN_EVAL_LLM_API_KEY="$LITELLM_MASTER_KEY"
  fi
  if [[ -n "${LITELLM_PORT:-}" && "${BIN_EVAL_LLM_BASE_URL:-}" == "http://127.0.0.1:4000" ]]; then
    export BIN_EVAL_LLM_BASE_URL="http://127.0.0.1:${LITELLM_PORT}"
  fi
  if [[ "${BIN_EVAL_MODEL_PROFILE:-}" == "checklist-evaluator" ]]; then
    local model
    model="$(curl -fsS -H "Authorization: Bearer ${BIN_EVAL_LLM_API_KEY}" "${BIN_EVAL_LLM_BASE_URL}/v1/models" | jq -r 'if any(.data[]?; .id == "gpt-5.4-mini") then "gpt-5.4-mini" else (.data[0].id // empty) end')"
    if [[ -n "$model" ]]; then
      export BIN_EVAL_MODEL_PROFILE="$model"
    fi
  fi
  if [[ -z "${BIN_EVAL_LLM_API_KEY:-}" || "${BIN_EVAL_LLM_API_KEY}" == "replace-with-local-llm-key" ]]; then
    echo "BIN_EVAL_LLM_API_KEY must point to a schema-capable local LLM runtime" >&2
    exit 1
  fi
}

wait_for_tcp() {
  local host="$1"
  local port="$2"
  for _ in $(seq 1 90); do
    if (echo >"/dev/tcp/${host}/${port}") >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "Timed out waiting for ${host}:${port}" >&2
  exit 1
}

cleanup() {
  if [[ -n "${API_PID:-}" ]]; then
    kill "$API_PID" >/dev/null 2>&1 || true
  fi
  if [[ -n "${WORKER_PID:-}" ]]; then
    kill "$WORKER_PID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

if [[ "${BIN_EVAL_EXTERNAL_STACK:-false}" == "true" ]]; then
  : "${BIN_EVAL_URL:?BIN_EVAL_URL is required for an external stack}"
else
  bin_eval_load_env_file deploy/compose/.env.example false
  configure_llm_defaults
  export BIN_EVAL_LISTEN_ADDR="${USER_LISTEN_ADDR:-127.0.0.1:18080}"
  export BIN_EVAL_URL="http://${BIN_EVAL_LISTEN_ADDR}"
  export BIN_EVAL_TEMPORAL_TASK_QUEUE="${USER_TEMPORAL_TASK_QUEUE:-bin-eval-smoke-$$}"

  docker compose --env-file deploy/compose/.env.example -f deploy/compose/docker-compose.yml config >/dev/null
  docker compose --env-file deploy/compose/.env.example -f deploy/compose/docker-compose.yml up -d postgres temporal garage
  wait_for_tcp 127.0.0.1 7233
  wait_for_tcp 127.0.0.1 3900

  go build -o bin/bin-eval-api ./cmd/bin-eval-api
  go build -o bin/bin-eval-worker ./cmd/bin-eval-worker

  BIN_EVAL_LISTEN_ADDR="$BIN_EVAL_LISTEN_ADDR" ./bin/bin-eval-api >"$DEBUG_DIR/api.log" 2>&1 &
  API_PID="$!"
  ./bin/bin-eval-worker >"$DEBUG_DIR/worker.log" 2>&1 &
  WORKER_PID="$!"
fi
export BIN_EVAL_GIT_SHA="${BIN_EVAL_GIT_SHA:-$(git rev-parse HEAD)}"
export BIN_EVAL_ENDPOINT_CLASS="${BIN_EVAL_ENDPOINT_CLASS:-local-live}"
export BIN_EVAL_FIXTURE_VERSION="${BIN_EVAL_FIXTURE_VERSION:-not-applicable}"
bin_eval_wait_for_api "$BIN_EVAL_URL"

good_rates=()
bad_rates=()
dimension_counts=()
candidate_question_counts=()
final_question_counts=()
evaluation_success_count=0
evaluation_runs=3
good_run_rates=()
bad_run_rates=()

for case_dir in fixtures/smoke/cases/*; do
  [[ -d "$case_dir" ]] || continue
  case_name="$(basename "$case_dir")"
  case_out="$DEBUG_DIR/$case_name"
  mkdir -p "$case_out"

  create_payload="$(jq -c --argjson evaluation_runs "$evaluation_runs" '{task, context, evaluation_runs:$evaluation_runs}' "$case_dir/task.json")"
  bin_eval_post_json "$BIN_EVAL_URL" "/checklists" "$create_payload" "$case_out/create_checklist.json"
  checklist_id="$(jq -r '.checklist_id' "$case_out/create_checklist.json")"
  bin_eval_poll_entity "${BIN_EVAL_URL}/checklists/${checklist_id}" "$case_out/checklist.json" "checklist ${checklist_id}"
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
  ' "$case_out/checklist.json" >/dev/null
  final_question_count="$(jq -r '.questions | length' "$case_out/checklist.json")"
  dimension_count="$(jq -r '.dimensions | length' "$case_out/checklist.json")"
  candidate_question_count="$(jq -r '.candidate_questions | length' "$case_out/checklist.json")"
  dimension_counts+=("$dimension_count")
  candidate_question_counts+=("$candidate_question_count")
  final_question_counts+=("$final_question_count")

  for quality in good bad; do
    answer_file="$case_dir/model_answer_${quality}.txt"
    eval_payload="$(jq -n --arg id "$checklist_id" --rawfile answer "$answer_file" '{checklist_id:$id, model_answer:$answer}')"
    bin_eval_post_json "$BIN_EVAL_URL" "/evaluations" "$eval_payload" "$case_out/create_evaluation_${quality}.json"
    evaluation_id="$(jq -r '.evaluation_id' "$case_out/create_evaluation_${quality}.json")"
    bin_eval_poll_entity "${BIN_EVAL_URL}/evaluations/${evaluation_id}" "$case_out/evaluation_${quality}.json" "evaluation ${evaluation_id}"
    jq -e --argjson final_count "$final_question_count" --argjson evaluation_runs "$evaluation_runs" '
      .status == "succeeded" and
      (.total_possible_points == $final_count) and
      (.judgments | type == "array" and length == $final_count) and
      all(.judgments[];
        (.runs | type == "array" and length == $evaluation_runs) and
        ((.runs | map(.run_index)) == [range(1; $evaluation_runs + 1)]) and
        all(.runs[]; (.answer == "yes" or .answer == "no") and (.evidence | type == "string" and length > 0)) and
        (. as $judgment |
          (($judgment.runs | map(select(.answer == "yes")) | length) > ($evaluation_runs / 2)) as $majority_yes |
          (($majority_yes and $judgment.answer == "yes") or (($majority_yes | not) and $judgment.answer == "no")))
      )
    ' "$case_out/evaluation_${quality}.json" >/dev/null
    rate="$(jq -r '.checklist_pass_rate' "$case_out/evaluation_${quality}.json")"
    failed="$(jq -c '.failed_question_ids' "$case_out/evaluation_${quality}.json")"
    echo "case=${case_name} answer=${quality} checklist_id=${checklist_id} evaluation_id=${evaluation_id} dimensions=${dimension_count} candidate_questions=${candidate_question_count} final_questions=${final_question_count} pass_rate=${rate} failed_question_ids=${failed}"
    if [[ "$quality" == "good" ]]; then
      good_rates+=("$rate")
      while IFS= read -r run_rate; do good_run_rates+=("$run_rate"); done < <(jq -r --argjson final_count "$final_question_count" '[range(1; (.judgments[0].runs | length) + 1) as $run | ([.judgments[].runs[] | select(.run_index == $run and .answer == "yes")] | length) / $final_count] | .[]' "$case_out/evaluation_${quality}.json")
    else
      bad_rates+=("$rate")
      while IFS= read -r run_rate; do bad_run_rates+=("$run_rate"); done < <(jq -r --argjson final_count "$final_question_count" '[range(1; (.judgments[0].runs | length) + 1) as $run | ([.judgments[].runs[] | select(.run_index == $run and .answer == "yes")] | length) / $final_count] | .[]' "$case_out/evaluation_${quality}.json")
    fi
    evaluation_success_count=$((evaluation_success_count + 1))
  done
done

good_json="$(printf '%s\n' "${good_rates[@]}" | jq -R 'tonumber' | jq -s '.')"
bad_json="$(printf '%s\n' "${bad_rates[@]}" | jq -R 'tonumber' | jq -s '.')"
dimension_json="$(printf '%s\n' "${dimension_counts[@]}" | jq -R 'tonumber' | jq -s '.')"
candidate_question_json="$(printf '%s\n' "${candidate_question_counts[@]}" | jq -R 'tonumber' | jq -s '.')"
final_question_json="$(printf '%s\n' "${final_question_counts[@]}" | jq -R 'tonumber' | jq -s '.')"
good_run_json="$(printf '%s\n' "${good_run_rates[@]}" | jq -R 'tonumber' | jq -s '.')"
bad_run_json="$(printf '%s\n' "${bad_run_rates[@]}" | jq -R 'tonumber' | jq -s '.')"
metrics="$(jq -n \
  --argjson good "$good_json" \
  --argjson bad "$bad_json" \
  --argjson dimensions "$dimension_json" \
  --argjson candidate_questions "$candidate_question_json" \
  --argjson final_questions "$final_question_json" \
  --argjson evaluation_runs "$evaluation_runs" \
  --argjson good_runs "$good_run_json" \
  --argjson bad_runs "$bad_run_json" \
  --arg git_sha "$BIN_EVAL_GIT_SHA" \
  --arg endpoint_class "$BIN_EVAL_ENDPOINT_CLASS" \
  --arg fixture_version "$BIN_EVAL_FIXTURE_VERSION" \
  --arg model_profile "${BIN_EVAL_MODEL_PROFILE:-unknown}" \
  --argjson success_count "$evaluation_success_count" '
  def mean: if length == 0 then 0 else add / length end;
  {
    case_count: ($good | length),
    dimension_counts: $dimensions,
    candidate_question_counts: $candidate_questions,
    final_question_counts: $final_questions,
    total_final_questions: ($final_questions | add // 0),
    evaluation_runs: $evaluation_runs,
    good_answer_run_pass_rates: $good_runs,
    bad_answer_run_pass_rates: $bad_runs,
    good_answer_mean_pass_rate: ($good | mean),
    bad_answer_mean_pass_rate: ($bad | mean),
    mean_pass_rate_gap: (($good | mean) - ($bad | mean)),
    all_checklists_succeeded: true,
    evaluation_success_count: $success_count,
    judgment_coverage: 1.0,
    limit_hit_count: 0,
    evidence: {
      git_sha: $git_sha,
      endpoint_class: $endpoint_class,
      fixture_version: $fixture_version,
      model_profile: $model_profile
    }
  }')"
echo "$metrics" | tee "$DEBUG_DIR/summary.json"

echo "$metrics" | jq -e '
  .case_count >= 2 and
  .evaluation_runs == 3 and
  all(.good_answer_run_pass_rates[]; . >= 0.70) and
  all(.bad_answer_run_pass_rates[]; . <= 0.60) and
  .good_answer_mean_pass_rate >= 0.80 and
  .bad_answer_mean_pass_rate <= 0.50 and
  .mean_pass_rate_gap >= 0.30 and
  .all_checklists_succeeded == true and
  .evaluation_success_count >= 4 and
  .judgment_coverage == 1.0 and
  .limit_hit_count == 0 and
  all(.final_question_counts[]; . >= 8) and
  (.evidence.git_sha | length > 0) and
  (.evidence.endpoint_class | length > 0) and
  (.evidence.fixture_version | length > 0) and
  (.evidence.model_profile | length > 0)
' >/dev/null
