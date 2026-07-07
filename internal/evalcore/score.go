package evalcore

func ScoreChecklist(questions []CandidateQuestion, weights []Weight, judgments []Judgment) (ScoreResult, error) {
	active, err := BuildActiveChecklist(questions, weights)
	if err != nil {
		return ScoreResult{}, err
	}
	if err := ValidateJudgments(questions, weights, judgments); err != nil {
		return ScoreResult{}, err
	}

	judgmentByID := make(map[string]Judgment, len(judgments))
	for _, judgment := range judgments {
		judgmentByID[judgment.QuestionID] = judgment
	}

	var result ScoreResult
	for _, question := range active {
		result.TotalPossiblePoints += question.Weight
		judgment := judgmentByID[question.ID]
		if judgment.Answer == AnswerYes {
			result.SatisfiedPoints += question.Weight
			continue
		}
		result.FailedQuestionIDs = append(result.FailedQuestionIDs, question.ID)
	}
	result.ChecklistPassRate = float64(result.SatisfiedPoints) / float64(result.TotalPossiblePoints)
	if result.FailedQuestionIDs == nil {
		result.FailedQuestionIDs = []string{}
	}
	return result, nil
}
