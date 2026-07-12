# bin-eval Rubric Refinement Open Decisions

This document reformats the clarifying questions from the preceding conversation into auditable decision records.

Decision status:

- Decided: 1B, 2B, 3A with scaled defaults `6/8/4/64`, 4A, 5B, 6A, 7A, 8A, 9A.
- Still open: None.

Terminology used below:

- **Rubric refinement path**: the new checklist creation path described in `plans/bin-eval-rubric-refinement-dag-plan.md`, centered on `AnalyzeDimensions`, `GenerateQuestionsForDimension`, `AssignWeights`, `SplitQuestion`, `BuildFinalChecklist`, and equal-count `ScoreChecklist`.
- **Diagnostic weight**: the replacement for current scoring-multiplier weight semantics. `0` deletes a candidate question, `1` keeps it as one final question, and `2..4` means the candidate must be split into that many final questions. It is returned for diagnostics and is not used as a score multiplier.
- **Final question**: the atomic binary question judged during evaluation. Final questions, not candidate questions or diagnostic weights, are the source of truth for scoring.
- **Old weighted path**: current behavior where question weights are used as point multipliers in scoring. The updated plan says to delete this runtime path rather than wrap it.
- **Local data reset**: resetting local Postgres data created by the development Compose stack so the new schema can be the only runtime data model.
- **LiteLLM profile**: the local OpenAI-compatible LiteLLM configuration used by the service through the existing LLM client and environment variables.
- **Checklist contract**: the JSON shape returned by `GET /checklists/{id}` for callers and curl scripts. In current code this is `checklistResponse` in `internal/api/router.go`, populated from `db.Checklist` in `internal/db/store.go`. In the new pipeline it is the full persisted checklist audit object: dimensions, candidate questions, diagnostic weights, and final binary questions.

### 1.1 Question

Can implementation reset existing local Postgres checklist/evaluation data, or must any existing local data be preserved outside the runtime path?

### 1.2 Context & clarification

The plan now requires one current data model and no old weighted-scoring compatibility path. Current local data may contain checklist rows shaped around candidate questions plus scoring-multiplier `weights`. The new model persists dimensions, candidate questions, diagnostic weights, final questions, evaluations, and judgments. This affects migration work in `migrations/`, store code in `internal/db/store.go`, and integration tests in `internal/db/db_integration_test.go`.

### 1.3 Options

- `Option A`: One-time offline archive, then reset local data
  - **Rubrics**: `Conf:70% | Invest:i | Blast:i | Reversal:ii | Fit:ii | Reuse:ii | Obs:i | Surface:ii | Perf:na`
  - **Approach**: Export existing local rows to an ignored debug/archive file, reset the local database, and apply the new rubric refinement schema as the only runtime model.
  - **Example**: Run a script that writes `debug/archive/old-weighted-checklists.json`, then recreates the local database before applying `migrations/0002_rubric_refinement.sql`.
  - **Architecture**: Keeps runtime code clean while preserving human-inspection data outside the application path.
  - **SSoT**: Postgres remains the runtime source of truth after reset; archived JSON is not loaded by the service.
  - **System limits**: Unknown - not available in local context.
  - **Trade-offs**: Better data integrity evidence, more one-time work, no runtime complexity.
- `Option B`: Direct local reset with no archive
  - **Rubrics**: `Conf:80% | Invest:ii | Blast:ii | Reversal:i | Fit:i | Reuse:i | Obs:ii | Surface:i | Perf:na`
  - **Approach**: Treat local data as disposable development state and reset it before applying the new schema.
  - **Example**: `make stop-local`, remove the local Postgres volume, start Compose again, run migrations.
  - **Architecture**: Best matches the one-way plan: one schema, one store contract, no runtime migration bridge.
  - **SSoT**: The new Postgres schema is the only source of runtime checklist/evaluation state.
  - **System limits**: Unknown - not available in local context.
  - **Trade-offs**: Smallest code and simplest migration; loses previous local rows.

### 1.4 Recommendation

I recommend **Option B** unless you know there is local data worth preserving. It best matches the direct-path requirement and avoids creating one-time archive tooling that may not be used.

Decision: **Selected Option B**. Existing local Postgres checklist/evaluation data can be reset; no archive path is required.

### 2.1 Question

Should successful API responses keep `weights` as diagnostics while still scoring only final binary questions?

