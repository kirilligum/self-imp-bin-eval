# bin-eval Reliability Audit Correction Decisions

This document captures only the decisions that remain open after auditing the implemented rubric-refinement pipeline and its verification harness. It does not reopen decisions already recorded in `ask_me/rubric-refinement-open-decisions.md`.

Decision status:

- Selected: 1A, 2A, 3A, 4B, 5A, 6B, 7B.
- Decision 6 clarification: the initial request that supplies the evaluation instruction must accept an `evaluation_runs` argument, with default `3`. This makes repetition a checklist-level policy rather than only a smoke-script setting. The aggregation rule for repeated judgments must be fixed in the implementation plan before code changes.
- Decision 7 clarification: CI must exercise the real bin-eval HTTP API and the configured OpenAI-compatible LLM HTTP boundary. Deterministic mocked LLM responses are acceptable for contract tests. The planned caching LLM API service will later provide replayable responses through that same boundary; bin-eval must not gain a cache-specific code path.

Settled constraints that every option below preserves:

- The service remains local-only for this phase and uses the existing LiteLLM profile.
- The HTTP API keeps the existing four routes.
- `weights` remain diagnostic data: `0` deletes, `1` keeps, and `2..4` requests decomposition into final questions.
- Final questions have equal score value.
- Defaults remain `max_dimensions=6`, `max_candidates_per_dimension=8`, `max_split_count=4`, and `max_final_questions=64`.
- Invalid structured model output fails closed without a repair prompt.
- Prompt prose should express semantic instructions; JSON Schema owns structural output shape.
- There is one canonical runtime path, with no legacy mode or provider fallback.

Terminology:

- **Current rubric plan**: `plans/bin-eval-rubric-refinement-dag-plan.md`, document `PLAN-BIN-EVAL-RUBRIC-REFINEMENT-003`.
- **Verification manifest**: the machine-readable mapping of `REQ-###` requirements to `TEST-###` tests, commands, and evidence. The current candidate is `docs/test-matrix.yml`.
- **False green**: a command exits successfully without executing its intended test. In this repository, `go test -run <nonexistent-name>` exits `0` and prints `[no tests to run]`.
- **Diagnostic weight**: `evalcore.Weight`. It records deletion, retention, or requested decomposition count and is never read by `ScoreChecklist` as a point multiplier.
- **Compositionality**: the number of independently judgeable obligations combined inside one candidate question. For example, “Does the answer identify the cause and provide a tested fix?” contains two obligations.
- **Projected final count**: the deterministic final-question count known immediately after `AssignWeights`: the sum of all positive diagnostic weights.
- **Structured failure**: a stable error code plus machine-readable details such as `evalcore.LimitDiagnostic`, distinct from a human-readable `error_message`.
- **Ambiguous commit**: a database transaction commits, but the activity loses its response, so Temporal cannot tell whether the side effect happened and retries it.
- **Idempotent terminal activity**: `SucceedChecklist`, `FailChecklist`, `SucceedEvaluation`, or `FailEvaluation` returns the same successful outcome when retried with the same data, without duplicating rows or corrupting state.
- **Raw LLM artifact**: the exact HTTP request body or response stream bytes exchanged by `llm.HTTPClient.GenerateJSON`, not a later reconstruction made by marshaling `GenerateRequest` or the decoded Go output.
- **Live quality evaluation**: `scripts/smoke_curl.sh` exercising the real local LiteLLM model, as opposed to deterministic unit tests using fake HTTP servers or Temporal activities.

### 1.1 Question

Which artifact should be the single source of truth for requirements-to-test traceability and executable phase verification?

### 1.2 Context & clarification

The current rubric plan defines `TEST-001` through `TEST-011`, while `docs/test-matrix.yml` uses a different inventory that includes older MVP requirements. Several focused commands in the plan reference test names that do not exist. Go treats those commands as successful even though no test runs. `scripts/validate_traceability.sh` validates tags and selected fields but does not execute commands, and its duplicate-ID check deduplicates IDs before counting them.

