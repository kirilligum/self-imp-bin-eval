package evalcore

import "fmt"

func BuildFinalChecklist(dimensions []Dimension, candidates []CandidateQuestion, weights []Weight, splits []SplitQuestions, limits ChecklistLimits) ([]FinalQuestion, error) {
	limits = limits.WithDefaults()
	if err := ValidateDimensions(dimensions, limits); err != nil {
		return nil, err
	}
	if err := ValidateCandidateQuestions(dimensions, candidates, limits); err != nil {
		return nil, err
	}
	if err := ValidateWeights(candidates, weights, limits); err != nil {
		return nil, err
	}

	candidateByID := make(map[string]CandidateQuestion, len(candidates))
	for _, candidate := range candidates {
		candidateByID[candidate.ID] = candidate
	}
	weightByCandidateID := make(map[string]Weight, len(weights))
	for _, weight := range weights {
		weightByCandidateID[weight.CandidateQuestionID] = weight
	}
	splitByCandidateID := make(map[string]SplitQuestions, len(splits))
	for _, split := range splits {
		candidate, ok := candidateByID[split.CandidateQuestionID]
		if !ok {
			return nil, semanticError(CodeInvalidSplits, "split references unknown candidate question id %q", split.CandidateQuestionID)
		}
		weight := weightByCandidateID[split.CandidateQuestionID]
		if weight.Weight <= 1 {
			return nil, semanticError(CodeInvalidSplits, "split references candidate id %q with weight %d", split.CandidateQuestionID, weight.Weight)
		}
		if _, exists := splitByCandidateID[split.CandidateQuestionID]; exists {
			return nil, semanticError(CodeInvalidSplits, "duplicate split for candidate question id %q", split.CandidateQuestionID)
		}
		if err := ValidateSplitQuestions(split, weight.Weight); err != nil {
			return nil, err
		}
		_ = candidate
		splitByCandidateID[split.CandidateQuestionID] = split
	}

	final := make([]FinalQuestion, 0, len(candidates))
	for _, candidate := range candidates {
		weight := weightByCandidateID[candidate.ID]
		switch {
		case weight.Weight == 0:
			continue
		case weight.Weight == 1:
			final = append(final, FinalQuestion{
				DimensionID:       candidate.DimensionID,
				SourceCandidateID: candidate.ID,
				Rationale:         candidate.Rationale,
				Question:          candidate.Question,
			})
		default:
			split, ok := splitByCandidateID[candidate.ID]
			if !ok {
				return nil, semanticError(CodeInvalidSplits, "missing split output for candidate question id %q", candidate.ID)
			}
			for _, question := range split.Questions {
				final = append(final, FinalQuestion{
					DimensionID:       candidate.DimensionID,
					SourceCandidateID: candidate.ID,
					Rationale:         question.Rationale,
					Question:          question.Question,
				})
			}
		}
	}
	if len(final) == 0 {
		return nil, semanticError(CodeInvalidFinalChecklist, "at least one final question is required")
	}
	if len(final) > limits.MaxFinalQuestions {
		return nil, limitError(CodeInvalidFinalChecklist, LimitDiagnostic{
			LimitName:       "max_final_questions",
			ConfiguredLimit: limits.MaxFinalQuestions,
			ObservedCount:   len(final),
			Stage:           "build_final_checklist",
		})
	}
	for i := range final {
		final[i].ID = fmt.Sprintf("q%d", i+1)
		final[i].Ordinal = i + 1
	}
	if err := ValidateFinalQuestions(dimensions, candidates, final, limits); err != nil {
		return nil, err
	}
	return final, nil
}
