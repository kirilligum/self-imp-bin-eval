package llm

import (
	"encoding/json"
	"fmt"

	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
)

const QuestionRequirementsPrompt = "Questions must be binary yes/no checks, atomic, answer-independent, tied to one concrete requirement, and answerable from a future model answer."
const QuestionOutputPrompt = "The response object has a questions array; each item has rationale and question."

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type GenerateRequest struct {
	PromptName   string     `json:"prompt_name"`
	ModelProfile string     `json:"model_profile"`
	Messages     []Message  `json:"messages"`
	SchemaName   string     `json:"schema_name"`
	Schema       JSONSchema `json:"schema"`
}

func BuildDimensionAnalysisRequest(task, contextText, modelProfile string, limits evalcore.ChecklistLimits) GenerateRequest {
	limits = limits.WithDefaults()
	payload := map[string]string{
		"task":    task,
		"context": contextText,
	}
	return GenerateRequest{
		PromptName:   "dimension_analysis",
		ModelProfile: modelProfile,
		SchemaName:   "dimension_analysis",
		Schema:       DimensionAnalysisSchema(limits),
		Messages: []Message{
			{Role: "system", Content: "Identify rubric dimensions for evaluating answers to the supplied task and context. Each dimension should cover a distinct requirement area with a concise rubric and rationale. The response object has a dimensions array; each item has name, rubric, and rationale."},
			{Role: "user", Content: mustJSON(payload)},
		},
	}
}

func BuildQuestionGenerationRequest(task, contextText, modelProfile string, dimension evalcore.Dimension, limits evalcore.ChecklistLimits) GenerateRequest {
	limits = limits.WithDefaults()
	payload := map[string]any{
		"task":      task,
		"context":   contextText,
		"dimension": dimension,
	}
	return GenerateRequest{
		PromptName:   "question_generation/" + dimension.ID,
		ModelProfile: modelProfile,
		SchemaName:   "question_generation",
		Schema:       QuestionGenerationSchema(limits),
		Messages: []Message{
			{Role: "system", Content: "Generate candidate evaluation questions for the supplied dimension. " + QuestionRequirementsPrompt + " " + QuestionOutputPrompt + " Do not leave rationale or question blank."},
			{Role: "user", Content: mustJSON(payload)},
		},
	}
}

func BuildWeightAssignmentRequest(task, contextText, modelProfile string, questions []evalcore.CandidateQuestion, limits evalcore.ChecklistLimits) GenerateRequest {
	limits = limits.WithDefaults()
	type questionPayload struct {
		ID          string `json:"id"`
		DimensionID string `json:"dimension_id"`
		Rationale   string `json:"rationale"`
		Question    string `json:"question"`
	}
	payloadQuestions := make([]questionPayload, 0, len(questions))
	for _, question := range questions {
		payloadQuestions = append(payloadQuestions, questionPayload{
			ID:          question.ID,
			DimensionID: question.DimensionID,
			Rationale:   question.Rationale,
			Question:    question.Question,
		})
	}
	payload := map[string]any{
		"task":            task,
		"context":         contextText,
		"candidate_count": len(payloadQuestions),
		"questions":       payloadQuestions,
	}
	return GenerateRequest{
		PromptName:   "weight_assignment",
		ModelProfile: modelProfile,
		SchemaName:   "weight_assignment",
		Schema:       WeightAssignmentSchema(limits, len(payloadQuestions)),
		Messages: []Message{
			{Role: "system", Content: "Assign one diagnostic weight for every supplied candidate question ID. The response object has a weights array; each item has candidate_question_id, rationale, and weight. The weights array length equals candidate_count from the user payload; never return an empty weights array when candidate_count is greater than zero. Use 0 to delete duplicate, redundant, too broad, or not useful questions. Use 1 for normal questions. Use a higher integer when a question is important or broad enough to split into that many more specific questions. There is one item per candidate ID and no blank rationale."},
			{Role: "user", Content: mustJSON(payload)},
		},
	}
}

func BuildQuestionSplittingRequest(task, contextText, modelProfile string, candidate evalcore.CandidateQuestion, weight evalcore.Weight, limits evalcore.ChecklistLimits) GenerateRequest {
	limits = limits.WithDefaults()
	payload := map[string]any{
		"task":               task,
		"context":            contextText,
		"candidate_question": candidate,
		"weight":             weight,
	}
	return GenerateRequest{
		PromptName:   "question_splitting/" + candidate.ID,
		ModelProfile: modelProfile,
		SchemaName:   "question_splitting",
		Schema:       QuestionSplittingSchema(limits, weight.Weight),
		Messages: []Message{
			{Role: "system", Content: "Split the supplied candidate question into the requested number of more specific questions. " + QuestionRequirementsPrompt + " " + QuestionOutputPrompt + " Do not leave rationale or question blank."},
			{Role: "user", Content: mustJSON(payload)},
		},
	}
}

func BuildBinaryJudgingRequest(task, contextText, modelAnswer, modelProfile string, questions []evalcore.FinalQuestion) GenerateRequest {
	type questionPayload struct {
		ID       string `json:"id"`
		Question string `json:"question"`
	}
	payloadQuestions := make([]questionPayload, 0, len(questions))
	for _, question := range questions {
		payloadQuestions = append(payloadQuestions, questionPayload{
			ID:       question.ID,
			Question: question.Question,
		})
	}
	payload := map[string]any{
		"task":           task,
		"context":        contextText,
		"model_answer":   modelAnswer,
		"question_count": len(payloadQuestions),
		"questions":      payloadQuestions,
	}
	return GenerateRequest{
		PromptName:   "binary_judging",
		ModelProfile: modelProfile,
		SchemaName:   "binary_judging",
		Schema:       BinaryJudgingSchema(len(payloadQuestions)),
		Messages: []Message{
			{Role: "system", Content: "For each supplied final question ID and text, judge whether the answer directly satisfies it. The response object has a judgments array; each item has question_id, evidence, and answer. Answer yes only when the model answer contains concrete evidence for the requirement; otherwise answer no. There is one judgment per ID and no blank evidence."},
			{Role: "user", Content: mustJSON(payload)},
		},
	}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("marshal prompt payload: %v", err))
	}
	return string(b)
}
