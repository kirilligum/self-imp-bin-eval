package llm

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
)

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

	t.Run("schemas constrain generated collection sizes", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			schema   JSONSchema
			property string
			min      int
			max      int
		}{
			{name: "dimensions", schema: DimensionAnalysisSchema(limits), property: "dimensions", min: 1, max: limits.MaxDimensions},
			{name: "questions", schema: QuestionGenerationSchema(limits), property: "questions", min: 1, max: limits.MaxCandidatesPerDimension},
			{name: "weights", schema: WeightAssignmentSchema(limits, 3), property: "weights", min: 3, max: 3},
			{name: "splits", schema: QuestionSplittingSchema(limits, 4), property: "questions", min: 4, max: 4},
			{name: "judgments", schema: BinaryJudgingSchema(5), property: "judgments", min: 5, max: 5},
		} {
			t.Run(tc.name, func(t *testing.T) {
				prop := tc.schema["properties"].(map[string]any)[tc.property].(map[string]any)
				if prop["minItems"] != tc.min || prop["maxItems"] != tc.max {
					t.Fatalf("schema %s %s min/max = %#v/%#v, want %d/%d", tc.name, tc.property, prop["minItems"], prop["maxItems"], tc.min, tc.max)
				}
			})
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
			if !strings.Contains(payload, QuestionRequirementsPrompt) {
				t.Fatalf("%s prompt missing shared fragment %q: %s", req.PromptName, QuestionRequirementsPrompt, payload)
			}
			for _, required := range []string{"yes answer always means", "satisfies the evaluation requirement", "avoid", "Never ask whether the answer is wrong", "omits", "prohibited content", "independently omitted", "actor", "metric", "time or duration"} {
				if !strings.Contains(payload, required) {
					t.Fatalf("%s prompt missing positive-orientation rule %q: %s", req.PromptName, required, payload)
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
		for _, want := range []string{"split count", "0", "semantically duplicate", "yes would indicate a defect", "1", "atomic", "partially satisfied", "2, 3, or 4", "independently judgeable", "independently present or absent", "actor", "metric", "time or duration", "one instruction"} {
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
		for _, want := range []string{"q2", "Does it mention alpha?", "answer", "yes contributes one satisfied point"} {
			if !strings.Contains(payload, want) {
				t.Fatalf("judging prompt missing %q: %s", want, payload)
			}
		}
	})
}

func TestP06CompositionalWeightsAndEffectiveLimits(t *testing.T) {
	limits := evalcore.ChecklistLimits{
		MaxDimensions:             9,
		MaxCandidatesPerDimension: 12,
		MaxSplitCount:             4,
		MaxFinalQuestions:         64,
	}
	dimension := evalcore.Dimension{ID: "d1", Ordinal: 1, Name: "Correctness", Rubric: "Check correctness.", Rationale: "Core."}
	candidate := evalcore.CandidateQuestion{ID: "c1", DimensionID: "d1", Ordinal: 1, Rationale: "coverage", Question: "Does it identify the cause and provide a tested fix?"}
	weight := evalcore.Weight{CandidateQuestionID: "c1", Rationale: "two obligations", Weight: 2}

	requests := []GenerateRequest{
		BuildDimensionAnalysisRequest("task", "context", "model", limits),
		BuildQuestionGenerationRequest("task", "context", "model", dimension, limits),
		BuildWeightAssignmentRequest("task", "context", "model", []evalcore.CandidateQuestion{candidate}, limits),
		BuildQuestionSplittingRequest("task", "context", "model", candidate, weight, limits),
		BuildBinaryJudgingRequest("task", "context", "answer", "model", []evalcore.FinalQuestion{{ID: "q1", Question: "Does it identify the cause?"}}),
	}
	for _, request := range requests {
		systemPrompt := request.Messages[0].Content
		for _, forbidden := range []string{"response object", "array", "item has", "return JSON", "Return only JSON"} {
			if strings.Contains(systemPrompt, forbidden) {
				t.Fatalf("%s system prompt duplicates output structure with %q: %s", request.PromptName, forbidden, systemPrompt)
			}
		}
	}

	weightPrompt := requests[2].Messages[0].Content
	for _, required := range []string{"0", "semantically duplicate", "1", "atomic", "2", "3", "4", "independently judgeable"} {
		if !strings.Contains(weightPrompt, required) {
			t.Fatalf("weight prompt missing %q: %s", required, weightPrompt)
		}
	}
	for _, forbidden := range []string{"important", "broad enough", "more important"} {
		if strings.Contains(weightPrompt, forbidden) {
			t.Fatalf("weight prompt contains importance-based split instruction %q: %s", forbidden, weightPrompt)
		}
	}

	generationQuestions := QuestionGenerationSchema(limits)["properties"].(map[string]any)["questions"].(map[string]any)
	if generationQuestions["maxItems"] != 12 {
		t.Fatalf("question generation maxItems = %#v, want 12", generationQuestions["maxItems"])
	}
}

func marshalString(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return string(b)
}
