package activities

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kirilligum/self-imp-bin-eval/internal/artifacts"
	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
	"github.com/kirilligum/self-imp-bin-eval/internal/llm"
)

// TEST-008
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
	assertObjectWritten(t, writer, artifacts.ChecklistLLMRequestKey("checklist-1", artifacts.PromptDimensionAnalysis))
	assertObjectWritten(t, writer, artifacts.ChecklistLLMResponseKey("checklist-1", artifacts.PromptDimensionAnalysis))

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
	assertObjectWritten(t, writer, artifacts.ChecklistDimensionQuestionGenerationRequestKey("checklist-1", "d1"))
	assertObjectWritten(t, writer, artifacts.ChecklistDimensionQuestionGenerationResponseKey("checklist-1", "d1"))

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
	assertObjectWritten(t, writer, artifacts.ChecklistLLMRequestKey("checklist-1", artifacts.PromptWeightAssignment))
	assertObjectWritten(t, writer, artifacts.ChecklistLLMResponseKey("checklist-1", artifacts.PromptWeightAssignment))

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
	assertObjectWritten(t, writer, artifacts.ChecklistQuestionSplittingRequestKey("checklist-1", "c2"))
	assertObjectWritten(t, writer, artifacts.ChecklistQuestionSplittingResponseKey("checklist-1", "c2"))

	finalQuestions, err := evalcore.BuildFinalChecklist(dResult.Dimensions, candidates, wResult.Weights, []evalcore.SplitQuestions{sResult.Split}, limits)
	if err != nil {
		t.Fatalf("BuildFinalChecklist() error = %v", err)
	}
	jResult, err := acts.JudgeAnswer(ctx, JudgeAnswerInput{
		EvaluationID: "evaluation-1",
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
	assertObjectWritten(t, writer, artifacts.EvaluationLLMRequestKey("evaluation-1", artifacts.PromptBinaryJudging))
	assertObjectWritten(t, writer, artifacts.EvaluationLLMResponseKey("evaluation-1", artifacts.PromptBinaryJudging))

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

func assertObjectWritten(t *testing.T, writer *fakeArtifactWriter, key string) {
	t.Helper()
	if len(writer.objects[key]) == 0 {
		t.Fatalf("artifact %s was not written; keys=%#v", key, writer.objects)
	}
}

type fakeArtifactWriter struct {
	objects map[string][]byte
}

func (w *fakeArtifactWriter) Write(ctx context.Context, key string, payload []byte) error {
	w.objects[key] = append([]byte(nil), payload...)
	return nil
}

func (w *fakeArtifactWriter) Read(ctx context.Context, key string) ([]byte, error) {
	return append([]byte(nil), w.objects[key]...), nil
}

type fakeLLMClient struct {
	requests map[string]llm.GenerateRequest
}

func (c *fakeLLMClient) GenerateJSON(ctx context.Context, req llm.GenerateRequest, out any) error {
	if c.requests == nil {
		c.requests = map[string]llm.GenerateRequest{}
	}
	c.requests[req.PromptName] = req
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
	return nil
}

type testingKey struct{}

func marshalString(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return string(b)
}
