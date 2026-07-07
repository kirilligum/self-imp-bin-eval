package llm

import (
	"strings"

	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
)

type JSONSchema map[string]any

type Validatable interface {
	Validate() error
}

type QuestionGenerationOutput struct {
	Questions []evalcore.DraftQuestion `json:"questions"`
}

func (o QuestionGenerationOutput) Validate() error {
	return evalcore.ValidateQuestionGeneration(o.Questions)
}

type WeightAssignmentOutput struct {
	Weights []evalcore.Weight `json:"weights"`
}

func (o WeightAssignmentOutput) Validate() error {
	if len(o.Weights) == 0 {
		return &ModelOutputError{Message: "weight assignment returned no weights"}
	}
	for _, weight := range o.Weights {
		if strings.TrimSpace(weight.QuestionID) == "" {
			return &ModelOutputError{Message: "weight assignment returned blank question_id"}
		}
		if strings.TrimSpace(weight.Rationale) == "" {
			return &ModelOutputError{Message: "weight assignment returned blank rationale"}
		}
		if weight.Weight < 0 || weight.Weight > 4 {
			return &ModelOutputError{Message: "weight assignment returned weight outside 0..4"}
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

func QuestionGenerationSchema() JSONSchema {
	return JSONSchema{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"questions"},
		"properties": map[string]any{
			"questions": map[string]any{
				"type":     "array",
				"minItems": 1,
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"rationale", "question"},
					"properties": map[string]any{
						"rationale": map[string]any{"type": "string", "minLength": 1},
						"question":  map[string]any{"type": "string", "minLength": 1},
					},
				},
			},
		},
	}
}

func WeightAssignmentSchema() JSONSchema {
	return JSONSchema{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"weights"},
		"properties": map[string]any{
			"weights": map[string]any{
				"type":     "array",
				"minItems": 1,
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"question_id", "rationale", "weight"},
					"properties": map[string]any{
						"question_id": map[string]any{"type": "string", "minLength": 1},
						"rationale":   map[string]any{"type": "string", "minLength": 1},
						"weight":      map[string]any{"type": "integer", "minimum": 0, "maximum": 4},
					},
				},
			},
		},
	}
}

func BinaryJudgingSchema() JSONSchema {
	return JSONSchema{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"judgments"},
		"properties": map[string]any{
			"judgments": map[string]any{
				"type":     "array",
				"minItems": 1,
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"question_id", "evidence", "answer"},
					"properties": map[string]any{
						"question_id": map[string]any{"type": "string", "minLength": 1},
						"evidence":    map[string]any{"type": "string", "minLength": 1},
						"answer":      map[string]any{"type": "string", "enum": []string{evalcore.AnswerYes, evalcore.AnswerNo}},
					},
				},
			},
		},
	}
}
