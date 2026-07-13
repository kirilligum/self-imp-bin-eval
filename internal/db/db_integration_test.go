//go:build integration

package db

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
	"github.com/kirilligum/self-imp-bin-eval/internal/failure"
)

func TestP06StructuredWorkflowFailures(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool := openTestPool(t, ctx)
	defer pool.Close()
	if err := ApplyMigrations(ctx, pool, "migrations"); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	cleanupDB(t, ctx, pool)

	store := NewStore(pool)
	checklistID, err := store.CreateChecklist(ctx, "task", "context", 3)
	if err != nil {
		t.Fatalf("CreateChecklist() error = %v", err)
	}
	details := failure.Details{
		WorkflowID:   "checklist-workflow-1",
		Stage:        "weight_assignment",
		ErrorClass:   failure.ClassModelOutputInvalid,
		ErrorCode:    string(evalcore.CodeInvalidFinalChecklist),
		Message:      "final question budget exceeded",
		Retryable:    false,
		AttemptCount: 3,
		Diagnostics: []evalcore.LimitDiagnostic{{
			LimitName:       "max_final_questions",
			ConfiguredLimit: 64,
			ObservedCount:   65,
			ChecklistID:     checklistID,
			Stage:           "weight_assignment",
		}},
		ArtifactReferences: []string{"checklists/" + checklistID + "/llm/weight_assignment/attempt-3/response.json"},
	}
	if err := store.FailChecklist(ctx, checklistID, details); err != nil {
		t.Fatalf("FailChecklist() error = %v", err)
	}

	got, err := store.GetChecklist(ctx, checklistID)
	if err != nil {
		t.Fatalf("GetChecklist() error = %v", err)
	}
	if got.Status != StatusFailed || got.Failure == nil {
		t.Fatalf("failed checklist = %#v", got)
	}
	if got.Failure.ID == "" || got.Failure.WorkflowID != details.WorkflowID || got.Failure.AttemptCount != 3 {
		t.Fatalf("failure identity/attempt metadata = %#v", got.Failure)
	}
	if len(got.Failure.Diagnostics) != 1 || got.Failure.Diagnostics[0] != details.Diagnostics[0] {
		t.Fatalf("failure diagnostics = %#v", got.Failure.Diagnostics)
	}
	if len(got.Failure.ArtifactReferences) != 1 || got.Failure.ArtifactReferences[0] != details.ArtifactReferences[0] {
		t.Fatalf("failure artifact references = %#v", got.Failure.ArtifactReferences)
	}

	var failureCount int
	if err := pool.QueryRow(ctx, `select count(*) from workflow_failures where checklist_id = $1`, checklistID).Scan(&failureCount); err != nil {
		t.Fatalf("count workflow failures error = %v", err)
	}
	if failureCount != 1 {
		t.Fatalf("workflow failure count = %d, want 1", failureCount)
	}
	if _, err := pool.Exec(ctx, `update workflow_failures set message = 'mutated' where checklist_id = $1`, checklistID); err == nil {
		t.Fatal("workflow failure update unexpectedly succeeded")
	}

	var evaluationID string
	if err := pool.QueryRow(ctx, `
		insert into evaluations (checklist_id, status, answer_artifact_key)
		values ($1, 'running', 'answer') returning id::text`, checklistID).Scan(&evaluationID); err != nil {
		t.Fatalf("create evaluation fixture error = %v", err)
	}
	assertWorkflowFailureEntityConstraint(t, ctx, pool, checklistID, evaluationID)

	evaluationFailure := details
	evaluationFailure.WorkflowID = "evaluation-workflow-1"
	evaluationFailure.Stage = "binary_judging"
	evaluationFailure.ErrorCode = "model_output_invalid"
	evaluationFailure.Message = "model output did not satisfy the schema"
	evaluationFailure.Diagnostics = nil
	if err := store.FailEvaluation(ctx, evaluationID, checklistID, evaluationFailure); err != nil {
		t.Fatalf("FailEvaluation() error = %v", err)
	}
	gotEvaluation, err := store.GetEvaluation(ctx, evaluationID)
	if err != nil {
		t.Fatalf("GetEvaluation() error = %v", err)
	}
	if gotEvaluation.Status != StatusFailed || gotEvaluation.Failure == nil || gotEvaluation.Failure.WorkflowID != evaluationFailure.WorkflowID {
		t.Fatalf("failed evaluation = %#v", gotEvaluation)
	}

	for _, table := range []string{"checklists", "evaluations"} {
		var count int
		if err := pool.QueryRow(ctx, `
			select count(*) from information_schema.columns
			where table_schema = current_schema() and table_name = $1 and column_name = 'error_message'`, table).Scan(&count); err != nil {
			t.Fatalf("inspect %s columns error = %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s.error_message still exists", table)
		}
	}
}

