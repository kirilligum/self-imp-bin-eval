# bin-eval Public Cloudflare Deployment

The production deployment runs on this computer at `https://bin-eval.prls.co`. The application API remains on `127.0.0.1:8080`. A loopback-only Nginx gateway on `127.0.0.1:18081` enforces bearer authentication, a 1 MiB request limit, a shared 10 requests/second rate with burst 20, and HTTPS security headers. A dedicated Cloudflare Tunnel terminates publicly trusted HTTPS and reaches the gateway through the connector's host network; the loopback origin is not directly reachable from the network.

Opening the public URL at `/` returns a small JSON service document without exposing application data. `/healthz` remains the unauthenticated liveness endpoint. All checklist and evaluation routes require the bearer token and return a JSON `401` challenge when it is missing or invalid.

The public API token is separate from Cloudflare, GitHub, NPM, and LiteLLM credentials. It is generated into ignored mode-`0600` file `deploy/local/bin-eval-public.env` and copied to the GitHub Actions secret `BIN_EVAL_PUBLIC_BEARER_TOKEN` without printing its value. The Cloudflare connector credential is stored separately in ignored mode-`0600` file `deploy/local/bin-eval-cloudflared-token`.

## Operator Commands

Install or repair the local services, generate the API token, provision Cloudflare DNS and the dedicated tunnel, configure GitHub, and start the public connector. `CLOUDFLARE_API_TOKEN` is needed only for provisioning and must have Zone Read, DNS Edit, and Cloudflare Tunnel Write permissions for `prls.co`. It can be exported for the command or stored in the ignored root `.env` file.

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

`make stop-public` stops the Cloudflare connector and Nginx. It does not stop the API, worker, Postgres, Temporal, Garage, or LiteLLM. The DNS record remains provisioned, but there is no route to the loopback service while the connector is stopped.

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

Rollback is complete when `tunnel_active=false`, the gateway is stopped, and `make status-local` still reports active dependency, API, and worker services.
