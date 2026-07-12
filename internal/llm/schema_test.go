package llm

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
)

// TEST-001
func TestRubricRefinementSchemasAndPrompts(t *testing.T) {
	limits := evalcore.DefaultChecklistLimits()

	t.Run("schemas describe required output families", func(t *testing.T) {
		for name, schema := range map[string]JSONSchema{
			"dimension_analysis":  DimensionAnalysisSchema(limits),
			"question_generation": QuestionGenerationSchema(limits),
			"weight_assignment":   WeightAssignmentSchema(limits, 2),
			"question_splitting":  QuestionSplittingSchema(limits, 2),
			"binary_judging":      BinaryJudgingSchema(2),
		} {
			schemaBytes, _ := json.Marshal(schema)
			if !strings.Contains(string(schemaBytes), `"additionalProperties":false`) {
				t.Fatalf("%s schema is not strict: %s", name, schemaBytes)
			}
		}

		weightBytes, _ := json.Marshal(WeightAssignmentSchema(limits, 2))
		for _, want := range []string{`"candidate_question_id"`, `"enum":[0,1,2,3,4]`, `"rationale"`} {
			if !strings.Contains(string(weightBytes), want) {
				t.Fatalf("weight schema missing %s: %s", want, weightBytes)
			}
		}
	})

	t.Run("output validation rejects structural violations", func(t *testing.T) {
		if err := (DimensionAnalysisOutput{Dimensions: []evalcore.DraftDimension{{Name: "Correctness", Rubric: "Checks correctness.", Rationale: "Needed."}}}).Validate(); err != nil {
			t.Fatalf("valid dimensions error = %v", err)
		}
		if err := (DimensionAnalysisOutput{}).Validate(); err == nil {
			t.Fatal("empty dimensions unexpectedly valid")
		}
		if err := (QuestionGenerationOutput{Questions: []evalcore.DraftQuestion{{Rationale: "r", Question: "q?"}}}).Validate(); err != nil {
			t.Fatalf("valid questions error = %v", err)
		}
		if err := (QuestionGenerationOutput{}).Validate(); err == nil {
			t.Fatal("empty questions unexpectedly valid")
		}
		if err := (WeightAssignmentOutput{Weights: []evalcore.Weight{{CandidateQuestionID: "c1", Rationale: "duplicate", Weight: 0}}}).Validate(); err != nil {
			t.Fatalf("valid weight zero error = %v", err)
		}
		if err := (WeightAssignmentOutput{Weights: []evalcore.Weight{{CandidateQuestionID: "c1", Rationale: "r", Weight: 5}}}).Validate(); err == nil {
			t.Fatal("weight 5 unexpectedly valid")
		}
		if err := (QuestionSplittingOutput{Questions: []evalcore.DraftQuestion{{Rationale: "r", Question: "q?"}}}).Validate(); err != nil {
			t.Fatalf("valid split error = %v", err)
		}
		if err := (BinaryJudgingOutput{Judgments: []evalcore.Judgment{{QuestionID: "q1", Evidence: "e", Answer: evalcore.AnswerYes}}}).Validate(); err != nil {
			t.Fatalf("valid judgment error = %v", err)
		}
		if err := (BinaryJudgingOutput{Judgments: []evalcore.Judgment{{QuestionID: "q1", Evidence: " ", Answer: "maybe"}}}).Validate(); err == nil {
			t.Fatal("invalid judgment unexpectedly valid")
		}
	})

	t.Run("prompts use shared question requirements without fixed generation count or json examples", func(t *testing.T) {
		dimension := evalcore.Dimension{ID: "d1", Ordinal: 1, Name: "Correctness", Rubric: "Check correctness.", Rationale: "Core."}
		generation := BuildQuestionGenerationRequest("task text", "context text", "checklist-evaluator", dimension, limits)
		splitting := BuildQuestionSplittingRequest("task text", "context text", "checklist-evaluator", evalcore.CandidateQuestion{
			ID: "c1", DimensionID: "d1", Ordinal: 1, Rationale: "r", Question: "Does it work?",
		}, evalcore.Weight{CandidateQuestionID: "c1", Rationale: "split", Weight: 2}, limits)

		for _, req := range []GenerateRequest{generation, splitting} {
			payload := marshalString(t, req)
			for _, shared := range []string{QuestionRequirementsPrompt, QuestionOutputPrompt} {
				if !strings.Contains(payload, shared) {
					t.Fatalf("%s prompt missing shared fragment %q: %s", req.PromptName, shared, payload)
				}
			}
			for _, forbidden := range []string{"Generate 5 to 8", "strong answer", "weak answer", "Return only JSON", `"questions":[`} {
				if strings.Contains(payload, forbidden) {
					t.Fatalf("%s prompt contains forbidden %q: %s", req.PromptName, forbidden, payload)
				}
			}
		}
	})

	t.Run("weight prompt explains duplicate deletion", func(t *testing.T) {
		req := BuildWeightAssignmentRequest("task", "context", "checklist-evaluator", []evalcore.CandidateQuestion{
			{ID: "c1", DimensionID: "d1", Ordinal: 1, Rationale: "r", Question: "Does it mention alpha?"},
		}, limits)
		payload := marshalString(t, req)
		for _, want := range []string{"weight", "0 to delete duplicate", "1 for normal", "higher integer"} {
			if !strings.Contains(payload, want) {
				t.Fatalf("weight prompt missing %q: %s", want, payload)
			}
		}
		if strings.Contains(payload, "Return only JSON") {
			t.Fatalf("weight prompt mentions JSON shape: %s", payload)
		}
	})

	t.Run("judging prompt excludes weights and rationales", func(t *testing.T) {
		req := BuildBinaryJudgingRequest("task", "context", "answer", "checklist-evaluator", []evalcore.FinalQuestion{
			{ID: "q2", Question: "Does it mention alpha?"},
		})
		payload := marshalString(t, req)
		for _, forbidden := range []string{"weight", "rationale", "candidate"} {
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
