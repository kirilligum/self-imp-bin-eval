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
  scripts/install-public.sh \
  scripts/public-gateway.sh \
  scripts/status-public.sh \
  scripts/backup-public.sh \
  scripts/test_public_ingress.sh \
  docs/public-deployment.md; do
  [[ -f "$file" ]] || fail "missing public deployment file: ${file}"
done

grep -Eq '^deploy/local/bin-eval-public\.env$' .gitignore || fail "public secret env must be ignored"
grep -Eq '127\.0\.0\.1:8080' deploy/local/bin-eval.env.example || fail "application API must remain on loopback"
grep -Eq '127\.0\.0\.1:\$\{BIN_EVAL_PUBLIC_GATEWAY_PORT\}' deploy/compose/nginx-public.conf.template || fail "gateway must bind to loopback"
grep -Eq 'limit_req_zone .*rate=10r/s' deploy/compose/nginx-public.conf.template || fail "gateway rate limit is missing"
grep -Eq 'client_max_body_size 1m' deploy/compose/nginx-public.conf.template || fail "gateway body limit is missing"
grep -Eq 'BIN_EVAL_PUBLIC_BEARER_TOKEN' deploy/compose/nginx-public.conf.template || fail "gateway bearer policy is missing"
grep -Eq 'BIN_EVAL_PUBLIC_HTTPS_PORT=8443' deploy/local/bin-eval-public.env.example || fail "public HTTPS port must be 8443"
grep -Eq 'tailscale funnel .*--bg' scripts/public-gateway.sh || fail "persistent Funnel start is missing"
grep -Eq 'tailscale funnel .* off' scripts/public-gateway.sh || fail "Funnel rollback is missing"
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

echo "public runtime contract ok"
