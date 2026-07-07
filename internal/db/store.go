package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
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
	TaskArtifactKey    string
	ContextArtifactKey string
	ErrorMessage       *string
	CreatedAt          time.Time
	CompletedAt        *time.Time
	Questions          []evalcore.CandidateQuestion
	Weights            []evalcore.Weight
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
	ErrorMessage        *string
	CreatedAt           time.Time
	CompletedAt         *time.Time
	Judgments           []evalcore.Judgment
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
	files, err := migrationFiles(dir)
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, file := range files {
		sqlBytes, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply migration %s: %w", file, err)
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

func (s *Store) CreateChecklist(ctx context.Context, taskArtifactKey, contextArtifactKey string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		insert into checklists (status, task_artifact_key, context_artifact_key)
		values ($1, $2, $3)
		returning id::text`, StatusRunning, taskArtifactKey, contextArtifactKey).Scan(&id)
	return id, err
}

func (s *Store) CreateChecklistForWorkflow(ctx context.Context) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		with generated as (select gen_random_uuid() as id)
		insert into checklists (id, status, task_artifact_key, context_artifact_key)
		select id,
		       $1,
		       'checklists/' || id::text || '/inputs/task.txt',
		       'checklists/' || id::text || '/inputs/context.txt'
		from generated
		returning id::text`, StatusRunning).Scan(&id)
	return id, err
}

