package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/kirilligum/self-imp-bin-eval/internal/artifacts"
	"github.com/kirilligum/self-imp-bin-eval/internal/db"
	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
	"github.com/kirilligum/self-imp-bin-eval/internal/failure"
	"github.com/kirilligum/self-imp-bin-eval/internal/llm"
	"go.temporal.io/sdk/temporal"
)

func TestLLMActivitiesWriteArtifactsAndPayloads(t *testing.T) {
	writer := &fakeArtifactWriter{objects: map[string][]byte{}}
	client := &fakeLLMClient{}
	acts := New(Dependencies{
		Artifacts:    writer,
		LLM:          client,
		ModelProfile: "checklist-evaluator",
	})
	limits := evalcore.DefaultChecklistLimits()
	ctx := context.Background()

	dResult, err := acts.AnalyzeDimensions(ctx, AnalyzeDimensionsInput{
		ChecklistID: "checklist-1",
		Task:        "task",
		Context:     "context",
		Limits:      limits,
	})
	if err != nil {
		t.Fatalf("AnalyzeDimensions() error = %v", err)
	}
	if len(dResult.Dimensions) != 1 || dResult.Dimensions[0].ID != "d1" {
		t.Fatalf("dimensions = %#v", dResult.Dimensions)
	}
	assertObjectWritten(t, writer, artifacts.ChecklistLLMRequestKey("checklist-1", artifacts.PromptDimensionAnalysis, 1))
	assertObjectWritten(t, writer, artifacts.ChecklistLLMResponseKey("checklist-1", artifacts.PromptDimensionAnalysis, 1))

	qResult, err := acts.GenerateQuestionsForDimension(ctx, GenerateQuestionsForDimensionInput{
		ChecklistID: "checklist-1",
		Task:        "task",
		Context:     "context",
		Dimension:   dResult.Dimensions[0],
		Limits:      limits,
	})
	if err != nil {
		t.Fatalf("GenerateQuestionsForDimension() error = %v", err)
	}
	candidates := evalcore.AssignCandidateQuestionIDs(dResult.Dimensions[0].ID, 1, qResult.Questions)
	if len(candidates) != 2 || candidates[0].ID != "c1" || candidates[0].DimensionID != "d1" {
		t.Fatalf("candidate questions = %#v", candidates)
	}
	assertObjectWritten(t, writer, artifacts.ChecklistLLMRequestKey("checklist-1", artifacts.PromptQuestionGeneration+"/d1", 1))
	assertObjectWritten(t, writer, artifacts.ChecklistLLMResponseKey("checklist-1", artifacts.PromptQuestionGeneration+"/d1", 1))

	wResult, err := acts.AssignWeights(ctx, AssignWeightsInput{
		ChecklistID:        "checklist-1",
		Task:               "task",
		Context:            "context",
		CandidateQuestions: candidates,
		Limits:             limits,
	})
	if err != nil {
		t.Fatalf("AssignWeights() error = %v", err)
	}
	assertObjectWritten(t, writer, artifacts.ChecklistLLMRequestKey("checklist-1", artifacts.PromptWeightAssignment, 1))
	assertObjectWritten(t, writer, artifacts.ChecklistLLMResponseKey("checklist-1", artifacts.PromptWeightAssignment, 1))

	sResult, err := acts.SplitQuestion(ctx, SplitQuestionInput{
		ChecklistID:       "checklist-1",
		Task:              "task",
		Context:           "context",
		CandidateQuestion: candidates[1],
		Weight:            wResult.Weights[1],
		Limits:            limits,
	})
	if err != nil {
		t.Fatalf("SplitQuestion() error = %v", err)
	}
	assertObjectWritten(t, writer, artifacts.ChecklistLLMRequestKey("checklist-1", artifacts.PromptQuestionSplitting+"/c2", 1))
	assertObjectWritten(t, writer, artifacts.ChecklistLLMResponseKey("checklist-1", artifacts.PromptQuestionSplitting+"/c2", 1))

	finalQuestions, err := evalcore.BuildFinalChecklist(dResult.Dimensions, candidates, wResult.Weights, []evalcore.SplitQuestions{sResult.Split}, limits)
	if err != nil {
		t.Fatalf("BuildFinalChecklist() error = %v", err)
	}
	jResult, err := acts.JudgeAnswer(ctx, JudgeAnswerInput{
		EvaluationID: "evaluation-1",
		RunIndex:     1,
		Task:         "task",
		Context:      "context",
		ModelAnswer:  "answer",
		Questions:    finalQuestions,
	})
	if err != nil {
		t.Fatalf("JudgeAnswer() error = %v", err)
	}
	if len(jResult.Judgments) != 2 || jResult.Judgments[0].QuestionID != "q1" || jResult.Judgments[1].QuestionID != "q2" {
		t.Fatalf("judgments = %#v", jResult.Judgments)
	}
	assertObjectWritten(t, writer, artifacts.EvaluationLLMRequestKey("evaluation-1", artifacts.PromptBinaryJudging+"/run-1", 1))
	assertObjectWritten(t, writer, artifacts.EvaluationLLMResponseKey("evaluation-1", artifacts.PromptBinaryJudging+"/run-1", 1))

	judgePayload := marshalString(t, client.requests[artifacts.PromptBinaryJudging])
	for _, forbidden := range []string{"weight", "rationale", "candidate", "Excluded?"} {
		if strings.Contains(judgePayload, forbidden) {
			t.Fatalf("judge payload contains %q: %s", forbidden, judgePayload)
		}
	}
	if !strings.Contains(judgePayload, "q1") || !strings.Contains(judgePayload, "Specific A?") || !strings.Contains(judgePayload, "Specific B?") {
		t.Fatalf("judge payload missing final questions: %s", judgePayload)
	}
}

