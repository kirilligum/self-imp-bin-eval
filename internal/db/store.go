package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
	"github.com/kirilligum/self-imp-bin-eval/internal/failure"
)

const (
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

type Store struct {
	pool *pgxpool.Pool
}

type Checklist struct {
	ID                 string
	Status             string
	EvaluationRuns     int
	TaskArtifactKey    string
	ContextArtifactKey string
	Failure            *failure.Record
	CreatedAt          time.Time
	CompletedAt        *time.Time
	Dimensions         []evalcore.Dimension
	CandidateQuestions []evalcore.CandidateQuestion
	Weights            []evalcore.Weight
	Questions          []evalcore.FinalQuestion
}

type Evaluation struct {
	ID                  string
	ChecklistID         string
	Status              string
	AnswerArtifactKey   string
	SatisfiedPoints     *int
	TotalPossiblePoints *int
	ChecklistPassRate   *float64
	FailedQuestionIDs   []string
	Failure             *failure.Record
	CreatedAt           time.Time
	CompletedAt         *time.Time
	RunJudgments        []evalcore.RunJudgment
	Judgments           []evalcore.AggregatedJudgment
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return NewStore(pool), nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func ApplyMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	if _, err := pool.Exec(ctx, `
		create table if not exists schema_migrations (
			filename text primary key,
			applied_at timestamptz not null default now()
		)`); err != nil {
		return err
	}
	files, err := migrationFiles(dir)
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, file := range files {
		name := filepath.Base(file)
		var applied bool
		if err := pool.QueryRow(ctx, `select exists(select 1 from schema_migrations where filename = $1)`, name).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		sqlBytes, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", file, err)
		}
		if _, err := tx.Exec(ctx, `insert into schema_migrations (filename) values ($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ApplyMigrations(ctx context.Context, dir string) error {
	return ApplyMigrations(ctx, s.pool, dir)
}

func migrationFiles(dir string) ([]string, error) {
	candidates := []string{dir, filepath.Join("..", "..", dir)}
	for _, candidate := range candidates {
		files, err := filepath.Glob(filepath.Join(candidate, "*.sql"))
		if err != nil {
			return nil, err
		}
		if len(files) > 0 {
			sort.Strings(files)
			return files, nil
		}
	}
	return nil, fmt.Errorf("no migration files found under %q", dir)
}

func (s *Store) CreateChecklist(ctx context.Context, taskArtifactKey, contextArtifactKey string, evaluationRuns int) (string, error) {
	if err := evalcore.ValidateEvaluationRuns(evaluationRuns, evaluationRuns); err != nil {
		return "", err
	}
	var id string
	err := s.pool.QueryRow(ctx, `
		insert into checklists (status, evaluation_runs, task_artifact_key, context_artifact_key)
		values ($1, $2, $3, $4)
		returning id::text`, StatusRunning, evaluationRuns, taskArtifactKey, contextArtifactKey).Scan(&id)
	return id, err
}

func (s *Store) CreateChecklistForWorkflow(ctx context.Context, evaluationRuns int) (string, error) {
	if err := evalcore.ValidateEvaluationRuns(evaluationRuns, evaluationRuns); err != nil {
		return "", err
	}
	var id string
	err := s.pool.QueryRow(ctx, `
		with generated as (select gen_random_uuid() as id)
		insert into checklists (id, status, evaluation_runs, task_artifact_key, context_artifact_key)
		select id,
		       $1,
		       $2,
		       'checklists/' || id::text || '/inputs/task.txt',
		       'checklists/' || id::text || '/inputs/context.txt'
		from generated
		returning id::text`, StatusRunning, evaluationRuns).Scan(&id)
	return id, err
}

func (s *Store) SucceedChecklist(ctx context.Context, checklistID string, dimensions []evalcore.Dimension, candidates []evalcore.CandidateQuestion, weights []evalcore.Weight, questions []evalcore.FinalQuestion, limits evalcore.ChecklistLimits) error {
	limits = limits.WithDefaults()
	if err := limits.Validate(); err != nil {
		return err
	}
	if err := evalcore.ValidateDimensions(dimensions, limits); err != nil {
		return err
	}
	if err := evalcore.ValidateCandidateQuestions(dimensions, candidates, limits); err != nil {
		return err
	}
	if err := evalcore.ValidateWeights(candidates, weights, limits); err != nil {
		return err
	}
	if err := evalcore.ValidateFinalQuestions(dimensions, candidates, questions, limits); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	status, err := lockStatus(ctx, tx, `select status from checklists where id = $1 for update`, checklistID)
	if err != nil {
		return err
	}
	if status != StatusRunning {
		if err := tx.Rollback(ctx); err != nil {
			return err
		}
		return s.compareSucceededChecklist(ctx, checklistID, dimensions, candidates, weights, questions)
	}

	for _, dimension := range dimensions {
		if _, err := tx.Exec(ctx, `
			insert into checklist_dimensions (checklist_id, id, ordinal, name, rubric, rationale)
			values ($1, $2, $3, $4, $5, $6)`,
			checklistID, dimension.ID, dimension.Ordinal, dimension.Name, dimension.Rubric, dimension.Rationale); err != nil {
			return err
		}
	}
	for _, candidate := range candidates {
		if _, err := tx.Exec(ctx, `
			insert into candidate_questions (checklist_id, id, dimension_id, ordinal, rationale, question)
			values ($1, $2, $3, $4, $5, $6)`,
			checklistID, candidate.ID, candidate.DimensionID, candidate.Ordinal, candidate.Rationale, candidate.Question); err != nil {
			return err
		}
	}
	for _, weight := range weights {
		if _, err := tx.Exec(ctx, `
			insert into question_weights (checklist_id, candidate_question_id, rationale, weight)
			values ($1, $2, $3, $4)`,
			checklistID, weight.CandidateQuestionID, weight.Rationale, weight.Weight); err != nil {
			return err
		}
	}
	for _, question := range questions {
		if _, err := tx.Exec(ctx, `
			insert into questions (checklist_id, id, ordinal, dimension_id, source_candidate_id, rationale, question)
			values ($1, $2, $3, $4, $5, $6, $7)`,
			checklistID, question.ID, question.Ordinal, question.DimensionID, question.SourceCandidateID, question.Rationale, question.Question); err != nil {
			return err
		}
	}
	tag, err := tx.Exec(ctx, `
		update checklists
		set status = $2, completed_at = now()
		where id = $1 and status = $3`, checklistID, StatusSucceeded, StatusRunning)
	if err != nil {
		return mapTerminalTriggerError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return tx.Commit(ctx)
}

func (s *Store) FailChecklist(ctx context.Context, checklistID string, details failure.Details) error {
	if err := details.Validate(); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	status, err := lockStatus(ctx, tx, `select status from checklists where id = $1 for update`, checklistID)
	if err != nil {
		return err
	}
	if status != StatusRunning {
		if err := tx.Rollback(ctx); err != nil {
			return err
		}
		return s.compareFailedChecklist(ctx, checklistID, details)
	}
	if _, err := insertWorkflowFailure(ctx, tx, &checklistID, nil, details); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		update checklists
		set status = $2, completed_at = now()
		where id = $1 and status = $3`, checklistID, StatusFailed, StatusRunning)
	if err != nil {
		return mapTerminalTriggerError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return tx.Commit(ctx)
}

func (s *Store) GetChecklist(ctx context.Context, checklistID string) (Checklist, error) {
	var checklist Checklist
	var completedAt sql.NullTime
	err := s.pool.QueryRow(ctx, `
		select id::text, status, evaluation_runs, task_artifact_key, context_artifact_key, created_at, completed_at
		from checklists where id = $1`, checklistID).
		Scan(&checklist.ID, &checklist.Status, &checklist.EvaluationRuns, &checklist.TaskArtifactKey, &checklist.ContextArtifactKey, &checklist.CreatedAt, &completedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Checklist{}, ErrNotFound
	}
	if err != nil {
		return Checklist{}, err
	}
	if completedAt.Valid {
		checklist.CompletedAt = &completedAt.Time
	}
	if checklist.Status == StatusFailed {
		checklist.Failure, err = s.getWorkflowFailure(ctx, &checklistID, nil)
		if err != nil {
			return Checklist{}, err
		}
	}

	dimensionRows, err := s.pool.Query(ctx, `
		select id, ordinal, name, rubric, rationale
		from checklist_dimensions where checklist_id = $1 order by ordinal`, checklistID)
	if err != nil {
		return Checklist{}, err
	}
	checklist.Dimensions, err = pgx.CollectRows(dimensionRows, pgx.RowToStructByName[evalcore.Dimension])
	if err != nil {
		return Checklist{}, err
	}

	candidateRows, err := s.pool.Query(ctx, `
		select id, dimension_id, ordinal, rationale, question
		from candidate_questions where checklist_id = $1 order by ordinal`, checklistID)
	if err != nil {
		return Checklist{}, err
	}
	checklist.CandidateQuestions, err = pgx.CollectRows(candidateRows, pgx.RowToStructByName[evalcore.CandidateQuestion])
	if err != nil {
		return Checklist{}, err
	}

	weightRows, err := s.pool.Query(ctx, `
		select w.candidate_question_id, w.rationale, w.weight
		from question_weights w
		join candidate_questions q on q.checklist_id = w.checklist_id and q.id = w.candidate_question_id
		where w.checklist_id = $1
		order by q.ordinal`, checklistID)
	if err != nil {
		return Checklist{}, err
	}
	defer weightRows.Close()
	for weightRows.Next() {
		var weight evalcore.Weight
		if err := weightRows.Scan(&weight.CandidateQuestionID, &weight.Rationale, &weight.Weight); err != nil {
			return Checklist{}, err
		}
		checklist.Weights = append(checklist.Weights, weight)
	}
	if err := weightRows.Err(); err != nil {
		return Checklist{}, err
	}

	finalRows, err := s.pool.Query(ctx, `
		select id, ordinal, dimension_id, source_candidate_id, rationale, question
		from questions where checklist_id = $1 order by ordinal`, checklistID)
	if err != nil {
		return Checklist{}, err
	}
	checklist.Questions, err = pgx.CollectRows(finalRows, pgx.RowToStructByName[evalcore.FinalQuestion])
	if err != nil {
		return Checklist{}, err
	}
	return checklist, nil
}

func (s *Store) CreateEvaluation(ctx context.Context, checklistID, answerArtifactKey string) (string, error) {
	var status string
	if err := s.pool.QueryRow(ctx, `select status from checklists where id = $1`, checklistID).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	} else if err != nil {
		return "", err
	}
	if status != StatusSucceeded {
		return "", ErrConflict
	}
	var id string
	err := s.pool.QueryRow(ctx, `
		insert into evaluations (checklist_id, status, answer_artifact_key)
		values ($1, $2, $3)
		returning id::text`, checklistID, StatusRunning, answerArtifactKey).Scan(&id)
	return id, err
}

func (s *Store) CreateEvaluationForWorkflow(ctx context.Context, checklistID string) (string, error) {
	var status string
	if err := s.pool.QueryRow(ctx, `select status from checklists where id = $1`, checklistID).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	} else if err != nil {
		return "", err
	}
	if status != StatusSucceeded {
		return "", ErrConflict
	}
	var id string
	err := s.pool.QueryRow(ctx, `
		with generated as (select gen_random_uuid() as id)
		insert into evaluations (id, checklist_id, status, answer_artifact_key)
		select id,
		       $1,
		       $2,
		       'evaluations/' || id::text || '/inputs/model_answer.txt'
		from generated
		returning id::text`, checklistID, StatusRunning).Scan(&id)
	return id, err
}

