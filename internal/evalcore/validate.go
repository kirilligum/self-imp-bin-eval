package evalcore

import "strings"

func ValidateDimensionGeneration(drafts []DraftDimension, limits ChecklistLimits) error {
	limits = limits.WithDefaults()
	if len(drafts) == 0 {
		return semanticError(CodeInvalidDimensionAnalysis, "dimension analysis returned no dimensions")
	}
	if len(drafts) > limits.MaxDimensions {
		return limitError(CodeInvalidDimensionAnalysis, LimitDiagnostic{
			LimitName:       "max_dimensions",
			ConfiguredLimit: limits.MaxDimensions,
			ObservedCount:   len(drafts),
			Stage:           "dimension_analysis",
		})
	}
	for i, draft := range drafts {
		if strings.TrimSpace(draft.Name) == "" {
			return semanticError(CodeInvalidDimensionAnalysis, "dimension %d has blank name", i+1)
		}
		if strings.TrimSpace(draft.Rubric) == "" {
			return semanticError(CodeInvalidDimensionAnalysis, "dimension %d has blank rubric", i+1)
		}
		if strings.TrimSpace(draft.Rationale) == "" {
			return semanticError(CodeInvalidDimensionAnalysis, "dimension %d has blank rationale", i+1)
		}
	}
	return nil
}

func ValidateDimensions(dimensions []Dimension, limits ChecklistLimits) error {
	limits = limits.WithDefaults()
	if len(dimensions) == 0 {
		return semanticError(CodeInvalidDimensionAnalysis, "no dimensions")
	}
	if len(dimensions) > limits.MaxDimensions {
		return limitError(CodeInvalidDimensionAnalysis, LimitDiagnostic{
			LimitName:       "max_dimensions",
			ConfiguredLimit: limits.MaxDimensions,
			ObservedCount:   len(dimensions),
			Stage:           "validate_dimensions",
		})
	}
	seen := make(map[string]struct{}, len(dimensions))
	for _, dimension := range dimensions {
		if strings.TrimSpace(dimension.ID) == "" {
			return semanticError(CodeInvalidDimensionAnalysis, "dimension has blank id")
		}
		if _, exists := seen[dimension.ID]; exists {
			return semanticError(CodeInvalidDimensionAnalysis, "duplicate dimension id %q", dimension.ID)
		}
		seen[dimension.ID] = struct{}{}
		if dimension.Ordinal <= 0 {
			return semanticError(CodeInvalidDimensionAnalysis, "dimension id %q has invalid ordinal %d", dimension.ID, dimension.Ordinal)
		}
		if strings.TrimSpace(dimension.Name) == "" {
			return semanticError(CodeInvalidDimensionAnalysis, "dimension id %q has blank name", dimension.ID)
		}
		if strings.TrimSpace(dimension.Rubric) == "" {
			return semanticError(CodeInvalidDimensionAnalysis, "dimension id %q has blank rubric", dimension.ID)
		}
		if strings.TrimSpace(dimension.Rationale) == "" {
			return semanticError(CodeInvalidDimensionAnalysis, "dimension id %q has blank rationale", dimension.ID)
		}
	}
	return nil
}

func ValidateQuestionGeneration(drafts []DraftQuestion, limits ChecklistLimits) error {
	limits = limits.WithDefaults()
	if len(drafts) == 0 {
		return semanticError(CodeInvalidQuestionGeneration, "question generation returned no questions")
	}
	if len(drafts) > limits.MaxCandidatesPerDimension {
		return limitError(CodeInvalidQuestionGeneration, LimitDiagnostic{
			LimitName:       "max_candidates_per_dimension",
			ConfiguredLimit: limits.MaxCandidatesPerDimension,
			ObservedCount:   len(drafts),
			Stage:           "question_generation",
		})
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

func ValidateCandidateQuestions(dimensions []Dimension, candidates []CandidateQuestion, limits ChecklistLimits) error {
	limits = limits.WithDefaults()
	dimensionByID := make(map[string]struct{}, len(dimensions))
	for _, dimension := range dimensions {
		dimensionByID[dimension.ID] = struct{}{}
	}
	countByDimension := make(map[string]int)
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.ID) == "" {
			return semanticError(CodeInvalidQuestionGeneration, "candidate question has blank id")
		}
		if _, exists := seen[candidate.ID]; exists {
			return semanticError(CodeInvalidQuestionGeneration, "duplicate candidate question id %q", candidate.ID)
		}
		seen[candidate.ID] = struct{}{}
		if _, ok := dimensionByID[candidate.DimensionID]; !ok {
			return semanticError(CodeInvalidQuestionGeneration, "candidate question id %q references unknown dimension id %q", candidate.ID, candidate.DimensionID)
		}
		if candidate.Ordinal <= 0 {
			return semanticError(CodeInvalidQuestionGeneration, "candidate question id %q has invalid ordinal %d", candidate.ID, candidate.Ordinal)
		}
		if strings.TrimSpace(candidate.Rationale) == "" {
			return semanticError(CodeInvalidQuestionGeneration, "candidate question id %q has blank rationale", candidate.ID)
		}
		if strings.TrimSpace(candidate.Question) == "" {
			return semanticError(CodeInvalidQuestionGeneration, "candidate question id %q has blank question", candidate.ID)
		}
		countByDimension[candidate.DimensionID]++
		if countByDimension[candidate.DimensionID] > limits.MaxCandidatesPerDimension {
			return limitError(CodeInvalidQuestionGeneration, LimitDiagnostic{
				LimitName:       "max_candidates_per_dimension",
				ConfiguredLimit: limits.MaxCandidatesPerDimension,
				ObservedCount:   countByDimension[candidate.DimensionID],
				Stage:           "validate_candidate_questions",
			})
		}
	}
	if len(candidates) == 0 {
		return semanticError(CodeInvalidQuestionGeneration, "no candidate questions")
	}
	return nil
}

