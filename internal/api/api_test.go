package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kirilligum/self-imp-bin-eval/internal/db"
	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
	"github.com/kirilligum/self-imp-bin-eval/internal/failure"
	"go.temporal.io/api/serviceerror"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
)

func TestP06TemporalIdempotency(t *testing.T) {
	t.Run("already started and ambiguous committed starts are accepted", func(t *testing.T) {
		alreadyStarted := &fakeWorkflowClient{startErr: serviceerror.NewWorkflowExecutionAlreadyStarted("exists", "request", "run")}
		starter := TemporalStarter{Client: alreadyStarted, TaskQueue: "queue"}
		if err := starter.StartCreateChecklist(context.Background(), "checklist-1", "task", "context"); err != nil {
			t.Fatalf("already-started workflow error = %v", err)
		}
		if alreadyStarted.describeCalls != 0 {
			t.Fatalf("already-started describe calls = %d, want 0", alreadyStarted.describeCalls)
		}

		ambiguousCommitted := &fakeWorkflowClient{
			startErr:         context.DeadlineExceeded,
			describeResponse: &workflowservice.DescribeWorkflowExecutionResponse{},
		}
		starter.Client = ambiguousCommitted
		if err := starter.StartEvaluateAnswer(context.Background(), "evaluation-1", "checklist-1", "answer"); err != nil {
			t.Fatalf("committed ambiguous workflow error = %v", err)
		}
		if ambiguousCommitted.describedWorkflowID != "evaluation-evaluation-1" {
			t.Fatalf("described workflow ID = %q", ambiguousCommitted.describedWorkflowID)
		}
	})

	t.Run("not found is definitive and other describe errors remain ambiguous", func(t *testing.T) {
		definitive := &fakeWorkflowClient{startErr: context.DeadlineExceeded, describeErr: serviceerror.NewNotFound("missing")}
		starter := TemporalStarter{Client: definitive, TaskQueue: "queue"}
		err := starter.StartCreateChecklist(context.Background(), "checklist-2", "task", "context")
		if err == nil || !IsDefinitiveWorkflowStartError(err) {
			t.Fatalf("definitive start error = %T %v", err, err)
		}

		ambiguous := &fakeWorkflowClient{startErr: context.DeadlineExceeded, describeErr: errors.New("describe unavailable")}
		starter.Client = ambiguous
		err = starter.StartCreateChecklist(context.Background(), "checklist-3", "task", "context")
		if err == nil || IsDefinitiveWorkflowStartError(err) {
			t.Fatalf("ambiguous start error = %T %v", err, err)
		}
	})

	t.Run("API records a definitive non-start and propagates persistence failure", func(t *testing.T) {
		store := newFakeStore()
		starter := &fakeStarter{createErr: newDefinitiveWorkflowStartError(errors.New("RAW_START_ERROR"))}
		router := NewRouter(Dependencies{Store: store, Starter: starter})
		resp := request(t, router, http.MethodPost, "/checklists", `{"task":"task","context":"context"}`)
		assertStatus(t, resp, http.StatusInternalServerError)
		got := store.checklists["checklist-1"]
		if got.Status != db.StatusFailed || got.Failure == nil || got.Failure.ErrorCode != "workflow_start_failed" || strings.Contains(got.Failure.Message, "RAW_START_ERROR") {
			t.Fatalf("definitive start failure = %#v", got)
		}

		store = newFakeStore()
		store.failChecklistErr = errors.New("failure persistence unavailable")
		router = NewRouter(Dependencies{Store: store, Starter: starter})
		resp = request(t, router, http.MethodPost, "/checklists", `{"task":"task","context":"context"}`)
		assertStatus(t, resp, http.StatusInternalServerError)
		if store.failChecklistCalls != 1 {
			t.Fatalf("failure persistence calls = %d, want 1", store.failChecklistCalls)
		}
	})
}