func TestP06ExactLLMArtifacts(t *testing.T) {
	const rawSentinel = "RAW_INVALID_MODEL_OUTPUT"
	requestBody := []byte(`{"model":"exact","stream":true}`)
	responseBody := []byte("data: RAW_INVALID_MODEL_OUTPUT\n\n")
	writer := &fakeArtifactWriter{objects: map[string][]byte{}}
	client := &traceLLMClient{
		trace: llm.Trace{RequestBody: requestBody, ResponseBody: responseBody, HTTPStatus: http.StatusOK},
		err:   &llm.ModelOutputError{Message: "LLM returned invalid JSON content"},
	}
	acts := New(Dependencies{Artifacts: writer, LLM: client, ModelProfile: "model"})
	_, err := acts.AnalyzeDimensions(context.Background(), AnalyzeDimensionsInput{
		ChecklistID: "checklist-exact",
		Task:        "task",
		Context:     "context",
		Limits:      evalcore.DefaultChecklistLimits(),
	})
	assertTemporalApplicationError(t, err, ErrorClassModelOutputInvalid, true)
	requestKey := artifacts.ChecklistLLMRequestKey("checklist-exact", artifacts.PromptDimensionAnalysis, 1)
	responseKey := artifacts.ChecklistLLMResponseKey("checklist-exact", artifacts.PromptDimensionAnalysis, 1)
	if !bytes.Equal(writer.objects[requestKey], requestBody) || !bytes.Equal(writer.objects[responseKey], responseBody) {
		t.Fatalf("activity traces changed bytes: %#v", writer.objects)
	}
	var appErr *temporal.ApplicationError
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %T", err)
	}
	var details failure.Details
	if decodeErr := appErr.Details(&details); decodeErr != nil {
		t.Fatalf("decode failure details error = %v", decodeErr)
	}
	if !slices.Equal(details.ArtifactReferences, []string{requestKey, responseKey}) || strings.Contains(details.Message, rawSentinel) {
		t.Fatalf("failure details = %#v", details)
	}
}

type traceLLMClient struct {
	trace llm.Trace
	err   error
}