### 2.2 Context & clarification

The existing API response shape is implemented by `checklistResponse` in `internal/api/router.go`. Today that struct has:

```go
type checklistResponse struct {
    ChecklistID  string                       `json:"checklist_id"`
    Status       string                       `json:"status"`
    ErrorMessage *string                      `json:"error_message,omitempty"`
    Questions    []evalcore.CandidateQuestion `json:"questions,omitempty"`
    Weights      []evalcore.Weight            `json:"weights,omitempty"`
}
```

The `weights` field is populated in `getChecklist` from `db.Checklist.Weights`. That store field is loaded from the current `weights` table in `internal/db/store.go`. The important correction is that we should keep this information because it explains why candidates were deleted, kept, or split. The old scoring-multiplier meaning is removed, but the new diagnostic meaning remains.

This is what "checklist contract" means here: the response contract for a succeeded checklist returned to curl/API callers. It does not mean the four HTTP routes change. `POST /checklists`, `GET /checklists/{id}`, `POST /evaluations`, and `GET /evaluations/{id}` stay the same; the succeeded checklist response body changes.

The checklist itself is the persisted evaluation artifact produced by checklist creation. In the target DAG, raw candidate questions appear after per-dimension question generation, diagnostic weights appear after the weight-assignment prompt, and the final binary question list appears after split generation plus deterministic final checklist assembly. The final binary questions are what evaluation judges and scoring use.

### 2.3 Options

- `Option A`: Full response replacement
  - **Rubrics**: `Conf:80% | Invest:i | Blast:i | Reversal:i | Fit:i | Reuse:i | Obs:i | Surface:i | Perf:na`
  - **Approach**: Remove `weights` from succeeded checklist responses and make `refinements` plus final `questions` the only current response contract.
  - **Example**: `GET /checklists/{id}` returns `dimensions`, `candidate_questions`, `refinements`, and `questions`, with no `weights` field.
  - **Architecture**: Aligns API semantics with the new scoring model and avoids two parallel client contracts.
  - **SSoT**: Final questions and refinements are the only public structured checklist state.
  - **System limits**: Unknown - not available in local context.
  - **Trade-offs**: Cleanest implementation; any external caller expecting `weights` must update now.
- `Option B`: Keep diagnostic weights as the only assignment output
  - **Rubrics**: `Conf:85% | Invest:i | Blast:i | Reversal:i | Fit:i | Reuse:i | Obs:i | Surface:i | Perf:na`
  - **Approach**: Keep the `weights` response field, redefine each weight as diagnostic assignment data, and do not expose a duplicate `refinements` array.
  - **Example**: `GET /checklists/{id}` returns `dimensions`, `candidate_questions`, `weights`, and final `questions`; each weight has the candidate ID, integer weight, and rationale. Scoring ignores weights and counts final question judgments equally.
  - **Architecture**: Maximizes LLM/debug visibility without adding a second route or a duplicated response field.
  - **SSoT**: Diagnostic weights are the single assignment record; final questions and judgments are the scoring source of truth.
  - **System limits**: Unknown - not available in local context.
  - **Trade-offs**: The word `weight` remains overloaded historically, so docs and tests must state that it is diagnostic only.
- `Option C`: Explicit versioned route for a new contract
  - **Rubrics**: `Conf:50% | Invest:ii | Blast:ii | Reversal:ii | Fit:ii | Reuse:ii | Obs:ii | Surface:ii | Perf:na`
  - **Approach**: Add a new explicit route namespace only if an existing external caller must keep old scoring-multiplier responses.
  - **Example**: Keep current four routes for current work only if required, and add `/v2/checklists/{id}` for the rubric contract.
  - **Architecture**: Creates a clearer public API transition if external clients exist, but the current project plan says public API compatibility is out of scope.
  - **SSoT**: The rubric refinement schema remains the application source of truth; route versioning only changes presentation.
  - **System limits**: Unknown - not available in local context.
  - **Trade-offs**: Safer for unknown external callers, but broadens surface area and conflicts with the desire for one current route contract.

### 2.4 Recommendation

I recommend **Option B** after your correction. Keeping weights gives the LLM and operator more diagnostic information, while avoiding duplicate assignment data by not returning a separate `refinements` array with the same contents.