func ValidateWeights(candidates []CandidateQuestion, weights []Weight, limits ChecklistLimits) error {
	limits = limits.WithDefaults()
	candidateByID := make(map[string]CandidateQuestion, len(candidates))
	for _, candidate := range candidates {
		if candidate.ID == "" {
			return semanticError(CodeInvalidWeights, "candidate question has blank id")
		}
		if _, exists := candidateByID[candidate.ID]; exists {
			return semanticError(CodeInvalidWeights, "duplicate candidate question id %q", candidate.ID)
		}
		candidateByID[candidate.ID] = candidate
	}
	weightByID := make(map[string]Weight)
	for _, weight := range weights {
		if strings.TrimSpace(weight.CandidateQuestionID) == "" {
			return semanticError(CodeInvalidWeights, "weight has blank candidate_question_id")
		}
		if _, exists := candidateByID[weight.CandidateQuestionID]; !exists {
			return semanticError(CodeInvalidWeights, "weight references unknown candidate question id %q", weight.CandidateQuestionID)
		}
		if _, duplicate := weightByID[weight.CandidateQuestionID]; duplicate {
			return semanticError(CodeInvalidWeights, "duplicate weight for candidate question id %q", weight.CandidateQuestionID)
		}
		if strings.TrimSpace(weight.Rationale) == "" {
			return semanticError(CodeInvalidWeights, "weight for candidate question id %q has blank rationale", weight.CandidateQuestionID)
		}
		if weight.Weight < 0 || weight.Weight > limits.MaxSplitCount {
			return semanticError(CodeInvalidWeights, "weight for candidate question id %q is outside 0..%d", weight.CandidateQuestionID, limits.MaxSplitCount)
		}
		weightByID[weight.CandidateQuestionID] = weight
	}
	for _, candidate := range candidates {
		if _, ok := weightByID[candidate.ID]; !ok {
			return semanticError(CodeInvalidWeights, "missing weight for candidate question id %q", candidate.ID)
		}
	}
	return nil
}

