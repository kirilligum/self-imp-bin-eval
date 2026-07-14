#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${ROOT_DIR}/scripts/lib/local_env.sh"

action="${1:-}"
if [[ "$action" != "start" && "$action" != "stop" ]]; then
  echo "usage: $0 start|stop" >&2
  exit 2
fi

bin_eval_require_tools curl docker jq tailscale
MODE="${BIN_EVAL_SYSTEMD_MODE:-$(bin_eval_systemd_mode)}"
export BIN_EVAL_ENV_FILE
BIN_EVAL_ENV_FILE="$(bin_eval_default_env_file_for_mode "$ROOT_DIR" "$MODE")"
PUBLIC_ENV_FILE="$(bin_eval_public_env_file "$ROOT_DIR")"
[[ -f "$PUBLIC_ENV_FILE" ]] || { echo "missing public env: run make install-public" >&2; exit 1; }

bin_eval_load_local_env "$ROOT_DIR"
bin_eval_load_public_env "$ROOT_DIR"

if [[ ! "${BIN_EVAL_PUBLIC_BEARER_TOKEN:-}" =~ ^[0-9a-f]{64}$ ]]; then
  echo "BIN_EVAL_PUBLIC_BEARER_TOKEN must be a 32-byte lowercase hex token" >&2
  exit 1
fi
gateway_port="${BIN_EVAL_PUBLIC_GATEWAY_PORT:-18081}"
https_port="${BIN_EVAL_PUBLIC_HTTPS_PORT:-8443}"
[[ "$gateway_port" =~ ^[0-9]+$ ]] || { echo "invalid gateway port" >&2; exit 1; }
[[ "$https_port" == "8443" ]] || { echo "public HTTPS port must be 8443" >&2; exit 1; }

compose=(docker compose --env-file "$BIN_EVAL_ENV_FILE" --env-file "$PUBLIC_ENV_FILE" -f "${ROOT_DIR}/deploy/compose/docker-compose.yml" --profile public)

if [[ "$action" == "stop" ]]; then
  tailscale funnel --https="$https_port" off >/dev/null 2>&1 || true
  "${compose[@]}" stop public-gateway >/dev/null 2>&1 || true
  echo "bin-eval public ingress stopped; local API and worker remain running"
  exit 0
fi

local_api_code="$(curl -sS -o /dev/null -w '%{http_code}' "http://127.0.0.1:8080/checklists/00000000-0000-0000-0000-000000000000" 2>/dev/null || true)"
[[ "$local_api_code" == "404" ]] || { echo "local API is not ready: HTTP ${local_api_code:-000}" >&2; exit 1; }

"${compose[@]}" up -d public-gateway
for _ in $(seq 1 60); do
  gateway_health="$(curl -sS -o /dev/null -w '%{http_code}' "http://127.0.0.1:${gateway_port}/healthz" 2>/dev/null || true)"
  [[ "$gateway_health" == "204" ]] && break
  sleep 1
done
[[ "${gateway_health:-000}" == "204" ]] || { "${compose[@]}" logs --no-color public-gateway >&2; exit 1; }

unauthorized_code="$(curl -sS -o /dev/null -w '%{http_code}' "http://127.0.0.1:${gateway_port}/checklists/00000000-0000-0000-0000-000000000000")"
authorized_code="$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer ${BIN_EVAL_PUBLIC_BEARER_TOKEN}" "http://127.0.0.1:${gateway_port}/checklists/00000000-0000-0000-0000-000000000000")"
[[ "$unauthorized_code" == "401" ]] || { echo "gateway accepted unauthenticated request: HTTP ${unauthorized_code}" >&2; exit 1; }
[[ "$authorized_code" == "404" ]] || { echo "gateway did not reach local API: HTTP ${authorized_code}" >&2; exit 1; }

tailscale funnel --yes --bg --https="$https_port" "http://127.0.0.1:${gateway_port}"

public_url="${BIN_EVAL_PUBLIC_URL:-}"
if [[ -z "$public_url" ]]; then
  dns_name="$(tailscale status --json | jq -er '.Self.DNSName | rtrimstr(".")')"
  public_url="https://${dns_name}:${https_port}"
fi
for _ in $(seq 1 120); do
  public_health="$(curl -sS -o /dev/null -w '%{http_code}' "${public_url}/healthz" 2>/dev/null || true)"
  [[ "$public_health" == "204" ]] && break
  sleep 5
done
[[ "${public_health:-000}" == "204" ]] || { echo "public Funnel did not become ready: ${public_url}" >&2; exit 1; }

echo "bin-eval public ingress started url=${public_url} auth=bearer gateway=healthy funnel=active token=redacted"
