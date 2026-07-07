# bin-eval Binary Checklist Evaluation MVP Implementation Plan

## 1. Title and metadata

- Project name: bin-eval
- Version: 1.0.0
- Owners: Kirill, product and engineering
- Date: 2026-07-07
- Document ID: PLAN-BIN-EVAL-001
- Summary: This plan defines the implementation path for bin-eval, a self-hosted Go service that evaluates model answers through generated binary yes/no questions. The MVP has one checklist creation path, one answer evaluation path, one LLM client boundary, one Temporal workflow per product action, one Postgres schema, one Garage artifact layout, one local Compose stack, and one deterministic Go scoring formula. The LLM generates candidate questions, assigns weights from 0 to 4, and judges answers with yes/no verdicts plus evidence. Weight 0 marks a generated question as excluded from the active checklist; active questions have weight 1 to 4. The judge never receives weights and never emits scalar scores.

## 2. Design consensus and trade-offs

- Topic: Project naming
  - Verdict: DECISION
  - Rationale: All binaries, packages, commands, and docs use bin-eval naming. The repository path remains `/home/kirill/p/self-imp-bin-eval`, and the Go module path is grounded in the current Git remote, `github.com/kirilligum/self-imp-bin-eval`.
- Topic: Judge emits scalar quality scores
  - Verdict: AGAINST
  - Rationale: The judge returns only one `yes` or `no` verdict per active question with brief evidence. Go owns `satisfied_points`, `total_possible_points`, `checklist_pass_rate`, and `failed_question_ids`, which keeps scoring deterministic and reproducible from persisted rows.
- Topic: Weight range
  - Verdict: DECISION
  - Rationale: The MVP uses LLM-assigned integer weights from 0 to 4. Weight 0 excludes a candidate question from the active checklist, allowing weight assignment to remove duplicate, redundant, too broad, or not useful questions without adding a separate deduplication or rewrite step.
- Topic: Duplicate handling during weight assignment
  - Verdict: DECISION
  - Rationale: The weight assignment prompt can use weight 0 for redundant candidate questions, but it must still return exactly one weight object per generated question ID. Duplicate weight rows, missing weights, and unknown question IDs are invalid semantic output.
- Topic: Question ID ownership
  - Verdict: DECISION
  - Rationale: Go assigns persistent IDs `q1..qN` in candidate array order. IDs are not renumbered after weight 0 exclusions, preserving auditability between candidate questions, assigned weights, Garage artifacts, and persisted structured state.
- Topic: Split context fields
  - Verdict: AGAINST
  - Rationale: The API accepts one canonical `context` field for all evaluator-side input available before seeing the answer. Splitting into system instruction, rubric, source documents, or product requirements would multiply schemas and prompt paths without MVP value.
- Topic: Dimensions, category scores, and reason codes
  - Verdict: AGAINST
  - Rationale: These are out of MVP scope. The score is one weighted pass rate over active binary questions. Per-question evidence provides diagnostic detail without adding additional scoring axes.
- Topic: Question deduplication or rewrite step
  - Verdict: AGAINST
  - Rationale: The MVP keeps one direct LLM sequence: question generation, weight assignment, binary judging. Weight 0 is the single exclusion mechanism; no separate question merge, rewrite, or cleanup activity is added.
- Topic: Prompt optimization loops
  - Verdict: AGAINST
  - Rationale: bin-eval is a reusable evaluation service MVP, not a prompt self-improvement product. Model behavior quality is measured through model conformance and golden-answer evals.
- Topic: Fallback providers, plain-text parsers, and schema repair prompts
  - Verdict: AGAINST
  - Rationale: The LLM boundary assumes schema-constrained JSON from one configured self-hosted runtime. Invalid JSON, schema violations, and semantic violations fail fast. Infrastructure failures use bounded retries only.
- Topic: Separate model-test CLI path
  - Verdict: AGAINST
  - Rationale: A standalone prompt/model conformance CLI would create a second way to execute the evaluator outside the product route. MVP validation uses the same HTTP API, Temporal workflows, activities, persistence, Garage artifacts, and LLM boundary that production uses.
- Topic: Garage and Postgres ownership
  - Verdict: DECISION
  - Rationale: Postgres owns canonical structured state. Garage owns byte-preserving raw task, context, model answer, prompt request, and prompt response artifacts. Postgres stores artifact keys where the API needs them; deterministic Garage key construction covers raw LLM payload lookup.
- Topic: Temporal side effects
  - Verdict: DECISION
  - Rationale: External side effects stay in Temporal activities. Pure validation, ID assignment, active-question filtering, and scoring stay in Go functions.
- Topic: Production dashboarding
  - Verdict: AGAINST
  - Rationale: Grafana, Loki, Tempo, Langfuse dashboards, and rich reporting are outside the MVP. The plan keeps structured logs and smoke output as the operational baseline.
- Topic: Repository and storage abstractions
  - Verdict: AGAINST
  - Rationale: The MVP uses concrete `internal/db.Store`, concrete Garage artifact functions, and concrete Temporal activity methods. The only production interface is the PRD-required `LLMClient.GenerateJSON` boundary.
- Topic: Repository layout after runtime selection
  - Verdict: DECISION
  - Rationale: `AGENTS.md` currently describes a minimal workspace and recommends documenting build/test commands as soon as tooling is selected. Because the selected stack is Go, implementation uses conventional Go service layout with `cmd/`, `internal/`, `migrations/`, `deploy/compose/`, `scripts/`, `fixtures/`, `docs/`, and `plans/`; P00 updates `AGENTS.md` to make that canonical instead of keeping the earlier generic `src/` suggestion.

## 3. PRD / stakeholder and system needs

- Problem: LLM answer evaluation through scalar judge scores is opaque, hard to debug, and difficult to reproduce. bin-eval decomposes evaluation into binary, task-specific questions and aggregates independent yes/no verdicts through deterministic Go scoring.
- Users: Internal engineers who evaluate model answers for a fixed task and context, reuse a checklist across multiple answers, and inspect question-level failures.
- Value: Reusable answer-independent checklists, explicit removal of weak or duplicate questions through weight 0, per-question evidence, deterministic weighted scoring, immutable persisted records, and raw-payload auditability.
- Business goals: Ship the smallest clean self-hosted implementation of binary checklist evaluation using Go, Temporal, Postgres, Garage, and one schema-constrained LLM boundary.
- Success metrics: All REQ acceptance criteria pass, all TEST definitions pass, EVAL-001 meets thresholds through the canonical API path, and the smoke path creates a checklist, evaluates answers, and returns succeeded weighted pass rates.
- Scope: `POST /checklists`, `GET /checklists/{id}`, `POST /evaluations`, `GET /evaluations/{id}`, question generation, Go ID assignment, weight assignment with 0 to 4 weights, active-question filtering, binary judging, Go scoring, Temporal workflows, Postgres persistence, Garage artifacts, one local Compose stack, and a curl-based smoke script.
- Non-goals: Dimensions, category-level scores, materialized weighted questions, reason-code enums, separate question deduplication, question rewrite steps, multi-judge scoring, learned weight calibration, manual review workflow, prompt optimization loops, legacy migration paths, fallback model providers, storage adapters for unused backends, dashboarding, and reporting UI.
- Dependencies: Go toolchain, Docker Engine with Compose v2, Postgres, Temporal, Garage, and a configured self-hosted LLM runtime reachable at `BIN_EVAL_LLM_BASE_URL` that supports schema-constrained JSON.
- Risks: The configured LLM may produce structurally valid but low-quality candidate questions, overuse weight 0, underuse weight 0 for duplicates, or fail to separate good and bad answers. Garage and Temporal local integration may be environment-sensitive. Docker image tags may need a documented owner decision if unavailable.
- Assumptions: The repo currently has no Go module, Makefile, source tree, or CI config. The file `AGENTS.md` documents a minimal workspace. The artifact `2606.27226` is research context only and is not executable source. The current Git remote is `https://github.com/kirilligum/self-imp-bin-eval.git`.
- Repository constraints: Keep the top level focused on metadata and entry points; add one canonical command per task as soon as tooling exists; add tests with the first behavior change; keep fixtures small and checked in under `fixtures/`; keep generated logs such as `firebase-debug.log` out of commits; inspect the tree before editing and preserve user changes.

## 4. SRS / canonical requirements

### Functional requirements

- REQ-001 (func): `POST /checklists` accepts `{task, context}`, starts `CreateChecklistWorkflow`, and returns `{checklist_id, status: "running"}`. Acceptance: a Postgres-generated UUID checklist ID is returned, a running checklist row exists, and a Temporal workflow execution is started for that ID.
- REQ-002 (func): `question_generation` returns schema-valid JSON containing at least one candidate question with non-empty `rationale` and `question`. Acceptance: empty question lists, blank rationales, and blank question text fail validation.
- REQ-003 (func): Go assigns persistent candidate question IDs `q1..qN` in array order after parsing. Acceptance: IDs are dense, stable, deterministic, and never supplied by the LLM.
- REQ-004 (func): `weight_assignment` returns exactly one weight object for every candidate question ID, with integer weight from 0 to 4. Acceptance: weight 0 marks exclusion from the active checklist; weights 1 to 4 mark active importance; missing, duplicate, out-of-range, or unknown-ID weights fail validation.
- REQ-005 (func): Checklist creation persists candidate questions, question rationales, weights, weight rationales, and artifact keys. Acceptance: successful checklists are immutable; semantic content is not edited after success.
- REQ-006 (func): `POST /evaluations` accepts `{checklist_id, model_answer}`, starts `EvaluateAnswerWorkflow` for an existing succeeded checklist, and returns `{evaluation_id, status: "running"}`. Acceptance: unknown or non-succeeded checklists do not start a running evaluation.
- REQ-007 (func): `binary_judging` receives task, context, model answer, and active questions only, with no weights and no rationales. Acceptance: the judging request payload contains question ID and question text only for weight 1 to 4 questions.
- REQ-008 (func): `binary_judging` returns exactly one judgment per active question with `question_id`, non-whitespace `evidence`, and `answer` in `{"yes","no"}`. Acceptance: missing, duplicate, inactive, unknown, blank-evidence, or non-enum judgments fail validation.
- REQ-009 (func): `ScoreChecklist` computes `satisfied_points`, `total_possible_points`, `checklist_pass_rate`, and `failed_question_ids` over active questions only. Acceptance: weight 0 questions require no judgment, never contribute to totals, and never appear in failed question IDs; all-zero checklists fail creation.
- REQ-010 (func): `GET /checklists/{id}` and `GET /evaluations/{id}` return documented running, succeeded, and failed response shapes. Acceptance: checklist success returns candidate questions and all weights including zero; evaluation success returns weighted score, failed active question IDs, and judgments.
- REQ-011 (func): Raw text artifacts are stored in Garage for task, context, model answer, question generation requests/responses, weight assignment requests/responses, and binary judging requests/responses. Acceptance: deterministic key construction locates each raw payload by checklist or evaluation ID.
- REQ-012 (func): Temporal runs one workflow per product action: `CreateChecklistWorkflow` and `EvaluateAnswerWorkflow`. Acceptance: external side effects execute only in activities; pure validation and scoring execute in Go functions.
- REQ-013 (func): Model behavior is validated only through the canonical API path using committed smoke fixtures. Acceptance: the e2e smoke command runs at least two cases, creates one checklist per case, evaluates good and bad answers against each case checklist, and reports aggregate pass-rate separation through persisted evaluations.
- REQ-014 (func): `scripts/smoke_curl.sh` drives create checklist, poll checklist, create evaluation, poll evaluation, and print final score fields through `make test-e2e`. Acceptance: the target exits zero only when every case checklist and required evaluations reach `succeeded` with parseable score output.

### Interface/API requirements

- REQ-020 (int): The only LLM interface is `LLMClient.GenerateJSON(ctx, req, out)`. Acceptance: Go code targets one configured OpenAI-compatible schema-constrained HTTP JSON endpoint under `BIN_EVAL_LLM_BASE_URL`; no provider SDK, fallback provider, plain-text parser, or repair prompt exists.
- REQ-021 (int): The async HTTP API exposes exactly four MVP routes: `POST /checklists`, `GET /checklists/{checklist_id}`, `POST /evaluations`, and `GET /evaluations/{evaluation_id}`. Acceptance: request and response payloads match the API contract field-for-field; POST accepted returns `202`, GET returns `200`, invalid JSON or invalid request body returns `400`, unknown IDs return `404`, evaluation creation against a non-succeeded checklist returns `409`, infrastructure failure returns `500`, and update/delete routes are absent.