func ValidateFinalQuestions(dimensions []Dimension, candidates []CandidateQuestion, questions []FinalQuestion, limits ChecklistLimits) error {
	limits = limits.WithDefaults()
	if len(questions) == 0 {
		return semanticError(CodeInvalidFinalChecklist, "at least one final question is required")
	}
	if len(questions) > limits.MaxFinalQuestions {
		return limitError(CodeInvalidFinalChecklist, LimitDiagnostic{
			LimitName:       "max_final_questions",
			ConfiguredLimit: limits.MaxFinalQuestions,
			ObservedCount:   len(questions),
			Stage:           "validate_final_questions",
		})
	}
	dimensionByID := make(map[string]struct{}, len(dimensions))
	for _, dimension := range dimensions {
		dimensionByID[dimension.ID] = struct{}{}
	}
	candidateByID := make(map[string]CandidateQuestion, len(candidates))
	for _, candidate := range candidates {
		candidateByID[candidate.ID] = candidate
	}
	seenID := make(map[string]struct{}, len(questions))
	seenOrdinal := make(map[int]struct{}, len(questions))
	for _, question := range questions {
		if strings.TrimSpace(question.ID) == "" {
			return semanticError(CodeInvalidFinalChecklist, "final question has blank id")
		}
		if _, exists := seenID[question.ID]; exists {
			return semanticError(CodeInvalidFinalChecklist, "duplicate final question id %q", question.ID)
		}
		seenID[question.ID] = struct{}{}
		if question.Ordinal <= 0 {
			return semanticError(CodeInvalidFinalChecklist, "final question id %q has invalid ordinal %d", question.ID, question.Ordinal)
		}
		if _, exists := seenOrdinal[question.Ordinal]; exists {
			return semanticError(CodeInvalidFinalChecklist, "duplicate final question ordinal %d", question.Ordinal)
		}
		seenOrdinal[question.Ordinal] = struct{}{}
		if _, ok := dimensionByID[question.DimensionID]; !ok {
			return semanticError(CodeInvalidFinalChecklist, "final question id %q references unknown dimension id %q", question.ID, question.DimensionID)
		}
		candidate, ok := candidateByID[question.SourceCandidateID]
		if !ok {
			return semanticError(CodeInvalidFinalChecklist, "final question id %q references unknown source candidate id %q", question.ID, question.SourceCandidateID)
		}
		if candidate.DimensionID != question.DimensionID {
			return semanticError(CodeInvalidFinalChecklist, "final question id %q dimension %q does not match source candidate dimension %q", question.ID, question.DimensionID, candidate.DimensionID)
		}
		if strings.TrimSpace(question.Rationale) == "" {
			return semanticError(CodeInvalidFinalChecklist, "final question id %q has blank rationale", question.ID)
		}
		if strings.TrimSpace(question.Question) == "" {
			return semanticError(CodeInvalidFinalChecklist, "final question id %q has blank question", question.ID)
		}
	}
	return nil
}

func ValidateSplitQuestions(split SplitQuestions, expectedCount int) error {
	if strings.TrimSpace(split.CandidateQuestionID) == "" {
		return semanticError(CodeInvalidSplits, "split has blank candidate_question_id")
	}
	if expectedCount <= 1 {
		return semanticError(CodeInvalidSplits, "split expected count must be greater than 1")
	}
	if len(split.Questions) != expectedCount {
		return semanticError(CodeInvalidSplits, "split for candidate question id %q returned %d questions, want %d", split.CandidateQuestionID, len(split.Questions), expectedCount)
	}
	for i, question := range split.Questions {
		if strings.TrimSpace(question.Rationale) == "" {
			return semanticError(CodeInvalidSplits, "split question %d for candidate id %q has blank rationale", i+1, split.CandidateQuestionID)
		}
		if strings.TrimSpace(question.Question) == "" {
			return semanticError(CodeInvalidSplits, "split question %d for candidate id %q has blank question", i+1, split.CandidateQuestionID)
		}
	}
	return nil
}

func ValidateJudgments(questions []FinalQuestion, judgments []Judgment) error {
	questionByID := make(map[string]FinalQuestion, len(questions))
	for _, question := range questions {
		if strings.TrimSpace(question.ID) == "" {
			return semanticError(CodeInvalidFinalChecklist, "final question has blank id")
		}
		if _, exists := questionByID[question.ID]; exists {
			return semanticError(CodeInvalidFinalChecklist, "duplicate final question id %q", question.ID)
		}
		questionByID[question.ID] = question
	}
	if len(questionByID) == 0 {
		return semanticError(CodeInvalidFinalChecklist, "at least one final question is required")
	}
	judgmentByID := make(map[string]Judgment)
	for _, judgment := range judgments {
		if _, exists := judgmentByID[judgment.QuestionID]; exists {
			return semanticError(CodeInvalidJudgments, "duplicate judgment for question id %q", judgment.QuestionID)
		}
		if _, active := questionByID[judgment.QuestionID]; !active {
			return semanticError(CodeInvalidJudgments, "judgment references unknown final question id %q", judgment.QuestionID)
		}
		if strings.TrimSpace(judgment.Evidence) == "" {
			return semanticError(CodeInvalidJudgments, "judgment for question id %q has blank evidence", judgment.QuestionID)
		}
		if judgment.Answer != AnswerYes && judgment.Answer != AnswerNo {
			return semanticError(CodeInvalidJudgments, "judgment for question id %q has invalid answer %q", judgment.QuestionID, judgment.Answer)
		}
		judgmentByID[judgment.QuestionID] = judgment
	}
	for _, question := range questions {
		if _, ok := judgmentByID[question.ID]; !ok {
			return semanticError(CodeInvalidJudgments, "missing judgment for final question id %q", question.ID)
		}
	}
	return nil
}