func (c *traceLLMClient) GenerateJSON(ctx context.Context, req llm.GenerateRequest, out any) (llm.Trace, error) {
	return c.trace, c.err
}

func TestLLMActivitiesFailureSideEffects(t *testing.T) {
	limits := evalcore.DefaultChecklistLimits()
	dimension := evalcore.Dimension{ID: "d1", Ordinal: 1, Name: "Correctness", Rubric: "Check correctness.", Rationale: "Core."}
	candidates := []evalcore.CandidateQuestion{
		{ID: "c1", DimensionID: "d1", Ordinal: 1, Rationale: "r1", Question: "Q1?"},
		{ID: "c2", DimensionID: "d1", Ordinal: 2, Rationale: "r2", Question: "Q2?"},
	}

	t.Run("request artifact failure is retryable after transport completes", func(t *testing.T) {
		requestKey := artifacts.ChecklistLLMRequestKey("checklist-1", artifacts.PromptDimensionAnalysis, 1)
		writer := &fakeArtifactWriter{objects: map[string][]byte{}, failWrites: map[string]error{requestKey: errors.New("garage unavailable")}}
		client := &fakeLLMClient{}
		acts := New(Dependencies{Artifacts: writer, LLM: client, ModelProfile: "model"})

		_, err := acts.AnalyzeDimensions(context.Background(), AnalyzeDimensionsInput{ChecklistID: "checklist-1", Task: "task", Context: "context", Limits: limits})
		assertTemporalApplicationError(t, err, ErrorClassInfraRetryable, false)
		if len(client.requests) != 1 {
			t.Fatalf("LLM calls = %#v, want completed transport", client.requests)
		}
		assertObjectWritten(t, writer, artifacts.ChecklistLLMResponseKey("checklist-1", artifacts.PromptDimensionAnalysis, 1))
	})

	t.Run("llm model output failure leaves only request artifact", func(t *testing.T) {
		writer := &fakeArtifactWriter{objects: map[string][]byte{}}
		client := &fakeLLMClient{err: &llm.ModelOutputError{Message: "invalid model output"}}
		acts := New(Dependencies{Artifacts: writer, LLM: client, ModelProfile: "model"})

		_, err := acts.AnalyzeDimensions(context.Background(), AnalyzeDimensionsInput{ChecklistID: "checklist-1", Task: "task", Context: "context", Limits: limits})
		assertTemporalApplicationError(t, err, ErrorClassModelOutputInvalid, true)
		assertObjectWritten(t, writer, artifacts.ChecklistLLMRequestKey("checklist-1", artifacts.PromptDimensionAnalysis, 1))
		assertObjectMissing(t, writer, artifacts.ChecklistLLMResponseKey("checklist-1", artifacts.PromptDimensionAnalysis, 1))
	})

	t.Run("semantic validation failure keeps llm response artifact", func(t *testing.T) {
		writer := &fakeArtifactWriter{objects: map[string][]byte{}}
		client := &fakeLLMClient{outputs: map[string]func(any){
			artifacts.PromptWeightAssignment: func(out any) {
				*out.(*llm.WeightAssignmentOutput) = llm.WeightAssignmentOutput{Weights: []evalcore.Weight{{CandidateQuestionID: "c1", Rationale: "missing c2", Weight: 1}}}
			},
		}}
		acts := New(Dependencies{Artifacts: writer, LLM: client, ModelProfile: "model"})

		_, err := acts.AssignWeights(context.Background(), AssignWeightsInput{ChecklistID: "checklist-1", Task: "task", Context: "context", CandidateQuestions: candidates, Limits: limits})
		assertTemporalApplicationError(t, err, ErrorClassModelOutputInvalid, true)
		assertObjectWritten(t, writer, artifacts.ChecklistLLMRequestKey("checklist-1", artifacts.PromptWeightAssignment, 1))
		assertObjectWritten(t, writer, artifacts.ChecklistLLMResponseKey("checklist-1", artifacts.PromptWeightAssignment, 1))
	})

	t.Run("response artifact failure is retryable", func(t *testing.T) {
		responseKey := artifacts.ChecklistLLMResponseKey("checklist-1", artifacts.PromptDimensionAnalysis, 1)
		writer := &fakeArtifactWriter{objects: map[string][]byte{}, failWrites: map[string]error{responseKey: errors.New("garage unavailable")}}
		client := &fakeLLMClient{}
		acts := New(Dependencies{Artifacts: writer, LLM: client, ModelProfile: "model"})

		_, err := acts.AnalyzeDimensions(context.Background(), AnalyzeDimensionsInput{ChecklistID: "checklist-1", Task: "task", Context: "context", Limits: limits})
		assertTemporalApplicationError(t, err, ErrorClassInfraRetryable, false)
		assertObjectWritten(t, writer, artifacts.ChecklistLLMRequestKey("checklist-1", artifacts.PromptDimensionAnalysis, 1))
		assertObjectMissing(t, writer, responseKey)
	})

	t.Run("invalid judgments keep response artifact and fail closed", func(t *testing.T) {
		writer := &fakeArtifactWriter{objects: map[string][]byte{}}
		client := &fakeLLMClient{outputs: map[string]func(any){
			artifacts.PromptBinaryJudging: func(out any) {
				*out.(*llm.BinaryJudgingOutput) = llm.BinaryJudgingOutput{Judgments: []evalcore.Judgment{{QuestionID: "q999", Evidence: "Unknown.", Answer: evalcore.AnswerYes}}}
			},
		}}
		acts := New(Dependencies{Artifacts: writer, LLM: client, ModelProfile: "model"})

		_, err := acts.JudgeAnswer(context.Background(), JudgeAnswerInput{
			EvaluationID: "evaluation-1",
			RunIndex:     1,
			Task:         "task",
			Context:      "context",
			ModelAnswer:  "answer",
			Questions:    []evalcore.FinalQuestion{{ID: "q1", Ordinal: 1, DimensionID: "d1", SourceCandidateID: "c1", Rationale: "r", Question: "Q1?"}},
		})
		assertTemporalApplicationError(t, err, ErrorClassModelOutputInvalid, true)
		assertObjectWritten(t, writer, artifacts.EvaluationLLMRequestKey("evaluation-1", artifacts.PromptBinaryJudging+"/run-1", 1))
		assertObjectWritten(t, writer, artifacts.EvaluationLLMResponseKey("evaluation-1", artifacts.PromptBinaryJudging+"/run-1", 1))
	})

	_ = dimension
}