Decision: **Selected Option B**. Keep `weights` in `GET /checklists/{id}` as diagnostic weight-assignment output. Do not use weights as scoring multipliers, and do not return a duplicate `refinements` field.

### 3.1 Question

What hard fan-out limits should the implementation enforce for dimensions, candidates, splits, and final questions?

### 3.2 Context & clarification

Fan-out limits bound LLM cost, Temporal activity count, persisted rows, and checklist size. The relevant plan requirement is `REQ-040`. The limits should live in one place and be used by prompt schema constraints, Go validation, and workflow checks. The affected surfaces include `internal/evalcore`, `internal/llm/schemas.go`, and workflow activity inputs.

### 3.3 Options

- `Option A`: Scaled central limits with a 64-question ceiling
  - **Rubrics**: `Conf:70% | Invest:i | Blast:i | Reversal:ii | Fit:i | Reuse:i | Obs:i | Surface:i | Perf:i`
  - **Approach**: Define one central limit struct with defaults `max_dimensions=6`, `max_candidates_per_dimension=8`, `max_split_count=4`, and `max_final_questions=64`; load overrides from one config source so limits can be adjusted after diagnostics show real limit pressure.
  - **Example**: `evalcore.ChecklistLimits{MaxDimensions:6, MaxCandidatesPerDimension:8, MaxSplitCount:4, MaxFinalQuestions:64}`.
  - **Architecture**: Centralized limits keep schema, validation, workflow behavior, logs, and smoke diagnostics aligned.
  - **SSoT**: `internal/evalcore` owns the canonical limit struct and validation; `internal/config` owns configured values; LLM schemas and workflows consume those values or mirrored constants tested against them.
  - **System limits**: defaults are `max_dimensions=6`, `max_candidates_per_dimension=8`, `max_split_count=4`, `max_final_questions=64`.
  - **Trade-offs**: Higher coverage budget with bounded fan-out; `max_split_count` stays 4 because the user-facing weight scale remains 0..4.
- `Option B`: Conservative MVP limits
  - **Rubrics**: `Conf:70% | Invest:ii | Blast:ii | Reversal:iii | Fit:ii | Reuse:ii | Obs:ii | Surface:i | Perf:ii`
  - **Approach**: Use smaller limits: `max_dimensions=3`, `max_candidates_per_dimension=4`, `max_split_count=3`, and `max_final_questions=16`.
  - **Example**: A checklist can create at most 12 candidates before weighting and 16 final questions after splitting.
  - **Architecture**: Same central-code shape as Option A, with smaller budgets.
  - **SSoT**: One evalcore-owned limit definition.
  - **System limits**: `max_dimensions=3`, `max_candidates_per_dimension=4`, `max_split_count=3`, `max_final_questions=16`.
  - **Trade-offs**: Lower cost and faster smoke tests, but may under-cover complex tasks.
- `Option C`: Larger coverage limits
  - **Rubrics**: `Conf:50% | Invest:iii | Blast:iii | Reversal:i | Fit:iii | Reuse:iii | Obs:iii | Surface:i | Perf:iii`
  - **Approach**: Use larger limits: `max_dimensions=8`, `max_candidates_per_dimension=10`, `max_split_count=4`, and `max_final_questions=96`.
  - **Example**: A complex task can produce broader rubric coverage before the final budget stops the workflow.
  - **Architecture**: Same centralized limit implementation, with higher activity and row counts.
  - **SSoT**: One evalcore-owned limit definition.
  - **System limits**: `max_dimensions=8`, `max_candidates_per_dimension=10`, `max_split_count=4`, `max_final_questions=96`.
  - **Trade-offs**: Better coverage for large tasks, more LLM calls, slower local curl path.

### 3.4 Recommendation

I recommend **Option A**. It is the best balance for a local MVP: bounded enough to keep curl usable, large enough to make binary scoring more meaningful.

Decision: **Selected Option A with scaled defaults `6/8/4/64`**. The implementation should record limit diagnostics whenever validation rejects output for exceeding a limit, including `limit_name`, `configured_limit`, `observed_count`, `checklist_id`, and prompt/workflow stage where available. Limits should be adjustable through one config path after diagnostics show which limit is binding. The other limits were scaled with the 64-question cap: dimensions move from 4 to 6, candidates per dimension move from 6 to 8, split count remains 4 because that is the diagnostic weight scale.

### 4.1 Question