func TestP06RepeatedEvaluation(t *testing.T) {
	store := newFakeStore()
	starter := &fakeStarter{}
	router := NewRouter(Dependencies{Store: store, Starter: starter, MaxEvaluationRuns: 5})

	resp := request(t, router, http.MethodPost, "/checklists", `{"task":"task","context":"context"}`)
	assertStatus(t, resp, http.StatusAccepted)
	if store.lastEvaluationRuns != 3 {
		t.Fatalf("default evaluation_runs = %d, want 3", store.lastEvaluationRuns)
	}
	resp = request(t, router, http.MethodPost, "/checklists", `{"task":"task","context":"context","evaluation_runs":5}`)
	assertStatus(t, resp, http.StatusAccepted)
	if store.lastEvaluationRuns != 5 {
		t.Fatalf("explicit evaluation_runs = %d, want 5", store.lastEvaluationRuns)
	}
	for _, body := range []string{
		`{"task":"task","context":"context","evaluation_runs":0}`,
		`{"task":"task","context":"context","evaluation_runs":2}`,
		`{"task":"task","context":"context","evaluation_runs":7}`,
		`{"task":"task","context":"context","evaluation_runs":-1}`,
	} {
		assertStatus(t, request(t, router, http.MethodPost, "/checklists", body), http.StatusBadRequest)
	}

	store.checklists["checklist-runs"] = db.Checklist{ID: "checklist-runs", Status: db.StatusSucceeded, EvaluationRuns: 3}
	resp = request(t, router, http.MethodGet, "/checklists/checklist-runs", "")
	assertJSONFields(t, resp, map[string]any{"evaluation_runs": float64(3)})

	store.evaluations["evaluation-runs"] = db.Evaluation{
		ID: "evaluation-runs", ChecklistID: "checklist-runs", Status: db.StatusSucceeded,
		Judgments: []evalcore.AggregatedJudgment{{
			QuestionID: "q1",
			Runs:       []evalcore.JudgmentRun{{RunIndex: 1, Evidence: "yes", Answer: evalcore.AnswerYes}, {RunIndex: 2, Evidence: "yes", Answer: evalcore.AnswerYes}, {RunIndex: 3, Evidence: "no", Answer: evalcore.AnswerNo}},
			Answer:     evalcore.AnswerYes,
		}},
	}
	resp = request(t, router, http.MethodGet, "/evaluations/evaluation-runs", "")
	var body map[string]any
	decodeBody(t, resp, &body)
	judgments := body["judgments"].([]any)
	judgment := judgments[0].(map[string]any)
	if judgment["answer"] != evalcore.AnswerYes || len(judgment["runs"].([]any)) != 3 {
		t.Fatalf("aggregated API judgment = %#v", judgment)
	}
}

type fakeWorkflowClient struct {
	startErr             error
	describeResponse     *workflowservice.DescribeWorkflowExecutionResponse
	describeErr          error
	describeCalls        int
	describedWorkflowID  string
	startWorkflowOptions client.StartWorkflowOptions
}

func (c *fakeWorkflowClient) ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow any, args ...any) (client.WorkflowRun, error) {
	c.startWorkflowOptions = options
	return nil, c.startErr
}

func (c *fakeWorkflowClient) DescribeWorkflowExecution(ctx context.Context, workflowID, runID string) (*workflowservice.DescribeWorkflowExecutionResponse, error) {
	c.describeCalls++
	c.describedWorkflowID = workflowID
	return c.describeResponse, c.describeErr
}