This decision affects `plans/bin-eval-rubric-refinement-dag-plan.md`, `docs/test-matrix.yml`, `scripts/validate_traceability.sh`, test function names under `internal/`, and the canonical Make targets.

### 1.3 Options

- `Option A`: Machine-readable verification manifest is canonical
  - **Rubrics**: `Conf:90% | Invest:i | Blast:i | Reversal:i | Fit:i | Reuse:i | Obs:i | Surface:ii | Perf:na`
  - **Approach**: Replace the current matrix with the rubric plan's exact `REQ-###` and `TEST-###` inventory. Parse it with a small typed Go validator, verify uniqueness and complete requirement coverage, verify each focused test pattern matches at least one test, and execute commands only through a dedicated verification target. The prose plan links to the manifest instead of duplicating full commands.
  - **Example**: `make verify-plan` parses `docs/test-matrix.yml`, runs `go test -list` before each focused Go command, fails if the match count is zero, and then executes the test.
  - **Architecture**: Adds one verification command at the repository shell boundary while leaving production packages unchanged.
  - **SSoT**: `docs/test-matrix.yml` owns requirement/test IDs and commands; the plan owns rationale and phase ordering but references manifest IDs.
  - **System limits**: Runtime is the sum of manifest commands; live and integration suites remain explicit release gates rather than being recursively invoked from `make lint`.
  - **Trade-offs**: Strongest auditability and eliminates false-green focused tests. It adds a YAML parser dependency or a small verification command and requires removing duplicated test definitions from the plan.
- `Option B`: Markdown plan is canonical and aggregate Make targets are executable
  - **Rubrics**: `Conf:80% | Invest:ii | Blast:ii | Reversal:ii | Fit:ii | Reuse:ii | Obs:ii | Surface:i | Perf:na`
  - **Approach**: Delete `docs/test-matrix.yml` and source test tags. Keep requirements and phase commands only in the plan, while canonical Make targets run complete packages rather than focused patterns.
  - **Example**: P03 verification is `go test ./internal/workflows -count=1`, avoiding a fragile `-run` pattern entirely.
  - **Architecture**: Uses the existing Makefile and test packages with no new parser or command.
  - **SSoT**: The rubric plan owns traceability; the Makefile owns executable aggregate commands.
  - **System limits**: Package-wide commands may execute more tests than a phase needs, but current package sizes are small.
  - **Trade-offs**: Smaller harness and no false-green focused names, but traceability remains human-reviewed rather than mechanically complete.

### 1.4 Recommendation

I recommend **Option A**. Correctness and auditable evidence outrank the small parser cost. A typed manifest gives one canonical mapping, can prove that every focused pattern executes at least one test, and avoids duplicating commands between the plan and test documentation.

Decision: **Selected Option A**. `docs/test-matrix.yml` becomes the canonical machine-readable verification manifest. The validator must parse it structurally, prove requirement coverage and unique IDs, ensure focused test patterns match at least one test, and execute manifest commands through one dedicated verification target.

### 2.1 Question

Should diagnostic weights `2..4` represent compositionality only, or should they also represent subjective importance?

### 2.2 Context & clarification

The current prompt in `BuildWeightAssignmentRequest` says a higher integer applies when a question is “important or broad enough” to split. Those are different properties. A critical but already atomic requirement cannot be split without creating correlated or duplicate questions. Conversely, a low-importance compound question may need decomposition simply to be judgeable.

Because `ScoreChecklist` gives every final question one point, splitting implicitly increases a source candidate's contribution to the score. The meaning of `evalcore.Weight.Weight` therefore directly affects score validity.

### 2.3 Options