Should structurally invalid model output fail the workflow immediately, with no prompt-specific repair loop?

### 4.2 Context & clarification

Earlier "semantic validation" wording means deterministic Go checks over parsed structured output. It is not validation "by meaning", not an extra LLM call, and not regex-based natural-language judging. JSON schema checks shape; Go checks cross-field and cross-object invariants that schema cannot fully express: every candidate has one diagnostic weight, split counts match exactly, no unknown IDs appear, final question budgets are respected, required text fields are non-blank, and lifecycle failures are classified with structured diagnostics. The relevant code will live around `internal/llm/schemas.go`, `internal/evalcore/validate.go`, and Temporal activity error handling.

JSON repair, structured-output retries, and other LLM-call fixing logic should stay behind a separate LLM-call module boundary. Prompt-specific workflow/activity code should not grow bespoke repair loops, because that boundary can later become a shared library or service responsible for returning a valid structured response or a structured failure.

### 4.3 Options

- `Option A`: Fail-fast at the activity boundary
  - **Rubrics**: `Conf:80% | Invest:i | Blast:i | Reversal:ii | Fit:i | Reuse:i | Obs:i | Surface:i | Perf:na`
  - **Approach**: Validate each LLM response inside the activity using schema parsing plus deterministic Go invariant checks, then return a non-retryable semantic error for invalid model output.
  - **Example**: `AssignWeights` returns a structured failure if a candidate ID is missing or duplicated; `SplitQuestion` returns a structured failure if it returns three questions when `weight=4` requested four.
  - **Architecture**: Keeps external-boundary validation near the LLM call while leaving deterministic final assembly in evalcore.
  - **SSoT**: Validation rules live in shared evalcore/LLM validation functions, not repeated in workflow code.
  - **System limits**: Unknown - not available in local context.
  - **Trade-offs**: Fast failure and simple behavior; no automatic recovery from model mistakes.
- `Option B`: Fail-fast at final checklist construction
  - **Rubrics**: `Conf:60% | Invest:ii | Blast:ii | Reversal:i | Fit:ii | Reuse:ii | Obs:ii | Surface:i | Perf:na`
  - **Approach**: Let activities return parsed output and centralize all structure/invariant validation in `BuildFinalChecklist`.
  - **Example**: `BuildFinalChecklist` rejects duplicate diagnostic weights and split-count mismatches.
  - **Architecture**: Strong functional core, but invalid data travels farther through workflow state before failure.
  - **SSoT**: `BuildFinalChecklist` is the central invariant gate.
  - **System limits**: Unknown - not available in local context.
  - **Trade-offs**: Centralized logic, weaker boundary observability for which LLM call produced the bad output.

### 4.4 Recommendation

I recommend **Option A**, backed by shared validation helpers so logic is not duplicated. It best matches fail-fast behavior at external boundaries while keeping the deterministic rules centralized.

Decision: **Selected Option A**. The validators perform deterministic structure and invariant validation of schema-parsed data. There is no extra repair LLM call and no regex attempt to judge semantic natural-language equivalence inside these validators. Generic JSON repair or retry behavior belongs in a separate LLM-call fixing module.

### 5.1 Question

Is exact duplicate validation enough, or should overlap be handled more aggressively in this MVP?

### 5.2 Context & clarification

Duplicate and overlap control affects score validity. If final questions repeat the same requirement, one concept gets counted multiple times. The plan avoids a separate dedupe stage and expects `AssignWeights` to mark duplicate candidates with `weight=0` before splitting.

### 5.3 Options

- `Option A`: Exact duplicate rejection in evalcore
  - **Rubrics**: `Conf:80% | Invest:i | Blast:i | Reversal:ii | Fit:i | Reuse:i | Obs:i | Surface:i | Perf:i`
  - **Approach**: Normalize final question text and reject exact duplicates during final checklist construction.
  - **Example**: Lowercase, trim whitespace, collapse spaces, then fail if two final questions normalize to the same string.
  - **Architecture**: Keeps one deterministic validation gate in evalcore.
  - **SSoT**: `BuildFinalChecklist` owns final question uniqueness.
  - **System limits**: O(n) or O(n log n) over final questions, bounded by `max_final_questions`.
  - **Trade-offs**: Simple and deterministic; does not catch semantic paraphrases.
