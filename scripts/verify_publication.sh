#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

branch="$(git branch --show-current)"
[[ -n "$branch" ]] || { echo "detached HEAD" >&2; exit 1; }
git fetch origin "$branch" --quiet
[[ "$(git rev-parse HEAD)" == "$(git rev-parse "origin/${branch}")" ]] || {
  echo "HEAD is not published to origin/${branch}" >&2
  exit 1
}
[[ -z "$(git status --porcelain)" ]] || {
  echo "working tree is not clean" >&2
  git status --short >&2
  exit 1
}

printf 'published_commit=%s branch=%s\n' "$(git rev-parse HEAD)" "$branch"