- `Option A`: Split count represents independently judgeable obligations only
  - **Rubrics**: `Conf:90% | Invest:i | Blast:i | Reversal:i | Fit:i | Reuse:i | Obs:i | Surface:i | Perf:na`
  - **Approach**: Define `2..4` as the number of distinct atomic obligations contained in the candidate. Importance influences upstream dimension/rubric coverage and candidate generation, not artificial duplication during splitting.
  - **Example**: “Does the answer identify the cause and provide a tested fix?” gets `2`; “Does the answer identify the cause?” gets `1` even when cause identification is critical.
  - **Architecture**: Keeps weight assignment, `BuildFinalChecklist`, and equal-count scoring, while making their shared semantic contract precise.
  - **SSoT**: The weight prompt defines decomposition semantics; `ValidateWeights` and `BuildFinalChecklist` enforce only the numeric and count invariants.
  - **System limits**: Projected final count remains the sum of positive weights and is bounded by 64.
  - **Trade-offs**: Produces defensible atomic questions and avoids duplicate weighting. Importance must be represented through rubric coverage rather than a direct multiplier.
- `Option B`: Split count combines importance and compositionality
  - **Rubrics**: `Conf:60% | Invest:ii | Blast:ii | Reversal:ii | Fit:ii | Reuse:ii | Obs:ii | Surface:i | Perf:na`
  - **Approach**: Keep higher weights for either broad or important candidates, but explicitly prohibit paraphrase duplication and require every split output to test a distinct aspect.
  - **Example**: A critical atomic security requirement may receive `3` only if the model can derive three non-overlapping observable aspects; otherwise it remains `1`.
  - **Architecture**: Preserves the current conceptual intent but puts more semantic burden on `AssignWeights` and `SplitQuestion`.
  - **SSoT**: Prompt prose owns both importance allocation and decomposition behavior.
  - **System limits**: Unknown - the local context contains no deterministic method for validating semantic non-overlap.
  - **Trade-offs**: Can allocate more score mass to important areas, but deterministic Go validation cannot prove that generated aspects are independent.

### 2.4 Recommendation

I recommend **Option A**. It gives diagnostic weights one mechanically understandable meaning and protects score integrity. Dimensions and rubrics are the better place to express importance because they shape coverage before candidate questions exist.

Decision: **Selected Option A**. Weights `2..4` represent the number of independently judgeable obligations in a compound candidate. Importance is expressed through dimension/rubric coverage and candidate generation, not by splitting an already atomic requirement into correlated questions.

### 3.1 Question

How should structured workflow failures and limit diagnostics be persisted and returned by the existing API?

### 3.2 Context & clarification

`evalcore.SemanticError` currently contains `Diagnostics []LimitDiagnostic`, but `failChecklist` and `failEvaluation` pass only `cause.Error()` to persistence. `db.Checklist` and `db.Evaluation` expose only `ErrorMessage`, so fields such as `limit_name`, `configured_limit`, `observed_count`, `checklist_id`, and `stage` are lost as structured data.

This decision affects migrations, `db.Checklist`, `db.Evaluation`, fail-activity inputs, Temporal error conversion, API failed-response shapes, and curl diagnostics. The four-route API surface remains unchanged.

### 3.3 Options

- `Option A`: Append-only workflow failure records
  - **Rubrics**: `Conf:70% | Invest:i | Blast:i | Reversal:i | Fit:ii | Reuse:i | Obs:i | Surface:ii | Perf:na`
  - **Approach**: Add a `workflow_failures` table containing entity type, entity ID, error code, stage, diagnostics JSON, activity metadata, and timestamps. Failed checklist/evaluation responses project the terminal failure record.
  - **Example**: One row records `{entity_type:"checklist", code:"limit_exceeded", stage:"weight_assignment", diagnostics:{...}}`.
  - **Architecture**: Creates a general failure ledger beside the existing lifecycle tables.
  - **SSoT**: The failure table owns structured failure details; entity rows own lifecycle status.
  - **System limits**: At most a small bounded number of terminal failure records per entity under the current workflow model.
  - **Trade-offs**: Strong history and future operational value, but adds a table and query path that the local MVP does not otherwise need.