func TestP06TemporalIdempotency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool := openTestPool(t, ctx)
	defer pool.Close()
	if err := ApplyMigrations(ctx, pool, "migrations"); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	cleanupDB(t, ctx, pool)
	store := NewStore(pool)

	dimensions := []evalcore.Dimension{{ID: "d1", Ordinal: 1, Name: "Correctness", Rubric: "Check correctness.", Rationale: "Core."}}
	candidates := []evalcore.CandidateQuestion{{ID: "c1", DimensionID: "d1", Ordinal: 1, Rationale: "atomic", Question: "Correct?"}}
	weights := []evalcore.Weight{{CandidateQuestionID: "c1", Rationale: "atomic", Weight: 1}}
	questions := []evalcore.FinalQuestion{{ID: "q1", Ordinal: 1, DimensionID: "d1", SourceCandidateID: "c1", Rationale: "atomic", Question: "Correct?"}}

	succeededChecklistID, err := store.CreateChecklist(ctx, "task", "context", 1)
	if err != nil {
		t.Fatalf("CreateChecklist() error = %v", err)
	}
	if err := store.SucceedChecklist(ctx, succeededChecklistID, dimensions, candidates, weights, questions, evalcore.DefaultChecklistLimits()); err != nil {
		t.Fatalf("SucceedChecklist() error = %v", err)
	}
	if err := store.SucceedChecklist(ctx, succeededChecklistID, dimensions, candidates, weights, questions, evalcore.DefaultChecklistLimits()); err != nil {
		t.Fatalf("identical SucceedChecklist retry error = %v", err)
	}
	changedQuestions := append([]evalcore.FinalQuestion(nil), questions...)
	changedQuestions[0].Question = "Different?"
	if err := store.SucceedChecklist(ctx, succeededChecklistID, dimensions, candidates, weights, changedQuestions, evalcore.DefaultChecklistLimits()); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting SucceedChecklist retry error = %v, want ErrConflict", err)
	}

	failedChecklistID, err := store.CreateChecklist(ctx, "task", "context", 1)
	if err != nil {
		t.Fatalf("CreateChecklist() for failure error = %v", err)
	}
	failedDetails := testFailureDetails("workflow-failed-checklist", "failed")
	if err := store.FailChecklist(ctx, failedChecklistID, failedDetails); err != nil {
		t.Fatalf("FailChecklist() error = %v", err)
	}
	if err := store.FailChecklist(ctx, failedChecklistID, failedDetails); err != nil {
		t.Fatalf("identical FailChecklist retry error = %v", err)
	}
	changedFailure := failedDetails
	changedFailure.Message = "different safe failure"
	if err := store.FailChecklist(ctx, failedChecklistID, changedFailure); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting FailChecklist retry error = %v, want ErrConflict", err)
	}

	judgments := []evalcore.RunJudgment{{RunIndex: 1, QuestionID: "q1", Evidence: "Present.", Answer: evalcore.AnswerYes}}
	aggregated, err := evalcore.AggregateJudgments(questions, judgments, 1)
	if err != nil {
		t.Fatalf("AggregateJudgments() error = %v", err)
	}
	succeededEvaluationID, err := store.CreateEvaluation(ctx, succeededChecklistID, "answer")
	if err != nil {
		t.Fatalf("CreateEvaluation() error = %v", err)
	}
	if err := store.SucceedEvaluation(ctx, succeededEvaluationID, succeededChecklistID, judgments, aggregated.Score); err != nil {
		t.Fatalf("SucceedEvaluation() error = %v", err)
	}
	if err := store.SucceedEvaluation(ctx, succeededEvaluationID, succeededChecklistID, judgments, aggregated.Score); err != nil {
		t.Fatalf("identical SucceedEvaluation retry error = %v", err)
	}
	changedJudgments := []evalcore.RunJudgment{{RunIndex: 1, QuestionID: "q1", Evidence: "Absent.", Answer: evalcore.AnswerNo}}
	changedAggregated, err := evalcore.AggregateJudgments(questions, changedJudgments, 1)
	if err != nil {
		t.Fatalf("changed AggregateJudgments() error = %v", err)
	}
	if err := store.SucceedEvaluation(ctx, succeededEvaluationID, succeededChecklistID, changedJudgments, changedAggregated.Score); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting SucceedEvaluation retry error = %v, want ErrConflict", err)
	}

	failedEvaluationID, err := store.CreateEvaluation(ctx, succeededChecklistID, "answer")
	if err != nil {
		t.Fatalf("CreateEvaluation() for failure error = %v", err)
	}
	failedEvaluationDetails := testFailureDetails("workflow-failed-evaluation", "failed")
	if err := store.FailEvaluation(ctx, failedEvaluationID, succeededChecklistID, failedEvaluationDetails); err != nil {
		t.Fatalf("FailEvaluation() error = %v", err)
	}
	if err := store.FailEvaluation(ctx, failedEvaluationID, succeededChecklistID, failedEvaluationDetails); err != nil {
		t.Fatalf("identical FailEvaluation retry error = %v", err)
	}
	changedEvaluationFailure := failedEvaluationDetails
	changedEvaluationFailure.ErrorCode = "different"
	if err := store.FailEvaluation(ctx, failedEvaluationID, succeededChecklistID, changedEvaluationFailure); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting FailEvaluation retry error = %v, want ErrConflict", err)
	}
}

