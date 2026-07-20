# bin-eval Curl Path

`bin-eval` binds locally to `127.0.0.1:8080` by default. The production endpoint is the authenticated Cloudflare Tunnel route `https://bin-eval.prls.co`, documented in `docs/public-deployment.md`. Both endpoints start workflows through Temporal, and the worker calls the existing LiteLLM Responses API at `http://127.0.0.1:4000/v1/responses` with model `gpt-5.4-mini`.

The current API is asynchronous. The curl workflow is therefore a sequence:

1. `POST /checklists`
2. `GET /checklists/{checklist_id}` until `status` is `succeeded`
3. `POST /evaluations`
4. `GET /evaluations/{evaluation_id}` until `status` is `succeeded`

## Local Service Commands

Install the local service units:

```fish
make install-local
```

Start the dependency Compose stack, API, and worker:

```fish
make start-local
```

Inspect local status:

```fish
make status-local
scripts/status-local.sh --json
```

Stop the local service:

```fish
make stop-local
```

Run the live curl validation:

```fish
make test-live-curl
```

`make install-local` auto-selects user-level systemd when passwordless sudo is unavailable. Set `BIN_EVAL_SYSTEMD_MODE=system` to install system-level units under `/etc/systemd/system` on a host where sudo is available. The system-level path uses `/etc/bin-eval/bin-eval.env`; the user-level path uses ignored local file `deploy/local/bin-eval.env`.

The local env file may leave `BIN_EVAL_LLM_API_KEY` empty. The service also loads `/home/kirill/p/litellm-chatgpt/.env`, and `internal/config.Load` reuses `LITELLM_MASTER_KEY` when a bin-eval-specific key is absent.

## Copy-Paste Curl Sequence

These Fish commands are safe to paste as a block after the selected endpoint is running. They use `curl -fsS` so HTTP failures stop the command, while successful responses are written to `debug/live-curl/` for inspection. The `bin_eval_curl` function adds the public bearer header when `BIN_EVAL_PUBLIC_BEARER_TOKEN` is set and otherwise calls the local API without authentication.

Set the API URL and curl authentication:

```fish
# Use the local-only API endpoint unless BIN_EVAL_URL is already set.
if not set -q BIN_EVAL_URL
    set -gx BIN_EVAL_URL http://127.0.0.1:8080
end

# Use one curl command for local and public endpoints. The token is read from the
# environment and is never embedded in this document or written to debug files.
function bin_eval_curl
    set -l auth_args
    if set -q BIN_EVAL_PUBLIC_BEARER_TOKEN; and test -n "$BIN_EVAL_PUBLIC_BEARER_TOKEN"
        set auth_args -H "Authorization: Bearer $BIN_EVAL_PUBLIC_BEARER_TOKEN"
    end
    command curl $auth_args $argv
end

# Keep request and response captures out of the repo; debug/ is ignored.
mkdir -p debug/live-curl
```

Create a checklist from a committed fixture:

```fish
# Extract the task fields and explicitly select three independent evaluation runs.
jq -c '{task, context, evaluation_runs:3}' fixtures/smoke/cases/release_notes/task.json > debug/live-curl/create_checklist_payload.json

# Start checklist generation. The API returns 202 with a checklist_id because
# question generation runs asynchronously in Temporal.
bin_eval_curl -fsS -X POST "$BIN_EVAL_URL/checklists" \
  -H 'Content-Type: application/json' \
  --data @debug/live-curl/create_checklist_payload.json \
  -o debug/live-curl/create_checklist.json

# Save the checklist ID for polling and later evaluation.
set checklist_id (jq -r '.checklist_id' debug/live-curl/create_checklist.json)
printf 'checklist_id=%s\n' "$checklist_id"
```

Poll the checklist until dimensions, candidate questions, diagnostic weights, and final questions are ready:

