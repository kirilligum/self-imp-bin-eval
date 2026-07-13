#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
source scripts/lib/local_env.sh
bin_eval_load_env_file deploy/compose/.env.example false
bin_eval_require_tools go jq git

DEBUG_DIR="${1:?debug capture directory is required}"
OUTPUT_DIR="${DEBUG_DIR}/llm-artifacts"
rm -rf "$OUTPUT_DIR"

if [[ -z "${BIN_EVAL_GIT_SHA:-}" ]]; then
  export BIN_EVAL_GIT_SHA="$(git rev-parse HEAD)"
fi

declare -A entity_prefixes=()
while IFS= read -r checklist_id; do
  [[ -n "$checklist_id" && "$checklist_id" != "null" ]] || continue
  entity_prefixes["checklists/${checklist_id}/"]=1
done < <(find "$DEBUG_DIR" -name checklist.json -type f -print0 | xargs -0 -r jq -r 'select(.status == "succeeded") | .checklist_id')
while IFS= read -r evaluation_id; do
  [[ -n "$evaluation_id" && "$evaluation_id" != "null" ]] || continue
  entity_prefixes["evaluations/${evaluation_id}/"]=1
done < <(find "$DEBUG_DIR" -name 'evaluation*.json' -type f -print0 | xargs -0 -r jq -r 'select(.status == "succeeded") | .evaluation_id')

if (( ${#entity_prefixes[@]} == 0 )); then
  echo "no succeeded checklist or evaluation captures found under ${DEBUG_DIR}" >&2
  exit 1
fi

args=()
for prefix in "${!entity_prefixes[@]}"; do
  args+=(--prefix "$prefix")
done
go run ./internal/cmd/captureartifacts --output "$OUTPUT_DIR" "${args[@]}"

manifest="${OUTPUT_DIR}/manifest.json"
jq -e '
  (.git_sha | type == "string" and length > 0) and
  (.endpoint_class | type == "string" and length > 0) and
  (.fixture_version | type == "string" and length > 0) and
  (.model_profile | type == "string" and length > 0) and
  (.objects | type == "array" and length > 0) and
  all(.objects[]; (.key | length > 0) and (.size >= 0) and (.sha256 | test("^[0-9a-f]{64}$")))
' "$manifest" >/dev/null

require_pair() {
  local prefix="$1"
  jq -e --arg prefix "$prefix" '
    any(.objects[]; (.key | startswith($prefix)) and (.key | endswith("/request.json"))) and
    any(.objects[]; (.key | startswith($prefix)) and (.key | endswith("/response.body")))
  ' "$manifest" >/dev/null || {
    echo "missing exact request/response artifact pair for ${prefix}" >&2
    exit 1
  }
}

while IFS= read -r checklist_file; do
  checklist_id="$(jq -r '.checklist_id' "$checklist_file")"
  require_pair "checklists/${checklist_id}/llm/dimension_analysis/"
  require_pair "checklists/${checklist_id}/llm/weight_assignment/"
  while IFS= read -r dimension_id; do
    require_pair "checklists/${checklist_id}/llm/question_generation/${dimension_id}/"
  done < <(jq -r '.dimensions[].id' "$checklist_file")
  while IFS= read -r candidate_id; do
    require_pair "checklists/${checklist_id}/llm/question_splitting/${candidate_id}/"
  done < <(jq -r '.weights[] | select(.weight > 1) | .candidate_question_id' "$checklist_file")
done < <(find "$DEBUG_DIR" -name checklist.json -type f | sort)

while IFS= read -r evaluation_file; do
  evaluation_id="$(jq -r '.evaluation_id' "$evaluation_file")"
  while IFS= read -r run_index; do
    require_pair "evaluations/${evaluation_id}/llm/binary_judging/run-${run_index}/"
  done < <(jq -r '.judgments[0].runs[].run_index' "$evaluation_file")
done < <(find "$DEBUG_DIR" -name 'evaluation*.json' -type f | sort)

artifact_count="$(jq '.objects | length' "$manifest")"
if [[ -f "${DEBUG_DIR}/summary.json" ]]; then
  tmp_summary="$(mktemp)"
  jq --arg manifest "llm-artifacts/manifest.json" --argjson artifact_count "$artifact_count" \
    '. + {artifact_manifest: $manifest, artifact_count: $artifact_count}' \
    "${DEBUG_DIR}/summary.json" > "$tmp_summary"
  mv "$tmp_summary" "${DEBUG_DIR}/summary.json"
fi

echo "exact artifact evidence ok: ${manifest}"
