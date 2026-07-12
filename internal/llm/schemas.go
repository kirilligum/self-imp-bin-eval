package llm

import (
	"strings"

	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
)

type JSONSchema map[string]any

type Validatable interface {
	Validate() error
}

type DimensionAnalysisOutput struct {
	Dimensions []evalcore.DraftDimension `json:"dimensions"`
}

func (o DimensionAnalysisOutput) Validate() error {
	return evalcore.ValidateDimensionGeneration(o.Dimensions, evalcore.DefaultChecklistLimits())
}

type QuestionGenerationOutput struct {
	Questions []evalcore.DraftQuestion `json:"questions"`
}

func (o QuestionGenerationOutput) Validate() error {
	return evalcore.ValidateQuestionGeneration(o.Questions, evalcore.DefaultChecklistLimits())
}

type WeightAssignmentOutput struct {
	Weights []evalcore.Weight `json:"weights"`
}

func (o WeightAssignmentOutput) Validate() error {
	if len(o.Weights) == 0 {
		return &ModelOutputError{Message: "weight assignment returned no weights"}
	}
	for _, weight := range o.Weights {
		if strings.TrimSpace(weight.CandidateQuestionID) == "" {
			return &ModelOutputError{Message: "weight assignment returned blank candidate_question_id"}
		}
		if strings.TrimSpace(weight.Rationale) == "" {
			return &ModelOutputError{Message: "weight assignment returned blank rationale"}
		}
		if weight.Weight < 0 || weight.Weight > evalcore.DefaultMaxSplitCount {
			return &ModelOutputError{Message: "weight assignment returned weight outside 0..4"}
		}
	}
	return nil
}

type QuestionSplittingOutput struct {
	Questions []evalcore.DraftQuestion `json:"questions"`
}

func (o QuestionSplittingOutput) Validate() error {
	if len(o.Questions) == 0 {
		return &ModelOutputError{Message: "question splitting returned no questions"}
	}
	for _, question := range o.Questions {
		if strings.TrimSpace(question.Rationale) == "" {
			return &ModelOutputError{Message: "question splitting returned blank rationale"}
		}
		if strings.TrimSpace(question.Question) == "" {
			return &ModelOutputError{Message: "question splitting returned blank question"}
		}
	}
	return nil
}

type BinaryJudgingOutput struct {
	Judgments []evalcore.Judgment `json:"judgments"`
}

func (o BinaryJudgingOutput) Validate() error {
	if len(o.Judgments) == 0 {
		return &ModelOutputError{Message: "binary judging returned no judgments"}
	}
	for _, judgment := range o.Judgments {
		if strings.TrimSpace(judgment.QuestionID) == "" {
			return &ModelOutputError{Message: "binary judging returned blank question_id"}
		}
		if strings.TrimSpace(judgment.Evidence) == "" {
			return &ModelOutputError{Message: "binary judging returned blank evidence"}
		}
		if judgment.Answer != evalcore.AnswerYes && judgment.Answer != evalcore.AnswerNo {
			return &ModelOutputError{Message: "binary judging returned invalid answer"}
		}
	}
	return nil
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
