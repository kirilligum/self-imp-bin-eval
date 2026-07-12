package evalcore

import "fmt"

func AssignDimensionIDs(drafts []DraftDimension) []Dimension {
	dimensions := make([]Dimension, len(drafts))
	for i, draft := range drafts {
		dimensions[i] = Dimension{
			ID:        fmt.Sprintf("d%d", i+1),
			Ordinal:   i + 1,
			Name:      draft.Name,
			Rubric:    draft.Rubric,
			Rationale: draft.Rationale,
		}
	}
	return dimensions
}

func AssignCandidateQuestionIDs(dimensionID string, startOrdinal int, drafts []DraftQuestion) []CandidateQuestion {
	questions := make([]CandidateQuestion, len(drafts))
	for i, draft := range drafts {
		ordinal := startOrdinal + i
		questions[i] = CandidateQuestion{
			ID:          fmt.Sprintf("c%d", ordinal),
			DimensionID: dimensionID,
			Ordinal:     ordinal,
			Rationale:   draft.Rationale,
			Question:    draft.Question,
		}
	}
	return questions
}
