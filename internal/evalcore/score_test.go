package evalcore

import "testing"

// TEST-001
func TestScoreChecklist(t *testing.T) {
	questions := AssignQuestionIDs([]DraftQuestion{
		{Rationale: "Excluded duplicate.", Question: "Does it include duplicate detail?"},
		{Rationale: "Must mention alpha.", Question: "Does it mention alpha?"},
		{Rationale: "Should mention beta.", Question: "Does it mention beta?"},
	})

	result, err := ScoreChecklist(questions, []Weight{
		{QuestionID: "q1", Weight: 0},
		{QuestionID: "q2", Weight: 4},
		{QuestionID: "q3", Weight: 2},
	}, []Judgment{
		{QuestionID: "q2", Evidence: "Alpha is present.", Answer: AnswerYes},
		{QuestionID: "q3", Evidence: "Beta is absent.", Answer: AnswerNo},
	})
	if err != nil {
		t.Fatalf("ScoreChecklist() error = %v", err)
	}
	if result.SatisfiedPoints != 4 {
		t.Fatalf("SatisfiedPoints = %d", result.SatisfiedPoints)
	}
	if result.TotalPossiblePoints != 6 {
		t.Fatalf("TotalPossiblePoints = %d", result.TotalPossiblePoints)
	}
	if result.ChecklistPassRate != float64(4)/float64(6) {
		t.Fatalf("ChecklistPassRate = %v", result.ChecklistPassRate)
	}
	if len(result.FailedQuestionIDs) != 1 || result.FailedQuestionIDs[0] != "q3" {
		t.Fatalf("FailedQuestionIDs = %#v", result.FailedQuestionIDs)
	}

	for _, tc := range []struct {
		name      string
		weights   []Weight
		judgments []Judgment
		code      ErrorCode
	}{
		{
			name:    "all zero failure",
			weights: []Weight{{QuestionID: "q1", Weight: 0}, {QuestionID: "q2", Weight: 0}, {QuestionID: "q3", Weight: 0}},
			code:    CodeInvalidWeights,
		},
		{
			name:    "missing active judgment",
			weights: []Weight{{QuestionID: "q1", Weight: 0}, {QuestionID: "q2", Weight: 4}, {QuestionID: "q3", Weight: 2}},
			judgments: []Judgment{
				{QuestionID: "q2", Evidence: "Alpha.", Answer: AnswerYes},
			},
			code: CodeInvalidJudgments,
		},
		{
			name:    "inactive judgment rejected",
			weights: []Weight{{QuestionID: "q1", Weight: 0}, {QuestionID: "q2", Weight: 4}, {QuestionID: "q3", Weight: 2}},
			judgments: []Judgment{
				{QuestionID: "q1", Evidence: "Excluded.", Answer: AnswerYes},
				{QuestionID: "q2", Evidence: "Alpha.", Answer: AnswerYes},
				{QuestionID: "q3", Evidence: "Beta.", Answer: AnswerYes},
			},
			code: CodeInvalidJudgments,
		},
		{
			name:    "invalid answer",
			weights: []Weight{{QuestionID: "q1", Weight: 0}, {QuestionID: "q2", Weight: 4}, {QuestionID: "q3", Weight: 2}},
			judgments: []Judgment{
				{QuestionID: "q2", Evidence: "Alpha.", Answer: "maybe"},
				{QuestionID: "q3", Evidence: "Beta.", Answer: AnswerYes},
			},
			code: CodeInvalidJudgments,
		},
		{
			name:    "duplicate weight",
			weights: []Weight{{QuestionID: "q1", Weight: 0}, {QuestionID: "q2", Weight: 4}, {QuestionID: "q2", Weight: 2}, {QuestionID: "q3", Weight: 2}},
			code:    CodeInvalidWeights,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ScoreChecklist(questions, tc.weights, tc.judgments)
			assertSemanticError(t, err, tc.code)
		})
	}
}
