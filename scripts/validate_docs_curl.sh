#!/usr/bin/env bash
set -euo pipefail

# TEST-007

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOC="${ROOT_DIR}/docs/curl.md"

fail() {
  echo "$1" >&2
  exit 1
}

[[ -f "$DOC" ]] || fail "missing docs/curl.md"

for pattern in \
  '^## Local Service Commands$' \
  '^## Copy-Paste Curl Sequence$' \
  '^## Live Curl Validation$' \
  'make install-local' \
  'make start-local' \
  'make status-local' \
  'make test-live-curl' \
  'POST /checklists' \
  'GET /checklists/' \
  'POST /evaluations' \
  'GET /evaluations/' \
  'checklist_id' \
  'dimensions' \
  'candidate_questions' \
  'candidate_question_id' \
  'final_question_count' \
  'evaluation_id' \
  'satisfied_points' \
  'total_possible_points' \
  'checklist_pass_rate' \
  'failed_question_ids' \
  'scripts/live_curl_example.sh'; do
  rg -n "$pattern" "$DOC" >/dev/null || fail "docs/curl.md missing pattern: $pattern"
done

if rg -n 'Bearer [A-Za-z0-9._-]{20,}|sk-[A-Za-z0-9]' "$DOC" >/dev/null; then
  fail "docs/curl.md appears to contain a secret literal"
fi

if rg -n 'set status\b' "$DOC" >/dev/null; then
  fail "docs/curl.md uses Fish read-only variable name: status"
fi

echo "docs curl contract ok"
