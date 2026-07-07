package evalcore

import "strings"

func ValidateQuestionGeneration(drafts []DraftQuestion) error {
	if len(drafts) == 0 {
		return semanticError(CodeInvalidQuestionGeneration, "question generation returned no questions")
	}
	for i, draft := range drafts {
		if strings.TrimSpace(draft.Rationale) == "" {
			return semanticError(CodeInvalidQuestionGeneration, "question %d has blank rationale", i+1)
		}
		if strings.TrimSpace(draft.Question) == "" {
			return semanticError(CodeInvalidQuestionGeneration, "question %d has blank question", i+1)
		}
	}
	return nil
}

func ValidateWeights(questions []CandidateQuestion, weights []Weight) error {
	_, err := BuildActiveChecklist(questions, weights)
	return err
}

func ValidateJudgments(questions []CandidateQuestion, weights []Weight, judgments []Judgment) error {
	coverage, err := buildCoverage(questions, weights)
	if err != nil {
		return err
	}
	if len(coverage.active) == 0 {
		return semanticError(CodeInvalidWeights, "at least one active question is required")
	}
	judgmentByID := make(map[string]Judgment)
	for _, judgment := range judgments {
		if _, exists := judgmentByID[judgment.QuestionID]; exists {
			return semanticError(CodeInvalidJudgments, "duplicate judgment for question id %q", judgment.QuestionID)
		}
		if _, active := coverage.activeByID[judgment.QuestionID]; !active {
			return semanticError(CodeInvalidJudgments, "judgment references inactive or unknown question id %q", judgment.QuestionID)
		}
		if strings.TrimSpace(judgment.Evidence) == "" {
			return semanticError(CodeInvalidJudgments, "judgment for question id %q has blank evidence", judgment.QuestionID)
		}
		if judgment.Answer != AnswerYes && judgment.Answer != AnswerNo {
			return semanticError(CodeInvalidJudgments, "judgment for question id %q has invalid answer %q", judgment.QuestionID, judgment.Answer)
		}
		judgmentByID[judgment.QuestionID] = judgment
	}
	for _, active := range coverage.active {
		if _, ok := judgmentByID[active.ID]; !ok {
			return semanticError(CodeInvalidJudgments, "missing judgment for active question id %q", active.ID)
		}
	}
	return nil
}