### Data requirements

- REQ-030 (data): Postgres implements tables `checklists`, `questions`, `weights`, `evaluations`, and `judgments`. Acceptance: migrations produce the specified columns, primary keys, foreign keys, status constraints, and unique coverage constraints.
- REQ-031 (data): Garage implements one bucket named `bin-eval-artifacts` with deterministic key layout for raw text and raw LLM payloads. Acceptance: key builder emits exactly the documented paths and byte-identical reads match writes.
- REQ-032 (data): Checklists and evaluations are immutable after success. Acceptance: allowed lifecycle transitions are `running -> succeeded` and `running -> failed`; no update endpoints exist.

### Non-functional requirements

- REQ-040 (reliability): Bounded retries apply only to infrastructure errors; invalid model output is non-retryable. Acceptance: network timeout, temporary LLM endpoint failure, temporary Garage failure, and temporary Postgres or Temporal connectivity failure use bounded retries; invalid JSON, schema violation, missing weight, invalid weight, missing judgment, invalid answer, and unknown question ID fail without retry.
- REQ-041 (security): Secrets load only from environment variables and are not written to logs, Postgres structured fields, or smoke output. Garage stores evaluator inputs and LLM payloads byte-for-byte for auditability; operators must not place secrets in `task`, `context`, or `model_answer`. Acceptance: config validation fails fast on missing required variables and tests assert secret redaction for logs.
- REQ-042 (nfr): bin-eval logs JSON to stdout with request ID, workflow ID, entity IDs, activity type, prompt name, model profile, status, error class, duration, and git SHA when available. Acceptance: unit tests assert log field presence and low-cardinality labels.
- REQ-044 (nfr): The repo provides reproducible top-level commands `make lint`, `make build`, `make test`, `make test-integration`, and `make test-e2e`. Acceptance: commands exist after P00 and exit zero at relevant phase boundaries.

### Error handling and telemetry expectations

- Invalid model output uses a typed semantic or schema error and fails the current workflow without a repair prompt.
- Infrastructure errors use bounded Temporal retries with capped attempts and backoff.
- Workflow failure persists `error_message` and terminal `failed` status.
- Activity logs use two retry-relevant `error_class` values: `model_output_invalid` and `infra_retryable`.
- Logs never include raw `task`, `context`, `model_answer`, prompt request, prompt response, or secret environment variable values.
- Smoke output includes entity IDs, final status, score fields, and failed active question IDs.

### Architecture diagram

```mermaid
flowchart TB
  client[curl or smoke script] --> api[bin-eval-api]
  api --> temporal[Temporal Server]
  temporal --> worker[bin-eval-worker]
  worker --> pg[(Postgres)]
  worker --> garage[(Garage bucket bin-eval-artifacts)]
  worker --> llm[Self-hosted schema-constrained LLM runtime]
  api --> pg
```

C4-style ASCII representation:

```text
[Person: Engineer]
  -> [bin-eval HTTP API: Go]
  -> [Temporal Server]
  -> [bin-eval Worker: Go workflows and activities]

[bin-eval Worker]
  -> [Postgres: checklists, questions, weights, evaluations, judgments]
  -> [Garage: raw task/context/answer and raw LLM payload artifacts]
  -> [Self-hosted LLM runtime: schema-constrained JSON]

[Smoke script or curl]
  -> [bin-eval HTTP API] for the only executable operator workflow in the MVP
```

## 5. Iterative implementation and test plan

### Phase strategy

- Build order: toolchain and commands, pure domain logic, LLM boundary, one Compose-backed persistence stack, Temporal workflows, HTTP API, e2e smoke and docs.
- Verification-first: every behavior-changing implementation subtask follows failing coverage for the same REQ and TEST command.
- Quality-first gate: live model behavior is measured through the same API smoke path that production users exercise.
- Compute controls: `branch_limits = 2`, `reflection_passes = 1`, `early_stop% = 30`.
- Standards tailoring note: This plan is standards-informed and does not claim ISO/IEEE/FAA compliance. For safety-critical use, add development assurance level assumptions, independence expectations, review and analysis evidence, structural coverage expectations, tool qualification assumptions, and certification data outputs.
- Git tags such as `phase-p00-complete` are phase-boundary checkpoints only.

### Risk register

- Risk: Model creates duplicate or low-value questions. Trigger: EVAL-001 shows poor good/bad separation or excessive active-question noise. Mitigation: revise prompts through a documented owner decision; retain weight 0 as the only exclusion mechanism.
- Risk: Model overuses or underuses weight 0. Trigger: e2e smoke fixtures show all-zero failure or redundant questions staying active. Mitigation: revise the weight prompt; no separate dedup step.
- Risk: Schema-constrained JSON is unsupported by the configured runtime. Trigger: TEST-007 or TEST-015 fails at the protocol layer. Mitigation: mark runtime unsupported and suspend until a compatible self-hosted endpoint is configured.
- Risk: Temporal or Garage local integration is unstable. Trigger: integration test flakiness above one failure per ten repeats. Mitigation: pin image tags and add readiness polling in the single Compose path.
- Risk: Secrets leak to logs. Trigger: TEST-009 detects a secret value in captured logs. Mitigation: central redaction helper and denylist of secret environment variable names.

### Suspension/resumption criteria

- Suspend when model conformance thresholds fail, an external image or runtime contract is unavailable, or a requirement ambiguity blocks reliable implementation.
- Resume after the owner decision is recorded and the phase's full TEST and EVAL set passes from the last green phase checkpoint.

### Resolved owner decisions

- DEC-001: Checklist and evaluation IDs use Postgres-generated UUIDs.
- DEC-002: The self-hosted LLM runtime exposes one OpenAI-compatible schema-constrained HTTP endpoint under `BIN_EVAL_LLM_BASE_URL`.
- DEC-003: Garage artifacts preserve raw submitted and generated payload bytes; redaction applies only to logs and smoke output.
- DEC-004: Temporal remains mandatory in the MVP because the PRD requires one workflow per product action.
- DEC-005: `make test-integration` may manage the single Compose dependency stack; there is no testcontainers path.
- DEC-006: Empty or whitespace-only judgment evidence is invalid model output.
- DEC-007: `ScoreChecklist`, judgment validation, and workflow payload projection use one shared active-checklist helper instead of duplicating active-question logic.
- DEC-008: API statuses are pinned: POST accepted returns `202`, GET returns `200`, invalid JSON or invalid request body returns `400`, unknown IDs return `404`, evaluation creation against a non-succeeded checklist returns `409`, and infrastructure failure returns `500`.
- DEC-009: EVAL-001 uses at least two smoke fixture cases, with good and bad answers evaluated through the same four-route API path.

### Phase P00: Reproducible Go workspace and runtime foundations exist

Phase goal: The repo has a Go module, canonical Makefile commands, binary stubs, config loading, and structured logging foundations.

Scope and objectives: REQ-041, REQ-042, REQ-044.

Impacted surfaces: `go.mod`, `Makefile`, `.gitignore`, `cmd/bin-eval-api/main.go`, `cmd/bin-eval-worker/main.go`, `internal/config/config.go`, `internal/config/config_test.go`, `internal/observability/log.go`, `internal/observability/log_test.go`, `AGENTS.md`.

Lifecycle evidence:
- Requirements evidence: REQ-041, REQ-042, REQ-044.
- Design/code surface evidence: Go module path, Makefile targets, config package, observability package.
- Verification method: TEST-009 and TEST-017.
- Validation purpose: every later phase relies on consistent commands, environment config, and JSON logs that do not expose secret values or raw evaluator payloads.
- Configuration checkpoint: `phase-p00-complete`.
- Risks and assumptions: Go 1.23.x or newer is installed; no existing source tree is present.

Plan-and-Solve subtasks:

- `P00.S01 Add failing coverage for top-level command contract`
  - Action: Record the missing Makefile baseline for the top-level command contract.
  - Why now: Command creation must be validated before later phases reference Makefile targets.
  - Files/surfaces: `Makefile`.
  - Requirement link: REQ-044.
  - Verification link: TEST-017.
  - Verification mode: RED.
  - Command/procedure: `make lint build test && make -n test-integration test-e2e`
  - Expected result: Non-zero exit because `Makefile` does not exist.
  - Evidence produced: Terminal output in execution log.
  - Stop/escalate condition: None.
  - Unlocks: P00.S02
- `P00.S02 Create Go module, binary stubs, and Makefile targets`
  - Action: Add `go.mod` with module `github.com/kirilligum/self-imp-bin-eval`, create API and worker binary stubs, create `Makefile` targets `lint`, `build`, `test`, `test-integration`, and `test-e2e`, and extend `.gitignore` for generated debug artifacts.
  - Why now: Later tests and phases need stable commands before use.
  - Files/surfaces: `go.mod`, `Makefile`, `.gitignore`, `cmd/bin-eval-api/main.go`, `cmd/bin-eval-worker/main.go`.
  - Requirement link: REQ-044.
  - Verification link: TEST-017.
  - Verification mode: GREEN.
  - Command/procedure: `make lint build test && make -n test-integration test-e2e`
  - Expected result: Exit 0.
  - Evidence produced: Code diff and passing command output.
  - Stop/escalate condition: Escalate if local Go toolchain cannot compile an empty module.
  - Unlocks: P00.S03
- `P00.S03 Add failing coverage for config and structured logging`
  - Action: Add tests for required environment variables, secret-value redaction in logs, raw payload omission from logs, and required JSON log fields.
  - Why now: Config and log behavior must be specified before implementation.
  - Files/surfaces: `internal/config/config_test.go`, `internal/observability/log_test.go`.
  - Requirement link: REQ-041, REQ-042.
  - Verification link: TEST-009.
  - Verification mode: RED.
  - Command/procedure: `go test ./internal/config ./internal/observability -run 'TestConfigValidation|TestStructuredLogFields' -count=1`
  - Expected result: Compile failure because packages are absent.
  - Evidence produced: Test files with `// TEST-009` tags and failing output.
  - Stop/escalate condition: None.
  - Unlocks: P00.S04
- `P00.S04 Implement config loading and JSON log setup`
  - Action: Implement `internal/config` env loading and `internal/observability` JSON log setup with redaction of configured secret values and no raw evaluator payload fields.
  - Why now: API, worker, and activities share these foundations.
  - Files/surfaces: `internal/config/config.go`, `internal/observability/log.go`.
  - Requirement link: REQ-041, REQ-042.
  - Verification link: TEST-009.
  - Verification mode: GREEN.
  - Command/procedure: `go test ./internal/config ./internal/observability -run 'TestConfigValidation|TestStructuredLogFields' -count=1`
  - Expected result: Exit 0.
  - Evidence produced: Code diff and passing output.
  - Stop/escalate condition: Escalate if required env names conflict with later deployment configuration.
  - Unlocks: P00.S05
- `P00.S05 Update repository guidance for the selected Go layout`
  - Action: Update `AGENTS.md` to list Go commands, `cmd/`, `internal/`, `migrations/`, `deploy/compose/`, `scripts/`, `fixtures/`, and `plans/`.
  - Why now: The repository guide currently describes a minimal workspace and must match the selected implementation layout.
  - Files/surfaces: `AGENTS.md`.
  - Requirement link: REQ-044.
  - Verification link: TEST-017.
  - Verification mode: VERIFY.
  - Command/procedure: `make lint build test && make -n test-integration test-e2e`
  - Expected result: Exit 0. No refactor needed because the phase creates only small foundation packages and binary stubs.
  - Evidence produced: Documentation diff and passing output.
  - Stop/escalate condition: Escalate if repository guidance conflicts with user-owned edits.
  - Unlocks: Phase exit

Exit gates:
- Proceed: TEST-009 and TEST-017 pass.
- Escalate: missing Go toolchain, env naming ambiguity, or repository guidance conflict.
- Stop: top-level commands cannot be made reproducible in this repo.