func TestP06RepeatedEvaluation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool := openTestPool(t, ctx)
	defer pool.Close()
	if err := ApplyMigrations(ctx, pool, "migrations"); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	cleanupDB(t, ctx, pool)
	store := NewStore(pool)

	checklistID, err := store.CreateChecklist(ctx, "task", "context", 3)
	if err != nil {
		t.Fatalf("CreateChecklist() error = %v", err)
	}
	dimensions := []evalcore.Dimension{{ID: "d1", Ordinal: 1, Name: "Correctness", Rubric: "Check correctness.", Rationale: "Core."}}
	candidates := []evalcore.CandidateQuestion{{ID: "c1", DimensionID: "d1", Ordinal: 1, Rationale: "r", Question: "Q1?"}, {ID: "c2", DimensionID: "d1", Ordinal: 2, Rationale: "r", Question: "Q2?"}}
	weights := []evalcore.Weight{{CandidateQuestionID: "c1", Rationale: "r", Weight: 1}, {CandidateQuestionID: "c2", Rationale: "r", Weight: 1}}
	questions := []evalcore.FinalQuestion{{ID: "q1", Ordinal: 1, DimensionID: "d1", SourceCandidateID: "c1", Rationale: "r", Question: "Q1?"}, {ID: "q2", Ordinal: 2, DimensionID: "d1", SourceCandidateID: "c2", Rationale: "r", Question: "Q2?"}}
	if err := store.SucceedChecklist(ctx, checklistID, dimensions, candidates, weights, questions, evalcore.DefaultChecklistLimits()); err != nil {
		t.Fatalf("SucceedChecklist() error = %v", err)
	}
	checklist, err := store.GetChecklist(ctx, checklistID)
	if err != nil || checklist.EvaluationRuns != 3 {
		t.Fatalf("persisted evaluation_runs = %d, error = %v", checklist.EvaluationRuns, err)
	}

	evaluationID, err := store.CreateEvaluation(ctx, checklistID, "answer")
	if err != nil {
		t.Fatalf("CreateEvaluation() error = %v", err)
	}
	runJudgments := []evalcore.RunJudgment{
		{RunIndex: 1, QuestionID: "q1", Evidence: "yes", Answer: evalcore.AnswerYes}, {RunIndex: 1, QuestionID: "q2", Evidence: "no", Answer: evalcore.AnswerNo},
		{RunIndex: 2, QuestionID: "q1", Evidence: "yes", Answer: evalcore.AnswerYes}, {RunIndex: 2, QuestionID: "q2", Evidence: "yes", Answer: evalcore.AnswerYes},
		{RunIndex: 3, QuestionID: "q1", Evidence: "no", Answer: evalcore.AnswerNo}, {RunIndex: 3, QuestionID: "q2", Evidence: "no", Answer: evalcore.AnswerNo},
	}
	aggregated, err := evalcore.AggregateJudgments(questions, runJudgments, 3)
	if err != nil {
		t.Fatalf("AggregateJudgments() error = %v", err)
	}
	if err := store.SucceedEvaluation(ctx, evaluationID, checklistID, runJudgments, aggregated.Score); err != nil {
		t.Fatalf("SucceedEvaluation() error = %v", err)
	}
	got, err := store.GetEvaluation(ctx, evaluationID)
	if err != nil {
		t.Fatalf("GetEvaluation() error = %v", err)
	}
	if len(got.RunJudgments) != 6 || len(got.Judgments) != 2 || got.Judgments[0].Answer != evalcore.AnswerYes || got.Judgments[1].Answer != evalcore.AnswerNo {
		t.Fatalf("repeated evaluation round trip = %#v", got)
	}
	var rowCount int
	if err := pool.QueryRow(ctx, `select count(*) from judgments where evaluation_id = $1`, evaluationID).Scan(&rowCount); err != nil || rowCount != 6 {
		t.Fatalf("judgment rows = %d, error = %v", rowCount, err)
	}
}