func TestP06StructuredWorkflowFailures(t *testing.T) {
	const rawSentinel = "RAW_PROVIDER_OUTPUT_MUST_NOT_LEAK"
	record := &failure.Record{
		ID: "failure-1",
		Details: failure.Details{
			WorkflowID:         "workflow-1",
			Stage:              "weight_assignment",
			ErrorClass:         failure.ClassModelOutputInvalid,
			ErrorCode:          string(evalcore.CodeInvalidFinalChecklist),
			Message:            "final question budget exceeded",
			AttemptCount:       3,
			Diagnostics:        []evalcore.LimitDiagnostic{{LimitName: "max_final_questions", ConfiguredLimit: 64, ObservedCount: 65}},
			ArtifactReferences: []string{"checklists/checklist-failed/llm/weight_assignment/attempt-3/response.body"},
		},
		CreatedAt: time.Unix(1_700_000_000, 0).UTC(),
	}
	store := newFakeStore()
	store.checklists["checklist-failed"] = db.Checklist{ID: "checklist-failed", Status: db.StatusFailed, Failure: record}
	store.evaluations["evaluation-failed"] = db.Evaluation{ID: "evaluation-failed", ChecklistID: "checklist-failed", Status: db.StatusFailed, Failure: record}
	router := NewRouter(Dependencies{Store: store, Starter: &fakeStarter{}})

	for _, path := range []string{"/checklists/checklist-failed", "/evaluations/evaluation-failed"} {
		resp := request(t, router, http.MethodGet, path, "")
		assertStatus(t, resp, http.StatusOK)
		body := resp.Body.String()
		if strings.Contains(body, rawSentinel) || strings.Contains(body, "error_message") {
			t.Fatalf("unsafe or legacy failure field in %s response: %s", path, body)
		}
		var got map[string]any
		decodeBody(t, resp, &got)
		projected, ok := got["failure"].(map[string]any)
		if !ok || projected["id"] != "failure-1" || projected["workflow_id"] != "workflow-1" || projected["attempt_count"] != float64(3) {
			t.Fatalf("structured failure in %s response = %#v", path, got["failure"])
		}
		if _, exists := projected["checklist_id"]; exists {
			t.Fatalf("internal entity foreign key exposed in %s response: %#v", path, projected)
		}
	}
}

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
		assertStatus(t, request(t, router, http.MethodPost, "/checklists", `{"task":"task","context":"context","extra":true}`), http.StatusBadRequest)
		assertStatus(t, request(t, router, http.MethodPost, "/checklists", `{"task":" ","context":"context"}`), http.StatusBadRequest)
		assertStatus(t, request(t, router, http.MethodPost, "/checklists", `{"task":"task","context":" "}`), http.StatusBadRequest)
	})

	t.Run("create checklist store and starter failures", func(t *testing.T) {
		store.createChecklistErr = errors.New("database unavailable")
		assertStatus(t, request(t, router, http.MethodPost, "/checklists", `{"task":"task","context":"context"}`), http.StatusInternalServerError)
		store.createChecklistErr = nil

		starter.createErr = errors.New("temporal unavailable")
		assertStatus(t, request(t, router, http.MethodPost, "/checklists", `{"task":"task","context":"context"}`), http.StatusInternalServerError)
		starter.createErr = nil
	})

	t.Run("get checklist shapes", func(t *testing.T) {
		store.checklists["running"] = db.Checklist{ID: "running", Status: db.StatusRunning}
		resp := request(t, router, http.MethodGet, "/checklists/running", "")
		assertJSONFields(t, resp, map[string]any{"checklist_id": "running", "status": "running"})
		assertJSONAbsent(t, resp, "error_message", "questions", "weights")

		store.checklists["failed"] = db.Checklist{ID: "failed", Status: db.StatusFailed, Failure: &failure.Record{ID: "failure", Details: failure.Details{Message: "bad"}}}
		resp = request(t, router, http.MethodGet, "/checklists/failed", "")
		assertJSONFields(t, resp, map[string]any{"checklist_id": "failed", "status": "failed"})
		assertJSONPresent(t, resp, "failure")
		assertJSONAbsent(t, resp, "questions", "weights")

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
		resp = request(t, router, http.MethodGet, "/checklists/succeeded", "")
		assertStatus(t, resp, http.StatusOK)
		var got map[string]any
		decodeBody(t, resp, &got)
		if got["status"] != "succeeded" || len(got["dimensions"].([]any)) != 1 || len(got["candidate_questions"].([]any)) != 2 || len(got["questions"].([]any)) != 1 || len(got["weights"].([]any)) != 2 {
			t.Fatalf("unexpected checklist success body: %#v", got)
		}
		if _, exists := got["error_message"]; exists {
			t.Fatalf("succeeded checklist included error_message: %#v", got)
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

	t.Run("invalid evaluation request and start failures", func(t *testing.T) {
		assertStatus(t, request(t, router, http.MethodPost, "/evaluations", `{bad-json`), http.StatusBadRequest)
		assertStatus(t, request(t, router, http.MethodPost, "/evaluations", `{"checklist_id":"succeeded","model_answer":"answer","extra":true}`), http.StatusBadRequest)
		assertStatus(t, request(t, router, http.MethodPost, "/evaluations", `{"checklist_id":" ","model_answer":"answer"}`), http.StatusBadRequest)
		assertStatus(t, request(t, router, http.MethodPost, "/evaluations", `{"checklist_id":"succeeded","model_answer":" "}`), http.StatusBadRequest)

		store.createEvaluationErr = db.ErrNotFound
		assertStatus(t, request(t, router, http.MethodPost, "/evaluations", `{"checklist_id":"missing","model_answer":"answer"}`), http.StatusNotFound)
		store.createEvaluationErr = nil

		starter.evaluateErr = errors.New("temporal unavailable")
		assertStatus(t, request(t, router, http.MethodPost, "/evaluations", `{"checklist_id":"succeeded","model_answer":"answer"}`), http.StatusInternalServerError)
		starter.evaluateErr = nil
	})

	t.Run("get evaluation shapes", func(t *testing.T) {
		store.evaluations["running"] = db.Evaluation{ID: "running", Status: db.StatusRunning}
		resp := request(t, router, http.MethodGet, "/evaluations/running", "")
		assertJSONFields(t, resp, map[string]any{"evaluation_id": "running", "status": "running"})
		assertJSONAbsent(t, resp, "error_message", "satisfied_points", "judgments")

		store.evaluations["failed"] = db.Evaluation{ID: "failed", Status: db.StatusFailed, Failure: &failure.Record{ID: "failure", Details: failure.Details{Message: "bad"}}}
		resp = request(t, router, http.MethodGet, "/evaluations/failed", "")
		assertJSONFields(t, resp, map[string]any{"evaluation_id": "failed", "status": "failed"})
		assertJSONPresent(t, resp, "failure")
		assertJSONAbsent(t, resp, "satisfied_points", "judgments")

		satisfied, total, rate := 4, 6, float64(4)/float64(6)
		store.evaluations["succeeded"] = db.Evaluation{
			ID:                  "succeeded",
			Status:              db.StatusSucceeded,
			SatisfiedPoints:     &satisfied,
			TotalPossiblePoints: &total,
			ChecklistPassRate:   &rate,
			FailedQuestionIDs:   []string{"q3"},
			Judgments: []evalcore.AggregatedJudgment{{
				QuestionID: "q2",
				Runs:       []evalcore.JudgmentRun{{RunIndex: 1, Evidence: "yes evidence", Answer: evalcore.AnswerYes}},
				Answer:     evalcore.AnswerYes,
			}},
		}
		resp = request(t, router, http.MethodGet, "/evaluations/succeeded", "")
		assertStatus(t, resp, http.StatusOK)
		var got map[string]any
		decodeBody(t, resp, &got)
		if got["satisfied_points"].(float64) != 4 || got["total_possible_points"].(float64) != 6 || len(got["judgments"].([]any)) != 1 {
			t.Fatalf("unexpected evaluation success body: %#v", got)
		}

		store.evaluations["all-yes"] = db.Evaluation{
			ID:                  "all-yes",
			Status:              db.StatusSucceeded,
			SatisfiedPoints:     &satisfied,
			TotalPossiblePoints: &total,
			ChecklistPassRate:   &rate,
			FailedQuestionIDs:   nil,
		}
		resp = request(t, router, http.MethodGet, "/evaluations/all-yes", "")
		assertStatus(t, resp, http.StatusOK)
		decodeBody(t, resp, &got)
		if failed, ok := got["failed_question_ids"].([]any); !ok || len(failed) != 0 {
			t.Fatalf("failed_question_ids = %#v, want empty array", got["failed_question_ids"])
		}
	})

	t.Run("not found and infrastructure errors", func(t *testing.T) {
		assertStatus(t, request(t, router, http.MethodGet, "/checklists/missing", ""), http.StatusNotFound)
		store.getChecklistErr = errors.New("database unavailable")
		assertStatus(t, request(t, router, http.MethodGet, "/checklists/running", ""), http.StatusInternalServerError)
		store.getChecklistErr = nil

		assertStatus(t, request(t, router, http.MethodGet, "/evaluations/missing", ""), http.StatusNotFound)
		store.getEvaluationErr = errors.New("database unavailable")
		assertStatus(t, request(t, router, http.MethodGet, "/evaluations/running", ""), http.StatusInternalServerError)
		store.getEvaluationErr = nil
	})
}

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

func assertJSONAbsent(t *testing.T, resp *httptest.ResponseRecorder, keys ...string) {
	t.Helper()
	var got map[string]any
	decodeBody(t, resp, &got)
	for _, key := range keys {
		if _, exists := got[key]; exists {
			t.Fatalf("field %s unexpectedly present in %#v", key, got)
		}
	}
}

func assertJSONPresent(t *testing.T, resp *httptest.ResponseRecorder, keys ...string) {
	t.Helper()
	var got map[string]any
	decodeBody(t, resp, &got)
	for _, key := range keys {
		if _, exists := got[key]; !exists {
			t.Fatalf("field %s unexpectedly absent from %#v", key, got)
		}
	}
}

func decodeBody(t *testing.T, resp *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(resp.Body.Bytes(), out); err != nil {
		t.Fatalf("decode response error = %v; body=%s", err, resp.Body.String())
	}
}

type fakeStore struct {
	checklists          map[string]db.Checklist
	evaluations         map[string]db.Evaluation
	createChecklistErr  error
	getChecklistErr     error
	createEvaluationErr error
	getEvaluationErr    error
	failChecklistErr    error
	failEvaluationErr   error
	failChecklistCalls  int
	failEvaluationCalls int
	lastEvaluationRuns  int
}

func newFakeStore() *fakeStore {
	return &fakeStore{checklists: map[string]db.Checklist{}, evaluations: map[string]db.Evaluation{}}
}

func (s *fakeStore) CreateChecklistForWorkflow(ctx context.Context, evaluationRuns int) (string, error) {
	if s.createChecklistErr != nil {
		return "", s.createChecklistErr
	}
	s.lastEvaluationRuns = evaluationRuns
	s.checklists["checklist-1"] = db.Checklist{ID: "checklist-1", Status: db.StatusRunning, EvaluationRuns: evaluationRuns, CreatedAt: time.Now()}
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
	if s.getEvaluationErr != nil {
		return db.Evaluation{}, s.getEvaluationErr
	}
	evaluation, ok := s.evaluations[id]
	if !ok {
		return db.Evaluation{}, db.ErrNotFound
	}
	return evaluation, nil
}

func (s *fakeStore) FailChecklist(ctx context.Context, checklistID string, details failure.Details) error {
	s.failChecklistCalls++
	if s.failChecklistErr != nil {
		return s.failChecklistErr
	}
	checklist := s.checklists[checklistID]
	checklist.Status = db.StatusFailed
	checklist.Failure = &failure.Record{ID: "failure-" + checklistID, ChecklistID: &checklistID, Details: details, CreatedAt: time.Now()}
	s.checklists[checklistID] = checklist
	return nil
}

func (s *fakeStore) FailEvaluation(ctx context.Context, evaluationID, checklistID string, details failure.Details) error {
	s.failEvaluationCalls++
	if s.failEvaluationErr != nil {
		return s.failEvaluationErr
	}
	evaluation := s.evaluations[evaluationID]
	evaluation.Status = db.StatusFailed
	evaluation.Failure = &failure.Record{ID: "failure-" + evaluationID, EvaluationID: &evaluationID, Details: details, CreatedAt: time.Now()}
	s.evaluations[evaluationID] = evaluation
	return nil
}

type fakeStarter struct {
	createdChecklistID string
	createdTask        string
	createdContext     string
	evaluatedID        string
	evaluatedChecklist string
	evaluatedAnswer    string
	createErr          error
	evaluateErr        error
}

func (s *fakeStarter) StartCreateChecklist(ctx context.Context, checklistID, task, contextText string) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.createdChecklistID = checklistID
	s.createdTask = task
	s.createdContext = contextText
	return nil
}

func (s *fakeStarter) StartEvaluateAnswer(ctx context.Context, evaluationID, checklistID, modelAnswer string) error {
	if s.evaluateErr != nil {
		return s.evaluateErr
	}
	s.evaluatedID = evaluationID
	s.evaluatedChecklist = checklistID
	s.evaluatedAnswer = modelAnswer
	return nil
}