func (s *Store) SucceedEvaluation(ctx context.Context, evaluationID, checklistID string, runJudgments []evalcore.RunJudgment, score evalcore.ScoreResult) error {
	checklist, err := s.GetChecklist(ctx, checklistID)
	if err != nil {
		return err
	}
	aggregated, err := evalcore.AggregateJudgments(checklist.Questions, runJudgments, checklist.EvaluationRuns)
	if err != nil {
		return err
	}
	if !scoreResultsEqual(score, aggregated.Score) {
		return &evalcore.SemanticError{
			Code:    evalcore.CodeInvalidJudgments,
			Message: fmt.Sprintf("score does not match judgments for evaluation %s", evaluationID),
		}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	status, err := lockStatus(ctx, tx, `select status from evaluations where id = $1 and checklist_id = $2 for update`, evaluationID, checklistID)
	if err != nil {
		return err
	}
	if status != StatusRunning {
		if err := tx.Rollback(ctx); err != nil {
			return err
		}
		return s.compareSucceededEvaluation(ctx, evaluationID, checklistID, runJudgments, score)
	}

	for _, judgment := range runJudgments {
		if _, err := tx.Exec(ctx, `
			insert into judgments (evaluation_id, run_index, checklist_id, question_id, evidence, answer)
			values ($1, $2, $3, $4, $5, $6)`,
			evaluationID, judgment.RunIndex, checklistID, judgment.QuestionID, judgment.Evidence, judgment.Answer); err != nil {
			return err
		}
	}
	tag, err := tx.Exec(ctx, `
		update evaluations
		set status = $3,
		    satisfied_points = $4,
		    total_possible_points = $5,
		    checklist_pass_rate = $6,
		    completed_at = now()
		where id = $1 and checklist_id = $2 and status = $7`,
		evaluationID, checklistID, StatusSucceeded, score.SatisfiedPoints, score.TotalPossiblePoints, score.ChecklistPassRate, StatusRunning)
	if err != nil {
		return mapTerminalTriggerError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return tx.Commit(ctx)
}

func (s *Store) FailEvaluation(ctx context.Context, evaluationID, checklistID string, details failure.Details) error {
	if err := details.Validate(); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	status, err := lockStatus(ctx, tx, `select status from evaluations where id = $1 and checklist_id = $2 for update`, evaluationID, checklistID)
	if err != nil {
		return err
	}
	if status != StatusRunning {
		if err := tx.Rollback(ctx); err != nil {
			return err
		}
		return s.compareFailedEvaluation(ctx, evaluationID, checklistID, details)
	}
	if _, err := insertWorkflowFailure(ctx, tx, nil, &evaluationID, details); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		update evaluations
		set status = $3, completed_at = now()
		where id = $1 and checklist_id = $2 and status = $4`,
		evaluationID, checklistID, StatusFailed, StatusRunning)
	if err != nil {
		return mapTerminalTriggerError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return tx.Commit(ctx)
}

func (s *Store) GetEvaluation(ctx context.Context, evaluationID string) (Evaluation, error) {
	var evaluation Evaluation
	var completedAt sql.NullTime
	var satisfied sql.NullInt64
	var total sql.NullInt64
	var rate sql.NullFloat64
	err := s.pool.QueryRow(ctx, `
		select id::text, checklist_id::text, status, answer_artifact_key,
		       satisfied_points, total_possible_points, checklist_pass_rate,
		       created_at, completed_at
		from evaluations where id = $1`, evaluationID).
		Scan(&evaluation.ID, &evaluation.ChecklistID, &evaluation.Status, &evaluation.AnswerArtifactKey,
			&satisfied, &total, &rate, &evaluation.CreatedAt, &completedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Evaluation{}, ErrNotFound
	}
	if err != nil {
		return Evaluation{}, err
	}
	if satisfied.Valid {
		v := int(satisfied.Int64)
		evaluation.SatisfiedPoints = &v
	}
	if total.Valid {
		v := int(total.Int64)
		evaluation.TotalPossiblePoints = &v
	}
	if rate.Valid {
		evaluation.ChecklistPassRate = &rate.Float64
	}
	if completedAt.Valid {
		evaluation.CompletedAt = &completedAt.Time
	}
	if evaluation.Status == StatusFailed {
		evaluation.Failure, err = s.getWorkflowFailure(ctx, nil, &evaluationID)
		if err != nil {
			return Evaluation{}, err
		}
	}

	rows, err := s.pool.Query(ctx, `
		select run_index, question_id, evidence, answer
		from judgments
		where evaluation_id = $1
		order by run_index, question_id`, evaluationID)
	if err != nil {
		return Evaluation{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var judgment evalcore.RunJudgment
		if err := rows.Scan(&judgment.RunIndex, &judgment.QuestionID, &judgment.Evidence, &judgment.Answer); err != nil {
			return Evaluation{}, err
		}
		evaluation.RunJudgments = append(evaluation.RunJudgments, judgment)
	}
	if err := rows.Err(); err != nil {
		return Evaluation{}, err
	}
	if evaluation.FailedQuestionIDs == nil {
		evaluation.FailedQuestionIDs = []string{}
	}
	if evaluation.Status == StatusSucceeded {
		checklist, err := s.GetChecklist(ctx, evaluation.ChecklistID)
		if err != nil {
			return Evaluation{}, err
		}
		aggregated, err := evalcore.AggregateJudgments(checklist.Questions, evaluation.RunJudgments, checklist.EvaluationRuns)
		if err != nil {
			return Evaluation{}, err
		}
		evaluation.Judgments = aggregated.Judgments
		evaluation.FailedQuestionIDs = aggregated.Score.FailedQuestionIDs
	}
	return evaluation, nil
}

func insertWorkflowFailure(ctx context.Context, tx pgx.Tx, checklistID, evaluationID *string, details failure.Details) (failure.Record, error) {
	diagnosticValues := details.Diagnostics
	if diagnosticValues == nil {
		diagnosticValues = []evalcore.LimitDiagnostic{}
	}
	artifactReferenceValues := details.ArtifactReferences
	if artifactReferenceValues == nil {
		artifactReferenceValues = []string{}
	}
	diagnostics, err := json.Marshal(diagnosticValues)
	if err != nil {
		return failure.Record{}, err
	}
	artifactReferences, err := json.Marshal(artifactReferenceValues)
	if err != nil {
		return failure.Record{}, err
	}
	record := failure.Record{ChecklistID: checklistID, EvaluationID: evaluationID, Details: details}
	err = tx.QueryRow(ctx, `
		insert into workflow_failures (
			checklist_id, evaluation_id, workflow_id, stage, error_class, error_code,
			message, retryable, attempt_count, diagnostics, artifact_references
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		returning id::text, created_at`,
		checklistID, evaluationID, details.WorkflowID, details.Stage, details.ErrorClass, details.ErrorCode,
		details.Message, details.Retryable, details.AttemptCount, diagnostics, artifactReferences,
	).Scan(&record.ID, &record.CreatedAt)
	return record, err
}

func (s *Store) getWorkflowFailure(ctx context.Context, checklistID, evaluationID *string) (*failure.Record, error) {
	var record failure.Record
	var checklist sql.NullString
	var evaluation sql.NullString
	var diagnostics []byte
	var artifactReferences []byte
	err := s.pool.QueryRow(ctx, `
		select id::text, checklist_id::text, evaluation_id::text, workflow_id, stage,
		       error_class, error_code, message, retryable, attempt_count,
		       diagnostics, artifact_references, created_at
		from workflow_failures
		where ($1::uuid is not null and checklist_id = $1::uuid)
		   or ($2::uuid is not null and evaluation_id = $2::uuid)`, checklistID, evaluationID).
		Scan(&record.ID, &checklist, &evaluation, &record.WorkflowID, &record.Stage,
			&record.ErrorClass, &record.ErrorCode, &record.Message, &record.Retryable, &record.AttemptCount,
			&diagnostics, &artifactReferences, &record.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if checklist.Valid {
		record.ChecklistID = &checklist.String
	}
	if evaluation.Valid {
		record.EvaluationID = &evaluation.String
	}
	if err := json.Unmarshal(diagnostics, &record.Diagnostics); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(artifactReferences, &record.ArtifactReferences); err != nil {
		return nil, err
	}
	return &record, nil
}

func lockStatus(ctx context.Context, tx pgx.Tx, query string, args ...any) (string, error) {
	var status string
	err := tx.QueryRow(ctx, query, args...).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return status, err
}

func (s *Store) compareSucceededChecklist(ctx context.Context, checklistID string, dimensions []evalcore.Dimension, candidates []evalcore.CandidateQuestion, weights []evalcore.Weight, questions []evalcore.FinalQuestion) error {
	existing, err := s.GetChecklist(ctx, checklistID)
	if err != nil {
		return err
	}
	if existing.Status == StatusSucceeded &&
		slices.Equal(existing.Dimensions, dimensions) &&
		slices.Equal(existing.CandidateQuestions, candidates) &&
		slices.Equal(existing.Weights, weights) &&
		slices.Equal(existing.Questions, questions) {
		return nil
	}
	return ErrConflict
}

func (s *Store) compareFailedChecklist(ctx context.Context, checklistID string, details failure.Details) error {
	existing, err := s.GetChecklist(ctx, checklistID)
	if err != nil {
		return err
	}
	if existing.Status == StatusFailed && existing.Failure != nil && existing.Failure.Details.Equal(details) {
		return nil
	}
	return ErrConflict
}

func (s *Store) compareSucceededEvaluation(ctx context.Context, evaluationID, checklistID string, runJudgments []evalcore.RunJudgment, score evalcore.ScoreResult) error {
	existing, err := s.GetEvaluation(ctx, evaluationID)
	if err != nil {
		return err
	}
	if existing.ChecklistID == checklistID && existing.Status == StatusSucceeded &&
		runJudgmentsEqual(existing.RunJudgments, runJudgments) &&
		existing.SatisfiedPoints != nil && *existing.SatisfiedPoints == score.SatisfiedPoints &&
		existing.TotalPossiblePoints != nil && *existing.TotalPossiblePoints == score.TotalPossiblePoints &&
		existing.ChecklistPassRate != nil && *existing.ChecklistPassRate == score.ChecklistPassRate &&
		slices.Equal(existing.FailedQuestionIDs, score.FailedQuestionIDs) {
		return nil
	}
	return ErrConflict
}

func runJudgmentsEqual(a, b []evalcore.RunJudgment) bool {
	if len(a) != len(b) {
		return false
	}
	type key struct {
		runIndex   int
		questionID string
	}
	indexed := make(map[key]evalcore.RunJudgment, len(a))
	for _, judgment := range a {
		indexed[key{runIndex: judgment.RunIndex, questionID: judgment.QuestionID}] = judgment
	}
	for _, judgment := range b {
		if indexed[key{runIndex: judgment.RunIndex, questionID: judgment.QuestionID}] != judgment {
			return false
		}
	}
	return true
}

func (s *Store) compareFailedEvaluation(ctx context.Context, evaluationID, checklistID string, details failure.Details) error {
	existing, err := s.GetEvaluation(ctx, evaluationID)
	if err != nil {
		return err
	}
	if existing.ChecklistID == checklistID && existing.Status == StatusFailed && existing.Failure != nil && existing.Failure.Details.Equal(details) {
		return nil
	}
	return ErrConflict
}

func mapTerminalTriggerError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "P0001" {
		return ErrConflict
	}
	return err
}

func scoreResultsEqual(a, b evalcore.ScoreResult) bool {
	if a.SatisfiedPoints != b.SatisfiedPoints ||
		a.TotalPossiblePoints != b.TotalPossiblePoints ||
		a.ChecklistPassRate != b.ChecklistPassRate ||
		len(a.FailedQuestionIDs) != len(b.FailedQuestionIDs) {
		return false
	}
	for i := range a.FailedQuestionIDs {
		if a.FailedQuestionIDs[i] != b.FailedQuestionIDs[i] {
			return false
		}
	}
	return true
}