Phase metrics:
- Confidence %: 95 - Standard Go scaffolding in a sparse repo.
- Long-term robustness %: 90 - Stable command and config foundations.
- Internal interactions: 2 - Config and observability are shared by all binaries.
- External interactions: 0 - No runtime services in this phase.
- Complexity %: 15 - Mostly scaffolding.
- Feature creep %: 0 - Only foundations required by later phases.
- Technical debt %: 5 - Binary stubs carry no product behavior.
- YAGNI score: 9 - Commands and packages are directly consumed later.
- MoSCoW: Must.
- Local/non-local scope: Local.
- Architectural changes count: 1.

### Phase P01: Pure evalcore domain logic handles candidate, active, and excluded questions

Phase goal: Go domain functions assign IDs, validate candidate outputs, validate weights with 0 exclusions, build one active-checklist projection, validate judgments, and score only active questions.

Scope and objectives: REQ-002, REQ-003, REQ-004, REQ-007, REQ-008, REQ-009.

Impacted surfaces: `internal/evalcore/types.go`, `internal/evalcore/ids.go`, `internal/evalcore/validate.go`, `internal/evalcore/active.go`, `internal/evalcore/score.go`, `internal/evalcore/*_test.go`.

Lifecycle evidence:
- Requirements evidence: REQ-002, REQ-003, REQ-004, REQ-007, REQ-008, REQ-009.
- Design/code surface evidence: Domain types, active-question projection, validation helpers, scoring function.
- Verification method: TEST-001, TEST-002, TEST-003, TEST-004, TEST-005.
- Validation purpose: Scoring and validation are the product trust boundary.
- Configuration checkpoint: `phase-p01-complete`.
- Risks and assumptions: Pure Go logic has no external dependencies.

Plan-and-Solve subtasks:

- `P01.S01 Add failing coverage for Go question ID assignment and active projection`
  - Action: Add tests for `AssignQuestionIDs` and `BuildActiveChecklist`, including stable IDs after weight 0 exclusion, deterministic active ordering, duplicate detection, unknown weight IDs, missing weights, invalid weight range, and all-zero failure.
  - Why now: ID stability and active filtering define how weight 0 works across the system.
  - Files/surfaces: `internal/evalcore/ids_test.go`.
  - Requirement link: REQ-003, REQ-004, REQ-009.
  - Verification link: TEST-002.
  - Verification mode: RED.
  - Command/procedure: `go test ./internal/evalcore -run 'TestAssignQuestionIDs|TestBuildActiveChecklist' -count=1`
  - Expected result: Compile failure because package and functions are absent.
  - Evidence produced: Test file with `// TEST-002` tag and failing output.
  - Stop/escalate condition: None.
  - Unlocks: P01.S02
- `P01.S02 Implement question ID assignment and active projection`
  - Action: Implement domain types, `AssignQuestionIDs`, and `BuildActiveChecklist` that validates weight coverage, returns only questions with weight 1 to 4, and preserves original IDs.
  - Why now: Validators and scoring depend on these types and helpers.
  - Files/surfaces: `internal/evalcore/types.go`, `internal/evalcore/ids.go`, `internal/evalcore/active.go`.
  - Requirement link: REQ-003, REQ-004, REQ-009.
  - Verification link: TEST-002.
  - Verification mode: GREEN.
  - Command/procedure: `go test ./internal/evalcore -run 'TestAssignQuestionIDs|TestBuildActiveChecklist' -count=1`
  - Expected result: Exit 0.
  - Evidence produced: Code diff and passing output.
  - Stop/escalate condition: Escalate if active projection semantics conflict with API response shape.
  - Unlocks: P01.S03
- `P01.S03 Add failing coverage for structural validators`
  - Action: Add validator tests for question generation, weights 0 to 4, exact weight coverage, duplicate weight rows, unknown weight IDs, active coverage, judgment coverage over active questions, inactive judgment rejection, empty or whitespace-only evidence rejection, and answer enum values.
  - Why now: Invalid model output must fail before implementation.
  - Files/surfaces: `internal/evalcore/validate_test.go`.
  - Requirement link: REQ-002, REQ-004, REQ-007, REQ-008.
  - Verification link: TEST-003, TEST-004, TEST-005.
  - Verification mode: RED.
  - Command/procedure: `go test ./internal/evalcore -run 'TestValidateQuestionGeneration|TestValidateWeights|TestValidateJudgments' -count=1`
  - Expected result: Compile failure because validators are absent.
  - Evidence produced: Test file with `// TEST-003`, `// TEST-004`, and `// TEST-005` tags plus failing output.
  - Stop/escalate condition: None.
  - Unlocks: P01.S04
- `P01.S04 Implement centralized structural validators`
  - Action: Implement `ValidateQuestionGeneration`, `ValidateWeights`, and `ValidateJudgments` with typed semantic errors while calling `BuildActiveChecklist` for shared active-question coverage.
  - Why now: LLM activities and workflows use these validators before persistence and scoring.
  - Files/surfaces: `internal/evalcore/validate.go`.
  - Requirement link: REQ-002, REQ-004, REQ-007, REQ-008.
  - Verification link: TEST-003, TEST-004, TEST-005.
  - Verification mode: GREEN.
  - Command/procedure: `go test ./internal/evalcore -run 'TestValidateQuestionGeneration|TestValidateWeights|TestValidateJudgments' -count=1`
  - Expected result: Exit 0.
  - Evidence produced: Code diff and passing output.
  - Stop/escalate condition: Escalate if semantic error taxonomy needs more categories than the retry policy recognizes.
  - Unlocks: P01.S05
- `P01.S05 Add failing coverage for deterministic weighted scoring`
  - Action: Add table-driven scoring tests for happy paths, weight 0 exclusions, all-zero failure, missing active judgment, inactive judgment rejection, duplicate rows, invalid weights, invalid answers, and failed active question ID ordering.
  - Why now: Scoring must be specified before implementation.
  - Files/surfaces: `internal/evalcore/score_test.go`.
  - Requirement link: REQ-009.
  - Verification link: TEST-001.
  - Verification mode: RED.
  - Command/procedure: `go test ./internal/evalcore -run TestScoreChecklist -count=1`
  - Expected result: Compile failure because `ScoreChecklist` is absent.
  - Evidence produced: Test file with `// TEST-001` tag and failing output.
  - Stop/escalate condition: None.
  - Unlocks: P01.S06
- `P01.S06 Implement ScoreChecklist over active questions only`
  - Action: Implement `ScoreChecklist` by calling `BuildActiveChecklist` so weight 0 questions contribute no points, require no judgment, and cannot appear in `failed_question_ids`.
  - Why now: Evaluation workflow depends on deterministic scoring.
  - Files/surfaces: `internal/evalcore/score.go`.
  - Requirement link: REQ-009.
  - Verification link: TEST-001.
  - Verification mode: GREEN.
  - Command/procedure: `go test ./internal/evalcore -run TestScoreChecklist -count=1`
  - Expected result: Exit 0.
  - Evidence produced: Code diff and passing output.
  - Stop/escalate condition: Escalate if API consumers need excluded-question diagnostics beyond persisted weights.
  - Unlocks: P01.S07
- `P01.S07 Refactor shared validation helpers`
  - Action: Keep duplicate detection, ID-set construction, and active-question coverage inside `BuildActiveChecklist` or private helpers used by it; remove any copied map-building logic from validators and scoring.
  - Why now: Validator and scoring implementation will otherwise duplicate weight and active-question logic.
  - Files/surfaces: `internal/evalcore/active.go`, `internal/evalcore/validate.go`, `internal/evalcore/score.go`.
  - Requirement link: REQ-002, REQ-004, REQ-007, REQ-008, REQ-009.
  - Verification link: TEST-001, TEST-002, TEST-003, TEST-004, TEST-005.
  - Verification mode: REFACTOR.
  - Command/procedure: `go test ./internal/evalcore -count=1`
  - Expected result: Exit 0 with unchanged behavior.
  - Evidence produced: Refactor diff and passing output.
  - Stop/escalate condition: Revert refactor if any evalcore behavior regresses.
  - Unlocks: Phase exit

Exit gates:
- Proceed: TEST-001 through TEST-005 pass.
- Escalate: weight 0 semantics, active projection, or all-zero checklist behavior becomes ambiguous.
- Stop: deterministic scoring cannot satisfy the PRD formula with exclusions.

Phase metrics:
- Confidence %: 95 - Pure functions with table-driven tests.
- Long-term robustness %: 95 - Stable scoring semantics and narrow surface.
- Internal interactions: 3 - Used by LLM validation, workflows, and API DTOs.
- External interactions: 0 - No I/O.
- Complexity %: 30 - Weight 0 active filtering adds edge cases.
- Feature creep %: 0 - No dimensions or rewrite step.
- Technical debt %: 5 - Shared helpers remove duplication without adding public abstractions.
- YAGNI score: 10 - Every function is consumed by later phases.
- MoSCoW: Must.
- Local/non-local scope: Local.
- Architectural changes count: 0.

### Phase P02: One schema-constrained LLM boundary exists

Phase goal: The service has one HTTP LLM client boundary plus prompt/schema builders for question generation, weight assignment, and binary judging.

Scope and objectives: REQ-002, REQ-004, REQ-007, REQ-008, REQ-020, REQ-040.

Impacted surfaces: `internal/llm/client.go`, `internal/llm/errors.go`, `internal/llm/prompts.go`, `internal/llm/schemas.go`, `internal/llm/client_test.go`, `internal/llm/schema_test.go`, `internal/llm/testdata/`.

Lifecycle evidence:
- Requirements evidence: REQ-002, REQ-004, REQ-007, REQ-008, REQ-020, REQ-040.
- Design/code surface evidence: One LLM interface, schema definitions, and prompt builders.
- Verification method: TEST-006 and TEST-007.
- Validation purpose: Prompt and schema contracts are implemented once before workflows call the model.
- Configuration checkpoint: `phase-p02-complete`.
- Risks and assumptions: `BIN_EVAL_LLM_BASE_URL` points to a self-hosted runtime supporting OpenAI-compatible schema-constrained JSON.

Plan-and-Solve subtasks:

- `P02.S01 Add failing coverage for LLM output schemas and prompts`
  - Action: Add tests for question generation, weight assignment, and binary judging schemas; assert weight schema allows 0 to 4, checklist creation prompts exclude `model_answer`, and judging schema excludes weights and rationales.
  - Why now: Schema and prompt contracts must exist before an HTTP client sends requests.
  - Files/surfaces: `internal/llm/schema_test.go`, `internal/llm/testdata/*.json`.
  - Requirement link: REQ-002, REQ-004, REQ-007, REQ-008, REQ-020.
  - Verification link: TEST-006.
  - Verification mode: RED.
  - Command/procedure: `go test ./internal/llm -run TestOutputSchemasAndPrompts -count=1`
  - Expected result: Compile failure because LLM package is absent.
  - Evidence produced: Test file with `// TEST-006` tag and fixtures.
  - Stop/escalate condition: None.
  - Unlocks: P02.S02
- `P02.S02 Implement LLM schemas and prompt builders`
  - Action: Implement schema definitions and prompt builders for `question_generation`, `weight_assignment`, and `binary_judging`, including weight 0 exclusion instructions.
  - Why now: Workflow activities depend on stable request construction.
  - Files/surfaces: `internal/llm/schemas.go`, `internal/llm/prompts.go`.
  - Requirement link: REQ-002, REQ-004, REQ-007, REQ-008, REQ-020.
  - Verification link: TEST-006.
  - Verification mode: GREEN.
  - Command/procedure: `go test ./internal/llm -run TestOutputSchemasAndPrompts -count=1`
  - Expected result: Exit 0.
  - Evidence produced: Code diff and passing output.
  - Stop/escalate condition: Escalate if schema-constrained output cannot represent evidence-before-answer ordering.
  - Unlocks: P02.S03
- `P02.S03 Add failing coverage for LLM client error behavior`
  - Action: Add HTTP stub tests asserting schema request payload, bearer token use, valid decode, invalid JSON non-retryable behavior, schema violation non-retryable behavior, and no fallback endpoint call.
  - Why now: The one LLM boundary must be specified before implementation.
  - Files/surfaces: `internal/llm/client_test.go`.
  - Requirement link: REQ-020, REQ-040.
  - Verification link: TEST-007.
  - Verification mode: RED.
  - Command/procedure: `go test ./internal/llm -run TestGenerateJSONClient -count=1`
  - Expected result: Compile failure because client is absent.
  - Evidence produced: Test file with `// TEST-007` tag and failing output.
  - Stop/escalate condition: None.
  - Unlocks: P02.S04