- `Option B`: Store one terminal `error_details` JSON object on each entity
  - **Rubrics**: `Conf:90% | Invest:ii | Blast:ii | Reversal:ii | Fit:i | Reuse:ii | Obs:ii | Surface:i | Perf:na`
  - **Approach**: Add nullable `error_code` and `error_details jsonb` columns to `checklists` and `evaluations`, retain `error_message` as a safe human summary, and return all three on failed GET responses.
  - **Example**: A failed checklist returns `error_code`, `error_message`, and `error_details: {diagnostics:[{limit_name, configured_limit, observed_count, checklist_id, stage}]}`.
  - **Architecture**: Extends the existing explicit running/succeeded/failed entity state without creating another service or route.
  - **SSoT**: Each terminal entity row owns its single terminal failure result.
  - **System limits**: Error detail size should be bounded to structured codes, counts, IDs, stages, and artifact references; raw prompts and model output are excluded.
  - **Trade-offs**: Smallest durable schema change and easy curl consumption. It records terminal state only, not every failed retry attempt.

### 3.4 Recommendation

I recommend **Option B**. The product exposes one terminal outcome per checklist or evaluation, so an entity-owned structured failure object fits the existing state machine and avoids an unnecessary event ledger.

Decision: **Selected Option A**. Add an append-only `workflow_failures` record for structured terminal and retry diagnostics. Checklist and evaluation lifecycle rows remain the status source of truth; the API projects the relevant terminal failure without adding routes.

### 4.1 Question

How much delivery machinery should be added to make API-to-Temporal starts and terminal database activities reliable?

### 4.2 Context & clarification

The API currently creates a running Postgres row and then calls `TemporalStarter`. If the start definitively fails, the row remains running. If the start result is ambiguous, the workflow may exist even though the API returns HTTP 500. Terminal activities are also not idempotent under ambiguous commits: a retry can encounter duplicate rows or an already-terminal entity and fail. Finally, `failChecklist` and `failEvaluation` ignore failure-activity errors.

The foundational methods are `Router.createChecklist`, `Router.createEvaluation`, `TemporalStarter.StartCreateChecklist`, `TemporalStarter.StartEvaluateAnswer`, `db.Store.SucceedChecklist`, `db.Store.FailChecklist`, `db.Store.SucceedEvaluation`, and `db.Store.FailEvaluation`.

### 4.3 Options

- `Option A`: Transactional outbox and dispatcher
  - **Rubrics**: `Conf:80% | Invest:i | Blast:i | Reversal:i | Fit:ii | Reuse:i | Obs:i | Surface:ii | Perf:ii`
  - **Approach**: Create the entity and an outbox record in one Postgres transaction. A dispatcher starts the workflow with a stable workflow ID and marks the outbox delivered. Terminal activities become idempotent.
  - **Example**: `POST /checklists` commits `checklists(status=running)` plus `workflow_outbox(kind=create_checklist)`; a worker drains the outbox until Temporal confirms the execution exists.
  - **Architecture**: Adds a durable delivery component between API persistence and Temporal.
  - **SSoT**: Postgres outbox owns start-delivery state; Temporal owns workflow execution; entity rows own product lifecycle state.
  - **System limits**: Requires polling or notification, a dispatcher concurrency limit, and retention for delivered rows. Exact operational limits are unknown in local context.
  - **Trade-offs**: Strongest crash consistency and production posture, but introduces a new persistent state machine and runtime component.
- `Option B`: Stable workflow IDs, explicit start resolution, and idempotent terminal methods
  - **Rubrics**: `Conf:90% | Invest:ii | Blast:ii | Reversal:ii | Fit:i | Reuse:ii | Obs:ii | Surface:i | Perf:i`
  - **Approach**: Keep the direct API start. Use existing stable IDs, treat “already started” as success, resolve ambiguous errors by querying Temporal for that ID, mark the entity failed only when absence is definitive, and make all four terminal store methods return success when the existing terminal state and persisted data match the requested outcome. Never ignore failure-persistence errors.
  - **Example**: Retrying `SucceedEvaluation` with identical judgments returns nil after verifying persisted rows and recomputed score; conflicting data returns `ErrConflict`.
  - **Architecture**: Strengthens the existing API, Temporal, and Postgres boundaries without adding another process or table.
  - **SSoT**: Stable Temporal workflow ID owns execution identity; Postgres owns product state; terminal methods own idempotent comparison rules.
  - **System limits**: Uses bounded Temporal client retries and one describe call only after an ambiguous start error.
  - **Trade-offs**: Fits local scope and addresses realistic retry behavior. A process crash between row creation and the first start call still requires a startup reconciliation scan if that case must be closed completely.

