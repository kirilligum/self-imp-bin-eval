package evalcore

import "testing"

// TEST-003
func TestValidateQuestionGeneration(t *testing.T) {
	valid := []DraftQuestion{{Rationale: "Targets the main requirement.", Question: "Does the answer state the main requirement?"}}
	if err := ValidateQuestionGeneration(valid); err != nil {
		t.Fatalf("valid question generation error = %v", err)
	}

	for _, tc := range []struct {
		name   string
		drafts []DraftQuestion
	}{
		{name: "empty", drafts: nil},
		{name: "blank rationale", drafts: []DraftQuestion{{Rationale: " ", Question: "Does it work?"}}},
		{name: "blank question", drafts: []DraftQuestion{{Rationale: "Reason", Question: "\n\t"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertSemanticError(t, ValidateQuestionGeneration(tc.drafts), CodeInvalidQuestionGeneration)
		})
	}
}

// TEST-004
func TestValidateWeights(t *testing.T) {
	questions := validQuestions()
	if err := ValidateWeights(questions, []Weight{
		{QuestionID: "q1", Rationale: "duplicate", Weight: 0},
		{QuestionID: "q2", Rationale: "central", Weight: 4},
	}); err != nil {
		t.Fatalf("valid weights error = %v", err)
	}

	for _, tc := range []struct {
		name    string
		weights []Weight
	}{
		{name: "missing", weights: []Weight{{QuestionID: "q1", Weight: 1}}},
		{name: "duplicate", weights: []Weight{{QuestionID: "q1", Weight: 1}, {QuestionID: "q1", Weight: 2}, {QuestionID: "q2", Weight: 1}}},
		{name: "unknown", weights: []Weight{{QuestionID: "q1", Weight: 1}, {QuestionID: "q2", Weight: 1}, {QuestionID: "qx", Weight: 1}}},
		{name: "negative", weights: []Weight{{QuestionID: "q1", Weight: -1}, {QuestionID: "q2", Weight: 1}}},
		{name: "too high", weights: []Weight{{QuestionID: "q1", Weight: 5}, {QuestionID: "q2", Weight: 1}}},
		{name: "all zero", weights: []Weight{{QuestionID: "q1", Weight: 0}, {QuestionID: "q2", Weight: 0}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertSemanticError(t, ValidateWeights(questions, tc.weights), CodeInvalidWeights)
		})
	}
}

// TEST-005
func TestValidateJudgments(t *testing.T) {
	questions := validQuestions()
	weights := []Weight{{QuestionID: "q1", Weight: 0}, {QuestionID: "q2", Weight: 4}}
	if err := ValidateJudgments(questions, weights, []Judgment{{QuestionID: "q2", Evidence: "The answer explicitly includes it.", Answer: AnswerYes}}); err != nil {
		t.Fatalf("valid judgments error = %v", err)
	}

	for _, tc := range []struct {
		name      string
		judgments []Judgment
	}{
		{name: "missing active judgment", judgments: nil},
		{name: "duplicate judgment", judgments: []Judgment{
			{QuestionID: "q2", Evidence: "One", Answer: AnswerYes},
			{QuestionID: "q2", Evidence: "Two", Answer: AnswerNo},
		}},
		{name: "inactive judgment rejected", judgments: []Judgment{
			{QuestionID: "q1", Evidence: "Excluded question evidence.", Answer: AnswerYes},
			{QuestionID: "q2", Evidence: "Active evidence.", Answer: AnswerYes},
		}},
		{name: "unknown judgment id", judgments: []Judgment{{QuestionID: "qx", Evidence: "No such question.", Answer: AnswerNo}, {QuestionID: "q2", Evidence: "Active.", Answer: AnswerYes}}},
		{name: "empty evidence", judgments: []Judgment{{QuestionID: "q2", Evidence: "", Answer: AnswerYes}}},
		{name: "whitespace evidence", judgments: []Judgment{{QuestionID: "q2", Evidence: " \n\t", Answer: AnswerYes}}},
		{name: "invalid answer", judgments: []Judgment{{QuestionID: "q2", Evidence: "Evidence.", Answer: "maybe"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertSemanticError(t, ValidateJudgments(questions, weights, tc.judgments), CodeInvalidJudgments)
		})
	}
}

func validQuestions() []CandidateQuestion {
	return AssignQuestionIDs([]DraftQuestion{
		{Rationale: "Duplicate detail.", Question: "Does it avoid duplicate details?"},
		{Rationale: "Central requirement.", Question: "Does it satisfy the central requirement?"},
	})
}