- `P02.S04 Implement one LLM client`
  - Action: Implement `LLMClient.GenerateJSON`, typed errors, configured base URL, API key handling, and schema-constrained request body.
  - Why now: Activities and workflow tests need one shared boundary.
  - Files/surfaces: `internal/llm/client.go`, `internal/llm/errors.go`.
  - Requirement link: REQ-020, REQ-040.
  - Verification link: TEST-007.
  - Verification mode: GREEN.
  - Command/procedure: `go test ./internal/llm -run TestGenerateJSONClient -count=1`
  - Expected result: Exit 0.
  - Evidence produced: Code diff and passing output.
  - Stop/escalate condition: Escalate if runtime API contract requires provider-specific SDK code.
  - Unlocks: P02.S05
- `P02.S05 Confirm no refactor needed for the LLM boundary`
  - Action: Inspect `internal/llm` for duplicate schema, validation, or prompt construction logic.
  - Why now: Prompt and schema construction must stay centralized before workflows call it.
  - Files/surfaces: `internal/llm`.
  - Requirement link: REQ-020.
  - Verification link: TEST-006, TEST-007.
  - Verification mode: VERIFY.
  - Command/procedure: `go test ./internal/llm -count=1`
  - Expected result: Exit 0. No refactor needed because prompt construction lives in `internal/llm` and semantic validation lives in `internal/evalcore`.
  - Evidence produced: Passing output and inspection note.
  - Stop/escalate condition: Convert to REFACTOR if validation is duplicated.
  - Unlocks: Phase exit

Exit gates:
- Proceed: TEST-006 and TEST-007 pass.
- Escalate: schema-constrained runtime contract is ambiguous or incompatible with a single HTTP client.
- Stop: no configured self-hosted runtime can satisfy the MVP LLM contract.

Phase metrics:
- Confidence %: 85 - The client and prompt builders are small after removing the extra CLI path.
- Long-term robustness %: 85 - One boundary isolates runtime changes.
- Internal interactions: 2 - evalcore and config.
- External interactions: 1 - Self-hosted LLM runtime.
- Complexity %: 25 - Schema outputs add edge cases, but there is no CLI aggregation path.
- Feature creep %: 0 - No fallback path and no separate model-test surface.
- Technical debt %: 5 - Tests use HTTP stubs while production code keeps one HTTP client.
- YAGNI score: 10 - Every function is consumed by workflow activities.
- MoSCoW: Must.
- Local/non-local scope: Local plus external runtime.
- Architectural changes count: 0.

### Phase P03: One Compose-backed persistence stack matches the MVP data contract

Phase goal: One Compose file starts MVP dependency services, and concrete Postgres plus Garage packages persist structured state and byte-preserving raw artifacts.

Scope and objectives: REQ-005, REQ-011, REQ-030, REQ-031, REQ-032, REQ-041, REQ-044.

Impacted surfaces: `deploy/compose/docker-compose.yml`, `deploy/compose/.env.example`, `deploy/compose/garage.toml`, `migrations/0001_init.sql`, `internal/db/store.go`, `internal/db/db_integration_test.go`, `internal/artifacts/keys.go`, `internal/artifacts/writer.go`, `internal/artifacts/artifacts_integration_test.go`.

Lifecycle evidence:
- Requirements evidence: REQ-005, REQ-011, REQ-030, REQ-031, REQ-032, REQ-041, REQ-044.
- Design/code surface evidence: One Compose dependency stack, migration DDL, concrete pgx store, Garage key builder, artifact writer.
- Verification method: TEST-010, TEST-011, TEST-014, and TEST-020.
- Validation purpose: Persistence must protect immutable structured state and raw-payload auditability without duplicate local infrastructure paths.
- Configuration checkpoint: `phase-p03-complete`.
- Risks and assumptions: Docker is available; Garage image `dxflrs/garage:v2.3.0`, Postgres image `postgres:16.4`, and Temporal image `temporalio/auto-setup:1.28.4` are pullable.

Plan-and-Solve subtasks:

- `P03.S01 Add failing coverage for Compose dependency stack`
  - Action: Add `.env.example` with non-secret placeholders and record the missing Compose baseline.
  - Why now: Integration tests must use the same dependency stack instead of creating disposable service paths.
  - Files/surfaces: `deploy/compose/.env.example`, `deploy/compose/docker-compose.yml`.
  - Requirement link: REQ-044.
  - Verification link: TEST-014.
  - Verification mode: RED.
  - Command/procedure: `make test-integration`
  - Expected result: Non-zero exit because `docker-compose.yml` is absent.
  - Evidence produced: Env template and command output.
  - Stop/escalate condition: None.
  - Unlocks: P03.S02
- `P03.S02 Implement the single Compose dependency stack`
  - Action: Add Compose services for `postgres:16.4`, `temporalio/auto-setup:1.28.4`, and `dxflrs/garage:v2.3.0` with one `.env.example` naming convention.
  - Why now: Postgres, Garage, and later Temporal tests need a common local dependency contract.
  - Files/surfaces: `deploy/compose/docker-compose.yml`, `deploy/compose/.env.example`, `deploy/compose/garage.toml`.
  - Requirement link: REQ-031, REQ-044.
  - Verification link: TEST-014.
  - Verification mode: GREEN.
  - Command/procedure: `make test-integration`
  - Expected result: Exit 0.
  - Evidence produced: Compose config diff and passing output.
  - Stop/escalate condition: Escalate if a pinned image tag is unavailable; use an owner decision before changing the tag.
  - Unlocks: P03.S03
- `P03.S03 Add failing coverage for Postgres migrations and concrete store`
  - Action: Add an integration test that applies migrations, inspects schema, inserts running and succeeded checklists, persists questions and weights including weight 0, creates evaluations, persists judgments and score, rejects invalid lifecycle transitions, rejects duplicate weight rows, rejects duplicate judgment rows, rejects cross-checklist judgments, and confirms raw task, context, answer, and LLM payload text are not stored in Postgres.
  - Why now: Schema and immutability must be specified before migrations.
  - Files/surfaces: `internal/db/db_integration_test.go`.
  - Requirement link: REQ-005, REQ-030, REQ-032.
  - Verification link: TEST-010, TEST-020.
  - Verification mode: RED.
  - Command/procedure: `make test-integration`
  - Expected result: Failure because migrations and db package are absent.
  - Evidence produced: Test file with `// TEST-010` tag and failing output.
  - Stop/escalate condition: Escalate if Docker is unavailable.
  - Unlocks: P03.S04
- `P03.S04 Implement migrations and concrete pgx store`
  - Action: Add DDL for `checklists`, `questions`, `weights`, `evaluations`, and `judgments`; implement `internal/db.Store` methods with explicit SQL constants and pgx; enforce lifecycle, uniqueness, cross-checklist foreign keys, and raw-text exclusion from structured tables.
  - Why now: Workflows and API reads depend on persisted state.
  - Files/surfaces: `migrations/0001_init.sql`, `internal/db/store.go`.
  - Requirement link: REQ-005, REQ-030, REQ-032.
  - Verification link: TEST-010, TEST-020.
  - Verification mode: GREEN.
  - Command/procedure: `make test-integration`
  - Expected result: Exit 0.
  - Evidence produced: Migration and store code diff plus passing output.
  - Stop/escalate condition: Escalate if schema cannot express exact weight coverage, judgment question integrity, and immutable lifecycle constraints.
  - Unlocks: P03.S05
- `P03.S05 Add failing coverage for Garage artifact layout`
  - Action: Add an integration test for deterministic keys, bucket writes, byte-identical reads, and required artifact key classes for checklist inputs, evaluation inputs, and LLM request/response payload families.
  - Why now: Raw artifact storage must be specified before writer implementation.
  - Files/surfaces: `internal/artifacts/artifacts_integration_test.go`.
  - Requirement link: REQ-011, REQ-031, REQ-041.
  - Verification link: TEST-011.
  - Verification mode: RED.
  - Command/procedure: `make test-integration`
  - Expected result: Failure because artifact package is absent.
  - Evidence produced: Test file with `// TEST-011` tag and failing output.
  - Stop/escalate condition: Escalate if Garage service cannot start after two attempts.
  - Unlocks: P03.S06
- `P03.S06 Implement Garage key builder and artifact writer`
  - Action: Implement deterministic key construction and S3-compatible Garage writes for checklist inputs, evaluation inputs, and all LLM request/response payloads without redacting artifact bytes.
  - Why now: Temporal activities write artifacts before and after LLM calls.
  - Files/surfaces: `internal/artifacts/keys.go`, `internal/artifacts/writer.go`.
  - Requirement link: REQ-011, REQ-031, REQ-041.
  - Verification link: TEST-011.
  - Verification mode: GREEN.
  - Command/procedure: `make test-integration`
  - Expected result: Exit 0.
  - Evidence produced: Code diff and passing output.
  - Stop/escalate condition: Escalate if Garage S3 API behavior differs from the expected path-style contract.
  - Unlocks: P03.S07
- `P03.S07 Confirm no refactor needed for persistence`
  - Action: Inspect persistence packages for duplicate key construction, raw text duplication in Postgres, unused storage abstractions, or duplicate Compose definitions.
  - Why now: This phase touches two storage systems and must keep ownership boundaries clear.
  - Files/surfaces: `internal/db`, `internal/artifacts`, `deploy/compose/docker-compose.yml`.
  - Requirement link: REQ-011, REQ-030, REQ-031, REQ-044.
  - Verification link: TEST-010, TEST-011, TEST-014, TEST-020.
  - Verification mode: VERIFY.
  - Command/procedure: `make test-integration`
  - Expected result: Exit 0. No refactor needed because Postgres stores structured state and Garage stores raw payload bytes through one key builder.
  - Evidence produced: Passing integration output and inspection note.
  - Stop/escalate condition: Convert to REFACTOR if raw text is duplicated into Postgres or another storage abstraction appears.
  - Unlocks: Phase exit

Exit gates:
- Proceed: TEST-010, TEST-011, TEST-014, and TEST-020 pass.
- Escalate: container availability, schema ambiguity, or Garage key ambiguity.
- Stop: canonical structured state and raw artifact split cannot be implemented without new storage scope.

Phase metrics:
- Confidence %: 82 - Declarative schema and deterministic keys are testable, but Garage local setup may need tuning.
- Long-term robustness %: 90 - Storage ownership and local dependency ownership are explicit.
- Internal interactions: 3 - db, artifacts, config.
- External interactions: 3 - Postgres, Temporal, and Garage.
- Complexity %: 40 - Integration tests and lifecycle constraints share one Compose path.
- Feature creep %: 0 - No alternate storage adapters or testcontainers path.
- Technical debt %: 5 - Concrete store keeps the data layer small.
- YAGNI score: 9 - Tables, keys, and Compose services map directly to PRD.
- MoSCoW: Must.
- Local/non-local scope: Non-local.
- Architectural changes count: 1.

### Phase P04: Temporal workflows execute checklist creation and answer evaluation

Phase goal: `CreateChecklistWorkflow` and `EvaluateAnswerWorkflow` orchestrate activities, apply retry policy, persist lifecycle status, and call pure evalcore functions.

Scope and objectives: REQ-001, REQ-002, REQ-003, REQ-004, REQ-006, REQ-007, REQ-008, REQ-009, REQ-012, REQ-040.

Impacted surfaces: `internal/workflows/create_checklist.go`, `internal/workflows/evaluate_answer.go`, `internal/workflows/*_test.go`, `internal/activities/llm.go`, `internal/activities/llm_test.go`, `internal/activities/postgres.go`, `internal/activities/garage.go`, `internal/activities/errors.go`, `internal/activities/retry_test.go`, `cmd/bin-eval-worker/main.go`.

