package llm

import (
	"encoding/json"
	"fmt"

	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
)

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

func BuildQuestionGenerationRequest(task, contextText, modelProfile string) GenerateRequest {
	payload := map[string]string{
		"task":    task,
		"context": contextText,
	}
	return GenerateRequest{
		PromptName:   "question_generation",
		ModelProfile: modelProfile,
		SchemaName:   "question_generation",
		Schema:       QuestionGenerationSchema(),
		Messages: []Message{
			{Role: "system", Content: "Generate 5 to 8 atomic, answer-independent binary yes/no checklist questions that distinguish a strong answer from a weak answer for the supplied task and context. Each question must check one concrete requirement and be answerable from a future model answer. Return only JSON shaped as {\"questions\":[{\"rationale\":\"non-empty reason this question matters\",\"question\":\"Does the answer ...?\"}]}. Do not leave rationale or question blank."},
			{Role: "user", Content: mustJSON(payload)},
		},
	}
}

func BuildWeightAssignmentRequest(task, contextText, modelProfile string, questions []evalcore.CandidateQuestion) GenerateRequest {
	type questionPayload struct {
		ID        string `json:"id"`
		Rationale string `json:"rationale"`
		Question  string `json:"question"`
	}
	payloadQuestions := make([]questionPayload, 0, len(questions))
	for _, question := range questions {
		payloadQuestions = append(payloadQuestions, questionPayload{
			ID:        question.ID,
			Rationale: question.Rationale,
			Question:  question.Question,
		})
	}
	payload := map[string]any{
		"task":      task,
		"context":   contextText,
		"questions": payloadQuestions,
	}
	return GenerateRequest{
		PromptName:   "weight_assignment",
		ModelProfile: modelProfile,
		SchemaName:   "weight_assignment",
		Schema:       WeightAssignmentSchema(),
		Messages: []Message{
			{Role: "system", Content: "Assign one integer weight from 0 to 4 for every supplied question ID. Use 4 for central requirements, 2 or 3 for useful supporting requirements, 1 for minor requirements, and 0 only to exclude redundant, duplicate, too broad, or not useful questions. Return only JSON shaped as {\"weights\":[{\"question_id\":\"q1\",\"rationale\":\"non-empty reason for this weight\",\"weight\":4}]}. Return exactly one weight object per ID and do not leave rationale blank."},
			{Role: "user", Content: mustJSON(payload)},
		},
	}
}

func BuildBinaryJudgingRequest(task, contextText, modelAnswer, modelProfile string, questions []evalcore.ActiveQuestion) GenerateRequest {
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
		"task":         task,
		"context":      contextText,
		"model_answer": modelAnswer,
		"questions":    payloadQuestions,
	}
	return GenerateRequest{
		PromptName:   "binary_judging",
		ModelProfile: modelProfile,
		SchemaName:   "binary_judging",
		Schema:       BinaryJudgingSchema(),
		Messages: []Message{
			{Role: "system", Content: "For each supplied question ID and text, judge whether the answer directly satisfies it. Answer yes only when the model answer contains concrete evidence for the requirement; otherwise answer no. Return only JSON shaped as {\"judgments\":[{\"question_id\":\"ID_FROM_INPUT\",\"evidence\":\"non-empty evidence from the answer or what is missing\",\"answer\":\"yes\"}]}. Return exactly one judgment per ID and do not leave evidence blank."},
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
