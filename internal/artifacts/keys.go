package artifacts

import "strconv"

const (
	PromptDimensionAnalysis  = "dimension_analysis"
	PromptQuestionGeneration = "question_generation"
	PromptWeightAssignment   = "weight_assignment"
	PromptQuestionSplitting  = "question_splitting"
	PromptBinaryJudging      = "binary_judging"
)

func ChecklistTaskKey(checklistID string) string {
	return "checklists/" + checklistID + "/inputs/task.txt"
}

func ChecklistContextKey(checklistID string) string {
	return "checklists/" + checklistID + "/inputs/context.txt"
}

func ChecklistLLMRequestKey(checklistID, promptName string, attempt int) string {
	return llmAttemptKey("checklists/"+checklistID, promptName, attempt, "request.json")
}

func ChecklistLLMResponseKey(checklistID, promptName string, attempt int) string {
	return llmAttemptKey("checklists/"+checklistID, promptName, attempt, "response.body")
}

func EvaluationAnswerKey(evaluationID string) string {
	return "evaluations/" + evaluationID + "/inputs/model_answer.txt"
}

func EvaluationLLMRequestKey(evaluationID, promptName string, attempt int) string {
	return llmAttemptKey("evaluations/"+evaluationID, promptName, attempt, "request.json")
}

func EvaluationLLMResponseKey(evaluationID, promptName string, attempt int) string {
	return llmAttemptKey("evaluations/"+evaluationID, promptName, attempt, "response.body")
}

func llmAttemptKey(entityPrefix, promptName string, attempt int, filename string) string {
	return entityPrefix + "/llm/" + promptName + "/attempt-" + strconv.Itoa(attempt) + "/" + filename
}

func RequiredKeys(checklistID, evaluationID string) []string {
	return []string{
		ChecklistTaskKey(checklistID),
		ChecklistContextKey(checklistID),
		ChecklistLLMRequestKey(checklistID, PromptDimensionAnalysis, 1),
		ChecklistLLMResponseKey(checklistID, PromptDimensionAnalysis, 1),
		ChecklistLLMRequestKey(checklistID, PromptWeightAssignment, 1),
		ChecklistLLMResponseKey(checklistID, PromptWeightAssignment, 1),
		EvaluationAnswerKey(evaluationID),
		EvaluationLLMRequestKey(evaluationID, PromptBinaryJudging+"/run-1", 1),
		EvaluationLLMResponseKey(evaluationID, PromptBinaryJudging+"/run-1", 1),
	}
}
