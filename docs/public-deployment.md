# bin-eval Public Tailscale Deployment

The production deployment runs on this computer. The application API remains on `127.0.0.1:8080`. A loopback-only Nginx gateway on `127.0.0.1:18081` enforces bearer authentication, a 1 MiB request limit, and a shared 10 requests/second rate with burst 20. Tailscale Funnel terminates HTTPS on public port `8443` and forwards to that gateway.

The public API token is separate from GitHub, NPM, and LiteLLM credentials. It is generated into ignored mode-`0600` file `deploy/local/bin-eval-public.env` and copied to the GitHub Actions secret `BIN_EVAL_PUBLIC_BEARER_TOKEN` without printing its value.

## Operator Commands

Install or repair the local services, generate the public token, configure GitHub, and start Funnel:

```fish
make install-public
```

Start, inspect, or stop only the public edge:

```fish
make start-public
make status-public
scripts/status-public.sh --json
make stop-public
```

`make stop-public` disables Funnel and stops Nginx. It does not stop the API, worker, Postgres, Temporal, Garage, or LiteLLM.

Create a consistent backup under ignored `backups/`:

```fish
make backup-public
```

The backup briefly disables public ingress, stops API and worker writes, stops Temporal and Garage, writes a compressed dump of every Postgres database, archives the stopped Garage metadata and data volumes, writes `SHA256SUMS`, and restores the prior service state.

## Public Curl

Load the ignored public URL and bearer token into Fish without printing either value:

```fish
set public_env deploy/local/bin-eval-public.env
set -gx BIN_EVAL_URL (string replace 'BIN_EVAL_PUBLIC_URL=' '' (grep '^BIN_EVAL_PUBLIC_URL=' $public_env))
set -gx BIN_EVAL_PUBLIC_BEARER_TOKEN (string replace 'BIN_EVAL_PUBLIC_BEARER_TOKEN=' '' (grep '^BIN_EVAL_PUBLIC_BEARER_TOKEN=' $public_env))
```

Verify the public edge and run the same checklist/evaluation workflow used locally and in CI:

```fish
make test-public-ingress
make test-public-curl
```

The complete raw Fish curl sequence is in `docs/curl.md`. Its `bin_eval_curl` function automatically sends the bearer token loaded above, so the same create, poll, evaluate, and score commands work against this public URL.

For an individual request, provide the bearer header:

```fish
curl -sS -o /dev/null -w 'HTTP %{http_code}\n' \
  -H "Authorization: Bearer $BIN_EVAL_PUBLIC_BEARER_TOKEN" \
  "$BIN_EVAL_URL/checklists/00000000-0000-0000-0000-000000000000"
```

The final request prints `HTTP 404` because the identifier does not exist; that response proves the authenticated request reached bin-eval through public HTTPS. It intentionally omits `-f` because curl treats a deliberate `404` as an error when fail-on-HTTP-error mode is enabled.

## Rollback

Disable public access while preserving the local service:

```fish
make stop-public
make status-local
```

Rollback is complete when `funnel_active=false`, the gateway is stopped, and `make status-local` still reports active dependency, API, and worker services.