### 4.4 Recommendation

I recommend **Option B** for the current local service. It fixes Temporal's normal at-least-once and ambiguous-result cases directly. A transactional outbox should be introduced only when external production availability requirements justify another state machine.

Decision: **Selected Option B**. Keep direct API-to-Temporal starts with stable workflow IDs, resolve ambiguous start results explicitly, make terminal store methods idempotent for identical input, reject conflicting retries, and never ignore failure-persistence errors.

### 5.1 Question

What exactly should “raw LLM request and response artifacts” mean at the `LLMClient.GenerateJSON` boundary?

### 5.2 Context & clarification

`Activities.runChecklistLLM` currently stores `json.Marshal(req)` before the call and `json.Marshal(out)` after successful decoding. Those are canonical reconstructions, not the actual HTTP body and SSE response bytes. Invalid output is not written as a response artifact; instead `ModelOutputError.Error()` embeds up to 800 characters of raw content, which can enter Temporal history and Postgres `error_message`.

This decision affects `llm.LLMClient`, `llm.HTTPClient.GenerateJSON`, `llm.ModelOutputError`, Garage artifact keys, activity artifact writes, and redaction tests.

### 5.3 Options

- `Option A`: Preserve exact transport bytes for every attempt
  - **Rubrics**: `Conf:90% | Invest:i | Blast:i | Reversal:i | Fit:i | Reuse:i | Obs:i | Surface:ii | Perf:ii`
  - **Approach**: Change the LLM boundary to return a trace containing the exact serialized HTTP request and complete response stream bytes on success or failure. Activities persist both under attempt-specific Garage keys. Errors contain only safe codes, provider status, artifact keys, and bounded structural diagnostics.
  - **Example**: `GenerateJSON(ctx, req, out) (llm.Trace, error)` returns `RequestBody`, `ResponseBody`, status, and provider request ID; `ModelOutputError` references the response artifact rather than embedding content.
  - **Architecture**: Keeps transport capture inside the one existing external LLM boundary and persistence in activities.
  - **SSoT**: Garage owns exact payload evidence; typed Go values own runtime behavior; safe errors own classification only.
  - **System limits**: Request and response capture must enforce configured byte ceilings. The current scanner allows up to 8 MiB per SSE line; no artifact retention limit is defined in local context.
  - **Trade-offs**: Best auditability and prevents raw output leakage through errors. It changes the LLM interface and requires attempt-aware artifact keys.
- `Option B`: Preserve canonical request JSON and raw decoded output text
  - **Rubrics**: `Conf:80% | Invest:ii | Blast:ii | Reversal:ii | Fit:ii | Reuse:ii | Obs:ii | Surface:i | Perf:i`
  - **Approach**: Continue storing the semantic `GenerateRequest`, but additionally store the exact concatenated output text before decoding for both success and failure. Do not retain SSE framing or the final wire request body.
  - **Example**: Garage stores `request.json` from `GenerateRequest` and `output.txt` from `response.output_text.*` events.
  - **Architecture**: Smaller extension to the current client and activity helper.
  - **SSoT**: Garage owns semantic prompts and model output text, while HTTP transport details remain transient.
  - **System limits**: Output capture remains bounded by the client's stream buffer and configured output-token budget.
  - **Trade-offs**: Usually sufficient for prompt debugging, but does not satisfy a literal byte-preserving transport audit.

### 5.4 Recommendation

I recommend **Option A** because the plan explicitly promises raw request/response auditability and the current error path risks leaking model output into operational state. The trace should remain inside `internal/llm`; no prompt-specific repair or alternate provider path is introduced.