func assertWorkflowFailureEntityConstraint(t *testing.T, ctx context.Context, pool *pgxpool.Pool, checklistID, evaluationID string) {
	t.Helper()
	columns := `(workflow_id, stage, error_class, error_code, message, retryable, attempt_count, diagnostics, artifact_references)`
	values := `('workflow', 'stage', 'class', 'code', 'safe', false, 1, '[]', '[]')`
	if _, err := pool.Exec(ctx, `insert into workflow_failures `+columns+` values `+values); err == nil {
		t.Fatal("workflow failure without an entity unexpectedly succeeded")
	}
	if _, err := pool.Exec(ctx, `insert into workflow_failures (checklist_id, evaluation_id, workflow_id, stage, error_class, error_code, message, retryable, attempt_count, diagnostics, artifact_references) values ($1, $2, 'workflow', 'stage', 'class', 'code', 'safe', false, 1, '[]', '[]')`, checklistID, evaluationID); err == nil {
		t.Fatal("workflow failure with both entities unexpectedly succeeded")
	}
}

func TestPostgresMigrationsAndConcreteStore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool := openTestPool(t, ctx)
	defer pool.Close()
	if err := ApplyMigrations(ctx, pool, "migrations"); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	if err := ApplyMigrations(ctx, pool, "migrations"); err != nil {
		t.Fatalf("ApplyMigrations() second run error = %v", err)
	}
	assertMigrationCount(t, ctx, pool)
	cleanupDB(t, ctx, pool)

	store := NewStore(pool)
	if _, err := store.GetChecklist(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetChecklist missing error = %v, want ErrNotFound", err)
	}
	if _, err := store.GetEvaluation(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetEvaluation missing error = %v, want ErrNotFound", err)
	}
	if _, err := store.CreateEvaluation(ctx, "00000000-0000-0000-0000-000000000000", "answer"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("CreateEvaluation missing checklist error = %v, want ErrNotFound", err)
	}

	checklistID, err := store.CreateChecklist(ctx, "checklists/test/inputs/task.txt", "checklists/test/inputs/context.txt", 1)
	if err != nil {
		t.Fatalf("CreateChecklist() error = %v", err)
	}

	dimensions := []evalcore.Dimension{{ID: "d1", Ordinal: 1, Name: "Correctness", Rubric: "Check correctness.", Rationale: "Core."}}
	candidates := []evalcore.CandidateQuestion{
		{ID: "c1", DimensionID: "d1", Ordinal: 1, Rationale: "redundant", Question: "Excluded?"},
		{ID: "c2", DimensionID: "d1", Ordinal: 2, Rationale: "important", Question: "Included?"},
	}
	weights := []evalcore.Weight{
		{CandidateQuestionID: "c1", Rationale: "duplicate", Weight: 0},
		{CandidateQuestionID: "c2", Rationale: "important", Weight: 1},
	}
	finalQuestions := []evalcore.FinalQuestion{{ID: "q1", Ordinal: 1, DimensionID: "d1", SourceCandidateID: "c2", Rationale: "important", Question: "Included?"}}
	if err := store.SucceedChecklist(ctx, checklistID, dimensions, candidates, weights, finalQuestions, evalcore.DefaultChecklistLimits()); err != nil {
		t.Fatalf("SucceedChecklist() error = %v", err)
	}
	gotChecklist, err := store.GetChecklist(ctx, checklistID)
	if err != nil {
		t.Fatalf("GetChecklist() error = %v", err)
	}
	if gotChecklist.Status != StatusSucceeded {
		t.Fatalf("checklist status = %s", gotChecklist.Status)
	}
	if len(gotChecklist.Weights) != 2 || gotChecklist.Weights[0].Weight != 0 || gotChecklist.Weights[1].Weight != 1 {
		t.Fatalf("weights did not round-trip including zero: %#v", gotChecklist.Weights)
	}
	if len(gotChecklist.Dimensions) != 1 || gotChecklist.Dimensions[0].ID != "d1" {
		t.Fatalf("dimensions did not round-trip: %#v", gotChecklist.Dimensions)
	}
	if len(gotChecklist.CandidateQuestions) != 2 || gotChecklist.CandidateQuestions[0].ID != "c1" || gotChecklist.CandidateQuestions[1].ID != "c2" {
		t.Fatalf("candidate questions did not round-trip: %#v", gotChecklist.CandidateQuestions)
	}
	if len(gotChecklist.Questions) != 1 || gotChecklist.Questions[0].ID != "q1" || gotChecklist.Questions[0].SourceCandidateID != "c2" {
		t.Fatalf("final questions did not round-trip: %#v", gotChecklist.Questions)
	}

	runningChecklistID, err := store.CreateChecklist(ctx, "task", "context", 1)
	if err != nil {
		t.Fatalf("CreateChecklist running error = %v", err)
	}
	if _, err := store.CreateEvaluation(ctx, runningChecklistID, "answer"); !errors.Is(err, ErrConflict) {
		t.Fatalf("CreateEvaluation against running checklist error = %v, want ErrConflict", err)
	}
	if err := store.FailChecklist(ctx, runningChecklistID, testFailureDetails("workflow-running", "not_ready")); err != nil {
		t.Fatalf("FailChecklist running error = %v", err)
	}
	if _, err := store.CreateEvaluation(ctx, runningChecklistID, "answer"); !errors.Is(err, ErrConflict) {
		t.Fatalf("CreateEvaluation against failed checklist error = %v, want ErrConflict", err)
	}

	evaluationID, err := store.CreateEvaluation(ctx, checklistID, "evaluations/test/inputs/model_answer.txt")
	if err != nil {
		t.Fatalf("CreateEvaluation() error = %v", err)
	}
	score, err := evalcore.ScoreChecklist(finalQuestions, []evalcore.Judgment{
		{QuestionID: "q1", Evidence: "It is included.", Answer: evalcore.AnswerYes},
	})
	if err != nil {
		t.Fatalf("ScoreChecklist() error = %v", err)
	}
	if err := store.SucceedEvaluation(ctx, evaluationID, checklistID, []evalcore.RunJudgment{
		{RunIndex: 1, QuestionID: "q1", Evidence: "It is included.", Answer: evalcore.AnswerYes},
	}, score); err != nil {
		t.Fatalf("SucceedEvaluation() error = %v", err)
	}
	gotEvaluation, err := store.GetEvaluation(ctx, evaluationID)
	if err != nil {
		t.Fatalf("GetEvaluation() error = %v", err)
	}
	if gotEvaluation.Status != StatusSucceeded || gotEvaluation.TotalPossiblePoints == nil || *gotEvaluation.TotalPossiblePoints != 1 {
		t.Fatalf("evaluation did not round-trip score: %#v", gotEvaluation)
	}
	if len(gotEvaluation.Judgments) != 1 || gotEvaluation.Judgments[0].QuestionID != "q1" {
		t.Fatalf("judgments did not round-trip active-only rows: %#v", gotEvaluation.Judgments)
	}

	mismatchedEvaluationID, err := store.CreateEvaluation(ctx, checklistID, "evaluations/test/inputs/mismatched.txt")
	if err != nil {
		t.Fatalf("CreateEvaluation mismatched score error = %v", err)
	}
	badScore := evalcore.ScoreResult{SatisfiedPoints: 0, TotalPossiblePoints: 1, ChecklistPassRate: 0, FailedQuestionIDs: []string{"q1"}}
	err = store.SucceedEvaluation(ctx, mismatchedEvaluationID, checklistID, []evalcore.RunJudgment{
		{RunIndex: 1, QuestionID: "q1", Evidence: "It is included.", Answer: evalcore.AnswerYes},
	}, badScore)
	var semanticErr *evalcore.SemanticError
	if !errors.As(err, &semanticErr) || semanticErr.Code != evalcore.CodeInvalidJudgments {
		t.Fatalf("SucceedEvaluation mismatched score error = %T %v, want invalid judgments semantic error", err, err)
	}
	gotMismatched, err := store.GetEvaluation(ctx, mismatchedEvaluationID)
	if err != nil {
		t.Fatalf("GetEvaluation mismatched error = %v", err)
	}
	if gotMismatched.Status != StatusRunning || len(gotMismatched.Judgments) != 0 {
		t.Fatalf("mismatched evaluation mutated despite failure: %#v", gotMismatched)
	}

	if err := store.FailChecklist(ctx, checklistID, testFailureDetails("workflow-late-checklist", "late_failure")); !errors.Is(err, ErrConflict) {
		t.Fatalf("terminal checklist update error = %v, want ErrConflict", err)
	}
	if err := store.FailEvaluation(ctx, evaluationID, checklistID, testFailureDetails("workflow-late-evaluation", "late_failure")); !errors.Is(err, ErrConflict) {
		t.Fatalf("terminal evaluation update error = %v, want ErrConflict", err)
	}

	assertDuplicateWeightRejected(t, ctx, pool, checklistID)
	assertDuplicateJudgmentRejected(t, ctx, pool, evaluationID, checklistID)
	assertCrossChecklistJudgmentRejected(t, ctx, store, pool, checklistID, evaluationID)
	assertNoRawInputColumns(t, ctx, pool)
}

