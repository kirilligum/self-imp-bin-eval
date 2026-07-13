#!/usr/bin/env bash
set -euo pipefail


ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

DEBUG_DIR="${1:-debug/smoke}"
REPORT="${DEBUG_DIR}/invariant_report.json"

fail() {
  echo "$1" >&2
  exit 1
}

[[ -d "$DEBUG_DIR" ]] || fail "missing smoke debug directory: $DEBUG_DIR"

tmp_report="$(mktemp)"
trap 'rm -f "$tmp_report"' EXIT

printf '[' > "$tmp_report"
first_case=1

for case_dir in "$DEBUG_DIR"/*; do
  [[ -d "$case_dir" ]] || continue
  [[ -f "$case_dir/checklist.json" ]] || continue
  case_name="$(basename "$case_dir")"

  jq -e '
    . as $root |
    $root.status == "succeeded" and
    ($root.evaluation_runs | type == "number" and . > 0 and . % 2 == 1) and
    ($root.dimensions | type == "array" and length > 0) and
    ($root.candidate_questions | type == "array" and length > 0) and
    ($root.weights | type == "array" and length == ($root.candidate_questions | length)) and
    ($root.questions | type == "array" and length >= 8) and
    (($root.candidate_questions | map(.id) | unique | length) == ($root.candidate_questions | length)) and
    (($root.weights | map(.candidate_question_id) | sort) == ($root.candidate_questions | map(.id) | sort)) and
    all($root.weights[]; (.weight | type == "number") and .weight >= 0 and .weight <= 4) and
    all($root.questions[]; .id != "" and .dimension_id != "" and .source_candidate_id != "" and .question != "") and
    all($root.questions[]; . as $q |
      any($root.dimensions[]; .id == $q.dimension_id) and
      any($root.candidate_questions[]; .id == $q.source_candidate_id and .dimension_id == $q.dimension_id) and
      any($root.weights[]; .candidate_question_id == $q.source_candidate_id and .weight > 0)
    ) and
    all($root.weights[]; . as $w |
      if $w.weight == 0 then
        all($root.questions[]; .source_candidate_id != $w.candidate_question_id)
      else
        any($root.questions[]; .source_candidate_id == $w.candidate_question_id)
      end
    )
  ' "$case_dir/checklist.json" >/dev/null || fail "checklist invariant failed for $case_name"

  final_count="$(jq -r '.questions | length' "$case_dir/checklist.json")"
  evaluation_runs="$(jq -r '.evaluation_runs' "$case_dir/checklist.json")"
  checklist_id="$(jq -r '.checklist_id' "$case_dir/checklist.json")"
  case_rates='{}'

  for quality in good bad; do
    eval_file="$case_dir/evaluation_${quality}.json"
    [[ -f "$eval_file" ]] || fail "missing evaluation capture: $eval_file"

    jq -e --argjson final_count "$final_count" --argjson evaluation_runs "$evaluation_runs" '
      .status == "succeeded" and
      (.total_possible_points == $final_count) and
      (.judgments | type == "array" and length == $final_count) and
      ((.judgments | map(.question_id) | sort) == (.judgments | map(.question_id) | unique | sort)) and
      all(.judgments[];
        (.answer == "yes" or .answer == "no") and
        (.runs | type == "array" and length == $evaluation_runs) and
        ((.runs | map(.run_index)) == [range(1; $evaluation_runs + 1)]) and
        all(.runs[]; (.answer == "yes" or .answer == "no") and (.evidence | type == "string" and length > 0)) and
        (. as $judgment |
          (($judgment.runs | map(select(.answer == "yes")) | length) > ($evaluation_runs / 2)) as $majority_yes |
          (($majority_yes and $judgment.answer == "yes") or (($majority_yes | not) and $judgment.answer == "no")))
      )
    ' "$eval_file" >/dev/null || fail "evaluation invariant failed for $case_name/$quality"

    computed="$(jq -n --slurpfile checklist "$case_dir/checklist.json" --slurpfile evaluation "$eval_file" '
      ($checklist[0].questions | map(.id)) as $question_ids |
      ($evaluation[0].judgments | map(select(.answer == "yes")) | length) as $yes_count |
      ($evaluation[0].judgments | map(select(.answer == "no") | .question_id)) as $failed_ids |
      {
        satisfied_points: $yes_count,
        total_possible_points: ($question_ids | length),
        checklist_pass_rate: (if ($question_ids | length) == 0 then 0 else ($yes_count / ($question_ids | length)) end),
        failed_question_ids: $failed_ids
      }
    ')"

    jq -e --argjson computed "$computed" '
      .satisfied_points == $computed.satisfied_points and
      .total_possible_points == $computed.total_possible_points and
      ((.checklist_pass_rate - $computed.checklist_pass_rate) | fabs) < 0.000000001 and
      (.failed_question_ids == $computed.failed_question_ids)
    ' "$eval_file" >/dev/null || fail "score recomputation failed for $case_name/$quality"

    rate="$(jq -r '.checklist_pass_rate' "$eval_file")"
    run_rates="$(jq --argjson final_count "$final_count" '[range(1; (.judgments[0].runs | length) + 1) as $run | ([.judgments[].runs[] | select(.run_index == $run and .answer == "yes")] | length) / $final_count]' "$eval_file")"
    case_rates="$(jq -n --argjson current "$case_rates" --arg quality "$quality" --argjson rate "$rate" --argjson run_rates "$run_rates" '$current + {($quality): {aggregate: $rate, runs: $run_rates}}')"
  done

  case_report="$(jq -n \
    --arg case_name "$case_name" \
    --arg checklist_id "$checklist_id" \
    --argjson final_count "$final_count" \
    --argjson rates "$case_rates" \
    --argjson evaluation_runs "$evaluation_runs" \
    '{case: $case_name, checklist_id: $checklist_id, final_question_count: $final_count, evaluation_runs: $evaluation_runs, pass_rates: $rates}')"
  if [[ "$first_case" -eq 0 ]]; then
    printf ',' >> "$tmp_report"
  fi
  first_case=0
  printf '%s' "$case_report" >> "$tmp_report"
done

printf ']' >> "$tmp_report"

case_count="$(jq 'length' "$tmp_report")"
[[ "$case_count" -gt 0 ]] || fail "no smoke cases found under $DEBUG_DIR"

jq -n --slurpfile cases "$tmp_report" \
  '{case_count: ($cases[0] | length), cases: $cases[0]}' > "$REPORT"

echo "smoke invariants ok: $REPORT"
