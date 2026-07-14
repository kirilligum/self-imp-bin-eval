#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${ROOT_DIR}/scripts/lib/local_env.sh"
bin_eval_require_tools curl docker gh jq openssl tailscale

public_env_file="$(bin_eval_public_env_file "$ROOT_DIR")"
if [[ ! -f "$public_env_file" ]]; then
  install -d -m 0755 "$(dirname "$public_env_file")"
  install -m 0600 "${ROOT_DIR}/deploy/local/bin-eval-public.env.example" "$public_env_file"
fi
chmod 0600 "$public_env_file"

bin_eval_load_public_env "$ROOT_DIR"
if [[ ! "${BIN_EVAL_PUBLIC_BEARER_TOKEN:-}" =~ ^[0-9a-f]{64}$ ]]; then
  token="$(openssl rand -hex 32)"
  sed -i "s/^BIN_EVAL_PUBLIC_BEARER_TOKEN=.*/BIN_EVAL_PUBLIC_BEARER_TOKEN=${token}/" "$public_env_file"
fi

https_port="${BIN_EVAL_PUBLIC_HTTPS_PORT:-8443}"
dns_name="$(tailscale status --json | jq -er '.Self.DNSName | rtrimstr(".")')"
public_url="https://${dns_name}:${https_port}"
sed -i "s|^BIN_EVAL_PUBLIC_URL=.*|BIN_EVAL_PUBLIC_URL=${public_url}|" "$public_env_file"
chmod 0600 "$public_env_file"

bin_eval_load_public_env "$ROOT_DIR"
printf '%s' "$BIN_EVAL_PUBLIC_BEARER_TOKEN" | gh secret set BIN_EVAL_PUBLIC_BEARER_TOKEN --repo kirilligum/self-imp-bin-eval
gh variable set BIN_EVAL_PUBLIC_URL --body "$BIN_EVAL_PUBLIC_URL" --repo kirilligum/self-imp-bin-eval

"${ROOT_DIR}/scripts/install-local-systemd.sh"
"${ROOT_DIR}/scripts/start-local.sh"
"${ROOT_DIR}/scripts/public-gateway.sh" start

echo "bin-eval public deployment installed url=${BIN_EVAL_PUBLIC_URL} token=redacted"
