#!/usr/bin/env bash
# TEST-111
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${ROOT_DIR}/scripts/lib/local_env.sh"
bin_eval_require_tools curl docker jq

if [[ -z "${BIN_EVAL_PUBLIC_URL:-}" || -z "${BIN_EVAL_PUBLIC_BEARER_TOKEN:-}" ]]; then
  bin_eval_load_public_env "$ROOT_DIR"
fi
: "${BIN_EVAL_PUBLIC_URL:?public URL is required}"
: "${BIN_EVAL_PUBLIC_BEARER_TOKEN:?public bearer token is required}"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

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

root_code="$(curl -sS -D "${tmp_dir}/root-headers.txt" -o "${tmp_dir}/root.json" -w '%{http_code}' "${BIN_EVAL_PUBLIC_URL}/")"
[[ "$root_code" == "200" ]] || { echo "public root HTTP ${root_code}, want 200" >&2; exit 1; }
jq -e '.service == "bin-eval" and .authentication == "bearer" and .health == "/healthz"' "${tmp_dir}/root.json" >/dev/null
for header in \
  'strict-transport-security: max-age=31536000; includeSubDomains' \
  'x-content-type-options: nosniff' \
  'referrer-policy: no-referrer' \
  'x-frame-options: DENY' \
  "content-security-policy: default-src 'none'; frame-ancestors 'none'; base-uri 'none'"; do
  grep -Fqi "$header" "${tmp_dir}/root-headers.txt" || { echo "public root missing security header: ${header}" >&2; exit 1; }
done

missing_code="$(curl -sS -D "${tmp_dir}/missing-headers.txt" -o "${tmp_dir}/missing.json" -w '%{http_code}' "${BIN_EVAL_PUBLIC_URL}/checklists/00000000-0000-0000-0000-000000000000")"
[[ "$missing_code" == "401" ]] || { echo "public missing-token HTTP ${missing_code}, want 401" >&2; exit 1; }
jq -e '.error == "authorization_required"' "${tmp_dir}/missing.json" >/dev/null
grep -Fqi 'www-authenticate: Bearer realm="bin-eval"' "${tmp_dir}/missing-headers.txt" || { echo "public 401 missing bearer challenge" >&2; exit 1; }
assert_code 401 -H 'Authorization: Bearer invalid' "${BIN_EVAL_PUBLIC_URL}/checklists/00000000-0000-0000-0000-000000000000"
assert_code 404 -H "Authorization: Bearer ${BIN_EVAL_PUBLIC_BEARER_TOKEN}" "${BIN_EVAL_PUBLIC_URL}/checklists/00000000-0000-0000-0000-000000000000"

status_json="$(${ROOT_DIR}/scripts/status-public.sh --json)"
jq -e '
  .gateway.health_http_code == "204" and
  .gateway.unauthorized_http_code == "401" and
  .gateway.authorized_api_http_code == "404" and
  .tunnel.active == true and
  .public_health_http_code == "204" and
  .secrets == "redacted"
' <<<"$status_json" >/dev/null
if grep -Fq "$BIN_EVAL_PUBLIC_BEARER_TOKEN" <<<"$status_json"; then
  echo "public status exposed bearer token" >&2
  exit 1
fi

echo "public ingress ok root=200 security_headers=present health=204 missing=401 invalid=401 authorized_api=404 tunnel=active token=redacted"
