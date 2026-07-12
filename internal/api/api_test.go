package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kirilligum/self-imp-bin-eval/internal/db"
	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
)

// TEST-013
func TestAPIContracts(t *testing.T) {
	store := newFakeStore()
	starter := &fakeStarter{}
	router := NewRouter(Dependencies{Store: store, Starter: starter})

	t.Run("create checklist accepted", func(t *testing.T) {
		resp := request(t, router, http.MethodPost, "/checklists", `{"task":"task","context":"context"}`)
		assertStatus(t, resp, http.StatusAccepted)
		assertJSONFields(t, resp, map[string]any{"checklist_id": "checklist-1", "status": "running"})
		if starter.createdChecklistID != "checklist-1" || starter.createdTask != "task" || starter.createdContext != "context" {
			t.Fatalf("starter create = %#v", starter)
		}
	})

	t.Run("invalid checklist request", func(t *testing.T) {
		assertStatus(t, request(t, router, http.MethodPost, "/checklists", `{bad-json`), http.StatusBadRequest)
		assertStatus(t, request(t, router, http.MethodPost, "/checklists", `{"task":" ","context":"context"}`), http.StatusBadRequest)
	})

	t.Run("get checklist shapes", func(t *testing.T) {
		store.checklists["running"] = db.Checklist{ID: "running", Status: db.StatusRunning}
		assertJSONFields(t, request(t, router, http.MethodGet, "/checklists/running", ""), map[string]any{"checklist_id": "running", "status": "running"})

		store.checklists["failed"] = db.Checklist{ID: "failed", Status: db.StatusFailed, ErrorMessage: strPtr("bad")}
		assertJSONFields(t, request(t, router, http.MethodGet, "/checklists/failed", ""), map[string]any{"checklist_id": "failed", "status": "failed", "error_message": "bad"})

		store.checklists["succeeded"] = db.Checklist{
			ID:     "succeeded",
			Status: db.StatusSucceeded,
			Dimensions: []evalcore.Dimension{
				{ID: "d1", Ordinal: 1, Name: "Correctness", Rubric: "Check correctness.", Rationale: "Core."},
			},
			CandidateQuestions: []evalcore.CandidateQuestion{
				{ID: "c1", DimensionID: "d1", Ordinal: 1, Rationale: "duplicate", Question: "Excluded?"},
				{ID: "c2", DimensionID: "d1", Ordinal: 2, Rationale: "important", Question: "Active?"},
			},
			Weights: []evalcore.Weight{
				{CandidateQuestionID: "c1", Rationale: "duplicate", Weight: 0},
				{CandidateQuestionID: "c2", Rationale: "important", Weight: 1},
			},
			Questions: []evalcore.FinalQuestion{
				{ID: "q1", Ordinal: 1, DimensionID: "d1", SourceCandidateID: "c2", Rationale: "important", Question: "Active?"},
			},
		}
		resp := request(t, router, http.MethodGet, "/checklists/succeeded", "")
		assertStatus(t, resp, http.StatusOK)
		var got map[string]any
		decodeBody(t, resp, &got)
		if got["status"] != "succeeded" || len(got["dimensions"].([]any)) != 1 || len(got["candidate_questions"].([]any)) != 2 || len(got["questions"].([]any)) != 1 || len(got["weights"].([]any)) != 2 {
			t.Fatalf("unexpected checklist success body: %#v", got)
		}
	})

	t.Run("create evaluation accepted and conflicts", func(t *testing.T) {
		resp := request(t, router, http.MethodPost, "/evaluations", `{"checklist_id":"succeeded","model_answer":"answer"}`)
		assertStatus(t, resp, http.StatusAccepted)
		assertJSONFields(t, resp, map[string]any{"evaluation_id": "evaluation-1", "status": "running"})
		if starter.evaluatedID != "evaluation-1" || starter.evaluatedAnswer != "answer" {
			t.Fatalf("starter evaluation = %#v", starter)
		}

		store.createEvaluationErr = db.ErrConflict
		assertStatus(t, request(t, router, http.MethodPost, "/evaluations", `{"checklist_id":"running","model_answer":"answer"}`), http.StatusConflict)
		store.createEvaluationErr = nil
	})

	t.Run("get evaluation shapes", func(t *testing.T) {
		store.evaluations["running"] = db.Evaluation{ID: "running", Status: db.StatusRunning}
		assertJSONFields(t, request(t, router, http.MethodGet, "/evaluations/running", ""), map[string]any{"evaluation_id": "running", "status": "running"})

		store.evaluations["failed"] = db.Evaluation{ID: "failed", Status: db.StatusFailed, ErrorMessage: strPtr("bad")}
		assertJSONFields(t, request(t, router, http.MethodGet, "/evaluations/failed", ""), map[string]any{"evaluation_id": "failed", "status": "failed", "error_message": "bad"})

		satisfied, total, rate := 4, 6, float64(4)/float64(6)
		store.evaluations["succeeded"] = db.Evaluation{
			ID:                  "succeeded",
			Status:              db.StatusSucceeded,
			SatisfiedPoints:     &satisfied,
			TotalPossiblePoints: &total,
			ChecklistPassRate:   &rate,
			FailedQuestionIDs:   []string{"q3"},
			Judgments:           []evalcore.Judgment{{QuestionID: "q2", Evidence: "yes evidence", Answer: evalcore.AnswerYes}},
		}
		resp := request(t, router, http.MethodGet, "/evaluations/succeeded", "")
		assertStatus(t, resp, http.StatusOK)
		var got map[string]any
		decodeBody(t, resp, &got)
		if got["satisfied_points"].(float64) != 4 || got["total_possible_points"].(float64) != 6 || len(got["judgments"].([]any)) != 1 {
			t.Fatalf("unexpected evaluation success body: %#v", got)
		}
	})

	t.Run("not found and infrastructure errors", func(t *testing.T) {
		assertStatus(t, request(t, router, http.MethodGet, "/checklists/missing", ""), http.StatusNotFound)
		store.getChecklistErr = errors.New("database unavailable")
		assertStatus(t, request(t, router, http.MethodGet, "/checklists/running", ""), http.StatusInternalServerError)
		store.getChecklistErr = nil
	})
}