Lifecycle evidence:
- Requirements evidence: REQ-001 through REQ-009, REQ-012, REQ-040.
- Design/code surface evidence: Workflow definitions, concrete activity methods, activity error mapping.
- Verification method: TEST-008, TEST-012, TEST-018, and TEST-019.
- Validation purpose: The product actions are durable and side effects are isolated in activities.
- Configuration checkpoint: `phase-p04-complete`.
- Risks and assumptions: Temporal Go SDK testsuite supports activity mocking required for order and payload assertions.

Plan-and-Solve subtasks:

- `P04.S01 Add failing coverage for LLM activity artifacts and payload projection`
  - Action: Add activity tests that assert question generation, weight assignment, and binary judging write raw request and response artifacts, call the single LLM client boundary, and build the judge request from active question IDs and question text only.
  - Why now: Activity side effects and active-only payload construction must be specified before workflow implementation depends on them.
  - Files/surfaces: `internal/activities/llm_test.go`.
  - Requirement link: REQ-007, REQ-011, REQ-020, REQ-040.
  - Verification link: TEST-008.
  - Verification mode: RED.
  - Command/procedure: `go test ./internal/activities -run TestLLMActivitiesWriteArtifactsAndPayloads -count=1`
  - Expected result: Compile failure because activity package behavior is absent.
  - Evidence produced: Test file with `// TEST-008` tag and failing output.
  - Stop/escalate condition: None.
  - Unlocks: P04.S02
- `P04.S02 Implement LLM activity artifact and payload behavior`
  - Action: Implement concrete LLM activity methods with one shared private helper for request artifact writing, prompt execution, response artifact writing, typed error mapping, and `BuildActiveChecklist`-based judge payload projection.
  - Why now: Workflows need deterministic activity behavior before orchestration tests.
  - Files/surfaces: `internal/activities/llm.go`, `internal/activities/garage.go`.
  - Requirement link: REQ-007, REQ-011, REQ-020, REQ-040.
  - Verification link: TEST-008.
  - Verification mode: GREEN.
  - Command/procedure: `go test ./internal/activities -run TestLLMActivitiesWriteArtifactsAndPayloads -count=1`
  - Expected result: Exit 0.
  - Evidence produced: Code diff and passing output.
  - Stop/escalate condition: Escalate if raw artifact capture would require a second LLM path.
  - Unlocks: P04.S03
- `P04.S03 Add failing coverage for checklist and evaluation workflows`
  - Action: Add Temporal testsuite coverage for happy paths, question exclusion through weight 0, all-zero checklist failure, judge payload excluding weights/rationales, missing weight failure, missing judgment failure, failed terminal status persistence, error message persistence, activity order, and non-succeeded checklist rejection.
  - Why now: Workflow behavior must be specified before orchestration code.
  - Files/surfaces: `internal/workflows/create_checklist_test.go`, `internal/workflows/evaluate_answer_test.go`, `internal/workflows/testdata/`.
  - Requirement link: REQ-001, REQ-002, REQ-003, REQ-004, REQ-006, REQ-007, REQ-008, REQ-009, REQ-011, REQ-012.
  - Verification link: TEST-012, TEST-019.
  - Verification mode: RED.
  - Command/procedure: `go test ./internal/workflows -run 'TestCreateChecklistWorkflow|TestEvaluateAnswerWorkflow|TestWorkflowFailurePersistence' -count=1`
  - Expected result: Compile failure because workflows are absent.
  - Evidence produced: Test files with `// TEST-012` and `// TEST-019` tags plus fixtures.
  - Stop/escalate condition: None.
  - Unlocks: P04.S04
- `P04.S04 Implement workflows and concrete persistence activities`
  - Action: Implement both workflows, concrete Postgres and Garage activity methods, worker registration, checklist lifecycle handling, evaluation lifecycle handling, artifact writes, LLM calls, validation, active projection, scoring, failure persistence, and status persistence.
  - Why now: This completes durable product behavior after pure logic, persistence, and LLM activities exist.
  - Files/surfaces: `internal/workflows/create_checklist.go`, `internal/workflows/evaluate_answer.go`, `internal/activities/postgres.go`, `internal/activities/garage.go`, `cmd/bin-eval-worker/main.go`.
  - Requirement link: REQ-001, REQ-002, REQ-003, REQ-004, REQ-006, REQ-007, REQ-008, REQ-009, REQ-011, REQ-012.
  - Verification link: TEST-012, TEST-019.
  - Verification mode: GREEN.
  - Command/procedure: `go test ./internal/workflows -run 'TestCreateChecklistWorkflow|TestEvaluateAnswerWorkflow|TestWorkflowFailurePersistence' -count=1`
  - Expected result: Exit 0.
  - Evidence produced: Code diff and passing output.
  - Stop/escalate condition: Escalate if workflow determinism conflicts with direct helper calls.
  - Unlocks: P04.S05
- `P04.S05 Add failing coverage for retry classification`
  - Action: Add tests mapping infrastructure errors to retryable Temporal application errors and all invalid model output cases to one non-retryable category.
  - Why now: Retry behavior must be specified before wiring activity options.
  - Files/surfaces: `internal/activities/retry_test.go`.
  - Requirement link: REQ-040.
  - Verification link: TEST-018.
  - Verification mode: RED.
  - Command/procedure: `go test ./internal/activities -run TestRetryClassification -count=1`
  - Expected result: Compile failure because classification code is absent.
  - Evidence produced: Test file with `// TEST-018` tag and failing output.
  - Stop/escalate condition: None.
  - Unlocks: P04.S06
- `P04.S06 Implement bounded retry policy and error mapping`
  - Action: Implement activity error mapping and workflow activity options with bounded attempts and backoff using only the retryable infrastructure and non-retryable model-output categories.
  - Why now: Workflows need correct failure semantics before API exposure.
  - Files/surfaces: `internal/activities/errors.go`, `internal/workflows/create_checklist.go`, `internal/workflows/evaluate_answer.go`.
  - Requirement link: REQ-040.
  - Verification link: TEST-018.
  - Verification mode: GREEN.
  - Command/procedure: `go test ./internal/activities -run TestRetryClassification -count=1`
  - Expected result: Exit 0.
  - Evidence produced: Code diff and passing output.
  - Stop/escalate condition: Escalate if an error class cannot be represented safely in Temporal retries.
  - Unlocks: P04.S07
- `P04.S07 Refactor shared workflow and activity plumbing`
  - Action: Remove duplicated request/response artifact handling, status-transition code, and active-question projection from activities and workflows by keeping one helper per responsibility.
  - Why now: The green workflow implementation touches repeated status and artifact operations.
  - Files/surfaces: `internal/activities/llm.go`, `internal/workflows/create_checklist.go`, `internal/workflows/evaluate_answer.go`.
  - Requirement link: REQ-011, REQ-012, REQ-020.
  - Verification link: TEST-008, TEST-012, TEST-018, TEST-019.
  - Verification mode: REFACTOR.
  - Command/procedure: `go test ./internal/workflows ./internal/activities -count=1`
  - Expected result: Exit 0 with unchanged behavior.
  - Evidence produced: Refactor diff and passing output.
  - Stop/escalate condition: Revert refactor if workflow or retry tests regress.
  - Unlocks: Phase exit

Exit gates:
- Proceed: TEST-008, TEST-012, TEST-018, and TEST-019 pass.
- Escalate: Temporal testsuite limitation, retry ambiguity, or workflow determinism issue.
- Stop: product workflows cannot meet the single-path contract.

Phase metrics:
- Confidence %: 80 - Largest orchestration surface.
- Long-term robustness %: 85 - Activity boundaries isolate side effects.
- Internal interactions: 6 - evalcore, llm, db, artifacts, config, observability.
- External interactions: 3 - Temporal, Postgres, Garage.
- Complexity %: 55 - Durable orchestration and failure handling.
- Feature creep %: 0 - One workflow per product action.
- Technical debt %: 10 - Refactor handles repeated LLM activity plumbing.
- YAGNI score: 9 - Workflow structure maps directly to PRD.
- MoSCoW: Must.
- Local/non-local scope: Non-local.
- Architectural changes count: 1.

### Phase P05: Async HTTP API exposes checklist and evaluation contracts

Phase goal: The Go API exposes four async routes with exact request and response payloads backed by Temporal and Postgres reads.

Scope and objectives: REQ-001, REQ-006, REQ-010, REQ-021, REQ-032.

Impacted surfaces: `internal/api/router.go`, `internal/api/checklists.go`, `internal/api/evaluations.go`, `internal/api/api_test.go`, `cmd/bin-eval-api/main.go`.

Lifecycle evidence:
- Requirements evidence: REQ-001, REQ-006, REQ-010, REQ-021, REQ-032.
- Design/code surface evidence: HTTP handlers, DTOs, test doubles, and concrete `db.Store` read methods.
- Verification method: TEST-013.
- Validation purpose: API consumers rely on exact async contracts and immutable read shapes.
- Configuration checkpoint: `phase-p05-complete`.
- Risks and assumptions: Authentication is outside MVP; deployment access control is handled by environment and network boundary.

Plan-and-Solve subtasks:

- `P05.S01 Add failing coverage for API contracts`
  - Action: Add httptest coverage for the four allowed routes, absence of update/delete routes, exact request validation, exact running/succeeded/failed response shapes, checklist success with weight 0 retained, evaluation success with active-only judgments, and pinned statuses: `202` for POST accepted, `200` for GET, `400` for invalid JSON or invalid request body, `404` for unknown IDs, `409` for evaluation creation against a non-succeeded checklist, and `500` for infrastructure failure.
  - Why now: API behavior must be specified before handler implementation.
  - Files/surfaces: `internal/api/api_test.go`.
  - Requirement link: REQ-001, REQ-006, REQ-010, REQ-021, REQ-032.
  - Verification link: TEST-013.
  - Verification mode: RED.
  - Command/procedure: `go test ./internal/api -run 'TestAPIContracts|TestAPIRouteSurface' -count=1`
  - Expected result: Compile failure because API package is absent.
  - Evidence produced: Test file with `// TEST-013` tag and failing output.
  - Stop/escalate condition: None.
  - Unlocks: P05.S02
- `P05.S02 Implement HTTP router and handlers`
  - Action: Implement route registration, DTO validation, Temporal workflow starts, direct `db.Store` reads, pinned status mapping, and JSON error responses.
  - Why now: Workflows and persistence are available for API wiring.
  - Files/surfaces: `internal/api/router.go`, `internal/api/checklists.go`, `internal/api/evaluations.go`, `cmd/bin-eval-api/main.go`.
  - Requirement link: REQ-001, REQ-006, REQ-010, REQ-021, REQ-032.
  - Verification link: TEST-013.
  - Verification mode: GREEN.
  - Command/procedure: `go test ./internal/api -run 'TestAPIContracts|TestAPIRouteSurface' -count=1`
  - Expected result: Exit 0.
  - Evidence produced: Code diff and passing output.
  - Stop/escalate condition: Escalate if API response fields conflict with persisted schema.
  - Unlocks: P05.S03
- `P05.S03 Confirm no refactor needed for API layer`
  - Action: Inspect DTO mapping and handler dependencies for duplicated response construction.
  - Why now: Handler code can drift from exact contract shapes if response construction is scattered.
  - Files/surfaces: `internal/api`.
  - Requirement link: REQ-010, REQ-021.
  - Verification link: TEST-013.
  - Verification mode: VERIFY.
  - Command/procedure: `go test ./internal/api -count=1`
  - Expected result: Exit 0. No refactor needed because route handlers use shared response DTO builders and one status-mapping helper.
  - Evidence produced: Passing output and inspection note.
  - Stop/escalate condition: Convert to REFACTOR if response DTOs are duplicated.
  - Unlocks: Phase exit

Exit gates:
- Proceed: TEST-013 passes.
- Escalate: API contract ambiguity or route behavior conflict.
- Stop: exact async API contract cannot be met without adding routes.

Phase metrics:
- Confidence %: 90 - Handlers are straightforward with test doubles.
- Long-term robustness %: 85 - DTO tests lock the contract.
- Internal interactions: 3 - API, db store, Temporal client.
- External interactions: 1 - Temporal at runtime.
- Complexity %: 30 - Async status shapes add cases.
- Feature creep %: 0 - Four routes only.
- Technical debt %: 5 - Shared DTO helpers prevent drift.
- YAGNI score: 10 - No auth/session/UI scope.
- MoSCoW: Must.
- Local/non-local scope: Non-local.
- Architectural changes count: 0.