func (s *Store) SucceedChecklist(ctx context.Context, checklistID string, questions []evalcore.CandidateQuestion, weights []evalcore.Weight) error {
	if err := evalcore.ValidateWeights(questions, weights); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, question := range questions {
		if _, err := tx.Exec(ctx, `
			insert into questions (checklist_id, id, ordinal, rationale, question)
			values ($1, $2, $3, $4, $5)`,
			checklistID, question.ID, question.Ordinal, question.Rationale, question.Question); err != nil {
			return err
		}
	}
	for _, weight := range weights {
		if _, err := tx.Exec(ctx, `
			insert into weights (checklist_id, question_id, rationale, weight)
			values ($1, $2, $3, $4)`,
			checklistID, weight.QuestionID, weight.Rationale, weight.Weight); err != nil {
			return err
		}
	}
	tag, err := tx.Exec(ctx, `
		update checklists
		set status = $2, completed_at = now(), error_message = null
		where id = $1 and status = $3`, checklistID, StatusSucceeded, StatusRunning)
	if err != nil {
		return mapTerminalTriggerError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return tx.Commit(ctx)
}

func (s *Store) FailChecklist(ctx context.Context, checklistID, message string) error {
	tag, err := s.pool.Exec(ctx, `
		update checklists
		set status = $2, error_message = $3, completed_at = now()
		where id = $1 and status = $4`, checklistID, StatusFailed, message, StatusRunning)
	if err != nil {
		return mapTerminalTriggerError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}

func (s *Store) GetChecklist(ctx context.Context, checklistID string) (Checklist, error) {
	var checklist Checklist
	var errorMessage sql.NullString
	var completedAt sql.NullTime
	err := s.pool.QueryRow(ctx, `
		select id::text, status, task_artifact_key, context_artifact_key, error_message, created_at, completed_at
		from checklists where id = $1`, checklistID).
		Scan(&checklist.ID, &checklist.Status, &checklist.TaskArtifactKey, &checklist.ContextArtifactKey, &errorMessage, &checklist.CreatedAt, &completedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Checklist{}, ErrNotFound
	}
	if err != nil {
		return Checklist{}, err
	}
	if errorMessage.Valid {
		checklist.ErrorMessage = &errorMessage.String
	}
	if completedAt.Valid {
		checklist.CompletedAt = &completedAt.Time
	}

	rows, err := s.pool.Query(ctx, `
		select id, ordinal, rationale, question
		from questions where checklist_id = $1 order by ordinal`, checklistID)
	if err != nil {
		return Checklist{}, err
	}
	checklist.Questions, err = pgx.CollectRows(rows, pgx.RowToStructByName[evalcore.CandidateQuestion])
	if err != nil {
		return Checklist{}, err
	}

	weightRows, err := s.pool.Query(ctx, `
		select w.question_id, w.rationale, w.weight
		from weights w
		join questions q on q.checklist_id = w.checklist_id and q.id = w.question_id
		where w.checklist_id = $1
		order by q.ordinal`, checklistID)
	if err != nil {
		return Checklist{}, err
	}
	defer weightRows.Close()
	for weightRows.Next() {
		var weight evalcore.Weight
		if err := weightRows.Scan(&weight.QuestionID, &weight.Rationale, &weight.Weight); err != nil {
			return Checklist{}, err
		}
		checklist.Weights = append(checklist.Weights, weight)
	}
	if err := weightRows.Err(); err != nil {
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

func (s *Store) SucceedEvaluation(ctx context.Context, evaluationID, checklistID string, judgments []evalcore.Judgment, score evalcore.ScoreResult) error {
	checklist, err := s.GetChecklist(ctx, checklistID)
	if err != nil {
		return err
	}
	if err := evalcore.ValidateJudgments(checklist.Questions, checklist.Weights, judgments); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, judgment := range judgments {
		if _, err := tx.Exec(ctx, `
			insert into judgments (evaluation_id, checklist_id, question_id, evidence, answer)
			values ($1, $2, $3, $4, $5)`,
			evaluationID, checklistID, judgment.QuestionID, judgment.Evidence, judgment.Answer); err != nil {
			return err
		}
	}
	tag, err := tx.Exec(ctx, `
		update evaluations
		set status = $3,
		    satisfied_points = $4,
		    total_possible_points = $5,
		    checklist_pass_rate = $6,
		    error_message = null,
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

func (s *Store) FailEvaluation(ctx context.Context, evaluationID, checklistID, message string) error {
	tag, err := s.pool.Exec(ctx, `
		update evaluations
		set status = $3, error_message = $4, completed_at = now()
		where id = $1 and checklist_id = $2 and status = $5`,
		evaluationID, checklistID, StatusFailed, message, StatusRunning)
	if err != nil {
		return mapTerminalTriggerError(err)
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	return nil
}

func (s *Store) GetEvaluation(ctx context.Context, evaluationID string) (Evaluation, error) {
	var evaluation Evaluation
	var errorMessage sql.NullString
	var completedAt sql.NullTime
	var satisfied sql.NullInt64
	var total sql.NullInt64
	var rate sql.NullFloat64
	err := s.pool.QueryRow(ctx, `
		select id::text, checklist_id::text, status, answer_artifact_key,
		       satisfied_points, total_possible_points, checklist_pass_rate,
		       error_message, created_at, completed_at
		from evaluations where id = $1`, evaluationID).
		Scan(&evaluation.ID, &evaluation.ChecklistID, &evaluation.Status, &evaluation.AnswerArtifactKey,
			&satisfied, &total, &rate, &errorMessage, &evaluation.CreatedAt, &completedAt)
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
	if errorMessage.Valid {
		evaluation.ErrorMessage = &errorMessage.String
	}
	if completedAt.Valid {
		evaluation.CompletedAt = &completedAt.Time
	}

	rows, err := s.pool.Query(ctx, `
		select question_id, evidence, answer
		from judgments
		where evaluation_id = $1
		order by question_id`, evaluationID)
	if err != nil {
		return Evaluation{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var judgment evalcore.Judgment
		if err := rows.Scan(&judgment.QuestionID, &judgment.Evidence, &judgment.Answer); err != nil {
			return Evaluation{}, err
		}
		evaluation.Judgments = append(evaluation.Judgments, judgment)
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
		score, err := evalcore.ScoreChecklist(checklist.Questions, checklist.Weights, evaluation.Judgments)
		if err != nil {
			return Evaluation{}, err
		}
		evaluation.FailedQuestionIDs = score.FailedQuestionIDs
	}
	return evaluation, nil
}

func mapTerminalTriggerError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "P0001" {
		return ErrConflict
	}
	return err
}
