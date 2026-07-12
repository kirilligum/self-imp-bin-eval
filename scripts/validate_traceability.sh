#!/usr/bin/env bash
set -euo pipefail

# TEST-021

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

MATRIX="docs/test-matrix.yml"

fail() {
  echo "$1" >&2
  exit 1
}

[[ -f "$MATRIX" ]] || fail "missing $MATRIX"

mapfile -t matrix_ids < <(awk '/^[[:space:]]*- id: TEST-[0-9]{3}$/ {print $3}' "$MATRIX" | sort -u)
[[ "${#matrix_ids[@]}" -gt 0 ]] || fail "$MATRIX contains no TEST ids"

while IFS= read -r id; do
  [[ -n "$id" ]] || continue
  count="$(printf '%s\n' "${matrix_ids[@]}" | awk -v id="$id" '$0 == id {count++} END {print count+0}')"
  [[ "$count" -eq 1 ]] || fail "$MATRIX has duplicate or missing entry for $id"
done < <(printf '%s\n' "${matrix_ids[@]}")

mapfile -t source_tags < <(
  rg -n '^[[:space:]]*(//|#) TEST-[0-9]{3}$' \
    internal scripts docs Makefile \
    --glob '!debug/**' \
    --glob '!bin/**' \
    --glob '!docs/test-matrix.yml' |
  sed -E 's/.*(TEST-[0-9]{3}).*/\1/' |
  sort -u
)
[[ "${#source_tags[@]}" -gt 0 ]] || fail "no source TEST tags found"

for tag in "${source_tags[@]}"; do
  if ! printf '%s\n' "${matrix_ids[@]}" | grep -Fxq "$tag"; then
    fail "source TEST tag is not represented in $MATRIX: $tag"
  fi
done

for id in "${matrix_ids[@]}"; do
  if ! printf '%s\n' "${source_tags[@]}" | grep -Fxq "$id"; then
    fail "$MATRIX entry has no matching source tag: $id"
  fi
done

awk '
  /^[[:space:]]*- id: TEST-[0-9]{3}$/ {
    if (current != "") {
      if (!has_reqs) {
        printf "matrix entry %s has no REQ list\n", current > "/dev/stderr"
        exit 1
      }
      if (!has_command) {
        printf "matrix entry %s has no command\n", current > "/dev/stderr"
        exit 1
      }
    }
    current = $3
    has_reqs = 0
    has_command = 0
  }
  current != "" && /^[[:space:]]*reqs: \[.*REQ-[0-9]{3}.*\]/ { has_reqs = 1 }
  current != "" && /^[[:space:]]*command: .+/ { has_command = 1 }
  END {
    if (current != "") {
      if (!has_reqs) {
        printf "matrix entry %s has no REQ list\n", current > "/dev/stderr"
        exit 1
      }
      if (!has_command) {
        printf "matrix entry %s has no command\n", current > "/dev/stderr"
        exit 1
      }
    }
  }
' "$MATRIX"

awk '
  /^[[:space:]]{6}- [A-Za-z0-9_./-]+$/ {
    path = $2
    if (system("[ -e \"" path "\" ]") != 0) {
      printf "matrix references missing file: %s\n", path > "/dev/stderr"
      exit 1
    }
  }
' "$MATRIX"

echo "traceability matrix ok"
