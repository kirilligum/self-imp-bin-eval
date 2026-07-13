package llm

import "github.com/kirilligum/self-imp-bin-eval/internal/evalcore"

type JSONSchema map[string]any

type Validatable interface {
	Validate() error
}

type DimensionAnalysisOutput struct {
	Dimensions []evalcore.DraftDimension `json:"dimensions"`
}

func (o DimensionAnalysisOutput) Validate() error {
	return evalcore.ValidateDimensionGeneration(o.Dimensions, evalcore.ChecklistLimits{MaxDimensions: len(o.Dimensions)})
}

type QuestionGenerationOutput struct {
	Questions []evalcore.DraftQuestion `json:"questions"`
}

func (o QuestionGenerationOutput) Validate() error {
	return evalcore.ValidateQuestionGeneration(o.Questions, evalcore.ChecklistLimits{MaxCandidatesPerDimension: len(o.Questions)})
}

type WeightAssignmentOutput struct {
	Weights []evalcore.Weight `json:"weights"`
}

func (o WeightAssignmentOutput) Validate() error {
	return evalcore.ValidateWeightShape(o.Weights)
}

type QuestionSplittingOutput struct {
	Questions []evalcore.DraftQuestion `json:"questions"`
}

func (o QuestionSplittingOutput) Validate() error {
	return evalcore.ValidateQuestionGeneration(o.Questions, evalcore.ChecklistLimits{MaxCandidatesPerDimension: len(o.Questions)})
}

type BinaryJudgingOutput struct {
	Judgments []evalcore.Judgment `json:"judgments"`
}

func (o BinaryJudgingOutput) Validate() error {
	return evalcore.ValidateJudgmentShape(o.Judgments)
}

func DimensionAnalysisSchema(limits evalcore.ChecklistLimits) JSONSchema {
	limits = limits.WithDefaults()
	return JSONSchema{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"dimensions"},
		"properties": map[string]any{
			"dimensions": map[string]any{
				"type":     "array",
				"minItems": 1,
				"maxItems": limits.MaxDimensions,
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"name", "rubric", "rationale"},
					"properties": map[string]any{
						"name":      map[string]any{"type": "string"},
						"rubric":    map[string]any{"type": "string"},
						"rationale": map[string]any{"type": "string"},
					},
				},
			},
		},
	}
}

func QuestionGenerationSchema(limits evalcore.ChecklistLimits) JSONSchema {
	limits = limits.WithDefaults()
	return JSONSchema{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"questions"},
		"properties": map[string]any{
			"questions": map[string]any{
				"type":     "array",
				"minItems": 1,
				"maxItems": limits.MaxCandidatesPerDimension,
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"rationale", "question"},
					"properties": map[string]any{
						"rationale": map[string]any{"type": "string"},
						"question":  map[string]any{"type": "string"},
					},
				},
			},
		},
	}
}

func WeightAssignmentSchema(limits evalcore.ChecklistLimits, candidateCount int) JSONSchema {
	limits = limits.WithDefaults()
	weightValues := make([]int, 0, limits.MaxSplitCount+1)
	for i := 0; i <= limits.MaxSplitCount; i++ {
		weightValues = append(weightValues, i)
	}
	return JSONSchema{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"weights"},
		"properties": map[string]any{
			"weights": map[string]any{
				"type":     "array",
				"minItems": candidateCount,
				"maxItems": candidateCount,
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"candidate_question_id", "rationale", "weight"},
					"properties": map[string]any{
						"candidate_question_id": map[string]any{"type": "string"},
						"rationale":             map[string]any{"type": "string"},
						"weight":                map[string]any{"type": "integer", "enum": weightValues},
					},
				},
			},
		},
	}
}

func QuestionSplittingSchema(limits evalcore.ChecklistLimits, splitCount int) JSONSchema {
	limits = limits.WithDefaults()
	return JSONSchema{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"questions"},
		"properties": map[string]any{
			"questions": map[string]any{
				"type":     "array",
				"minItems": splitCount,
				"maxItems": splitCount,
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"rationale", "question"},
					"properties": map[string]any{
						"rationale": map[string]any{"type": "string"},
						"question":  map[string]any{"type": "string"},
					},
				},
			},
		},
	}
}

func BinaryJudgingSchema(questionCount int) JSONSchema {
	return JSONSchema{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"judgments"},
		"properties": map[string]any{
			"judgments": map[string]any{
				"type":     "array",
				"minItems": questionCount,
				"maxItems": questionCount,
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"question_id", "evidence", "answer"},
					"properties": map[string]any{
						"question_id": map[string]any{"type": "string"},
						"evidence":    map[string]any{"type": "string"},
						"answer":      map[string]any{"type": "string", "enum": []string{evalcore.AnswerYes, evalcore.AnswerNo}},
					},
				},
			},
		},
	}
}
