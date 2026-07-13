package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/kirilligum/self-imp-bin-eval/internal/db"
	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
	"github.com/kirilligum/self-imp-bin-eval/internal/failure"
	"github.com/kirilligum/self-imp-bin-eval/internal/workflows"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
)

type Store interface {
	CreateChecklistForWorkflow(ctx context.Context, evaluationRuns int) (string, error)
	GetChecklist(ctx context.Context, id string) (db.Checklist, error)
	FailChecklist(ctx context.Context, checklistID string, details failure.Details) error
	CreateEvaluationForWorkflow(ctx context.Context, checklistID string) (string, error)
	GetEvaluation(ctx context.Context, id string) (db.Evaluation, error)
	FailEvaluation(ctx context.Context, evaluationID, checklistID string, details failure.Details) error
}

type Starter interface {
	StartCreateChecklist(ctx context.Context, checklistID, task, contextText string) error
	StartEvaluateAnswer(ctx context.Context, evaluationID, checklistID, modelAnswer string) error
}

type Dependencies struct {
	Store             Store
	Starter           Starter
	MaxEvaluationRuns int
}

type Router struct {
	store             Store
	starter           Starter
	maxEvaluationRuns int
}

func NewRouter(deps Dependencies) http.Handler {
	maxEvaluationRuns := deps.MaxEvaluationRuns
	if maxEvaluationRuns <= 0 {
		maxEvaluationRuns = evalcore.DefaultMaxEvaluationRuns
	}
	r := &Router{store: deps.Store, starter: deps.Starter, maxEvaluationRuns: maxEvaluationRuns}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /checklists", r.createChecklist)
	mux.HandleFunc("GET /checklists/{checklist_id}", r.getChecklist)
	mux.HandleFunc("POST /evaluations", r.createEvaluation)
	mux.HandleFunc("GET /evaluations/{evaluation_id}", r.getEvaluation)
	return mux
}

type TemporalStarter struct {
	Client          WorkflowClient
	TaskQueue       string
	ChecklistLimits evalcore.ChecklistLimits
}

type WorkflowClient interface {
	ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow any, args ...any) (client.WorkflowRun, error)
	DescribeWorkflowExecution(ctx context.Context, workflowID, runID string) (*workflowservice.DescribeWorkflowExecutionResponse, error)
}

type workflowStartError struct {
	definitive bool
	cause      error
}

func (e *workflowStartError) Error() string {
	if e.definitive {
		return "workflow did not start"
	}
	return "workflow start outcome is ambiguous"
}

func (e *workflowStartError) Unwrap() error { return e.cause }

func newDefinitiveWorkflowStartError(cause error) error {
	return &workflowStartError{definitive: true, cause: cause}
}

func IsDefinitiveWorkflowStartError(err error) bool {
	var startError *workflowStartError
	return errors.As(err, &startError) && startError.definitive
}

func (s TemporalStarter) StartCreateChecklist(ctx context.Context, checklistID, task, contextText string) error {
	workflowID := "checklist-" + checklistID
	return s.startWorkflow(ctx, workflowID, workflows.CreateChecklistWorkflow, workflows.CreateChecklistInput{
		ChecklistID: checklistID,
		Task:        task,
		Context:     contextText,
		Limits:      s.ChecklistLimits.WithDefaults(),
	})
}

func (s TemporalStarter) StartEvaluateAnswer(ctx context.Context, evaluationID, checklistID, modelAnswer string) error {
	workflowID := "evaluation-" + evaluationID
	return s.startWorkflow(ctx, workflowID, workflows.EvaluateAnswerWorkflow, workflows.EvaluateAnswerInput{
		EvaluationID: evaluationID,
		ChecklistID:  checklistID,
		ModelAnswer:  modelAnswer,
	})
}