### Phase P06: End-to-end smoke path and docs prove MVP acceptance

Phase goal: A curl-first smoke script and operator docs prove checklist creation, answer evaluation, active-only judging, deterministic scoring, persistence, artifacts, and failure behavior.

Scope and objectives: REQ-001 through REQ-014, REQ-020, REQ-021, REQ-030 through REQ-032, REQ-040 through REQ-044.

Impacted surfaces: `scripts/smoke_curl.sh`, `docs/curl.md`, `fixtures/smoke/cases/*/task.json`, `fixtures/smoke/cases/*/model_answer_good.txt`, `fixtures/smoke/cases/*/model_answer_bad.txt`, `debug/smoke/`.

Lifecycle evidence:
- Requirements evidence: All REQs.
- Design/code surface evidence: Smoke script, curl docs, fixtures, e2e report.
- Verification method: TEST-015 and EVAL-001.
- Validation purpose: The full user workflow is exercised against running services and the configured model.
- Configuration checkpoint: `phase-p06-complete`.
- Risks and assumptions: The Compose dependency stack is running; API and worker Go binaries are running; self-hosted LLM runtime is reachable; `jq` and `curl` are installed.

Plan-and-Solve subtasks:

- `P06.S01 Add failing coverage for e2e smoke command`
  - Action: Add at least two smoke case directories, each with one task/context payload and paired good/bad answers, then record the missing e2e target baseline.
  - Why now: The e2e command must fail before the script exists.
  - Files/surfaces: `fixtures/smoke/cases/*/task.json`, `fixtures/smoke/cases/*/model_answer_good.txt`, `fixtures/smoke/cases/*/model_answer_bad.txt`, `scripts/smoke_curl.sh`, `Makefile`.
  - Requirement link: REQ-014.
  - Verification link: TEST-015.
  - Verification mode: RED.
  - Command/procedure: `make test-e2e`
  - Expected result: Non-zero exit because `scripts/smoke_curl.sh` or the `test-e2e` target is absent.
  - Evidence produced: Fixtures and command output.
  - Stop/escalate condition: None.
  - Unlocks: P06.S02
- `P06.S02 Implement smoke script and curl documentation`
  - Action: Implement `make test-e2e` as the only e2e command and implement script steps that iterate smoke cases, create one checklist per case, poll checklist, evaluate good and bad answers against the same checklist, poll both evaluations, print score fields, print failed active question IDs, compute per-case and mean pass-rate separation, and capture JSON responses under `debug/smoke/`; add curl docs for the four routes and local start commands.
  - Why now: API, workflows, persistence, and Compose are available.
  - Files/surfaces: `scripts/smoke_curl.sh`, `docs/curl.md`.
  - Requirement link: REQ-001, REQ-006, REQ-010, REQ-014, REQ-021.
  - Verification link: TEST-015.
  - Verification mode: GREEN.
  - Command/procedure: `make test-e2e`
  - Expected result: Exit 0 with succeeded checklist JSON, two succeeded evaluation JSON payloads per case, final score output, and a pass-rate separation summary.
  - Evidence produced: Script diff, docs diff, smoke JSON outputs, and terminal output.
  - Stop/escalate condition: Escalate on timeout or failed terminal status; attach captured JSON to execution log.
  - Unlocks: P06.S03
- `P06.S03 Measure API golden-answer separation`
  - Action: Execute the canonical e2e target over all smoke cases and record per-case and mean separation metrics from persisted evaluations.
  - Why now: The end-to-end scoring surface is complete and can be checked against model-quality thresholds without a second model-test path.
  - Files/surfaces: `fixtures/smoke/`, `debug/smoke/`.
  - Requirement link: REQ-009, REQ-013.
  - Verification link: EVAL-001.
  - Verification mode: MEASURE.
  - Command/procedure: `make test-e2e`
  - Expected result: EVAL-001 thresholds pass.
  - Evidence produced: Smoke JSON outputs and metric summary under `debug/smoke/`.
  - Stop/escalate condition: Suspend on threshold miss; threshold changes require owner decision.
  - Unlocks: P06.S04
- `P06.S04 Refactor smoke and docs for one canonical e2e path`
  - Action: Remove duplicated fixture payload construction between docs and script by making docs reference the committed case fixture files and `make test-e2e`.
  - Why now: The smoke implementation and docs can diverge after green e2e.
  - Files/surfaces: `scripts/smoke_curl.sh`, `docs/curl.md`, `fixtures/smoke/`.
  - Requirement link: REQ-014, REQ-044.
  - Verification link: TEST-015.
  - Verification mode: REFACTOR.
  - Command/procedure: `make test-e2e`
  - Expected result: Exit 0 with unchanged smoke behavior.
  - Evidence produced: Refactor diff and passing smoke output.
  - Stop/escalate condition: Revert refactor if smoke output changes unexpectedly.
  - Unlocks: Phase exit

Exit gates:
- Proceed: TEST-015 passes and EVAL-001 meets thresholds.
- Escalate: live stack instability, unavailable self-hosted runtime, or golden quality miss.
- Stop: MVP acceptance criteria cannot be demonstrated through the four-route API.

Phase metrics:
- Confidence %: 75 - Live e2e depends on local services and model behavior.
- Long-term robustness %: 85 - Smoke becomes the acceptance path.
- Internal interactions: 7 - API, worker, workflows, activities, db, artifacts, llm.
- External interactions: 4 - Temporal, Postgres, Garage, LLM runtime.
- Complexity %: 45 - Full-stack orchestration and polling.
- Feature creep %: 0 - No UI or dashboards.
- Technical debt %: 10 - Shell smoke is linear but sufficient for MVP.
- YAGNI score: 9 - Script and docs directly support acceptance.
- MoSCoW: Must.
- Local/non-local scope: Non-local.
- Architectural changes count: 0.

## 6. Evaluations

```yaml
evals:
  - id: EVAL-001
    purpose: holdout
    metrics:
      - case_count
      - good_answer_mean_pass_rate
      - bad_answer_mean_pass_rate
      - mean_pass_rate_gap
      - all_checklists_succeeded
      - evaluation_success_count
      - judgment_coverage
    thresholds:
      case_count: ">=2"
      good_answer_mean_pass_rate: ">=0.80"
      bad_answer_mean_pass_rate: "<=0.50"
      mean_pass_rate_gap: ">=0.30"
      all_checklists_succeeded: true
      evaluation_success_count: ">=4"
      judgment_coverage: 1.0
    seeds: "fixtures/smoke/cases/*/task.json, fixtures/smoke/cases/*/model_answer_good.txt, fixtures/smoke/cases/*/model_answer_bad.txt"
    runtime_budget: 6m
```

Any metric threshold change requires a documented owner decision.

## 7. Tests

### 7.1 Test inventory

- Current repo state: no `package.json`, no Makefile, no Go module, no `scripts/`, and no CI config are present before P00.
- P00 creates exact commands:
  - `make lint`
  - `make build`
  - `make test`
  - `make test-integration`
  - `make test-e2e`
- Test runners after P00:
  - Go `testing`
  - Docker Compose CLI for the shared Postgres, Temporal, and Garage dependency stack and static validation
  - Temporal Go SDK testsuite for workflow tests
  - Bash with `curl` and `jq` for e2e smoke
- File globs after implementation:
  - Unit tests: `internal/**/*_test.go`, `cmd/**/*_test.go`
  - Integration tests: files with `//go:build integration`
  - E2E smoke: `scripts/smoke_curl.sh`
  - Fixtures: `fixtures/**`, `internal/**/testdata/**`

### 7.2 Test suites overview

- name: Unit
  - purpose: Pure logic, schemas, client behavior, API handlers, and retry mapping.
  - runner: Go `testing`
  - command: `make test`
  - runtime budget: 90s
  - when it runs: pre-commit and CI
- name: Integration
  - purpose: Compose config validation, Postgres migrations, queries, Garage artifact layout, and storage behavior.
  - runner: Go `testing` against the shared Compose dependency stack
  - command: `make test-integration`
  - runtime budget: 10m
  - when it runs: CI
- name: E2E
  - purpose: Four-route API acceptance path over running services.
  - runner: Bash, curl, jq
  - command: `make test-e2e`
  - runtime budget: 6m
  - when it runs: nightly and release gate
- name: Static
  - purpose: Formatting, vetting, build, and command contract rendering.
  - runner: Make and Docker Compose CLI
  - command: `make lint build`
  - runtime budget: 60s
  - when it runs: pre-commit and CI

### 7.3 Test definitions

- id: TEST-001
  - name: ScoreChecklist active-only weighted scoring
  - type: unit
  - verifies: REQ-009
  - location: `internal/evalcore/score_test.go`
  - command: `go test ./internal/evalcore -run TestScoreChecklist -count=1`
  - fixtures/mocks/data: Table cases for active weights, excluded weight 0 questions, all-zero failure, duplicate rows, invalid weights, invalid answers, failed IDs, and shared `BuildActiveChecklist` projection reuse.
  - deterministic controls: Pure functions; `-count=1`.
  - pass_criteria: Expected score fields and exact error classes match every table case.
  - expected_runtime: <5s
- id: TEST-002
  - name: Question ID assignment and active filtering
  - type: unit
  - verifies: REQ-003, REQ-004, REQ-009
  - location: `internal/evalcore/ids_test.go`
  - command: `go test ./internal/evalcore -run 'TestAssignQuestionIDs|TestBuildActiveChecklist' -count=1`
  - fixtures/mocks/data: Draft question slices and weight slices containing 0, 1, and 4 plus duplicate, unknown, missing, out-of-range, and all-zero cases.
  - deterministic controls: Pure functions; `-count=1`.
  - pass_criteria: IDs are `q1..qN`; active projection excludes weight 0, preserves original IDs, rejects invalid weight coverage, and returns one shared active-question representation for validators and scoring.
  - expected_runtime: <5s
- id: TEST-003
  - name: Question generation validation
  - type: unit
  - verifies: REQ-002
  - location: `internal/evalcore/validate_test.go`
  - command: `go test ./internal/evalcore -run TestValidateQuestionGeneration -count=1`
  - fixtures/mocks/data: Empty lists, blank rationales, blank questions, and valid draft lists.
  - deterministic controls: Pure functions; `-count=1`.
  - pass_criteria: Valid inputs pass; each structural violation returns typed semantic error.
  - expected_runtime: <5s
- id: TEST-004
  - name: Weight assignment validation with exclusion
  - type: unit
  - verifies: REQ-004
  - location: `internal/evalcore/validate_test.go`
  - command: `go test ./internal/evalcore -run TestValidateWeights -count=1`
  - fixtures/mocks/data: Missing weight, duplicate weight, unknown ID, weight -1, weight 0, weight 4, weight 5, all-zero set.
  - deterministic controls: Pure functions; `-count=1`.
  - pass_criteria: Exactly one weight per candidate, range 0 to 4, and at least one active question are enforced.
  - expected_runtime: <5s
- id: TEST-005
  - name: Binary judgment validation over active questions
  - type: unit
  - verifies: REQ-007, REQ-008
  - location: `internal/evalcore/validate_test.go`
  - command: `go test ./internal/evalcore -run TestValidateJudgments -count=1`
  - fixtures/mocks/data: Active and excluded questions, missing judgment, duplicate judgment, inactive judgment, unknown ID, empty evidence, whitespace-only evidence, answer `maybe`.
  - deterministic controls: Pure functions; `-count=1`.
  - pass_criteria: Exactly one judgment per active question, no inactive judgments, answer is `yes` or `no`, and evidence contains non-whitespace text.
  - expected_runtime: <5s
- id: TEST-006
  - name: LLM output schemas and prompt payloads
  - type: unit
  - verifies: REQ-002, REQ-004, REQ-007, REQ-008, REQ-020
  - location: `internal/llm/schema_test.go`
  - command: `go test ./internal/llm -run TestOutputSchemasAndPrompts -count=1`
  - fixtures/mocks/data: JSON fixtures for valid and invalid question generation, weight assignment, and binary judging; prompt payload fixtures for checklist creation and judging.
  - deterministic controls: Committed fixtures; `-count=1`.
  - pass_criteria: Valid fixtures pass schema validation, invalid fixtures fail, weight schema allows 0 to 4, checklist creation prompts exclude `model_answer`, and judge prompt excludes weights and rationales.
  - expected_runtime: <5s