- `Option B`: Prompt-level overlap prevention only
  - **Rubrics**: `Conf:60% | Invest:ii | Blast:ii | Reversal:iii | Fit:ii | Reuse:ii | Obs:ii | Surface:i | Perf:ii`
  - **Approach**: Add prompt requirements asking the model to avoid overlap, but do not add deterministic duplicate validation.
  - **Example**: `AssignWeights` prompt says to mark duplicate candidate questions with `weight=0`.
  - **Architecture**: Minimal code, weaker correctness because model compliance is not a contract.
  - **SSoT**: The prompt carries the behavior; no deterministic source of truth.
  - **System limits**: Unknown - not available in local context.
  - **Trade-offs**: Smallest implementation, less robust and harder to audit.
- `Option C`: Semantic overlap scoring in the refinement assignment prompt
  - **Rubrics**: `Conf:50% | Invest:iii | Blast:iii | Reversal:i | Fit:iii | Reuse:iii | Obs:iii | Surface:ii | Perf:iii`
  - **Approach**: Ask refinement assignment to explicitly reason about overlap and choose which candidates to delete, with exact duplicate validation still in evalcore.
  - **Example**: Refinement rationale includes "deleted because candidate c004 covers the same requirement."
  - **Architecture**: Keeps one LLM step but makes that prompt more complex.
  - **SSoT**: Deterministic exact duplicate checks stay in evalcore; semantic overlap remains an LLM judgment.
  - **System limits**: Unknown - not available in local context.
  - **Trade-offs**: Better coverage quality, more prompt burden and more variable output quality.

### 5.4 Recommendation

I recommended **Option A** before your decision, but your reasoning is stronger for this MVP: exact duplicates are unlikely with long probabilistic LLM-generated question text, and semantic duplication is better handled in the weight prompt by assigning `weight=0` to duplicates.

Decision: **Selected Option B**. The implementation should rely on the weight assignment prompt to mark semantic duplicates for removal with `weight=0`. It should not add exact duplicate validation or a separate dedupe stage in this MVP.

### 6.1 Question

Are the planned smoke quality thresholds acceptable for first implementation?

### 6.2 Context & clarification

The plan currently sets `EVAL-001` thresholds: good answer mean pass rate `>= 0.80`, bad answer mean pass rate `<= 0.50`, mean gap `>= 0.30`, final question count `>= 8 per case`, and judgment coverage `== 1.0`. These thresholds determine whether `make test-e2e` is a hard acceptance gate.

### 6.3 Options

- `Option A`: Keep current thresholds as hard gates
  - **Rubrics**: `Conf:60% | Invest:i | Blast:i | Reversal:ii | Fit:i | Reuse:i | Obs:i | Surface:i | Perf:na`
  - **Approach**: Implement the plan exactly and require the smoke metrics to meet the current thresholds before acceptance.
  - **Example**: Fail `make test-e2e` if good/bad pass-rate gap is below `0.30`.
  - **Architecture**: Treats evaluation quality as part of the product contract, not a manual inspection step.
  - **SSoT**: Smoke scripts own threshold enforcement and summary artifacts.
  - **System limits**: Unknown - not available in local context.
  - **Trade-offs**: Strong acceptance gate, but thresholds may need adjustment after real model output.
- `Option B`: First run records metrics, second run enforces thresholds
  - **Rubrics**: `Conf:50% | Invest:ii | Blast:ii | Reversal:i | Fit:ii | Reuse:ii | Obs:ii | Surface:ii | Perf:na`
  - **Approach**: During implementation, capture initial metrics and then commit threshold enforcement once observed output confirms realistic values.
  - **Example**: `debug/smoke/summary.json` is inspected once, then thresholds are encoded in `scripts/smoke_curl.sh`.
  - **Architecture**: More empirical, but adds a temporary acceptance distinction.
  - **SSoT**: Final thresholds still live in smoke scripts after calibration.
  - **System limits**: Unknown - not available in local context.
  - **Trade-offs**: Reduces risk of arbitrary threshold failure, but weakens "all tests green" until thresholds are set.