Decision: **Selected Option A**. Capture exact HTTP request and response bytes for every LLM attempt at the existing `internal/llm` boundary, persist them under attempt-aware Garage keys, and keep raw content out of Temporal and Postgres error strings.

### 6.1 Question

How many repeated live LiteLLM evaluations should the canonical `make test-e2e` quality gate run?

### 6.2 Context & clarification

The current smoke gate evaluates each committed fixture once. That proves one observed outcome but does not characterize stochastic variation. The plan's execution-log template includes mean, standard deviation, confidence interval, and sample size, but the script currently reports only means across different fixture cases. It also omits the required minimum of eight final questions per case and structured limit-hit diagnostics.

Repeated runs use the same API, Temporal workflows, Postgres, Garage, and local LiteLLM path. They are not a fallback implementation. They increase runtime and LLM usage approximately linearly.

### 6.3 Options

- `Option A`: Five repetitions per fixture with distribution-aware gates
  - **Rubrics**: `Conf:80% | Invest:i | Blast:i | Reversal:i | Fit:i | Reuse:i | Obs:i | Surface:i | Perf:ii`
  - **Approach**: Run five independent checklist/evaluation cycles per fixture, retain per-run diagnostics, calculate mean, standard deviation, and a 95% confidence interval, and enforce the existing quality thresholds plus final-question and judgment-coverage invariants.
  - **Example**: Two fixtures produce ten checklists and twenty evaluations in one `make test-e2e` run.
  - **Architecture**: Extends the existing canonical smoke script without creating another product execution path.
  - **SSoT**: `scripts/smoke_curl.sh` owns quality thresholds and statistical summaries; `validate_smoke_invariants.sh` independently recomputes persisted invariants.
  - **System limits**: Approximately five times current LLM calls, workflow duration, and artifact volume. Exact LiteLLM throughput and billing limits are unknown - not available in local context.
  - **Trade-offs**: Meaningful reliability evidence and catches model flakiness, at materially higher local runtime.
- `Option B`: Three repetitions per fixture with min/mean gates
  - **Rubrics**: `Conf:70% | Invest:ii | Blast:ii | Reversal:ii | Fit:ii | Reuse:ii | Obs:ii | Surface:i | Perf:i`
  - **Approach**: Run three cycles per fixture and gate on mean separation plus a minimum acceptable good-answer score and maximum bad-answer score for every run.
  - **Example**: Fail if any good run is below `0.70`, any bad run is above `0.60`, or aggregate plan thresholds fail.
  - **Architecture**: Same smoke path with a smaller sample and simpler statistics.
  - **SSoT**: The smoke script owns repeated quality acceptance.
  - **System limits**: Approximately three times current LLM calls, workflow duration, and artifact volume.
  - **Trade-offs**: Better than one observation and cheaper than Option A, but too few samples for a persuasive confidence interval.
- `Option C`: One run per fixture with complete deterministic invariants
  - **Rubrics**: `Conf:60% | Invest:iii | Blast:iii | Reversal:iii | Fit:iii | Reuse:iii | Obs:iii | Surface:i | Perf:i`
  - **Approach**: Keep one live run but enforce every currently missing criterion: minimum final count, exact judgment ID coverage, limit diagnostics, score recomputation, and complete per-case evidence.
  - **Example**: `make test-e2e` remains near its current runtime but fails if a case has fewer than eight final questions.
  - **Architecture**: Tightens existing scripts only.
  - **SSoT**: Smoke and invariant scripts own acceptance.
  - **System limits**: Current approximate runtime and LLM usage.
  - **Trade-offs**: Direct and inexpensive, but cannot distinguish a stable model behavior from a lucky run.

### 6.4 Recommendation

I recommend **Option A**. The user explicitly prioritizes reliability over runtime, and model quality is the product's central nondeterministic behavior. Five repetitions are still modest but provide substantially better evidence than one run.

