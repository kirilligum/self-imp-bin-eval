package evalcore

import "testing"

// TEST-002
func TestValidateDimensionGeneration(t *testing.T) {
	valid := []DraftDimension{{Name: "Correctness", Rubric: "Checks correctness.", Rationale: "Main scoring dimension."}}
	if err := ValidateDimensionGeneration(valid, DefaultChecklistLimits()); err != nil {
		t.Fatalf("valid dimension generation error = %v", err)
	}

	for _, tc := range []struct {
		name  string
		input []DraftDimension
	}{
		{name: "empty", input: nil},
		{name: "blank name", input: []DraftDimension{{Name: " ", Rubric: "r", Rationale: "why"}}},
		{name: "blank rubric", input: []DraftDimension{{Name: "n", Rubric: " ", Rationale: "why"}}},
		{name: "blank rationale", input: []DraftDimension{{Name: "n", Rubric: "r", Rationale: " "}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertSemanticError(t, ValidateDimensionGeneration(tc.input, DefaultChecklistLimits()), CodeInvalidDimensionAnalysis)
		})
	}
}

// TEST-002
func TestValidateQuestionGeneration(t *testing.T) {
	valid := []DraftQuestion{{Rationale: "Targets the main requirement.", Question: "Does the answer state the main requirement?"}}
	if err := ValidateQuestionGeneration(valid, DefaultChecklistLimits()); err != nil {
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
			assertSemanticError(t, ValidateQuestionGeneration(tc.drafts, DefaultChecklistLimits()), CodeInvalidQuestionGeneration)
		})
	}
}

// TEST-002
func TestValidateWeights(t *testing.T) {
	_, candidates := validRubricInputs()
	if err := ValidateWeights(candidates, []Weight{
		{CandidateQuestionID: "c1", Rationale: "duplicate", Weight: 0},
		{CandidateQuestionID: "c2", Rationale: "normal", Weight: 1},
		{CandidateQuestionID: "c3", Rationale: "split", Weight: 2},
		{CandidateQuestionID: "c4", Rationale: "split", Weight: 4},
	}, DefaultChecklistLimits()); err != nil {
		t.Fatalf("valid weights error = %v", err)
	}

	for _, tc := range []struct {
		name    string
		weights []Weight
	}{
		{name: "missing", weights: []Weight{{CandidateQuestionID: "c1", Rationale: "r", Weight: 1}}},
		{name: "duplicate", weights: []Weight{{CandidateQuestionID: "c1", Rationale: "r", Weight: 1}, {CandidateQuestionID: "c1", Rationale: "r", Weight: 2}, {CandidateQuestionID: "c2", Rationale: "r", Weight: 1}, {CandidateQuestionID: "c3", Rationale: "r", Weight: 1}, {CandidateQuestionID: "c4", Rationale: "r", Weight: 1}}},
		{name: "unknown", weights: []Weight{{CandidateQuestionID: "c1", Rationale: "r", Weight: 1}, {CandidateQuestionID: "c2", Rationale: "r", Weight: 1}, {CandidateQuestionID: "c3", Rationale: "r", Weight: 1}, {CandidateQuestionID: "c4", Rationale: "r", Weight: 1}, {CandidateQuestionID: "cx", Rationale: "r", Weight: 1}}},
		{name: "blank rationale", weights: []Weight{{CandidateQuestionID: "c1", Rationale: " ", Weight: 1}, {CandidateQuestionID: "c2", Rationale: "r", Weight: 1}, {CandidateQuestionID: "c3", Rationale: "r", Weight: 1}, {CandidateQuestionID: "c4", Rationale: "r", Weight: 1}}},
		{name: "negative", weights: []Weight{{CandidateQuestionID: "c1", Rationale: "r", Weight: -1}, {CandidateQuestionID: "c2", Rationale: "r", Weight: 1}, {CandidateQuestionID: "c3", Rationale: "r", Weight: 1}, {CandidateQuestionID: "c4", Rationale: "r", Weight: 1}}},
		{name: "too high", weights: []Weight{{CandidateQuestionID: "c1", Rationale: "r", Weight: 5}, {CandidateQuestionID: "c2", Rationale: "r", Weight: 1}, {CandidateQuestionID: "c3", Rationale: "r", Weight: 1}, {CandidateQuestionID: "c4", Rationale: "r", Weight: 1}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertSemanticError(t, ValidateWeights(candidates, tc.weights, DefaultChecklistLimits()), CodeInvalidWeights)
		})
	}
}

// TEST-002
func TestValidateJudgments(t *testing.T) {
	questions := []FinalQuestion{
		{ID: "q1", Ordinal: 1, DimensionID: "d1", SourceCandidateID: "c1", Rationale: "r", Question: "Q1?"},
		{ID: "q2", Ordinal: 2, DimensionID: "d1", SourceCandidateID: "c2", Rationale: "r", Question: "Q2?"},
	}
	if err := ValidateJudgments(questions, []Judgment{
		{QuestionID: "q1", Evidence: "The answer explicitly includes it.", Answer: AnswerYes},
		{QuestionID: "q2", Evidence: "The answer misses it.", Answer: AnswerNo},
	}); err != nil {
		t.Fatalf("valid judgments error = %v", err)
	}

	for _, tc := range []struct {
		name      string
		judgments []Judgment
	}{
		{name: "missing final judgment", judgments: []Judgment{{QuestionID: "q1", Evidence: "One", Answer: AnswerYes}}},
		{name: "duplicate judgment", judgments: []Judgment{
			{QuestionID: "q1", Evidence: "One", Answer: AnswerYes},
			{QuestionID: "q1", Evidence: "Two", Answer: AnswerNo},
			{QuestionID: "q2", Evidence: "Two", Answer: AnswerNo},
		}},
		{name: "unknown judgment id", judgments: []Judgment{{QuestionID: "qx", Evidence: "No such question.", Answer: AnswerNo}, {QuestionID: "q2", Evidence: "Active.", Answer: AnswerYes}}},
		{name: "empty evidence", judgments: []Judgment{{QuestionID: "q1", Evidence: "", Answer: AnswerYes}, {QuestionID: "q2", Evidence: "Two", Answer: AnswerNo}}},
		{name: "invalid answer", judgments: []Judgment{{QuestionID: "q1", Evidence: "Evidence.", Answer: "maybe"}, {QuestionID: "q2", Evidence: "Two", Answer: AnswerNo}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertSemanticError(t, ValidateJudgments(questions, tc.judgments), CodeInvalidJudgments)
		})
	}
}

func assertSemanticError(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	semantic, ok := err.(*SemanticError)
	if !ok {
		t.Fatalf("error type = %T, want *SemanticError: %v", err, err)
	}
	if semantic.Code != code {
		t.Fatalf("error code = %q, want %q: %v", semantic.Code, code, err)
	}
}