- `Option C`: Conservative thresholds for first release
  - **Rubrics**: `Conf:50% | Invest:iii | Blast:iii | Reversal:iii | Fit:iii | Reuse:iii | Obs:iii | Surface:i | Perf:na`
  - **Approach**: Lower first-pass quality gates, for example good `>= 0.70`, bad `<= 0.60`, gap `>= 0.20`, then tighten later.
  - **Example**: Accept weaker separation while the rubric prompts stabilize.
  - **Architecture**: Easier initial green path, but risks normalizing weak eval quality.
  - **SSoT**: Smoke scripts still own threshold enforcement.
  - **System limits**: Unknown - not available in local context.
  - **Trade-offs**: More likely to pass, less meaningful as an evaluation acceptance gate.

### 6.4 Recommendation

I recommend **Option A**. The current thresholds are reasonable for an MVP and keep the implementation honest. If they fail, we should inspect artifacts and fix prompts or fixtures, not weaken the contract immediately.

Decision: **Selected Option A**. Keep the thresholds as hard gates and record diagnostics in smoke output, including checklist ID, evaluation ID, final question count, judgment count, pass rate, failed question IDs, limit hit counts, and per-case good/bad classification.

### 7.1 Question

Should question-count bounds live only in schema and Go validation, with no fixed required count in prompt text?

### 7.2 Context & clarification

You asked to remove the required number of questions from the generation prompt. The system still needs upper bounds to protect runtime cost and checklist size. The relevant surfaces are `internal/llm/prompts.go`, `internal/llm/schemas.go`, and evalcore validation.

### 7.3 Options

- `Option A`: Schema and Go validation own all count bounds
  - **Rubrics**: `Conf:80% | Invest:i | Blast:i | Reversal:ii | Fit:i | Reuse:i | Obs:i | Surface:i | Perf:i`
  - **Approach**: Prompts describe quality requirements only. JSON schema and Go validation enforce `maxItems`, split counts, and final question budget.
  - **Example**: Prompt says "Generate concrete binary questions for this rubric"; schema has `maxItems`, and Go rejects over-budget outputs.
  - **Architecture**: Clean split between semantic prompt instructions and structural contracts.
  - **SSoT**: Count limits live in schema/Go, not repeated in prose prompts.
  - **System limits**: Use the fan-out limits chosen in Question 3.
  - **Trade-offs**: Direct and DRY; model may return fewer candidates than desired.
- `Option B`: Prompt uses non-numeric concision guidance plus schema/Go bounds
  - **Rubrics**: `Conf:70% | Invest:ii | Blast:ii | Reversal:i | Fit:ii | Reuse:ii | Obs:ii | Surface:i | Perf:ii`
  - **Approach**: Prompt avoids exact counts but says to produce a concise, sufficient set; schema and Go still enforce hard maximums.
  - **Example**: Prompt says "Generate a concise set of non-overlapping binary questions."
  - **Architecture**: Slightly more prompt influence, still keeps numeric limits out of prompt text.
  - **SSoT**: Numeric limits remain in schema/Go.
  - **System limits**: Use the fan-out limits chosen in Question 3.
  - **Trade-offs**: May improve model behavior, but adds a subjective prompt term.

### 7.4 Recommendation

I recommend **Option A**. It follows your instruction most directly: no required question count in the prompt, with structural bounds handled by schema and validation.

Decision: **Selected Option A**. Numeric count bounds live in schema/config/Go validation, not prompt prose.

### 8.1 Question

Should `GET /checklists/{id}` expose all generated structured metadata, including rationales?

### 8.2 Context & clarification

The plan currently exposes `dimensions`, `candidate_questions`, diagnostic `weights`, and final `questions`. Rationales are useful for debugging why candidates were kept, deleted, or split. Raw prompts, task/context, and model answers remain in Garage rather than Postgres response bodies.

### 8.3 Options

- `Option A`: Expose all structured metadata and rationales
  - **Rubrics**: `Conf:80% | Invest:i | Blast:i | Reversal:ii | Fit:i | Reuse:i | Obs:i | Surface:ii | Perf:na`
  - **Approach**: Return dimensions, candidates, diagnostic weights, rationales, and final questions in succeeded checklist responses.
  - **Example**: A weight object includes `candidate_question_id`, `weight`, and `rationale`.
  - **Architecture**: Matches the audit/debug nature of this local evaluation service.
  - **SSoT**: Postgres structured rows are the response source; raw artifacts stay in Garage.
  - **System limits**: Unknown - not available in local context.
  - **Trade-offs**: Most inspectable, larger response surface.