Decision: **Selected Option B**, with default `evaluation_runs=3`. The initial API call that supplies the evaluation instruction must allow the caller to choose this value. The implementation plan must define a bounded request contract, persistence, repeated-judgment behavior, deterministic aggregation, diagnostics, and tests before implementation; the smoke harness must pass the argument explicitly rather than owning a separate repetition setting.

### 7.1 Question

Where should the full verification gates run automatically, given that LiteLLM and the Compose stack currently exist only on this machine?

### 7.2 Context & clarification

There is currently no CI workflow. `make lint`, `make build`, `make test`, `make test-integration`, and `make test-e2e` were run manually before pushing directly to `master`. Hosted GitHub runners cannot reach the machine-local LiteLLM endpoint or local persistent service without additional infrastructure.

This decision affects `.github/workflows/`, secret handling, Docker availability, evidence retention, and whether `master` can receive an unverified commit.

### 7.3 Options

- `Option A`: Self-hosted runner executes every canonical gate before publication
  - **Rubrics**: `Conf:70% | Invest:i | Blast:i | Reversal:i | Fit:ii | Reuse:i | Obs:i | Surface:ii | Perf:ii`
  - **Approach**: Register this machine or another controlled machine as a self-hosted GitHub Actions runner. Require lint, build, unit, integration, repeated live e2e, and artifact upload before merging or pushing production-bound changes.
  - **Example**: A protected `master` branch requires a self-hosted `full-verification` job that uses the existing local LiteLLM and Compose configuration.
  - **Architecture**: Automates the exact local runtime environment already used by the service.
  - **SSoT**: Make targets remain canonical; CI only invokes them and retains reports.
  - **System limits**: Runner concurrency, availability, trust boundary, and maintenance requirements are unknown - not available in local context.
  - **Trade-offs**: Full automatic evidence with one command path, but adds operational responsibility and exposes a trusted machine to repository jobs.
- `Option B`: Hosted full-stack CI plus required local release verification
  - **Rubrics**: `Conf:90% | Invest:ii | Blast:ii | Reversal:ii | Fit:i | Reuse:ii | Obs:ii | Surface:i | Perf:i`
  - **Approach**: GitHub-hosted CI starts the complete bin-eval dependency and application stack, calls the real bin-eval HTTP API, and points the existing OpenAI-compatible LLM boundary at a CI-reachable endpoint. Deterministic jobs may use a schema-conformant HTTP mock. A required live job calls a real LLM API; later it points to the separately developed caching LLM API service without changing bin-eval code. One local `make verify-release` command runs the same gates and writes a commit-addressed report before pushing `master`.
  - **Example**: PR contract tests start Postgres, Temporal, Garage, API, worker, and an HTTP LLM stub; the live quality job sets `BIN_EVAL_LLM_BASE_URL` to the real or caching LLM service and sends curl requests only to bin-eval.
  - **Architecture**: Preserves one bin-eval runtime path. Test determinism and live model behavior differ only by the external service configured at the existing LLM HTTP boundary.
  - **SSoT**: Make targets own commands; bin-eval's public HTTP API owns product execution; the external LLM endpoint owns generation and optional caching.
  - **System limits**: Hosted GitHub Actions limits and real LLM API limits require verification from official documentation. Request-level repetition limits follow Question 6.
  - **Trade-offs**: Full-stack contract coverage remains self-contained. The live job still depends on an external endpoint until the caching service can run inside or alongside CI.

### 7.4 Recommendation

I recommend **Option B** for the local-only phase. It keeps every test on the actual bin-eval API path, permits deterministic HTTP-boundary fixtures, and lets the caching service replace the direct real-LLM endpoint through configuration rather than a new runtime branch.

Decision: **Selected Option B**, with a required real-LLM CI job. CI must start the complete bin-eval stack and call the real bin-eval HTTP API. A schema-conformant mocked LLM HTTP endpoint is allowed for deterministic contract coverage. A separate live quality job must use a real LLM API; once available, the caching LLM API service becomes that configured endpoint and provides repeatability without bin-eval-specific cache logic.