func (s TemporalStarter) startWorkflow(ctx context.Context, workflowID string, workflowFn, input any) error {
	_, startErr := s.Client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:                       workflowID,
		TaskQueue:                s.TaskQueue,
		WorkflowIDReusePolicy:    enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		WorkflowIDConflictPolicy: enumspb.WORKFLOW_ID_CONFLICT_POLICY_USE_EXISTING,
	}, workflowFn, input)
	if startErr == nil {
		return nil
	}
	var alreadyStarted *serviceerror.WorkflowExecutionAlreadyStarted
	if errors.As(startErr, &alreadyStarted) {
		return nil
	}
	if described, describeErr := s.Client.DescribeWorkflowExecution(ctx, workflowID, ""); describeErr == nil && described != nil {
		return nil
	} else {
		var notFound *serviceerror.NotFound
		if errors.As(describeErr, &notFound) {
			return newDefinitiveWorkflowStartError(startErr)
		}
		return &workflowStartError{cause: fmt.Errorf("start error: %w; describe error: %v", startErr, describeErr)}
	}
}

type createChecklistRequest struct {
	Task           string `json:"task"`
	Context        string `json:"context"`
	EvaluationRuns *int   `json:"evaluation_runs"`
}

type createEvaluationRequest struct {
	ChecklistID string `json:"checklist_id"`
	ModelAnswer string `json:"model_answer"`
}

type acceptedChecklistResponse struct {
	ChecklistID    string `json:"checklist_id"`
	Status         string `json:"status"`
	EvaluationRuns int    `json:"evaluation_runs"`
}

type acceptedEvaluationResponse struct {
	EvaluationID string `json:"evaluation_id"`
	Status       string `json:"status"`
}

type checklistResponse struct {
	ChecklistID        string                       `json:"checklist_id"`
	Status             string                       `json:"status"`
	EvaluationRuns     int                          `json:"evaluation_runs,omitempty"`
	Failure            *failure.Record              `json:"failure,omitempty"`
	Dimensions         []evalcore.Dimension         `json:"dimensions,omitempty"`
	CandidateQuestions []evalcore.CandidateQuestion `json:"candidate_questions,omitempty"`
	Weights            []evalcore.Weight            `json:"weights,omitempty"`
	Questions          []evalcore.FinalQuestion     `json:"questions,omitempty"`
}

type evaluationResponse struct {
	EvaluationID        string                        `json:"evaluation_id"`
	ChecklistID         string                        `json:"checklist_id,omitempty"`
	Status              string                        `json:"status"`
	Failure             *failure.Record               `json:"failure,omitempty"`
	SatisfiedPoints     *int                          `json:"satisfied_points,omitempty"`
	TotalPossiblePoints *int                          `json:"total_possible_points,omitempty"`
	ChecklistPassRate   *float64                      `json:"checklist_pass_rate,omitempty"`
	FailedQuestionIDs   *[]string                     `json:"failed_question_ids,omitempty"`
	Judgments           []evalcore.AggregatedJudgment `json:"judgments,omitempty"`
}

func (r *Router) createChecklist(w http.ResponseWriter, req *http.Request) {
	var body createChecklistRequest
	if err := decodeRequest(req, &body); err != nil || strings.TrimSpace(body.Task) == "" || strings.TrimSpace(body.Context) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	evaluationRuns := evalcore.DefaultEvaluationRuns
	if body.EvaluationRuns != nil {
		evaluationRuns = *body.EvaluationRuns
	}
	if err := evalcore.ValidateEvaluationRuns(evaluationRuns, r.maxEvaluationRuns); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_evaluation_runs")
		return
	}
	id, err := r.store.CreateChecklistForWorkflow(req.Context(), evaluationRuns)
	if err != nil {
		writeError(w, statusFromError(err), "create_checklist_failed")
		return
	}
	if err := r.starter.StartCreateChecklist(req.Context(), id, body.Task, body.Context); err != nil {
		if persistErr := r.persistDefinitiveStartFailure(err, "checklist-"+id, func(details failure.Details) error {
			return r.store.FailChecklist(req.Context(), id, details)
		}); persistErr != nil {
			writeError(w, http.StatusInternalServerError, "persist_workflow_failure_failed")
			return
		}
		writeError(w, http.StatusInternalServerError, "start_workflow_failed")
		return
	}
	writeJSON(w, http.StatusAccepted, acceptedChecklistResponse{ChecklistID: id, Status: db.StatusRunning, EvaluationRuns: evaluationRuns})
}