// TEST-013
func TestAPIRouteSurface(t *testing.T) {
	router := NewRouter(Dependencies{Store: newFakeStore(), Starter: &fakeStarter{}})
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPut, "/checklists/checklist-1"},
		{http.MethodDelete, "/checklists/checklist-1"},
		{http.MethodPut, "/evaluations/evaluation-1"},
		{http.MethodDelete, "/evaluations/evaluation-1"},
	} {
		resp := request(t, router, tc.method, tc.path, "")
		if resp.Code >= 200 && resp.Code < 300 {
			t.Fatalf("%s %s returned success %d", tc.method, tc.path, resp.Code)
		}
	}
}

func request(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
}

func assertStatus(t *testing.T, resp *httptest.ResponseRecorder, want int) {
	t.Helper()
	if resp.Code != want {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, want, resp.Body.String())
	}
}

func assertJSONFields(t *testing.T, resp *httptest.ResponseRecorder, want map[string]any) {
	t.Helper()
	var got map[string]any
	decodeBody(t, resp, &got)
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("field %s = %#v, want %#v; body=%#v", key, got[key], value, got)
		}
	}
}

func decodeBody(t *testing.T, resp *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(resp.Body.Bytes(), out); err != nil {
		t.Fatalf("decode response error = %v; body=%s", err, resp.Body.String())
	}
}

func strPtr(s string) *string { return &s }

type fakeStore struct {
	checklists          map[string]db.Checklist
	evaluations         map[string]db.Evaluation
	getChecklistErr     error
	createEvaluationErr error
}

func newFakeStore() *fakeStore {
	return &fakeStore{checklists: map[string]db.Checklist{}, evaluations: map[string]db.Evaluation{}}
}

func (s *fakeStore) CreateChecklistForWorkflow(ctx context.Context) (string, error) {
	s.checklists["checklist-1"] = db.Checklist{ID: "checklist-1", Status: db.StatusRunning, CreatedAt: time.Now()}
	return "checklist-1", nil
}

func (s *fakeStore) GetChecklist(ctx context.Context, id string) (db.Checklist, error) {
	if s.getChecklistErr != nil {
		return db.Checklist{}, s.getChecklistErr
	}
	checklist, ok := s.checklists[id]
	if !ok {
		return db.Checklist{}, db.ErrNotFound
	}
	return checklist, nil
}

func (s *fakeStore) CreateEvaluationForWorkflow(ctx context.Context, checklistID string) (string, error) {
	if s.createEvaluationErr != nil {
		return "", s.createEvaluationErr
	}
	s.evaluations["evaluation-1"] = db.Evaluation{ID: "evaluation-1", ChecklistID: checklistID, Status: db.StatusRunning}
	return "evaluation-1", nil
}

func (s *fakeStore) GetEvaluation(ctx context.Context, id string) (db.Evaluation, error) {
	evaluation, ok := s.evaluations[id]
	if !ok {
		return db.Evaluation{}, db.ErrNotFound
	}
	return evaluation, nil
}

type fakeStarter struct {
	createdChecklistID string
	createdTask        string
	createdContext     string
	evaluatedID        string
	evaluatedChecklist string
	evaluatedAnswer    string
}

func (s *fakeStarter) StartCreateChecklist(ctx context.Context, checklistID, task, contextText string) error {
	s.createdChecklistID = checklistID
	s.createdTask = task
	s.createdContext = contextText
	return nil
}

func (s *fakeStarter) StartEvaluateAnswer(ctx context.Context, evaluationID, checklistID, modelAnswer string) error {
	s.evaluatedID = evaluationID
	s.evaluatedChecklist = checklistID
	s.evaluatedAnswer = modelAnswer
	return nil
}