- `Option B`: Expose dimensions, diagnostic weights, and final questions only
  - **Rubrics**: `Conf:70% | Invest:ii | Blast:ii | Reversal:i | Fit:ii | Reuse:ii | Obs:ii | Surface:i | Perf:na`
  - **Approach**: Return enough to evaluate the final checklist while hiding candidate question details from the default API response.
  - **Example**: `candidate_questions` is omitted; `questions` still include `source_candidate_id`.
  - **Architecture**: Smaller response surface, but weakens traceability from final question back to source candidate text.
  - **SSoT**: Postgres still owns all structured data; API chooses a smaller projection.
  - **System limits**: Unknown - not available in local context.
  - **Trade-offs**: Cleaner response, less useful for debugging model-generated checklists.
- `Option C`: Expose final questions only
  - **Rubrics**: `Conf:60% | Invest:iii | Blast:iii | Reversal:iii | Fit:iii | Reuse:iii | Obs:iii | Surface:i | Perf:na`
  - **Approach**: Return only final judged questions and score-relevant fields.
  - **Example**: Checklist response contains `questions` but not dimensions, candidates, or diagnostic weights.
  - **Architecture**: Minimal API surface, but conflicts with the plan's inspectability goals.
  - **SSoT**: Final questions are the only API-visible checklist state.
  - **System limits**: Unknown - not available in local context.
  - **Trade-offs**: Small response, poor auditability and harder prompt debugging.

### 8.4 Recommendation

I recommend **Option A**. This project is currently an operator/debuggable eval service, and the generated rationale trail is important for trusting and improving the rubric pipeline.

Decision: **Selected Option A**. `GET /checklists/{id}` should expose all generated structured metadata, including rationales.

### 9.1 Question

Which LiteLLM model profile should the new multi-call DAG use?

### 9.2 Context & clarification

The existing service uses a local OpenAI-compatible LiteLLM setup and environment-driven model profile. The new DAG increases the number of LLM calls: dimension analysis, per-dimension question generation, refinement assignment, splitting, and judging. Adding multiple model profiles would increase configuration surface and test burden.

### 9.3 Options

- `Option A`: Reuse the current default LiteLLM profile
  - **Rubrics**: `Conf:80% | Invest:i | Blast:i | Reversal:ii | Fit:i | Reuse:i | Obs:ii | Surface:i | Perf:na`
  - **Approach**: Use the same configured model profile for every new rubric refinement prompt family.
  - **Example**: All activities continue reading the existing `BIN_EVAL_MODEL_PROFILE` or current config equivalent.
  - **Architecture**: Matches current local architecture and keeps one LLM configuration path.
  - **SSoT**: Existing config remains the source of model selection.
  - **System limits**: Unknown - not available in local context.
  - **Trade-offs**: Smallest implementation; no per-step model optimization.
- `Option B`: One explicit rubric profile for all checklist construction prompts
  - **Rubrics**: `Conf:60% | Invest:ii | Blast:ii | Reversal:i | Fit:ii | Reuse:ii | Obs:i | Surface:ii | Perf:na`
  - **Approach**: Add one config value for rubric construction and use the existing/default profile for judging.
  - **Example**: `BIN_EVAL_RUBRIC_MODEL_PROFILE` for dimensions, generation, refinement, and splitting.
  - **Architecture**: Still compact, but adds a second model-selection path.
  - **SSoT**: Config owns the split between construction and judging.
  - **System limits**: Unknown - not available in local context.
  - **Trade-offs**: More tunable, more config and tests.
- `Option C`: Separate profiles per prompt family
  - **Rubrics**: `Conf:40% | Invest:iii | Blast:iii | Reversal:iii | Fit:iii | Reuse:iii | Obs:iii | Surface:iii | Perf:na`
  - **Approach**: Configure different models for dimension analysis, question generation, refinement, splitting, and judging.
  - **Example**: Five prompt-family-specific model profile settings.
  - **Architecture**: High flexibility, but broadens configuration and increases duplication risk.
  - **SSoT**: Config owns every prompt-family model choice.
  - **System limits**: Unknown - not available in local context.
  - **Trade-offs**: Maximum tuning surface, not justified for this MVP.

### 9.4 Recommendation

I recommend **Option A**. Reuse the current LiteLLM profile first and keep model selection as one existing configuration path.

Decision: **Selected Option A**. Reuse the current LiteLLM model profile for all rubric refinement prompt families.
