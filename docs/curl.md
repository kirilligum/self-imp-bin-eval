# bin-eval Local Curl Path

`bin-eval` is a local-only service by default. The API binds to `127.0.0.1:8080`, starts workflows through Temporal, and the worker calls the existing LiteLLM Responses API at `http://127.0.0.1:4000/v1/responses` with model `gpt-5.4-mini`.

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

These Fish commands are safe to paste as a block after the local service is running. They use `curl -fsS` so HTTP failures stop the command, while successful responses are written to `debug/live-curl/` for inspection.

Set the local API URL:

```fish
# Use the local-only API endpoint unless BIN_EVAL_URL is already set.
if not set -q BIN_EVAL_URL
    set -gx BIN_EVAL_URL http://127.0.0.1:8080
end

# Keep request and response captures out of the repo; debug/ is ignored.
mkdir -p debug/live-curl
```

Create a checklist from a committed fixture:

```fish
# Extract the fields accepted by POST /checklists from the fixture task.
jq -c '{task, context}' fixtures/smoke/cases/release_notes/task.json > debug/live-curl/create_checklist_payload.json

# Start checklist generation. The API returns 202 with a checklist_id because
# question generation runs asynchronously in Temporal.
curl -fsS -X POST "$BIN_EVAL_URL/checklists" \
  -H 'Content-Type: application/json' \
  --data @debug/live-curl/create_checklist_payload.json \
  -o debug/live-curl/create_checklist.json

# Save the checklist ID for polling and later evaluation.
set checklist_id (jq -r '.checklist_id' debug/live-curl/create_checklist.json)
printf 'checklist_id=%s\n' "$checklist_id"
```

Poll the checklist until questions and weights are ready:

```fish
for attempt in (seq 1 150)
  # Poll the checklist state. It starts as running and finishes as succeeded or failed.
  curl -fsS "$BIN_EVAL_URL/checklists/$checklist_id" \
    -o debug/live-curl/checklist.json

  set status (jq -r '.status' debug/live-curl/checklist.json)
  if test "$status" = succeeded
    # On success, the response contains generated binary questions and weights.
    jq '{status, questions, weights}' debug/live-curl/checklist.json
    break
  end
  if test "$status" = failed
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
curl -fsS -X POST "$BIN_EVAL_URL/evaluations" \
  -H 'Content-Type: application/json' \
  --data @debug/live-curl/create_evaluation_payload.json \
  -o debug/live-curl/create_evaluation.json

# Save the evaluation ID for polling the score.
set evaluation_id (jq -r '.evaluation_id' debug/live-curl/create_evaluation.json)
printf 'evaluation_id=%s\n' "$evaluation_id"
```

Poll the evaluation until score fields are ready:

```fish
for attempt in (seq 1 150)
  # Poll the evaluation state until the worker has scored the answer.
  curl -fsS "$BIN_EVAL_URL/evaluations/$evaluation_id" \
    -o debug/live-curl/evaluation.json

  set status (jq -r '.status' debug/live-curl/evaluation.json)
  if test "$status" = succeeded
    # These fields are the final binary-checklist score and supporting judgments.
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
  if test "$status" = failed
    # Print the full failure response before exiting.
    jq . debug/live-curl/evaluation.json
    exit 1
  end

  # Workflows are asynchronous; wait briefly before polling again.
  sleep 2
end
```

## Live Curl Validation

The script version of the same curl sequence is:

```fish
scripts/live_curl_example.sh
```

It writes:

- `debug/live-curl/create_checklist.json`
- `debug/live-curl/checklist.json`
- `debug/live-curl/create_evaluation.json`
- `debug/live-curl/evaluation.json`
- `debug/live-curl/summary.json`

The summary includes `checklist_id`, `evaluation_id`, `question_count`, `satisfied_points`, `total_possible_points`, `checklist_pass_rate`, `failed_question_ids`, and `judgment_count`.

## Existing Smoke Path

`make test-e2e` remains the canonical end-to-end regression command for transient local processes. It starts the local Compose dependencies, starts short-lived API and worker binaries, creates one checklist per fixture case, evaluates good and bad answers, polls the same four async routes, and writes captured JSON to `debug/smoke/`.

Use `make test-live-curl` when validating the persistent local service. Use `make test-e2e` when validating the full smoke behavior without installing systemd units.
