package evalcore

import "fmt"

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

func AggregateJudgments(questions []FinalQuestion, runJudgments []RunJudgment, evaluationRuns int) (AggregationResult, error) {
	if err := ValidateEvaluationRuns(evaluationRuns, evaluationRuns); err != nil {
		return AggregationResult{}, err
	}
	byRun := make(map[int][]Judgment, evaluationRuns)
	for _, runJudgment := range runJudgments {
		if runJudgment.RunIndex < 1 || runJudgment.RunIndex > evaluationRuns {
			return AggregationResult{}, semanticError(CodeInvalidJudgments, "run_index %d is outside 1..%d", runJudgment.RunIndex, evaluationRuns)
		}
		byRun[runJudgment.RunIndex] = append(byRun[runJudgment.RunIndex], Judgment{
			QuestionID: runJudgment.QuestionID,
			Evidence:   runJudgment.Evidence,
			Answer:     runJudgment.Answer,
		})
	}
	judgmentByRunAndQuestion := make(map[int]map[string]Judgment, evaluationRuns)
	for runIndex := 1; runIndex <= evaluationRuns; runIndex++ {
		judgments := byRun[runIndex]
		if err := ValidateJudgments(questions, judgments); err != nil {
			return AggregationResult{}, fmt.Errorf("validate evaluation run %d: %w", runIndex, err)
		}
		judgmentByQuestion := make(map[string]Judgment, len(judgments))
		for _, judgment := range judgments {
			judgmentByQuestion[judgment.QuestionID] = judgment
		}
		judgmentByRunAndQuestion[runIndex] = judgmentByQuestion
	}

	result := AggregationResult{Judgments: make([]AggregatedJudgment, 0, len(questions))}
	derived := make([]Judgment, 0, len(questions))
	for _, question := range questions {
		aggregated := AggregatedJudgment{QuestionID: question.ID, Runs: make([]JudgmentRun, 0, evaluationRuns)}
		yesCount := 0
		for runIndex := 1; runIndex <= evaluationRuns; runIndex++ {
			judgment := judgmentByRunAndQuestion[runIndex][question.ID]
			aggregated.Runs = append(aggregated.Runs, JudgmentRun{RunIndex: runIndex, Evidence: judgment.Evidence, Answer: judgment.Answer})
			if judgment.Answer == AnswerYes {
				yesCount++
			}
		}
		aggregated.Answer = AnswerNo
		if yesCount > evaluationRuns/2 {
			aggregated.Answer = AnswerYes
		}
		result.Judgments = append(result.Judgments, aggregated)
		derived = append(derived, Judgment{QuestionID: question.ID, Evidence: "derived from repeated judgments", Answer: aggregated.Answer})
	}
	score, err := ScoreChecklist(questions, derived)
	if err != nil {
		return AggregationResult{}, err
	}
	result.Score = score
	return result, nil
}