```fish
for attempt in (seq 1 600)
  # Poll the checklist state. It starts as running and finishes as succeeded or failed.
  bin_eval_curl -fsS "$BIN_EVAL_URL/checklists/$checklist_id" \
    -o debug/live-curl/checklist.json

  set checklist_state (jq -r '.status' debug/live-curl/checklist.json)
  if test "$checklist_state" = succeeded
    # On success, dimensions and candidate_questions are diagnostic trace data.
    # weights use candidate_question_id and explain delete/keep/split decisions.
    # questions is the final binary checklist used for evaluation scoring.
    jq '{
      status,
      evaluation_runs,
      dimension_count: (.dimensions | length),
      candidate_question_count: (.candidate_questions | length),
      final_question_count: (.questions | length),
      weights,
      questions
    }' debug/live-curl/checklist.json
    break
  end
  if test "$checklist_state" = failed
    # Print the full failure response before exiting.
    jq . debug/live-curl/checklist.json
    exit 1
  end

  # Workflows are asynchronous; wait briefly before polling again.
  sleep 2
end
```

Create an evaluation against that checklist:

```fish
# Build the evaluation request from the generated checklist and a model answer.
jq -n --arg id "$checklist_id" \
  --rawfile answer fixtures/smoke/cases/release_notes/model_answer_good.txt \
  '{checklist_id:$id, model_answer:$answer}' \
  > debug/live-curl/create_evaluation_payload.json

# Start answer evaluation. This also returns 202 because scoring runs async.
bin_eval_curl -fsS -X POST "$BIN_EVAL_URL/evaluations" \
  -H 'Content-Type: application/json' \
  --data @debug/live-curl/create_evaluation_payload.json \
  -o debug/live-curl/create_evaluation.json

# Save the evaluation ID for polling the score.
set evaluation_id (jq -r '.evaluation_id' debug/live-curl/create_evaluation.json)
printf 'evaluation_id=%s\n' "$evaluation_id"
```

Poll the evaluation until score fields are ready:

```fish
for attempt in (seq 1 600)
  # Poll the evaluation state until the worker has scored the answer.
  bin_eval_curl -fsS "$BIN_EVAL_URL/evaluations/$evaluation_id" \
    -o debug/live-curl/evaluation.json

  set evaluation_state (jq -r '.status' debug/live-curl/evaluation.json)
  if test "$evaluation_state" = succeeded
    # Each judgment has one majority answer and a runs array containing every
    # independently generated answer and its evidence.
    jq '{
      status,
      satisfied_points,
      total_possible_points,
      checklist_pass_rate,
      failed_question_ids,
      judgments
    }' debug/live-curl/evaluation.json
    break
  end
  if test "$evaluation_state" = failed
    # Print the full failure response before exiting.
    jq . debug/live-curl/evaluation.json
    exit 1
  end

  # Workflows are asynchronous; wait briefly before polling again.
  sleep 2
end
```

## Live Curl Validation

Run the canonical curl workflow against the persistent local service:

```fish
make test-live-curl
```

It writes:

- `debug/live-curl/incident_response/checklist.json`
- `debug/live-curl/incident_response/evaluation_good.json`
- `debug/live-curl/incident_response/evaluation_bad.json`
- `debug/live-curl/release_notes/checklist.json`
- `debug/live-curl/release_notes/evaluation_good.json`
- `debug/live-curl/release_notes/evaluation_bad.json`
- `debug/live-curl/summary.json`
- `debug/live-curl/invariant_report.json`
- `debug/live-curl/llm-artifacts/manifest.json`
- exact LLM request and response files under `debug/live-curl/llm-artifacts/objects/`

The summary includes `checklist_id`, `evaluation_id`, `dimension_count`, `candidate_question_count`, `final_question_count`, `evaluation_runs`, diagnostic `weights`, `satisfied_points`, `total_possible_points`, `checklist_pass_rate`, `failed_question_ids`, `judgment_count`, per-run pass rates, the artifact count, and the artifact manifest path. The manifest records the Garage key, byte count, and SHA-256 digest for every captured request and response without storing authorization headers.

## Existing Smoke Path

TEST-008 is the single executable curl workflow. `make test-e2e` runs it with transient local processes: it starts the local Compose dependencies, starts short-lived API and worker binaries, creates one checklist per fixture case, evaluates good and bad answers, polls the same four async routes, and writes captured JSON plus the exact LLM artifact export to `debug/smoke/`.

`make test-live-curl` selects the same TEST-008 command, assertions, quality thresholds, and artifact capture with the persistent service URL and environment. Use it when validating the installed local service. Use `make test-e2e` when validating the same behavior with transient API and worker processes.
