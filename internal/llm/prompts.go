package llm

import (
	"encoding/json"
	"fmt"

	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
)

const QuestionRequirementsPrompt = "Questions must be binary yes/no checks, atomic, answer-independent, tied to one concrete requirement, and answerable from a future model answer."

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
			{Role: "system", Content: "Identify distinct rubric dimensions for evaluating answers to the supplied task and context. Give each dimension a concise rubric and rationale."},
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
			{Role: "system", Content: "Generate candidate evaluation questions for the supplied dimension. " + QuestionRequirementsPrompt + " Give a rationale for each question."},
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
			{Role: "system", Content: "Assign one diagnostic split count to every supplied candidate question ID and explain each assignment. Use 0 to delete a question that is not useful or is semantically duplicate. Use 1 when the question contains one atomic requirement. Use 2, 3, or 4 only when the question combines exactly that many independently judgeable requirements and should be split into that many questions."},
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
			{Role: "system", Content: "Split the supplied candidate question into exactly the requested number of specific questions. " + QuestionRequirementsPrompt + " Give a rationale for each question."},
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
			{Role: "system", Content: "For each supplied final question ID and text, judge whether the model answer directly satisfies it. Answer yes only when the model answer contains concrete evidence for the requirement; otherwise answer no. Explain the evidence for every judgment."},
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
