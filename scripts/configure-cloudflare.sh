#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C
umask 077

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${ROOT_DIR}/scripts/lib/local_env.sh"
bin_eval_require_tools curl jq

bin_eval_load_env_file "${ROOT_DIR}/.env" false
bin_eval_load_public_env "$ROOT_DIR"

: "${CLOUDFLARE_API_TOKEN:?CLOUDFLARE_API_TOKEN is required for provisioning}"
: "${BIN_EVAL_PUBLIC_HOSTNAME:?public hostname is required}"
: "${BIN_EVAL_PUBLIC_URL:?public URL is required}"
: "${BIN_EVAL_CLOUDFLARED_TUNNEL_NAME:?Cloudflare tunnel name is required}"
: "${BIN_EVAL_CLOUDFLARED_ORIGIN:?Cloudflare tunnel origin is required}"

[[ "$BIN_EVAL_PUBLIC_HOSTNAME" == "bin-eval.prls.co" ]] || {
  echo "public hostname must be bin-eval.prls.co" >&2
  exit 1
}
[[ "$BIN_EVAL_PUBLIC_URL" == "https://${BIN_EVAL_PUBLIC_HOSTNAME}" ]] || {
  echo "public URL must be https://${BIN_EVAL_PUBLIC_HOSTNAME}" >&2
  exit 1
}
[[ "$BIN_EVAL_CLOUDFLARED_ORIGIN" == "http://127.0.0.1:${BIN_EVAL_PUBLIC_GATEWAY_PORT:-18081}" ]] || {
  echo "Cloudflare tunnel origin must target the loopback public gateway" >&2
  exit 1
}

api="https://api.cloudflare.com/client/v4"
zone_name="prls.co"
token_file="${ROOT_DIR}/deploy/local/bin-eval-cloudflared-token"

cf_get() {
  curl -sS -H "Authorization: Bearer ${CLOUDFLARE_API_TOKEN}" "${api}/$1"
}

cf_write() {
  local method="$1" path="$2" payload="$3"
  curl -sS -X "$method" \
    -H "Authorization: Bearer ${CLOUDFLARE_API_TOKEN}" \
    -H 'Content-Type: application/json' \
    --data "$payload" \
    "${api}/${path}"
}

require_success() {
  local operation="$1" response="$2"
  if ! jq -e '.success == true' <<<"$response" >/dev/null; then
    printf 'Cloudflare operation failed: %s\n' "$operation" >&2
    jq -r '.errors[]? | "error " + (.code | tostring) + ": " + .message' <<<"$response" >&2
    exit 1
  fi
}

zone_response="$(cf_get "zones?name=${zone_name}")"
require_success "resolve ${zone_name} zone" "$zone_response"
[[ "$(jq '.result | length' <<<"$zone_response")" == "1" ]] || {
  echo "expected exactly one Cloudflare zone named ${zone_name}" >&2
  exit 1
}
zone_id="$(jq -r '.result[0].id' <<<"$zone_response")"
account_id="$(jq -r '.result[0].account.id' <<<"$zone_response")"

tunnels_response="$(cf_get "accounts/${account_id}/cfd_tunnel?is_deleted=false&name=${BIN_EVAL_CLOUDFLARED_TUNNEL_NAME}")"
require_success "list bin-eval tunnels" "$tunnels_response"
tunnel_count="$(jq '[.result[] | select(.name == $name)] | length' --arg name "$BIN_EVAL_CLOUDFLARED_TUNNEL_NAME" <<<"$tunnels_response")"
if [[ "$tunnel_count" == "0" ]]; then
  tunnel_response="$(cf_write POST "accounts/${account_id}/cfd_tunnel" "$(jq -nc --arg name "$BIN_EVAL_CLOUDFLARED_TUNNEL_NAME" '{name:$name,config_src:"cloudflare"}')")"
  require_success "create bin-eval tunnel" "$tunnel_response"
  tunnel_id="$(jq -r '.result.id' <<<"$tunnel_response")"
elif [[ "$tunnel_count" == "1" ]]; then
  tunnel_id="$(jq -r --arg name "$BIN_EVAL_CLOUDFLARED_TUNNEL_NAME" '.result[] | select(.name == $name) | .id' <<<"$tunnels_response")"
  config_source="$(jq -r --arg name "$BIN_EVAL_CLOUDFLARED_TUNNEL_NAME" '.result[] | select(.name == $name) | .config_src' <<<"$tunnels_response")"
  [[ "$config_source" == "cloudflare" ]] || {
    echo "existing tunnel ${BIN_EVAL_CLOUDFLARED_TUNNEL_NAME} is not remotely managed" >&2
    exit 1
  }
else
  echo "multiple active tunnels are named ${BIN_EVAL_CLOUDFLARED_TUNNEL_NAME}" >&2
  exit 1
fi

config_payload="$(jq -nc \
  --arg hostname "$BIN_EVAL_PUBLIC_HOSTNAME" \
  --arg service "$BIN_EVAL_CLOUDFLARED_ORIGIN" \
  '{config:{ingress:[{hostname:$hostname,service:$service},{service:"http_status:404"}]}}')"
config_response="$(cf_write PUT "accounts/${account_id}/cfd_tunnel/${tunnel_id}/configurations" "$config_payload")"
require_success "configure bin-eval tunnel ingress" "$config_response"

dns_response="$(cf_get "zones/${zone_id}/dns_records?name=${BIN_EVAL_PUBLIC_HOSTNAME}")"
require_success "read bin-eval DNS record" "$dns_response"
dns_count="$(jq '.result | length' <<<"$dns_response")"
dns_target="${tunnel_id}.cfargotunnel.com"
dns_payload="$(jq -nc \
  --arg name "$BIN_EVAL_PUBLIC_HOSTNAME" \
  --arg content "$dns_target" \
  '{type:"CNAME",name:$name,content:$content,ttl:1,proxied:true}')"
if [[ "$dns_count" == "0" ]]; then
  dns_write_response="$(cf_write POST "zones/${zone_id}/dns_records" "$dns_payload")"
elif [[ "$dns_count" == "1" && "$(jq -r '.result[0].type' <<<"$dns_response")" == "CNAME" ]]; then
  dns_id="$(jq -r '.result[0].id' <<<"$dns_response")"
  dns_write_response="$(cf_write PUT "zones/${zone_id}/dns_records/${dns_id}" "$dns_payload")"
else
  echo "refusing to replace unexpected DNS records for ${BIN_EVAL_PUBLIC_HOSTNAME}" >&2
  exit 1
fi
require_success "write bin-eval DNS record" "$dns_write_response"

token_response="$(cf_get "accounts/${account_id}/cfd_tunnel/${tunnel_id}/token")"
require_success "get bin-eval tunnel token" "$token_response"
tunnel_token="$(jq -r '.result' <<<"$token_response")"
[[ "$tunnel_token" != "null" && ${#tunnel_token} -ge 100 ]] || {
  echo "Cloudflare returned an invalid tunnel token" >&2
  exit 1
}

install -d -m 0755 "$(dirname "$token_file")"
temporary="$(mktemp "${token_file}.tmp.XXXXXX")"
trap 'rm -f "$temporary"' EXIT
printf '%s' "$tunnel_token" >"$temporary"
chmod 0600 "$temporary"
mv -f "$temporary" "$token_file"
trap - EXIT

echo "configured Cloudflare tunnel name=${BIN_EVAL_CLOUDFLARED_TUNNEL_NAME} hostname=${BIN_EVAL_PUBLIC_HOSTNAME} origin=${BIN_EVAL_CLOUDFLARED_ORIGIN} token=redacted"
