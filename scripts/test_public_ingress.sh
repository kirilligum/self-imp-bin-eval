#!/usr/bin/env bash
# TEST-111
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${ROOT_DIR}/scripts/lib/local_env.sh"
bin_eval_require_tools curl jq tailscale

if [[ -z "${BIN_EVAL_PUBLIC_URL:-}" || -z "${BIN_EVAL_PUBLIC_BEARER_TOKEN:-}" ]]; then
  bin_eval_load_public_env "$ROOT_DIR"
fi
: "${BIN_EVAL_PUBLIC_URL:?public URL is required}"
: "${BIN_EVAL_PUBLIC_BEARER_TOKEN:?public bearer token is required}"

assert_code() {
  local expected="$1"
  shift
  local actual
  actual="$(curl -sS -o /dev/null -w '%{http_code}' "$@")"
  [[ "$actual" == "$expected" ]] || {
    echo "public ingress HTTP ${actual}, want ${expected}" >&2
    exit 1
  }
}

assert_code 204 "${BIN_EVAL_PUBLIC_URL}/healthz"
assert_code 401 "${BIN_EVAL_PUBLIC_URL}/checklists/00000000-0000-0000-0000-000000000000"
assert_code 401 -H 'Authorization: Bearer invalid' "${BIN_EVAL_PUBLIC_URL}/checklists/00000000-0000-0000-0000-000000000000"
assert_code 404 -H "Authorization: Bearer ${BIN_EVAL_PUBLIC_BEARER_TOKEN}" "${BIN_EVAL_PUBLIC_URL}/checklists/00000000-0000-0000-0000-000000000000"

status_json="$(${ROOT_DIR}/scripts/status-public.sh --json)"
jq -e '
  .gateway.health_http_code == "204" and
  .gateway.unauthorized_http_code == "401" and
  .gateway.authorized_api_http_code == "404" and
  .funnel.active == true and
  .public_health_http_code == "204" and
  .secrets == "redacted"
' <<<"$status_json" >/dev/null
if grep -Fq "$BIN_EVAL_PUBLIC_BEARER_TOKEN" <<<"$status_json"; then
  echo "public status exposed bearer token" >&2
  exit 1
fi

echo "public ingress ok health=204 missing=401 invalid=401 authorized_api=404 funnel=active token=redacted"
