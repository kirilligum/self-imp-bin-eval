#!/usr/bin/env bash
# TEST-109
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

fail() {
  echo "$1" >&2
  exit 1
}

for target in install-public start-public stop-public status-public backup-public test-public-gateway test-public-ingress test-public-curl; do
  grep -Eq "^${target}:" Makefile || fail "missing Makefile target: ${target}"
done

for file in \
  deploy/compose/nginx-public.conf.template \
  deploy/local/bin-eval-public.env.example \
  scripts/configure-cloudflare.sh \
  scripts/install-public.sh \
  scripts/public-gateway.sh \
  scripts/status-public.sh \
  scripts/backup-public.sh \
  scripts/test_public_ingress.sh \
  docs/public-deployment.md; do
  [[ -f "$file" ]] || fail "missing public deployment file: ${file}"
done

grep -Eq '^deploy/local/bin-eval-public\.env$' .gitignore || fail "public secret env must be ignored"
grep -Eq '^deploy/local/bin-eval-cloudflared-token$' .gitignore || fail "Cloudflare tunnel token must be ignored"
grep -Eq '127\.0\.0\.1:8080' deploy/local/bin-eval.env.example || fail "application API must remain on loopback"
grep -Eq '127\.0\.0\.1:\$\{BIN_EVAL_PUBLIC_GATEWAY_PORT\}' deploy/compose/nginx-public.conf.template || fail "gateway must bind to loopback"
grep -Eq 'limit_req_zone .*rate=10r/s' deploy/compose/nginx-public.conf.template || fail "gateway rate limit is missing"
grep -Eq 'client_max_body_size 1m' deploy/compose/nginx-public.conf.template || fail "gateway body limit is missing"
grep -Eq 'BIN_EVAL_PUBLIC_BEARER_TOKEN' deploy/compose/nginx-public.conf.template || fail "gateway bearer policy is missing"
grep -Fq 'Strict-Transport-Security "max-age=31536000; includeSubDomains"' deploy/compose/nginx-public.conf.template || fail "gateway HSTS policy is missing"
grep -Fq "Content-Security-Policy \"default-src 'none'; frame-ancestors 'none'; base-uri 'none'\"" deploy/compose/nginx-public.conf.template || fail "gateway content security policy is missing"
grep -Fq "return 200 '{\"service\":\"bin-eval\",\"authentication\":\"bearer\",\"health\":\"/healthz\"}'" deploy/compose/nginx-public.conf.template || fail "public service document is missing"
grep -Fq "return 401 '{\"error\":\"authorization_required\"}'" deploy/compose/nginx-public.conf.template || fail "JSON authorization challenge is missing"
grep -Eq 'BIN_EVAL_PUBLIC_HOSTNAME=bin-eval\.prls\.co' deploy/local/bin-eval-public.env.example || fail "canonical public hostname is missing"
grep -Eq 'BIN_EVAL_PUBLIC_URL=https://bin-eval\.prls\.co' deploy/local/bin-eval-public.env.example || fail "canonical public URL is missing"
grep -Eq 'BIN_EVAL_CLOUDFLARED_TUNNEL_NAME=shaman-bin-eval' deploy/local/bin-eval-public.env.example || fail "dedicated tunnel name is missing"
grep -Eq 'BIN_EVAL_CLOUDFLARED_ORIGIN=http://127\.0\.0\.1:18081' deploy/local/bin-eval-public.env.example || fail "loopback tunnel origin is missing"
grep -Eq 'cloudflare/cloudflared@sha256:' deploy/compose/docker-compose.yml || fail "cloudflared image must be digest-pinned"
grep -Eq 'user:.*BIN_EVAL_CLOUDFLARED_UID.*BIN_EVAL_CLOUDFLARED_GID' deploy/compose/docker-compose.yml || fail "cloudflared must run as the host token owner"
grep -Eq 'network_mode: host' deploy/compose/docker-compose.yml || fail "public connector must share the loopback network namespace"
grep -Eq 'bin-eval-cloudflared-token:/run/secrets/tunnel-token:ro' deploy/compose/docker-compose.yml || fail "cloudflared token must be mounted read-only"
grep -Eq 'cfd_tunnel/.*/configurations' scripts/configure-cloudflare.sh || fail "Cloudflare ingress provisioning is missing"
grep -Eq 'cfargotunnel\.com' scripts/configure-cloudflare.sh || fail "Cloudflare DNS tunnel target is missing"
grep -Eq 'up -d --force-recreate public-gateway cloudflared' scripts/public-gateway.sh || fail "public start must apply gateway and tunnel configuration"
grep -Eq 'stop cloudflared public-gateway' scripts/public-gateway.sh || fail "public rollback must stop tunnel and gateway"
grep -Eq 'pg_dumpall' scripts/backup-public.sh || fail "Postgres backup is missing"
grep -Eq 'garage-meta' scripts/backup-public.sh || fail "Garage metadata backup is missing"
grep -Eq 'garage-data' scripts/backup-public.sh || fail "Garage data backup is missing"
grep -Eq 'sha256sum' scripts/backup-public.sh || fail "backup checksums are missing"
grep -Eq 'Public ingress gate' .github/workflows/ci.yml || fail "live CI public ingress gate is missing"
grep -Eq 'BIN_EVAL_PUBLIC_BEARER_TOKEN' scripts/lib/http.sh || fail "canonical curl helper cannot authenticate publicly"
grep -Eq '^function bin_eval_curl$' docs/curl.md || fail "documented curl sequence cannot authenticate publicly"
grep -Fq -- "-w 'HTTP %{http_code}\\n'" docs/public-deployment.md || fail "public 404 probe must report its expected status"
if grep -Eq '^[[:space:]]*curl -f' docs/public-deployment.md; then
  fail "intentional public 404 probe must not enable curl fail-on-error"
fi

if grep -REn 'GITHUB_TOKEN|GITHUB_PAT|NPM_TOKEN' deploy docs scripts .github \
  --exclude='*.example' --exclude='validate_public_runtime_contract.sh' >/dev/null; then
  fail "GitHub or NPM credentials must not be used by public deployment"
fi

if rg -n 'tailscale|Tailscale|funnel|Funnel|tail71d19c' \
  deploy/compose/docker-compose.yml deploy/local/bin-eval-public.env.example \
  scripts/install-public.sh scripts/public-gateway.sh scripts/status-public.sh \
  scripts/test_public_ingress.sh docs/public-deployment.md docs/curl.md >/dev/null; then
  fail "legacy Tailscale public ingress references remain"
fi

echo "public runtime contract ok"
