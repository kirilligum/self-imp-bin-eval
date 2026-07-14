#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${ROOT_DIR}/scripts/lib/local_env.sh"
bin_eval_require_tools curl jq tailscale

JSON=false
[[ "${1:-}" == "--json" ]] && JSON=true
public_env_file="$(bin_eval_public_env_file "$ROOT_DIR")"

if [[ -f "$public_env_file" ]]; then
  bin_eval_load_public_env "$ROOT_DIR"
fi

gateway_port="${BIN_EVAL_PUBLIC_GATEWAY_PORT:-18081}"
https_port="${BIN_EVAL_PUBLIC_HTTPS_PORT:-8443}"
public_url="${BIN_EVAL_PUBLIC_URL:-}"
gateway_health="$(curl -sS -o /dev/null -w '%{http_code}' "http://127.0.0.1:${gateway_port}/healthz" 2>/dev/null || true)"
gateway_unauthorized="$(curl -sS -o /dev/null -w '%{http_code}' "http://127.0.0.1:${gateway_port}/checklists/00000000-0000-0000-0000-000000000000" 2>/dev/null || true)"
gateway_authorized="000"
if [[ -n "${BIN_EVAL_PUBLIC_BEARER_TOKEN:-}" ]]; then
  gateway_authorized="$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer ${BIN_EVAL_PUBLIC_BEARER_TOKEN}" "http://127.0.0.1:${gateway_port}/checklists/00000000-0000-0000-0000-000000000000" 2>/dev/null || true)"
fi
funnel_text="$(tailscale funnel status 2>&1 || true)"
funnel_active=false
if [[ -n "$public_url" ]] && grep -Fq "$public_url" <<<"$funnel_text"; then
  funnel_active=true
fi
public_health="000"
if [[ -n "$public_url" ]]; then
  public_health="$(curl -sS -o /dev/null -w '%{http_code}' "${public_url}/healthz" 2>/dev/null || true)"
fi

summary="$(jq -n \
  --arg public_url "$public_url" \
  --argjson https_port "$https_port" \
  --arg gateway_health "$gateway_health" \
  --arg gateway_unauthorized "$gateway_unauthorized" \
  --arg gateway_authorized "$gateway_authorized" \
  --arg public_health "$public_health" \
  --arg funnel_active "$funnel_active" \
  --arg env_exists "$([[ -f "$public_env_file" ]] && printf true || printf false)" \
  --arg token_set "$([[ -n "${BIN_EVAL_PUBLIC_BEARER_TOKEN:-}" ]] && printf true || printf false)" '
  {
    public_url: $public_url,
    https_port: $https_port,
    gateway: {
      health_http_code: $gateway_health,
      unauthorized_http_code: $gateway_unauthorized,
      authorized_api_http_code: $gateway_authorized
    },
    funnel: {active: ($funnel_active == "true")},
    public_health_http_code: $public_health,
    env_file_exists: ($env_exists == "true"),
    bearer_token_set: ($token_set == "true"),
    secrets: "redacted"
  }')"

if [[ "$JSON" == "true" ]]; then
  printf '%s\n' "$summary"
else
  jq -r '
    "public_url=\(.public_url)",
    "gateway_health=\(.gateway.health_http_code)",
    "gateway_unauthorized=\(.gateway.unauthorized_http_code)",
    "gateway_authorized_api=\(.gateway.authorized_api_http_code)",
    "funnel_active=\(.funnel.active)",
    "public_health=\(.public_health_http_code)",
    "secrets=redacted"
  ' <<<"$summary"
fi
