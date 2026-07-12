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
)

// TEST-010
// TEST-020
func TestPostgresMigrationsAndConcreteStore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool := openTestPool(t, ctx)
	defer pool.Close()
	if err := ApplyMigrations(ctx, pool, "migrations"); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	cleanupDB(t, ctx, pool)

	store := NewStore(pool)
	checklistID, err := store.CreateChecklist(ctx, "checklists/test/inputs/task.txt", "checklists/test/inputs/context.txt")
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
	if err := store.SucceedChecklist(ctx, checklistID, dimensions, candidates, weights, finalQuestions); err != nil {
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

	runningChecklistID, err := store.CreateChecklist(ctx, "task", "context")
	if err != nil {
		t.Fatalf("CreateChecklist running error = %v", err)
	}
	if _, err := store.CreateEvaluation(ctx, runningChecklistID, "answer"); !errors.Is(err, ErrConflict) {
		t.Fatalf("CreateEvaluation against running checklist error = %v, want ErrConflict", err)
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
	if err := store.SucceedEvaluation(ctx, evaluationID, checklistID, []evalcore.Judgment{
		{QuestionID: "q1", Evidence: "It is included.", Answer: evalcore.AnswerYes},
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

	if err := store.FailChecklist(ctx, checklistID, "late failure"); !errors.Is(err, ErrConflict) {
		t.Fatalf("terminal checklist update error = %v, want ErrConflict", err)
	}
	if err := store.FailEvaluation(ctx, evaluationID, checklistID, "late failure"); !errors.Is(err, ErrConflict) {
		t.Fatalf("terminal evaluation update error = %v, want ErrConflict", err)
	}

	assertDuplicateWeightRejected(t, ctx, pool, checklistID)
	assertDuplicateJudgmentRejected(t, ctx, pool, evaluationID, checklistID)
	assertCrossChecklistJudgmentRejected(t, ctx, store, pool, checklistID, evaluationID)
	assertNoRawInputColumns(t, ctx, pool)
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
	if _, err := pool.Exec(ctx, "truncate judgments, evaluations, question_weights, questions, candidate_questions, checklist_dimensions, checklists restart identity cascade"); err != nil {
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
	_, err := pool.Exec(ctx, `insert into judgments (evaluation_id, checklist_id, question_id, evidence, answer) values ($1, $2, 'q1', 'duplicate', 'yes')`, evaluationID, checklistID)
	if err == nil {
		t.Fatal("duplicate judgment insert unexpectedly succeeded")
	}
}

func assertCrossChecklistJudgmentRejected(t *testing.T, ctx context.Context, store *Store, pool *pgxpool.Pool, checklistID, evaluationID string) {
	t.Helper()
	otherID, err := store.CreateChecklist(ctx, "other-task", "other-context")
	if err != nil {
		t.Fatalf("CreateChecklist other error = %v", err)
	}
	otherDimensions := []evalcore.Dimension{{ID: "d1", Ordinal: 1, Name: "Other", Rubric: "Other.", Rationale: "Other."}}
	otherCandidates := []evalcore.CandidateQuestion{{ID: "c1", DimensionID: "d1", Ordinal: 1, Rationale: "r", Question: "Other?"}}
	otherWeights := []evalcore.Weight{{CandidateQuestionID: "c1", Rationale: "r", Weight: 1}}
	otherFinal := []evalcore.FinalQuestion{{ID: "q1", Ordinal: 1, DimensionID: "d1", SourceCandidateID: "c1", Rationale: "r", Question: "Other?"}}
	if err := store.SucceedChecklist(ctx, otherID, otherDimensions, otherCandidates, otherWeights, otherFinal); err != nil {
		t.Fatalf("SucceedChecklist other error = %v", err)
	}
	_, err = pool.Exec(ctx, `insert into judgments (evaluation_id, checklist_id, question_id, evidence, answer) values ($1, $2, 'q1', 'cross', 'yes')`, evaluationID, otherID)
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
