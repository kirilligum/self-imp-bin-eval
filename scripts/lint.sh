#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

mapfile -t files < <(find . -name '*.go' -not -path './.git/*' -print)
if [[ "${#files[@]}" -gt 0 ]]; then
  unformatted="$(gofmt -l "${files[@]}")"
  if [[ -n "$unformatted" ]]; then
    printf '%s\n' "$unformatted" >&2
    exit 1
  fi
fi

scripts/validate_local_runtime_contract.sh
scripts/validate_docs_curl.sh
go vet ./...