- id: TEST-007
  - name: GenerateJSON client contract
  - type: unit
  - verifies: REQ-020, REQ-040
  - location: `internal/llm/client_test.go`
  - command: `go test ./internal/llm -run TestGenerateJSONClient -count=1`
  - fixtures/mocks/data: `httptest.Server` responses for valid JSON, invalid JSON, schema violation, and transient 503.
  - deterministic controls: In-process HTTP server; fixed response sequence; `-count=1`.
  - pass_criteria: Valid responses decode, invalid model output is non-retryable, transient infrastructure error is classified retryable, and no fallback endpoint is called.
  - expected_runtime: <10s
- id: TEST-008
  - name: LLM activities write artifacts and active-only payloads
  - type: unit
  - verifies: REQ-007, REQ-011, REQ-020, REQ-040
  - location: `internal/activities/llm_test.go`
  - command: `go test ./internal/activities -run TestLLMActivitiesWriteArtifactsAndPayloads -count=1`
  - fixtures/mocks/data: Fake artifact writer, fake `LLMClient`, questions with weight 0 and weights 1 to 4, expected request/response artifact bytes.
  - deterministic controls: In-process fakes; fixed JSON payloads; `-count=1`.
  - pass_criteria: Each LLM activity writes raw request and response artifacts, calls exactly one configured LLM client, judge request contains active question IDs/text only, and invalid model output is returned as non-retryable.
  - expected_runtime: <10s
- id: TEST-009
  - name: Config validation and structured logging
  - type: unit
  - verifies: REQ-041, REQ-042
  - location: `internal/config/config_test.go`, `internal/observability/log_test.go`
  - command: `go test ./internal/config ./internal/observability -run 'TestConfigValidation|TestStructuredLogFields' -count=1`
  - fixtures/mocks/data: `t.Setenv` values and captured JSON log buffer.
  - deterministic controls: Isolated test env; fixed secret sentinel values; `-count=1`.
  - pass_criteria: Missing env names are reported without values, secret values are redacted, raw evaluator payload fields are absent, and required log fields exist.
  - expected_runtime: <5s
- id: TEST-010
  - name: Postgres migrations and concrete store
  - type: integration
  - verifies: REQ-005, REQ-030, REQ-032
  - location: `internal/db/db_integration_test.go`
  - command: `make test-integration`
  - fixtures/mocks/data: Shared Compose `postgres:16.4` service and in-test seed rows.
  - deterministic controls: Test-owned schema cleanup; UTC timestamps; `-count=1`.
  - pass_criteria: Schema matches snapshot, lifecycle transitions are enforced, persisted weight 0 rows and active scores round-trip through the canonical integration target.
  - expected_runtime: <3m
- id: TEST-011
  - name: Garage artifact writer and key layout
  - type: integration
  - verifies: REQ-011, REQ-031, REQ-041
  - location: `internal/artifacts/artifacts_integration_test.go`
  - command: `make test-integration`
  - fixtures/mocks/data: Shared Compose `dxflrs/garage:v2.3.0` service and sample payloads.
  - deterministic controls: Test-owned bucket prefix; byte-for-byte comparison; `-count=1`.
  - pass_criteria: Keys match contract, required artifact families are covered, and read bytes equal written bytes.
  - expected_runtime: <3m
- id: TEST-012
  - name: Temporal workflow orchestration
  - type: integration
  - verifies: REQ-001, REQ-002, REQ-003, REQ-004, REQ-006, REQ-007, REQ-008, REQ-009, REQ-012
  - location: `internal/workflows/create_checklist_test.go`, `internal/workflows/evaluate_answer_test.go`
  - command: `go test ./internal/workflows -run 'TestCreateChecklistWorkflow|TestEvaluateAnswerWorkflow|TestWorkflowFailurePersistence' -count=1`
  - fixtures/mocks/data: Temporal testsuite, fake activities, scripted LLM outputs.
  - deterministic controls: Testsuite virtual time; fixed fixtures; `-count=1`.
  - pass_criteria: Workflow step order, terminal statuses, active-only judging payload, and persisted score match expectations.
  - expected_runtime: <20s
- id: TEST-013
  - name: HTTP API contracts
  - type: unit
  - verifies: REQ-001, REQ-006, REQ-010, REQ-021, REQ-032
  - location: `internal/api/api_test.go`
  - command: `go test ./internal/api -run 'TestAPIContracts|TestAPIRouteSurface' -count=1`
  - fixtures/mocks/data: `httptest`, test workflow starter, test db read data.
  - deterministic controls: In-process HTTP server; fixed JSON payloads; `-count=1`.
  - pass_criteria: Four routes return exact running, succeeded, and failed payloads; update/delete routes are absent; `202`, `200`, `400`, `404`, `409`, and `500` status cases return stable JSON errors.
  - expected_runtime: <10s
- id: TEST-014
  - name: Compose configuration static validation
  - type: static
  - verifies: REQ-044
  - location: `deploy/compose/docker-compose.yml`
  - command: `make test-integration`
  - fixtures/mocks/data: `deploy/compose/.env.example`
  - deterministic controls: Static Compose render inside the canonical integration target; pinned service images.
  - pass_criteria: Compose config renders successfully with no undefined variables before integration tests run.
  - expected_runtime: <10s
- id: TEST-015
  - name: End-to-end smoke path
  - type: e2e
  - verifies: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006, REQ-007, REQ-008, REQ-009, REQ-010, REQ-011, REQ-012, REQ-013, REQ-014, REQ-021, REQ-040
  - location: `scripts/smoke_curl.sh`
  - command: `make test-e2e`
  - fixtures/mocks/data: `fixtures/smoke/cases/*/task.json`, `fixtures/smoke/cases/*/model_answer_good.txt`, `fixtures/smoke/cases/*/model_answer_bad.txt`
  - deterministic controls: 300s poll ceiling, fixed fixtures, configured self-hosted LLM runtime.
  - pass_criteria: Script exits zero, each case creates one succeeded checklist and two succeeded evaluations, final score JSON parses, active failed question IDs are printed, and aggregate pass-rate separation summary meets EVAL-001.
  - expected_runtime: <6m
- id: TEST-017
  - name: Top-level Makefile command contract
  - type: static
  - verifies: REQ-044
  - location: `Makefile`
  - command: `make lint build test && make -n test-integration test-e2e`
  - fixtures/mocks/data: None.
  - deterministic controls: Go module mode; `-count=1` inside `make test`.
  - pass_criteria: Unit/static commands exit zero and integration/e2e targets render without executing services.
  - expected_runtime: <90s
- id: TEST-018
  - name: Activity retry classification
  - type: unit
  - verifies: REQ-040
  - location: `internal/activities/retry_test.go`
  - command: `go test ./internal/activities -run TestRetryClassification -count=1`
  - fixtures/mocks/data: Constructed infrastructure error instances and invalid model-output error instances.
  - deterministic controls: Pure mapping; `-count=1`.
  - pass_criteria: Infrastructure errors map to retryable Temporal errors; all invalid model-output cases map to one non-retryable category.
  - expected_runtime: <5s
- id: TEST-019
  - name: Workflow failure persistence
  - type: integration
  - verifies: REQ-001, REQ-006, REQ-012, REQ-040
  - location: `internal/workflows/create_checklist_test.go`, `internal/workflows/evaluate_answer_test.go`
  - command: `go test ./internal/workflows -run 'TestCreateChecklistWorkflow|TestEvaluateAnswerWorkflow|TestWorkflowFailurePersistence' -count=1`
  - fixtures/mocks/data: Temporal testsuite, fake activities returning invalid JSON, schema violations, semantic validation errors, and infrastructure errors.
  - deterministic controls: Testsuite virtual time; fixed error instances; `-count=1`.
  - pass_criteria: Non-retryable semantic/model failures persist `failed` status and `error_message`, infra failures use bounded retry behavior, and no succeeded rows are written after failure.
  - expected_runtime: <20s
- id: TEST-020
  - name: Postgres integrity negative cases
  - type: integration
  - verifies: REQ-005, REQ-030, REQ-032
  - location: `internal/db/db_integration_test.go`
  - command: `make test-integration`
  - fixtures/mocks/data: Shared Compose `postgres:16.4` service and in-test seed rows for duplicate weights, duplicate judgments, cross-checklist judgments, immutable success transitions, and raw-text probes.
  - deterministic controls: Test-owned schema cleanup; UTC timestamps; `-count=1`.
  - pass_criteria: Database or store layer rejects duplicate/cross-checklist rows, rejects semantic updates after success, preserves terminal status invariants, and stores only artifact keys plus structured state.
  - expected_runtime: <3m
### 7.4 Manual checks, optional

No CHECK items are required for this MVP plan. All acceptance controls use executable TEST or EVAL entries.

## 8. Data contract

### Schema snapshot

Postgres:

- `checklists(id primary key, status, task_artifact_key, context_artifact_key, error_message null, created_at, completed_at null)`
- `questions(checklist_id references checklists(id), id, ordinal, rationale, question, primary key(checklist_id, id))`
- `weights(checklist_id, question_id, rationale, weight, primary key(checklist_id, question_id), foreign key(checklist_id, question_id) references questions(checklist_id, id))`
- `evaluations(id primary key, checklist_id references checklists(id), status, answer_artifact_key, satisfied_points null, total_possible_points null, checklist_pass_rate null, error_message null, created_at, completed_at null, unique(id, checklist_id))`
- `judgments(evaluation_id, checklist_id, question_id, evidence, answer, primary key(evaluation_id, question_id), foreign key(evaluation_id, checklist_id) references evaluations(id, checklist_id), foreign key(checklist_id, question_id) references questions(checklist_id, id))`

Garage bucket: `bin-eval-artifacts`.

Garage key layout:

- `checklists/{checklist_id}/inputs/task.txt`
- `checklists/{checklist_id}/inputs/context.txt`
- `checklists/{checklist_id}/llm/question_generation/request.json`
- `checklists/{checklist_id}/llm/question_generation/response.json`
- `checklists/{checklist_id}/llm/weight_assignment/request.json`
- `checklists/{checklist_id}/llm/weight_assignment/response.json`
- `evaluations/{evaluation_id}/inputs/model_answer.txt`
- `evaluations/{evaluation_id}/llm/binary_judging/request.json`
- `evaluations/{evaluation_id}/llm/binary_judging/response.json`

### Invariants

- Checklist and evaluation statuses are `running`, `succeeded`, or `failed`.
- Allowed terminal transitions are `running -> succeeded` and `running -> failed`.
- Candidate question IDs are stable `q1..qN`.
- Every candidate question has exactly one weight row.
- Weight 0 means excluded from active checklist.
- Active questions have weights 1 to 4.
- At least one active question is required for a succeeded checklist.
- Every active question has exactly one judgment per succeeded evaluation.
- Judgment evidence must contain non-whitespace text.
- Each judgment's `checklist_id` matches its evaluation's `checklist_id`.
- Excluded questions have no judgments.
- Score is recomputable from persisted questions, weights, and judgments.
- No update endpoint changes semantic content after success.

### Privacy/data quality constraints

- `task`, `context`, `model_answer`, candidate questions, active questions, and LLM payloads are sent only to the configured self-hosted LLM runtime.
- Secrets are read from environment variables and redacted before logs; Garage artifacts preserve submitted and generated payload bytes.
- Operators must not place secrets in task, context, or model answer payloads.
- Weight 0 is the only exclusion mechanism; there is no question rewrite, merge, or separate cleanup step.

## 9. Reproducibility

