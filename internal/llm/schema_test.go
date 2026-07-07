package llm

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
)

// TEST-006
func TestOutputSchemasAndPrompts(t *testing.T) {
	t.Run("schemas describe required output families", func(t *testing.T) {
		questionSchema := QuestionGenerationSchema()
		questionBytes, _ := json.Marshal(questionSchema)
		if !strings.Contains(string(questionBytes), `"questions"`) {
			t.Fatalf("question schema missing questions: %s", questionBytes)
		}

		weightSchema := WeightAssignmentSchema()
		weightBytes, _ := json.Marshal(weightSchema)
		for _, want := range []string{`"minimum":0`, `"maximum":4`, `"question_id"`, `"rationale"`} {
			if !strings.Contains(string(weightBytes), want) {
				t.Fatalf("weight schema missing %s: %s", want, weightBytes)
			}
		}

		judgeSchema := BinaryJudgingSchema()
		judgeBytes, _ := json.Marshal(judgeSchema)
		for _, want := range []string{`"judgments"`, `"evidence"`, `"yes"`, `"no"`} {
			if !strings.Contains(string(judgeBytes), want) {
				t.Fatalf("judge schema missing %s: %s", want, judgeBytes)
			}
		}
	})

	t.Run("output validation rejects structural violations", func(t *testing.T) {
		if err := (QuestionGenerationOutput{Questions: []evalcore.DraftQuestion{{Rationale: "r", Question: "q?"}}}).Validate(); err != nil {
			t.Fatalf("valid questions error = %v", err)
		}
		if err := (QuestionGenerationOutput{}).Validate(); err == nil {
			t.Fatal("empty questions unexpectedly valid")
		}
		if err := (WeightAssignmentOutput{Weights: []evalcore.Weight{{QuestionID: "q1", Rationale: "r", Weight: 0}}}).Validate(); err != nil {
			t.Fatalf("valid weight zero error = %v", err)
		}
		if err := (WeightAssignmentOutput{Weights: []evalcore.Weight{{QuestionID: "q1", Rationale: "r", Weight: 5}}}).Validate(); err == nil {
			t.Fatal("weight 5 unexpectedly valid")
		}
		if err := (BinaryJudgingOutput{Judgments: []evalcore.Judgment{{QuestionID: "q1", Evidence: "e", Answer: evalcore.AnswerYes}}}).Validate(); err != nil {
			t.Fatalf("valid judgment error = %v", err)
		}
		if err := (BinaryJudgingOutput{Judgments: []evalcore.Judgment{{QuestionID: "q1", Evidence: " ", Answer: "maybe"}}}).Validate(); err == nil {
			t.Fatal("invalid judgment unexpectedly valid")
		}
	})

	t.Run("checklist creation prompt excludes model answer", func(t *testing.T) {
		req := BuildQuestionGenerationRequest("task text", "context text", "checklist-evaluator")
		payload := marshalString(t, req)
		if strings.Contains(payload, "model_answer") {
			t.Fatalf("question generation prompt contains model_answer: %s", payload)
		}
		if !strings.Contains(payload, "task text") || !strings.Contains(payload, "context text") {
			t.Fatalf("question generation prompt missing task/context: %s", payload)
		}
	})

	t.Run("judging prompt excludes weights and rationales", func(t *testing.T) {
		req := BuildBinaryJudgingRequest("task", "context", "answer", "checklist-evaluator", []evalcore.ActiveQuestion{
			{ID: "q2", Question: "Does it mention alpha?", Weight: 4},
		})
		payload := marshalString(t, req)
		for _, forbidden := range []string{"weight", "rationale"} {
			if strings.Contains(payload, forbidden) {
				t.Fatalf("judging prompt contains %q: %s", forbidden, payload)
			}
		}
		for _, want := range []string{"q2", "Does it mention alpha?", "answer"} {
			if !strings.Contains(payload, want) {
				t.Fatalf("judging prompt missing %q: %s", want, payload)
			}
		}
	})
}

func marshalString(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return string(b)
}
