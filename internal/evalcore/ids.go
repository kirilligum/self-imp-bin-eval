package evalcore

import "fmt"

func AssignQuestionIDs(drafts []DraftQuestion) []CandidateQuestion {
	questions := make([]CandidateQuestion, len(drafts))
	for i, draft := range drafts {
		questions[i] = CandidateQuestion{
			ID:        fmt.Sprintf("q%d", i+1),
			Ordinal:   i + 1,
			Rationale: draft.Rationale,
			Question:  draft.Question,
		}
	}
	return questions
}