func testFailureDetails(workflowID, code string) failure.Details {
	return failure.Details{
		WorkflowID:   workflowID,
		Stage:        "test",
		ErrorClass:   failure.ClassInfraNonRetryable,
		ErrorCode:    code,
		Message:      "safe test failure",
		AttemptCount: 1,
	}
}

func assertMigrationCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	files, err := migrationFiles("migrations")
	if err != nil {
		t.Fatalf("migrationFiles() error = %v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `select count(*) from schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("schema_migrations count error = %v", err)
	}
	if count != len(files) {
		t.Fatalf("schema_migrations count = %d, want %d", count, len(files))
	}
}

func openTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("BIN_EVAL_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://bin_eval:bin_eval@127.0.0.1:55432/bin_eval?sslmode=disable"
	}
	var pool *pgxpool.Pool
	var err error
	for i := 0; i < 60; i++ {
		pool, err = pgxpool.New(ctx, databaseURL)
		if err == nil {
			if pingErr := pool.Ping(ctx); pingErr == nil {
				return pool
			}
			pool.Close()
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("postgres unavailable: %v", err)
	return nil
}

func cleanupDB(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, "truncate workflow_failures, judgments, evaluations, question_weights, questions, candidate_questions, checklist_dimensions, checklists restart identity cascade"); err != nil {
		t.Fatalf("cleanup error = %v", err)
	}
}

