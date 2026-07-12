package evalcore

func ScoreChecklist(questions []FinalQuestion, judgments []Judgment) (ScoreResult, error) {
	if err := ValidateJudgments(questions, judgments); err != nil {
		return ScoreResult{}, err
	}

	judgmentByID := make(map[string]Judgment, len(judgments))
	for _, judgment := range judgments {
		judgmentByID[judgment.QuestionID] = judgment
	}

	result := ScoreResult{TotalPossiblePoints: len(questions)}
	for _, question := range questions {
		judgment := judgmentByID[question.ID]
		if judgment.Answer == AnswerYes {
			result.SatisfiedPoints++
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