func (r *Router) getChecklist(w http.ResponseWriter, req *http.Request) {
	checklist, err := r.store.GetChecklist(req.Context(), req.PathValue("checklist_id"))
	if err != nil {
		writeError(w, statusFromError(err), "get_checklist_failed")
		return
	}
	resp := checklistResponse{ChecklistID: checklist.ID, Status: checklist.Status, EvaluationRuns: checklist.EvaluationRuns}
	if checklist.Status == db.StatusFailed {
		resp.Failure = checklist.Failure
	}
	if checklist.Status == db.StatusSucceeded {
		resp.Dimensions = checklist.Dimensions
		resp.CandidateQuestions = checklist.CandidateQuestions
		resp.Weights = checklist.Weights
		resp.Questions = checklist.Questions
	}
	writeJSON(w, http.StatusOK, resp)
}

func (r *Router) createEvaluation(w http.ResponseWriter, req *http.Request) {
	var body createEvaluationRequest
	if err := decodeRequest(req, &body); err != nil || strings.TrimSpace(body.ChecklistID) == "" || strings.TrimSpace(body.ModelAnswer) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	id, err := r.store.CreateEvaluationForWorkflow(req.Context(), body.ChecklistID)
	if err != nil {
		writeError(w, statusFromError(err), "create_evaluation_failed")
		return
	}
	if err := r.starter.StartEvaluateAnswer(req.Context(), id, body.ChecklistID, body.ModelAnswer); err != nil {
		if persistErr := r.persistDefinitiveStartFailure(err, "evaluation-"+id, func(details failure.Details) error {
			return r.store.FailEvaluation(req.Context(), id, body.ChecklistID, details)
		}); persistErr != nil {
			writeError(w, http.StatusInternalServerError, "persist_workflow_failure_failed")
			return
		}
		writeError(w, http.StatusInternalServerError, "start_workflow_failed")
		return
	}
	writeJSON(w, http.StatusAccepted, acceptedEvaluationResponse{EvaluationID: id, Status: db.StatusRunning})
}

func (r *Router) persistDefinitiveStartFailure(startErr error, workflowID string, persist func(failure.Details) error) error {
	if !IsDefinitiveWorkflowStartError(startErr) {
		return nil
	}
	return persist(failure.Details{
		WorkflowID:   workflowID,
		Stage:        "start_workflow",
		ErrorClass:   failure.ClassInfraNonRetryable,
		ErrorCode:    "workflow_start_failed",
		Message:      "workflow did not start",
		AttemptCount: 1,
	})
}

func (r *Router) getEvaluation(w http.ResponseWriter, req *http.Request) {
	evaluation, err := r.store.GetEvaluation(req.Context(), req.PathValue("evaluation_id"))
	if err != nil {
		writeError(w, statusFromError(err), "get_evaluation_failed")
		return
	}
	resp := evaluationResponse{
		EvaluationID: evaluation.ID,
		ChecklistID:  evaluation.ChecklistID,
		Status:       evaluation.Status,
	}
	if evaluation.Status == db.StatusFailed {
		resp.Failure = evaluation.Failure
	}
	if evaluation.Status == db.StatusSucceeded {
		resp.SatisfiedPoints = evaluation.SatisfiedPoints
		resp.TotalPossiblePoints = evaluation.TotalPossiblePoints
		resp.ChecklistPassRate = evaluation.ChecklistPassRate
		failedQuestionIDs := evaluation.FailedQuestionIDs
		if failedQuestionIDs == nil {
			failedQuestionIDs = []string{}
		}
		resp.FailedQuestionIDs = &failedQuestionIDs
		resp.Judgments = evaluation.Judgments
	}
	writeJSON(w, http.StatusOK, resp)
}

func decodeRequest(req *http.Request, out any) error {
	decoder := json.NewDecoder(req.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func statusFromError(err error) int {
	switch {
	case errors.Is(err, db.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, db.ErrConflict):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