func assertDuplicateWeightRejected(t *testing.T, ctx context.Context, pool *pgxpool.Pool, checklistID string) {
	t.Helper()
	_, err := pool.Exec(ctx, `insert into question_weights (checklist_id, candidate_question_id, rationale, weight) values ($1, 'c2', 'duplicate', 1)`, checklistID)
	if err == nil {
		t.Fatal("duplicate weight insert unexpectedly succeeded")
	}
}

func assertDuplicateJudgmentRejected(t *testing.T, ctx context.Context, pool *pgxpool.Pool, evaluationID, checklistID string) {
	t.Helper()
	_, err := pool.Exec(ctx, `insert into judgments (evaluation_id, run_index, checklist_id, question_id, evidence, answer) values ($1, 1, $2, 'q1', 'duplicate', 'yes')`, evaluationID, checklistID)
	if err == nil {
		t.Fatal("duplicate judgment insert unexpectedly succeeded")
	}
}

func assertCrossChecklistJudgmentRejected(t *testing.T, ctx context.Context, store *Store, pool *pgxpool.Pool, checklistID, evaluationID string) {
	t.Helper()
	otherID, err := store.CreateChecklist(ctx, "other-task", "other-context", 1)
	if err != nil {
		t.Fatalf("CreateChecklist other error = %v", err)
	}
	otherDimensions := []evalcore.Dimension{{ID: "d1", Ordinal: 1, Name: "Other", Rubric: "Other.", Rationale: "Other."}}
	otherCandidates := []evalcore.CandidateQuestion{{ID: "c1", DimensionID: "d1", Ordinal: 1, Rationale: "r", Question: "Other?"}}
	otherWeights := []evalcore.Weight{{CandidateQuestionID: "c1", Rationale: "r", Weight: 1}}
	otherFinal := []evalcore.FinalQuestion{{ID: "q1", Ordinal: 1, DimensionID: "d1", SourceCandidateID: "c1", Rationale: "r", Question: "Other?"}}
	if err := store.SucceedChecklist(ctx, otherID, otherDimensions, otherCandidates, otherWeights, otherFinal, evalcore.DefaultChecklistLimits()); err != nil {
		t.Fatalf("SucceedChecklist other error = %v", err)
	}
	_, err = pool.Exec(ctx, `insert into judgments (evaluation_id, run_index, checklist_id, question_id, evidence, answer) values ($1, 2, $2, 'q1', 'cross', 'yes')`, evaluationID, otherID)
	if err == nil {
		t.Fatal("cross-checklist judgment insert unexpectedly succeeded")
	}
	_ = checklistID
}

func assertNoRawInputColumns(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	rows, err := pool.Query(ctx, `
		select table_name, column_name
		from information_schema.columns
		where table_schema = 'public'
		and table_name in ('checklists', 'evaluations')
		and column_name in ('task', 'context', 'model_answer', 'prompt_request', 'prompt_response')`)
	if err != nil {
		t.Fatalf("information_schema query error = %v", err)
	}
	defer rows.Close()
	var found []string
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			t.Fatalf("scan error = %v", err)
		}
		found = append(found, table+"."+column)
	}
	if len(found) > 0 {
		t.Fatalf("raw input columns found in Postgres: %s", strings.Join(found, ", "))
	}
}
