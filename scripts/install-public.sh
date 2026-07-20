#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${ROOT_DIR}/scripts/lib/local_env.sh"
bin_eval_require_tools curl docker gh jq openssl

bin_eval_load_env_file "${ROOT_DIR}/.env" false

public_env_file="$(bin_eval_public_env_file "$ROOT_DIR")"
existing_token=""
if [[ -f "$public_env_file" ]]; then
  existing_token="$(sed -n 's/^BIN_EVAL_PUBLIC_BEARER_TOKEN=//p' "$public_env_file")"
fi
if [[ ! "$existing_token" =~ ^[0-9a-f]{64}$ ]]; then
  existing_token="$(openssl rand -hex 32)"
fi

install -d -m 0755 "$(dirname "$public_env_file")"
temporary="$(mktemp "${public_env_file}.tmp.XXXXXX")"
trap 'rm -f "$temporary"' EXIT
install -m 0600 "${ROOT_DIR}/deploy/local/bin-eval-public.env.example" "$temporary"
sed -i "s/^BIN_EVAL_PUBLIC_BEARER_TOKEN=.*/BIN_EVAL_PUBLIC_BEARER_TOKEN=${existing_token}/" "$temporary"
sed -i "s/^BIN_EVAL_CLOUDFLARED_UID=.*/BIN_EVAL_CLOUDFLARED_UID=$(id -u)/" "$temporary"
sed -i "s/^BIN_EVAL_CLOUDFLARED_GID=.*/BIN_EVAL_CLOUDFLARED_GID=$(id -g)/" "$temporary"
mv -f "$temporary" "$public_env_file"
trap - EXIT
chmod 0600 "$public_env_file"

bin_eval_load_public_env "$ROOT_DIR"
"${ROOT_DIR}/scripts/configure-cloudflare.sh"
printf '%s' "$BIN_EVAL_PUBLIC_BEARER_TOKEN" | gh secret set BIN_EVAL_PUBLIC_BEARER_TOKEN --repo kirilligum/self-imp-bin-eval
gh variable set BIN_EVAL_PUBLIC_URL --body "$BIN_EVAL_PUBLIC_URL" --repo kirilligum/self-imp-bin-eval

"${ROOT_DIR}/scripts/install-local-systemd.sh"
"${ROOT_DIR}/scripts/start-local.sh"
"${ROOT_DIR}/scripts/public-gateway.sh" start

echo "bin-eval public deployment installed url=${BIN_EVAL_PUBLIC_URL} token=redacted"
