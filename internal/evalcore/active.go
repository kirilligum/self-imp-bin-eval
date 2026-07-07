package evalcore

func BuildActiveChecklist(questions []CandidateQuestion, weights []Weight) ([]ActiveQuestion, error) {
	coverage, err := buildCoverage(questions, weights)
	if err != nil {
		return nil, err
	}
	if len(coverage.active) == 0 {
		return nil, semanticError(CodeInvalidWeights, "at least one active question is required")
	}
	return coverage.active, nil
}

type coverage struct {
	active       []ActiveQuestion
	activeByID   map[string]ActiveQuestion
	questionByID map[string]CandidateQuestion
}

func buildCoverage(questions []CandidateQuestion, weights []Weight) (coverage, error) {
	c := coverage{
		activeByID:   make(map[string]ActiveQuestion),
		questionByID: make(map[string]CandidateQuestion),
	}
	for _, question := range questions {
		if question.ID == "" {
			return c, semanticError(CodeInvalidWeights, "candidate question has blank id")
		}
		if _, exists := c.questionByID[question.ID]; exists {
			return c, semanticError(CodeInvalidWeights, "duplicate candidate question id %q", question.ID)
		}
		c.questionByID[question.ID] = question
	}
	weightByID := make(map[string]Weight)
	for _, weight := range weights {
		question, exists := c.questionByID[weight.QuestionID]
		if !exists {
			return c, semanticError(CodeInvalidWeights, "weight references unknown question id %q", weight.QuestionID)
		}
		if _, duplicate := weightByID[weight.QuestionID]; duplicate {
			return c, semanticError(CodeInvalidWeights, "duplicate weight for question id %q", weight.QuestionID)
		}
		if weight.Weight < 0 || weight.Weight > 4 {
			return c, semanticError(CodeInvalidWeights, "weight for question id %q is outside 0..4", weight.QuestionID)
		}
		weightByID[weight.QuestionID] = weight
		if weight.Weight > 0 {
			active := ActiveQuestion{
				ID:       question.ID,
				Ordinal:  question.Ordinal,
				Question: question.Question,
				Weight:   weight.Weight,
			}
			c.active = append(c.active, active)
			c.activeByID[active.ID] = active
		}
	}
	for _, question := range questions {
		if _, ok := weightByID[question.ID]; !ok {
			return c, semanticError(CodeInvalidWeights, "missing weight for question id %q", question.ID)
		}
	}
	return c, nil
}
