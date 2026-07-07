package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/kirilligum/self-imp-bin-eval/internal/db"
	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
	"github.com/kirilligum/self-imp-bin-eval/internal/workflows"
	"go.temporal.io/sdk/client"
)

type Store interface {
	CreateChecklistForWorkflow(ctx context.Context) (string, error)
	GetChecklist(ctx context.Context, id string) (db.Checklist, error)
	CreateEvaluationForWorkflow(ctx context.Context, checklistID string) (string, error)
	GetEvaluation(ctx context.Context, id string) (db.Evaluation, error)
}

type Starter interface {
	StartCreateChecklist(ctx context.Context, checklistID, task, contextText string) error
	StartEvaluateAnswer(ctx context.Context, evaluationID, checklistID, modelAnswer string) error
}

type Dependencies struct {
	Store   Store
	Starter Starter
}

type Router struct {
	store   Store
	starter Starter
}

func NewRouter(deps Dependencies) http.Handler {
	r := &Router{store: deps.Store, starter: deps.Starter}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /checklists", r.createChecklist)
	mux.HandleFunc("GET /checklists/{checklist_id}", r.getChecklist)
	mux.HandleFunc("POST /evaluations", r.createEvaluation)
	mux.HandleFunc("GET /evaluations/{evaluation_id}", r.getEvaluation)
	return mux
}

type TemporalStarter struct {
	Client    client.Client
	TaskQueue string
}

func (s TemporalStarter) StartCreateChecklist(ctx context.Context, checklistID, task, contextText string) error {
	_, err := s.Client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        "checklist-" + checklistID,
		TaskQueue: s.TaskQueue,
	}, workflows.CreateChecklistWorkflow, workflows.CreateChecklistInput{
		ChecklistID: checklistID,
		Task:        task,
		Context:     contextText,
	})
	return err
}

func (s TemporalStarter) StartEvaluateAnswer(ctx context.Context, evaluationID, checklistID, modelAnswer string) error {
	_, err := s.Client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        "evaluation-" + evaluationID,
		TaskQueue: s.TaskQueue,
	}, workflows.EvaluateAnswerWorkflow, workflows.EvaluateAnswerInput{
		EvaluationID: evaluationID,
		ChecklistID:  checklistID,
		ModelAnswer:  modelAnswer,
	})
	return err
}

type createChecklistRequest struct {
	Task    string `json:"task"`
	Context string `json:"context"`
}

type createEvaluationRequest struct {
	ChecklistID string `json:"checklist_id"`
	ModelAnswer string `json:"model_answer"`
}

type acceptedChecklistResponse struct {
	ChecklistID string `json:"checklist_id"`
	Status      string `json:"status"`
}

type acceptedEvaluationResponse struct {
	EvaluationID string `json:"evaluation_id"`
	Status       string `json:"status"`
}

type checklistResponse struct {
	ChecklistID  string                       `json:"checklist_id"`
	Status       string                       `json:"status"`
	ErrorMessage *string                      `json:"error_message,omitempty"`
	Questions    []evalcore.CandidateQuestion `json:"questions,omitempty"`
	Weights      []evalcore.Weight            `json:"weights,omitempty"`
}

type evaluationResponse struct {
	EvaluationID        string              `json:"evaluation_id"`
	ChecklistID         string              `json:"checklist_id,omitempty"`
	Status              string              `json:"status"`
	ErrorMessage        *string             `json:"error_message,omitempty"`
	SatisfiedPoints     *int                `json:"satisfied_points,omitempty"`
	TotalPossiblePoints *int                `json:"total_possible_points,omitempty"`
	ChecklistPassRate   *float64            `json:"checklist_pass_rate,omitempty"`
	FailedQuestionIDs   *[]string           `json:"failed_question_ids,omitempty"`
	Judgments           []evalcore.Judgment `json:"judgments,omitempty"`
}

func (r *Router) createChecklist(w http.ResponseWriter, req *http.Request) {
	var body createChecklistRequest
	if err := decodeRequest(req, &body); err != nil || strings.TrimSpace(body.Task) == "" || strings.TrimSpace(body.Context) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	id, err := r.store.CreateChecklistForWorkflow(req.Context())
	if err != nil {
		writeError(w, statusFromError(err), "create_checklist_failed")
		return
	}
	if err := r.starter.StartCreateChecklist(req.Context(), id, body.Task, body.Context); err != nil {
		writeError(w, http.StatusInternalServerError, "start_workflow_failed")
		return
	}
	writeJSON(w, http.StatusAccepted, acceptedChecklistResponse{ChecklistID: id, Status: db.StatusRunning})
}

func (r *Router) getChecklist(w http.ResponseWriter, req *http.Request) {
	checklist, err := r.store.GetChecklist(req.Context(), req.PathValue("checklist_id"))
	if err != nil {
		writeError(w, statusFromError(err), "get_checklist_failed")
		return
	}
	resp := checklistResponse{ChecklistID: checklist.ID, Status: checklist.Status}
	if checklist.Status == db.StatusFailed {
		resp.ErrorMessage = checklist.ErrorMessage
	}
	if checklist.Status == db.StatusSucceeded {
		resp.Questions = checklist.Questions
		resp.Weights = checklist.Weights
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
		writeError(w, http.StatusInternalServerError, "start_workflow_failed")
		return
	}
	writeJSON(w, http.StatusAccepted, acceptedEvaluationResponse{EvaluationID: id, Status: db.StatusRunning})
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
		resp.ErrorMessage = evaluation.ErrorMessage
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