func TestPostgresBackedActivities(t *testing.T) {
	t.Run("load running checklist does not read artifacts", func(t *testing.T) {
		writer := &fakeArtifactWriter{objects: map[string][]byte{}, failReads: map[string]error{"unexpected": errors.New("unexpected read")}}
		store := &fakeActivityStore{checklist: db.Checklist{ID: "checklist-1", Status: db.StatusRunning}}
		acts := New(Dependencies{Artifacts: writer, Store: store})

		result, err := acts.LoadChecklist(context.Background(), LoadChecklistInput{ChecklistID: "checklist-1"})
		if err != nil {
			t.Fatalf("LoadChecklist() error = %v", err)
		}
		if result.Checklist.Status != db.StatusRunning || result.Task != "" || result.Context != "" || len(writer.reads) != 0 {
			t.Fatalf("result=%#v reads=%#v", result, writer.reads)
		}
	})

	t.Run("load succeeded checklist reads task and context artifacts", func(t *testing.T) {
		checklist := db.Checklist{
			ID:                 "checklist-1",
			Status:             db.StatusSucceeded,
			TaskArtifactKey:    artifacts.ChecklistTaskKey("checklist-1"),
			ContextArtifactKey: artifacts.ChecklistContextKey("checklist-1"),
		}
		writer := &fakeArtifactWriter{objects: map[string][]byte{
			checklist.TaskArtifactKey:    []byte("task"),
			checklist.ContextArtifactKey: []byte("context"),
		}}
		acts := New(Dependencies{Artifacts: writer, Store: &fakeActivityStore{checklist: checklist}})

		result, err := acts.LoadChecklist(context.Background(), LoadChecklistInput{ChecklistID: "checklist-1"})
		if err != nil {
			t.Fatalf("LoadChecklist() error = %v", err)
		}
		if result.Task != "task" || result.Context != "context" {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("load succeeded checklist artifact failure is retryable", func(t *testing.T) {
		checklist := db.Checklist{
			ID:                 "checklist-1",
			Status:             db.StatusSucceeded,
			TaskArtifactKey:    artifacts.ChecklistTaskKey("checklist-1"),
			ContextArtifactKey: artifacts.ChecklistContextKey("checklist-1"),
		}
		writer := &fakeArtifactWriter{objects: map[string][]byte{}, failReads: map[string]error{checklist.TaskArtifactKey: errors.New("garage unavailable")}}
		acts := New(Dependencies{Artifacts: writer, Store: &fakeActivityStore{checklist: checklist}})

		_, err := acts.LoadChecklist(context.Background(), LoadChecklistInput{ChecklistID: "checklist-1"})
		assertTemporalApplicationError(t, err, ErrorClassInfraRetryable, false)
	})

	t.Run("terminal store semantic error is nonretryable", func(t *testing.T) {
		store := &fakeActivityStore{terminalErr: &evalcore.SemanticError{Code: evalcore.CodeInvalidFinalChecklist, Message: "bad final checklist"}}
		acts := New(Dependencies{Store: store})

		err := acts.SucceedChecklist(context.Background(), SucceedChecklistInput{ChecklistID: "checklist-1"})
		assertTemporalApplicationError(t, err, ErrorClassModelOutputInvalid, true)
	})
}

func assertObjectWritten(t *testing.T, writer *fakeArtifactWriter, key string) {
	t.Helper()
	if len(writer.objects[key]) == 0 {
		t.Fatalf("artifact %s was not written; keys=%#v", key, writer.objects)
	}
}

func assertObjectMissing(t *testing.T, writer *fakeArtifactWriter, key string) {
	t.Helper()
	if len(writer.objects[key]) != 0 {
		t.Fatalf("artifact %s was written unexpectedly", key)
	}
}

func assertTemporalApplicationError(t *testing.T, err error, wantType string, wantNonRetryable bool) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	var appErr *temporal.ApplicationError
	if !errors.As(err, &appErr) {
		t.Fatalf("error type = %T, want temporal application error: %v", err, err)
	}
	if appErr.Type() != wantType || appErr.NonRetryable() != wantNonRetryable {
		t.Fatalf("application error type=%s nonRetryable=%v, want %s/%v", appErr.Type(), appErr.NonRetryable(), wantType, wantNonRetryable)
	}
}

type fakeArtifactWriter struct {
	objects    map[string][]byte
	failWrites map[string]error
	failReads  map[string]error
	reads      []string
}

func (w *fakeArtifactWriter) Write(ctx context.Context, key string, payload []byte) error {
	if err := w.failWrites[key]; err != nil {
		return err
	}
	if w.objects == nil {
		w.objects = map[string][]byte{}
	}
	w.objects[key] = append([]byte(nil), payload...)
	return nil
}

func (w *fakeArtifactWriter) Read(ctx context.Context, key string) ([]byte, error) {
	w.reads = append(w.reads, key)
	if err := w.failReads[key]; err != nil {
		return nil, err
	}
	return append([]byte(nil), w.objects[key]...), nil
}

type fakeLLMClient struct {
	requests map[string]llm.GenerateRequest
	outputs  map[string]func(any)
	err      error
}

func (c *fakeLLMClient) GenerateJSON(ctx context.Context, req llm.GenerateRequest, out any) (llm.Trace, error) {
	if c.requests == nil {
		c.requests = map[string]llm.GenerateRequest{}
	}
	c.requests[req.PromptName] = req
	requestBody, _ := json.Marshal(req)
	trace := llm.Trace{RequestBody: requestBody}
	if c.err != nil {
		return trace, c.err
	}
	if output := c.outputs[req.PromptName]; output != nil {
		output(out)
		responseBody, _ := json.Marshal(out)
		trace.ResponseBody = responseBody
		trace.ResponseBytesRead = len(responseBody)
		trace.HTTPStatus = http.StatusOK
		return trace, nil
	}
	switch req.PromptName {
	case artifacts.PromptDimensionAnalysis:
		*out.(*llm.DimensionAnalysisOutput) = llm.DimensionAnalysisOutput{Dimensions: []evalcore.DraftDimension{
			{Name: "Correctness", Rubric: "Check correctness.", Rationale: "Core dimension."},
		}}
	case artifacts.PromptQuestionGeneration + "/d1":
		*out.(*llm.QuestionGenerationOutput) = llm.QuestionGenerationOutput{Questions: []evalcore.DraftQuestion{
			{Rationale: "excluded", Question: "Excluded?"},
			{Rationale: "split", Question: "Broad active?"},
		}}
	case artifacts.PromptWeightAssignment:
		*out.(*llm.WeightAssignmentOutput) = llm.WeightAssignmentOutput{Weights: []evalcore.Weight{
			{CandidateQuestionID: "c1", Rationale: "duplicate", Weight: 0},
			{CandidateQuestionID: "c2", Rationale: "important", Weight: 2},
		}}
	case artifacts.PromptQuestionSplitting + "/c2":
		*out.(*llm.QuestionSplittingOutput) = llm.QuestionSplittingOutput{Questions: []evalcore.DraftQuestion{
			{Rationale: "detail a", Question: "Specific A?"},
			{Rationale: "detail b", Question: "Specific B?"},
		}}
	case artifacts.PromptBinaryJudging:
		*out.(*llm.BinaryJudgingOutput) = llm.BinaryJudgingOutput{Judgments: []evalcore.Judgment{
			{QuestionID: "q1", Evidence: "The answer satisfies A.", Answer: evalcore.AnswerYes},
			{QuestionID: "q2", Evidence: "The answer satisfies B.", Answer: evalcore.AnswerYes},
		}}
	default:
		t := ctx.Value(testingKey{}).(*testing.T)
		t.Fatalf("unexpected prompt %s", req.PromptName)
	}
	responseBody, _ := json.Marshal(out)
	trace.ResponseBody = responseBody
	trace.ResponseBytesRead = len(responseBody)
	trace.HTTPStatus = http.StatusOK
	return trace, nil
}

type testingKey struct{}

type fakeActivityStore struct {
	checklist   db.Checklist
	getErr      error
	terminalErr error
}

func (s *fakeActivityStore) GetChecklist(ctx context.Context, checklistID string) (db.Checklist, error) {
	if s.getErr != nil {
		return db.Checklist{}, s.getErr
	}
	return s.checklist, nil
}

func (s *fakeActivityStore) SucceedChecklist(ctx context.Context, checklistID string, dimensions []evalcore.Dimension, candidates []evalcore.CandidateQuestion, weights []evalcore.Weight, questions []evalcore.FinalQuestion, limits evalcore.ChecklistLimits) error {
	return s.terminalErr
}

func (s *fakeActivityStore) FailChecklist(ctx context.Context, checklistID string, details failure.Details) error {
	return s.terminalErr
}

func (s *fakeActivityStore) SucceedEvaluation(ctx context.Context, evaluationID, checklistID string, runJudgments []evalcore.RunJudgment, score evalcore.ScoreResult) error {
	return s.terminalErr
}

func (s *fakeActivityStore) FailEvaluation(ctx context.Context, evaluationID, checklistID string, details failure.Details) error {
	return s.terminalErr
}

func marshalString(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return string(b)
}
