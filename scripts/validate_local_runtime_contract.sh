#!/usr/bin/env bash
set -euo pipefail

# TEST-001

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "$1" >&2
  exit 1
}

for target in install-local start-local stop-local status-local test-live-curl; do
  rg -n "^${target}:" Makefile >/dev/null || fail "missing Makefile target: ${target}"
done

for script in \
  scripts/install-local-systemd.sh \
  scripts/start-local.sh \
  scripts/stop-local.sh \
  scripts/status-local.sh \
  scripts/live_curl_example.sh \
  scripts/validate_litellm_responses.sh \
  scripts/validate_docs_curl.sh \
  scripts/docker-compose-local.sh \
  scripts/wait-for-tcp.sh; do
  [[ -x "$script" ]] || fail "script is missing or not executable: ${script}"
done

for unit in \
  deploy/systemd/bin-eval-deps.service.in \
  deploy/systemd/bin-eval-api.service.in \
  deploy/systemd/bin-eval-worker.service.in; do
  [[ -f "$unit" ]] || fail "missing systemd unit template: ${unit}"
  rg -n "EnvironmentFile=.*BIN_EVAL_ENV_FILE" "$unit" >/dev/null || fail "unit does not reference bin-eval env file: ${unit}"
done

[[ -f deploy/local/bin-eval.env.example ]] || fail "missing deploy/local/bin-eval.env.example"
rg -n '^BIN_EVAL_LISTEN_ADDR=127\.0\.0\.1:8080$' deploy/local/bin-eval.env.example >/dev/null || fail "local env example must bind to localhost"
rg -n '^BIN_EVAL_MODEL_PROFILE=gpt-5\.4-mini$' deploy/local/bin-eval.env.example >/dev/null || fail "local env example must use gpt-5.4-mini"
rg -n '^deploy/local/bin-eval\.env$' .gitignore >/dev/null || fail "deploy/local/bin-eval.env must be ignored"

for anchor in "## Local Service Commands" "## Copy-Paste Curl Sequence" "## Live Curl Validation"; do
  rg -n "^${anchor}$" docs/curl.md >/dev/null || fail "missing docs anchor: ${anchor}"
done

echo "local runtime contract ok"
