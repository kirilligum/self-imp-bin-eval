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

	ctx := context.Background()
	qResult, err := acts.GenerateQuestions(ctx, GenerateQuestionsInput{
		ChecklistID: "checklist-1",
		Task:        "task",
		Context:     "context",
	})
	if err != nil {
		t.Fatalf("GenerateQuestions() error = %v", err)
	}
	if len(qResult.Questions) != 2 || qResult.Questions[0].ID != "q1" {
		t.Fatalf("questions = %#v", qResult.Questions)
	}
	assertObjectWritten(t, writer, artifacts.ChecklistLLMRequestKey("checklist-1", artifacts.PromptQuestionGeneration))
	assertObjectWritten(t, writer, artifacts.ChecklistLLMResponseKey("checklist-1", artifacts.PromptQuestionGeneration))

	wResult, err := acts.AssignWeights(ctx, AssignWeightsInput{
		ChecklistID: "checklist-1",
		Task:        "task",
		Context:     "context",
		Questions:   qResult.Questions,
	})
	if err != nil {
		t.Fatalf("AssignWeights() error = %v", err)
	}
	assertObjectWritten(t, writer, artifacts.ChecklistLLMRequestKey("checklist-1", artifacts.PromptWeightAssignment))
	assertObjectWritten(t, writer, artifacts.ChecklistLLMResponseKey("checklist-1", artifacts.PromptWeightAssignment))

	jResult, err := acts.JudgeAnswer(ctx, JudgeAnswerInput{
		EvaluationID: "evaluation-1",
		Task:         "task",
		Context:      "context",
		ModelAnswer:  "answer",
		Questions:    qResult.Questions,
		Weights:      wResult.Weights,
	})
	if err != nil {
		t.Fatalf("JudgeAnswer() error = %v", err)
	}
	if len(jResult.Judgments) != 1 || jResult.Judgments[0].QuestionID != "q2" {
		t.Fatalf("judgments = %#v", jResult.Judgments)
	}
	assertObjectWritten(t, writer, artifacts.EvaluationLLMRequestKey("evaluation-1", artifacts.PromptBinaryJudging))
	assertObjectWritten(t, writer, artifacts.EvaluationLLMResponseKey("evaluation-1", artifacts.PromptBinaryJudging))

	judgePayload := marshalString(t, client.requests[artifacts.PromptBinaryJudging])
	for _, forbidden := range []string{"weight", "rationale", "q1"} {
		if strings.Contains(judgePayload, forbidden) {
			t.Fatalf("judge payload contains %q: %s", forbidden, judgePayload)
		}
	}
	if !strings.Contains(judgePayload, "q2") || !strings.Contains(judgePayload, "Active?") {
		t.Fatalf("judge payload missing active question: %s", judgePayload)
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
	case artifacts.PromptQuestionGeneration:
		*out.(*llm.QuestionGenerationOutput) = llm.QuestionGenerationOutput{Questions: []evalcore.DraftQuestion{
			{Rationale: "excluded", Question: "Excluded?"},
			{Rationale: "active", Question: "Active?"},
		}}
	case artifacts.PromptWeightAssignment:
		*out.(*llm.WeightAssignmentOutput) = llm.WeightAssignmentOutput{Weights: []evalcore.Weight{
			{QuestionID: "q1", Rationale: "duplicate", Weight: 0},
			{QuestionID: "q2", Rationale: "important", Weight: 4},
		}}
	case artifacts.PromptBinaryJudging:
		*out.(*llm.BinaryJudgingOutput) = llm.BinaryJudgingOutput{Judgments: []evalcore.Judgment{
			{QuestionID: "q2", Evidence: "The answer satisfies it.", Answer: evalcore.AnswerYes},
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
