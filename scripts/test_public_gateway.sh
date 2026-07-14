#!/usr/bin/env bash
# TEST-110
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEMPLATE="${ROOT_DIR}/deploy/compose/nginx-public.conf.template"
[[ -f "$TEMPLATE" ]] || { echo "missing gateway template: ${TEMPLATE}" >&2; exit 1; }

bin_eval_require_tools() {
  local tool
  for tool in "$@"; do
    command -v "$tool" >/dev/null || { echo "missing required tool: ${tool}" >&2; exit 1; }
  done
}
bin_eval_require_tools curl docker jq seq

suffix="$$"
backend_port=$((22000 + suffix % 1000 * 2))
gateway_port=$((backend_port + 1))
backend_name="bin-eval-public-backend-${suffix}"
gateway_name="bin-eval-public-gateway-${suffix}"
token="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
tmp_dir="$(mktemp -d)"

cleanup() {
  docker rm -f "$gateway_name" "$backend_name" >/dev/null 2>&1 || true
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

printf '%s\n' \
  'server {' \
  "    listen 127.0.0.1:${backend_port};" \
  '    location / {' \
  '        default_type application/json;' \
  "        return 200 '{\"backend\":\"ok\"}';" \
  '    }' \
  '}' >"${tmp_dir}/backend.conf"

docker run -d \
  --name "$backend_name" \
  --network host \
  -v "${tmp_dir}/backend.conf:/etc/nginx/conf.d/default.conf:ro" \
  nginx:1.28.2-alpine >/dev/null

docker run -d \
  --name "$gateway_name" \
  --network host \
  -e 'NGINX_ENVSUBST_FILTER=^BIN_EVAL_PUBLIC_(BEARER_TOKEN|GATEWAY_PORT|BACKEND_URL)$' \
  -e "BIN_EVAL_PUBLIC_BEARER_TOKEN=${token}" \
  -e "BIN_EVAL_PUBLIC_GATEWAY_PORT=${gateway_port}" \
  -e "BIN_EVAL_PUBLIC_BACKEND_URL=http://127.0.0.1:${backend_port}" \
  -v "${TEMPLATE}:/etc/nginx/templates/default.conf.template:ro" \
  nginx:1.28.2-alpine >/dev/null

base_url="http://127.0.0.1:${gateway_port}"
for _ in $(seq 1 30); do
  code="$(curl -sS -o /dev/null -w '%{http_code}' "${base_url}/healthz" 2>/dev/null || true)"
  [[ "$code" == "204" ]] && break
  sleep 0.2
done
[[ "${code:-000}" == "204" ]] || { docker logs "$gateway_name" >&2; exit 1; }

assert_code() {
  local expected="$1"
  shift
  local actual
  actual="$(curl -sS -o /dev/null -w '%{http_code}' "$@")"
  [[ "$actual" == "$expected" ]] || {
    echo "HTTP status ${actual}, want ${expected}: $*" >&2
    exit 1
  }
}

assert_code 204 "${base_url}/healthz"
assert_code 401 "${base_url}/checklists/missing"
assert_code 401 -H 'Authorization: Bearer invalid' "${base_url}/checklists/missing"

response="$(curl -fsS -H "Authorization: Bearer ${token}" "${base_url}/checklists/missing")"
jq -e '.backend == "ok"' <<<"$response" >/dev/null

dd if=/dev/zero of="${tmp_dir}/oversized.bin" bs=1048577 count=1 status=none
assert_code 413 -X POST -H "Authorization: Bearer ${token}" --data-binary "@${tmp_dir}/oversized.bin" "${base_url}/evaluations"

rate_codes="${tmp_dir}/rate-codes.txt"
for _ in $(seq 1 80); do
  curl -sS -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer ${token}" "${base_url}/checklists/missing" >>"$rate_codes"
done
grep -qx '429' "$rate_codes" || { echo "gateway did not enforce rate limit" >&2; exit 1; }

gateway_logs="$(docker logs "$gateway_name" 2>&1)"
if grep -Fq "$token" <<<"$gateway_logs"; then
  echo "gateway logs exposed bearer token" >&2
  exit 1
fi

printf 'public gateway behavior ok health=204 missing=401 invalid=401 valid=200 oversized=413 rate_limited=429 logs=redacted\n'