- Seeds: golden fixtures in `fixtures/smoke/`; Go tests use fixed fixtures and `-count=1`.
- Hardware assumptions: x86_64 or arm64 host, 4 vCPU, 8 GB RAM, SSD, Docker Engine 27 or newer with Compose v2.
- OS/driver/container tag assumptions: Ubuntu 24.04 LTS or equivalent, Go 1.23.x or newer, `postgres:16.4`, `temporalio/auto-setup:1.28.4`, `dxflrs/garage:v2.3.0`.
- Container tag references: Garage docs identify `dxflrs/garage` and recommend fixed tags such as `v2.3.0` (https://garagehq.deuxfleurs.fr/documentation/cookbook/real-world/); Temporal release listings publish server image tags such as `1.28.4` (https://github.com/temporalio/temporal/releases).
- Relevant environment variables: `BIN_EVAL_ENV`, `BIN_EVAL_DATABASE_URL`, `BIN_EVAL_TEMPORAL_ADDRESS`, `BIN_EVAL_GARAGE_ENDPOINT`, `BIN_EVAL_GARAGE_ACCESS_KEY`, `BIN_EVAL_GARAGE_SECRET_KEY`, `BIN_EVAL_ARTIFACT_BUCKET`, `BIN_EVAL_LLM_BASE_URL`, `BIN_EVAL_LLM_API_KEY`, `BIN_EVAL_MODEL_PROFILE`, `BIN_EVAL_URL`.
- Test determinism controls: UTC timestamps in tests, shared Compose services with test-owned cleanup, fixed smoke fixtures, and 300s smoke poll ceiling.

## 10. Requirements Traceability Matrix

| Phase | REQ-### | TEST-### | Test Path | Command |
|---|---|---|---|---|
| P04 | REQ-001 | TEST-012 | `internal/workflows/create_checklist_test.go` | `go test ./internal/workflows -run 'TestCreateChecklistWorkflow|TestEvaluateAnswerWorkflow|TestWorkflowFailurePersistence' -count=1` |
| P04 | REQ-001 | TEST-019 | `internal/workflows/create_checklist_test.go` | `go test ./internal/workflows -run 'TestCreateChecklistWorkflow|TestEvaluateAnswerWorkflow|TestWorkflowFailurePersistence' -count=1` |
| P05 | REQ-001 | TEST-013 | `internal/api/api_test.go` | `go test ./internal/api -run 'TestAPIContracts|TestAPIRouteSurface' -count=1` |
| P06 | REQ-001 | TEST-015 | `scripts/smoke_curl.sh` | `make test-e2e` |
| P01 | REQ-002 | TEST-003 | `internal/evalcore/validate_test.go` | `go test ./internal/evalcore -run TestValidateQuestionGeneration -count=1` |
| P02 | REQ-002 | TEST-006 | `internal/llm/schema_test.go` | `go test ./internal/llm -run TestOutputSchemasAndPrompts -count=1` |
| P01 | REQ-003 | TEST-002 | `internal/evalcore/ids_test.go` | `go test ./internal/evalcore -run 'TestAssignQuestionIDs|TestBuildActiveChecklist' -count=1` |
| P01 | REQ-004 | TEST-004 | `internal/evalcore/validate_test.go` | `go test ./internal/evalcore -run TestValidateWeights -count=1` |
| P02 | REQ-004 | TEST-006 | `internal/llm/schema_test.go` | `go test ./internal/llm -run TestOutputSchemasAndPrompts -count=1` |
| P03 | REQ-005 | TEST-010 | `internal/db/db_integration_test.go` | `make test-integration` |
| P03 | REQ-005 | TEST-020 | `internal/db/db_integration_test.go` | `make test-integration` |
| P04 | REQ-006 | TEST-012 | `internal/workflows/evaluate_answer_test.go` | `go test ./internal/workflows -run 'TestCreateChecklistWorkflow|TestEvaluateAnswerWorkflow|TestWorkflowFailurePersistence' -count=1` |
| P04 | REQ-006 | TEST-019 | `internal/workflows/evaluate_answer_test.go` | `go test ./internal/workflows -run 'TestCreateChecklistWorkflow|TestEvaluateAnswerWorkflow|TestWorkflowFailurePersistence' -count=1` |
| P05 | REQ-006 | TEST-013 | `internal/api/api_test.go` | `go test ./internal/api -run 'TestAPIContracts|TestAPIRouteSurface' -count=1` |
| P01 | REQ-007 | TEST-005 | `internal/evalcore/validate_test.go` | `go test ./internal/evalcore -run TestValidateJudgments -count=1` |
| P02 | REQ-007 | TEST-006 | `internal/llm/schema_test.go` | `go test ./internal/llm -run TestOutputSchemasAndPrompts -count=1` |
| P04 | REQ-007 | TEST-008 | `internal/activities/llm_test.go` | `go test ./internal/activities -run TestLLMActivitiesWriteArtifactsAndPayloads -count=1` |
| P01 | REQ-008 | TEST-005 | `internal/evalcore/validate_test.go` | `go test ./internal/evalcore -run TestValidateJudgments -count=1` |
| P01 | REQ-009 | TEST-001 | `internal/evalcore/score_test.go` | `go test ./internal/evalcore -run TestScoreChecklist -count=1` |
| P01 | REQ-009 | TEST-002 | `internal/evalcore/ids_test.go` | `go test ./internal/evalcore -run 'TestAssignQuestionIDs|TestBuildActiveChecklist' -count=1` |
| P05 | REQ-010 | TEST-013 | `internal/api/api_test.go` | `go test ./internal/api -run 'TestAPIContracts|TestAPIRouteSurface' -count=1` |
| P03 | REQ-011 | TEST-011 | `internal/artifacts/artifacts_integration_test.go` | `make test-integration` |
| P04 | REQ-011 | TEST-008 | `internal/activities/llm_test.go` | `go test ./internal/activities -run TestLLMActivitiesWriteArtifactsAndPayloads -count=1` |
| P04 | REQ-012 | TEST-012 | `internal/workflows/create_checklist_test.go` | `go test ./internal/workflows -run 'TestCreateChecklistWorkflow|TestEvaluateAnswerWorkflow|TestWorkflowFailurePersistence' -count=1` |
| P04 | REQ-012 | TEST-019 | `internal/workflows/create_checklist_test.go` | `go test ./internal/workflows -run 'TestCreateChecklistWorkflow|TestEvaluateAnswerWorkflow|TestWorkflowFailurePersistence' -count=1` |
| P06 | REQ-013 | TEST-015 | `scripts/smoke_curl.sh` | `make test-e2e` |
| P06 | REQ-014 | TEST-015 | `scripts/smoke_curl.sh` | `make test-e2e` |
| P02 | REQ-020 | TEST-006 | `internal/llm/schema_test.go` | `go test ./internal/llm -run TestOutputSchemasAndPrompts -count=1` |
| P02 | REQ-020 | TEST-007 | `internal/llm/client_test.go` | `go test ./internal/llm -run TestGenerateJSONClient -count=1` |
| P04 | REQ-020 | TEST-008 | `internal/activities/llm_test.go` | `go test ./internal/activities -run TestLLMActivitiesWriteArtifactsAndPayloads -count=1` |
| P05 | REQ-021 | TEST-013 | `internal/api/api_test.go` | `go test ./internal/api -run 'TestAPIContracts|TestAPIRouteSurface' -count=1` |
| P03 | REQ-030 | TEST-010 | `internal/db/db_integration_test.go` | `make test-integration` |
| P03 | REQ-030 | TEST-020 | `internal/db/db_integration_test.go` | `make test-integration` |
| P03 | REQ-031 | TEST-011 | `internal/artifacts/artifacts_integration_test.go` | `make test-integration` |
| P03 | REQ-032 | TEST-010 | `internal/db/db_integration_test.go` | `make test-integration` |
| P03 | REQ-032 | TEST-020 | `internal/db/db_integration_test.go` | `make test-integration` |
| P04 | REQ-040 | TEST-018 | `internal/activities/retry_test.go` | `go test ./internal/activities -run TestRetryClassification -count=1` |
| P04 | REQ-040 | TEST-008 | `internal/activities/llm_test.go` | `go test ./internal/activities -run TestLLMActivitiesWriteArtifactsAndPayloads -count=1` |
| P04 | REQ-040 | TEST-019 | `internal/workflows/create_checklist_test.go` | `go test ./internal/workflows -run 'TestCreateChecklistWorkflow|TestEvaluateAnswerWorkflow|TestWorkflowFailurePersistence' -count=1` |
| P06 | REQ-040 | TEST-015 | `scripts/smoke_curl.sh` | `make test-e2e` |
| P00 | REQ-041 | TEST-009 | `internal/config/config_test.go`, `internal/observability/log_test.go` | `go test ./internal/config ./internal/observability -run 'TestConfigValidation|TestStructuredLogFields' -count=1` |
| P03 | REQ-041 | TEST-011 | `internal/artifacts/artifacts_integration_test.go` | `make test-integration` |
| P00 | REQ-042 | TEST-009 | `internal/config/config_test.go`, `internal/observability/log_test.go` | `go test ./internal/config ./internal/observability -run 'TestConfigValidation|TestStructuredLogFields' -count=1` |
| P00 | REQ-044 | TEST-017 | `Makefile` | `make lint build test && make -n test-integration test-e2e` |
| P03 | REQ-044 | TEST-014 | `deploy/compose/docker-compose.yml` | `make test-integration` |

## 11. Execution log template

```markdown
# Execution Log - PLAN-BIN-EVAL-001

## Phase Pxx: ______
- Phase Status: Pending/Done
- Checkpoint tag: phase-pxx-complete
- Commit: ______

### Completed Steps
- [ ] Pxx.S01 - result: ______ - evidence: ______
- [ ] Pxx.S02 - result: ______ - evidence: ______

### Quantitative Results
- Metric: ______ - mean +/- std: ______ - 95% CI: ______ - threshold: ______ - pass/fail: ______

### Issues/Resolutions
- Issue: ______ - Resolution: ______

### Failed Attempts
- Attempt: ______ - Subtask: ______ - Why it failed: ______ - Branch limit remaining: ______

### Deviations
- Deviation from plan: ______ - Justification: ______ - ADR: ______

### Lessons Learned
- ______

### ADR Updates
- ADR-###: ______
```

## 12. Appendix: ADR index

- ADR-001: Project and binary names use bin-eval throughout.
- ADR-002: Weight scale is 0 to 4; weight 0 excludes a candidate question from the active checklist.
- ADR-003: Go assigns stable IDs before weighting and does not renumber after exclusions.
- ADR-004: Weight assignment is the only removal mechanism; no separate deduplication or rewrite step.
- ADR-005: Judge receives active question IDs and question text only; no weights or rationales.
- ADR-006: Invalid JSON, schema violations, and semantic model output errors are non-retryable.
- ADR-007: One schema-constrained `LLMClient.GenerateJSON` boundary; no provider SDK, fallback provider, plain-text parser, or repair prompt.
- ADR-008: One canonical `context` field carries all evaluator-side input.
- ADR-009: Postgres owns structured state; Garage owns raw payload artifacts.
- ADR-010: Local Compose packages Postgres, Temporal, and Garage only; API and worker run as Go binaries for the MVP.
- ADR-011: Any EVAL threshold or pinned container tag change requires a documented owner decision.
- ADR-012: Checklist and evaluation IDs use Postgres-generated UUIDs.
- ADR-013: The self-hosted LLM runtime contract is one OpenAI-compatible schema-constrained HTTP endpoint.
- ADR-014: Garage stores raw payloads byte-for-byte; logs and smoke output redact secrets and omit raw evaluator payloads.
- ADR-015: Temporal is mandatory for the MVP workflow implementation.
- ADR-016: `make test-integration` manages the single Compose dependency stack; no testcontainers path exists.
- ADR-017: Empty or whitespace-only judgment evidence is invalid model output.
- ADR-018: Active-question projection is centralized in `BuildActiveChecklist`; validators, workflows, and scoring must not duplicate weight-0 logic.
- ADR-019: HTTP status behavior is pinned to `202`, `200`, `400`, `404`, `409`, and `500` cases in the API tests.
- ADR-020: EVAL-001 uses at least two smoke cases through `make test-e2e`; no separate model-test CLI exists.

## 13. Consistency check

- All REQs appear in the RTM.
- All TEST IDs referenced in phases, evals, or RTM are defined in Section 7.3.
- Every phase has ordered Plan-and-Solve subtasks with explicit verification modes.
- Every behavior-changing implementation subtask is preceded by a RED coverage subtask.
- No behavior-changing implementation subtask uses CHECK-### as its only verification link.
- Every phase has populated metrics.
- Every subtask includes a TEST-###, EVAL-###, or CHECK-### link plus an exact command/procedure.
- No invented commands are referenced before P00 creates them.
- No placeholder or context-dependent references remain.
