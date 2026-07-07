# curl Smoke Path

`make test-e2e` is the canonical end-to-end command. It starts the local Compose dependencies, starts the Go API and worker binaries, creates one checklist per fixture case, evaluates good and bad answers, polls the four async routes, and writes captured JSON to `debug/smoke/`.

Required runtime variables:

- `BIN_EVAL_LLM_BASE_URL`
- `BIN_EVAL_LLM_API_KEY`
- `BIN_EVAL_MODEL_PROFILE`

When running on this machine with local LiteLLM, the smoke script can reuse `LITELLM_MASTER_KEY` and `LITELLM_PORT` if the `BIN_EVAL_*` LLM variables are not explicitly set.

## Routes

Create a checklist:

```bash
curl -sS -X POST "$BIN_EVAL_URL/checklists" \
  -H 'Content-Type: application/json' \
  --data @fixtures/smoke/cases/release_notes/task.json
```

Poll a checklist:

```bash
curl -sS "$BIN_EVAL_URL/checklists/<checklist_id>"
```

Create an evaluation:

```bash
jq -n --arg id "<checklist_id>" \
  --rawfile answer fixtures/smoke/cases/release_notes/model_answer_good.txt \
  '{checklist_id:$id, model_answer:$answer}' |
curl -sS -X POST "$BIN_EVAL_URL/evaluations" \
  -H 'Content-Type: application/json' \
  --data @-
```

Poll an evaluation:

```bash
curl -sS "$BIN_EVAL_URL/evaluations/<evaluation_id>"
```

The smoke command exits zero only when every checklist and evaluation reaches `succeeded` and the aggregate good/bad pass-rate separation meets EVAL-001.

