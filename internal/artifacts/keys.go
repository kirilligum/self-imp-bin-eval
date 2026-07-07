package artifacts

const (
	PromptQuestionGeneration = "question_generation"
	PromptWeightAssignment   = "weight_assignment"
	PromptBinaryJudging      = "binary_judging"
)

func ChecklistTaskKey(checklistID string) string {
	return "checklists/" + checklistID + "/inputs/task.txt"
}

func ChecklistContextKey(checklistID string) string {
	return "checklists/" + checklistID + "/inputs/context.txt"
}

func ChecklistLLMRequestKey(checklistID, promptName string) string {
	return "checklists/" + checklistID + "/llm/" + promptName + "/request.json"
}

func ChecklistLLMResponseKey(checklistID, promptName string) string {
	return "checklists/" + checklistID + "/llm/" + promptName + "/response.json"
}

func EvaluationAnswerKey(evaluationID string) string {
	return "evaluations/" + evaluationID + "/inputs/model_answer.txt"
}

func EvaluationLLMRequestKey(evaluationID, promptName string) string {
	return "evaluations/" + evaluationID + "/llm/" + promptName + "/request.json"
}

func EvaluationLLMResponseKey(evaluationID, promptName string) string {
	return "evaluations/" + evaluationID + "/llm/" + promptName + "/response.json"
}

func RequiredKeys(checklistID, evaluationID string) []string {
	return []string{
		ChecklistTaskKey(checklistID),
		ChecklistContextKey(checklistID),
		ChecklistLLMRequestKey(checklistID, PromptQuestionGeneration),
		ChecklistLLMResponseKey(checklistID, PromptQuestionGeneration),
		ChecklistLLMRequestKey(checklistID, PromptWeightAssignment),
		ChecklistLLMResponseKey(checklistID, PromptWeightAssignment),
		EvaluationAnswerKey(evaluationID),
		EvaluationLLMRequestKey(evaluationID, PromptBinaryJudging),
		EvaluationLLMResponseKey(evaluationID, PromptBinaryJudging),
	}
}
