package evalcore

import "testing"

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

func TestValidateDimensionsDecisionTable(t *testing.T) {
	valid, _ := validRubricInputs()
	second := Dimension{ID: "d2", Ordinal: 2, Name: "Evidence", Rubric: "Check evidence.", Rationale: "Support."}

	for _, tc := range []struct {
		name       string
		dimensions []Dimension
		limits     ChecklistLimits
		limitName  string
	}{
		{name: "empty", dimensions: nil},
		{name: "blank id", dimensions: mutateDimensions(valid, func(d []Dimension) { d[0].ID = " " })},
		{name: "duplicate id", dimensions: append(cloneDimensions(valid), Dimension{ID: "d1", Ordinal: 2, Name: "Evidence", Rubric: "Check evidence.", Rationale: "Support."})},
		{name: "invalid ordinal", dimensions: mutateDimensions(valid, func(d []Dimension) { d[0].Ordinal = 0 })},
		{name: "blank name", dimensions: mutateDimensions(valid, func(d []Dimension) { d[0].Name = " " })},
		{name: "blank rubric", dimensions: mutateDimensions(valid, func(d []Dimension) { d[0].Rubric = " " })},
		{name: "blank rationale", dimensions: mutateDimensions(valid, func(d []Dimension) { d[0].Rationale = " " })},
		{name: "over max", dimensions: append(cloneDimensions(valid), second), limits: ChecklistLimits{MaxDimensions: 1}, limitName: "max_dimensions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateDimensions(tc.dimensions, tc.limits.WithDefaults())
			assertSemanticError(t, err, CodeInvalidDimensionAnalysis)
			if tc.limitName != "" {
				assertSingleLimitDiagnostic(t, err, tc.limitName)
			}
		})
	}
}

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

func TestValidateCandidateQuestionsDecisionTable(t *testing.T) {
	dimensions, candidates := validRubricInputs()

	for _, tc := range []struct {
		name       string
		candidates []CandidateQuestion
		limits     ChecklistLimits
		limitName  string
	}{
		{name: "empty", candidates: nil},
		{name: "blank id", candidates: mutateCandidates(candidates, func(c []CandidateQuestion) { c[0].ID = " " })},
		{name: "duplicate id", candidates: mutateCandidates(candidates, func(c []CandidateQuestion) { c[1].ID = c[0].ID })},
		{name: "unknown dimension", candidates: mutateCandidates(candidates, func(c []CandidateQuestion) { c[0].DimensionID = "d999" })},
		{name: "invalid ordinal", candidates: mutateCandidates(candidates, func(c []CandidateQuestion) { c[0].Ordinal = 0 })},
		{name: "blank rationale", candidates: mutateCandidates(candidates, func(c []CandidateQuestion) { c[0].Rationale = " " })},
		{name: "blank question", candidates: mutateCandidates(candidates, func(c []CandidateQuestion) { c[0].Question = "\n\t" })},
		{name: "over max per dimension", candidates: candidates[:2], limits: ChecklistLimits{MaxCandidatesPerDimension: 1}, limitName: "max_candidates_per_dimension"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCandidateQuestions(dimensions, tc.candidates, tc.limits.WithDefaults())
			assertSemanticError(t, err, CodeInvalidQuestionGeneration)
			if tc.limitName != "" {
				assertSingleLimitDiagnostic(t, err, tc.limitName)
			}
		})
	}
}

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

func TestValidateFinalQuestionsDecisionTable(t *testing.T) {
	dimensions, candidates := validRubricInputs()
	valid := []FinalQuestion{
		{ID: "q1", Ordinal: 1, DimensionID: "d1", SourceCandidateID: "c1", Rationale: "r1", Question: "Q1?"},
		{ID: "q2", Ordinal: 2, DimensionID: "d1", SourceCandidateID: "c2", Rationale: "r2", Question: "Q2?"},
	}

	for _, tc := range []struct {
		name       string
		dimensions []Dimension
		questions  []FinalQuestion
		limits     ChecklistLimits
		limitName  string
	}{
		{name: "empty", questions: nil},
		{name: "blank id", questions: mutateFinalQuestions(valid, func(q []FinalQuestion) { q[0].ID = " " })},
		{name: "duplicate id", questions: mutateFinalQuestions(valid, func(q []FinalQuestion) { q[1].ID = q[0].ID })},
		{name: "invalid ordinal", questions: mutateFinalQuestions(valid, func(q []FinalQuestion) { q[0].Ordinal = 0 })},
		{name: "duplicate ordinal", questions: mutateFinalQuestions(valid, func(q []FinalQuestion) { q[1].Ordinal = q[0].Ordinal })},
		{name: "unknown dimension", questions: mutateFinalQuestions(valid, func(q []FinalQuestion) { q[0].DimensionID = "d999" })},
		{name: "unknown source", questions: mutateFinalQuestions(valid, func(q []FinalQuestion) { q[0].SourceCandidateID = "c999" })},
		{name: "dimension mismatch", dimensions: append(cloneDimensions(dimensions), Dimension{ID: "d2", Ordinal: 2, Name: "Evidence", Rubric: "Check evidence.", Rationale: "Support."}), questions: mutateFinalQuestions(valid, func(q []FinalQuestion) { q[0].DimensionID = "d2" })},
		{name: "blank rationale", questions: mutateFinalQuestions(valid, func(q []FinalQuestion) { q[0].Rationale = " " })},
		{name: "blank question", questions: mutateFinalQuestions(valid, func(q []FinalQuestion) { q[0].Question = "\n\t" })},
		{name: "over max", questions: valid, limits: ChecklistLimits{MaxFinalQuestions: 1}, limitName: "max_final_questions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testDimensions := dimensions
			if tc.dimensions != nil {
				testDimensions = tc.dimensions
			}
			err := ValidateFinalQuestions(testDimensions, candidates, tc.questions, tc.limits.WithDefaults())
			assertSemanticError(t, err, CodeInvalidFinalChecklist)
			if tc.limitName != "" {
				assertSingleLimitDiagnostic(t, err, tc.limitName)
			}
		})
	}
}

func TestValidateSplitQuestionsDecisionTable(t *testing.T) {
	valid := SplitQuestions{CandidateQuestionID: "c1", Questions: []DraftQuestion{
		{Rationale: "r1", Question: "Q1?"},
		{Rationale: "r2", Question: "Q2?"},
	}}
	if err := ValidateSplitQuestions(valid, 2); err != nil {
		t.Fatalf("valid split error = %v", err)
	}

	for _, tc := range []struct {
		name          string
		split         SplitQuestions
		expectedCount int
	}{
		{name: "blank id", split: mutateSplit(valid, func(s *SplitQuestions) { s.CandidateQuestionID = " " }), expectedCount: 2},
		{name: "expected count too low", split: valid, expectedCount: 1},
		{name: "wrong count", split: mutateSplit(valid, func(s *SplitQuestions) { s.Questions = s.Questions[:1] }), expectedCount: 2},
		{name: "blank rationale", split: mutateSplit(valid, func(s *SplitQuestions) { s.Questions[0].Rationale = " " }), expectedCount: 2},
		{name: "blank question", split: mutateSplit(valid, func(s *SplitQuestions) { s.Questions[0].Question = "\n\t" }), expectedCount: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertSemanticError(t, ValidateSplitQuestions(tc.split, tc.expectedCount), CodeInvalidSplits)
		})
	}
}

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

func TestP06CompositionalWeightsAndEffectiveLimits(t *testing.T) {
	_, candidates := validRubricInputs()

	t.Run("fixed split scale rejects configured values above four", func(t *testing.T) {
		err := ValidateWeights(candidates[:1], []Weight{{CandidateQuestionID: "c1", Rationale: "invalid", Weight: 5}}, ChecklistLimits{MaxSplitCount: 8})
		assertSemanticError(t, err, CodeInvalidWeights)
	})

	t.Run("configured candidate limit replaces the default", func(t *testing.T) {
		drafts := make([]DraftQuestion, 10)
		for i := range drafts {
			drafts[i] = DraftQuestion{Rationale: "coverage", Question: "Question?"}
		}
		if err := ValidateQuestionGeneration(drafts, ChecklistLimits{MaxCandidatesPerDimension: 10}); err != nil {
			t.Fatalf("ValidateQuestionGeneration() error = %v", err)
		}
	})

	t.Run("projected final count reports the effective limit", func(t *testing.T) {
		weights := []Weight{
			{CandidateQuestionID: "c1", Rationale: "split", Weight: 4},
			{CandidateQuestionID: "c2", Rationale: "split", Weight: 2},
		}
		count, err := ValidateProjectedFinalQuestionCount("checklist-limit", weights, ChecklistLimits{MaxFinalQuestions: 5})
		if count != 6 {
			t.Fatalf("projected count = %d, want 6", count)
		}
		assertSemanticError(t, err, CodeInvalidFinalChecklist)
		assertSingleLimitDiagnostic(t, err, "max_final_questions")
		semantic := err.(*SemanticError)
		if semantic.Diagnostics[0].ChecklistID != "checklist-limit" || semantic.Diagnostics[0].Stage != "weight_assignment" {
			t.Fatalf("diagnostic = %#v", semantic.Diagnostics[0])
		}
	})
}

func cloneDimensions(in []Dimension) []Dimension {
	return append([]Dimension(nil), in...)
}

func mutateDimensions(in []Dimension, mutate func([]Dimension)) []Dimension {
	out := cloneDimensions(in)
	mutate(out)
	return out
}

func mutateCandidates(in []CandidateQuestion, mutate func([]CandidateQuestion)) []CandidateQuestion {
	out := append([]CandidateQuestion(nil), in...)
	mutate(out)
	return out
}

func mutateFinalQuestions(in []FinalQuestion, mutate func([]FinalQuestion)) []FinalQuestion {
	out := append([]FinalQuestion(nil), in...)
	mutate(out)
	return out
}

func mutateSplit(in SplitQuestions, mutate func(*SplitQuestions)) SplitQuestions {
	out := SplitQuestions{
		CandidateQuestionID: in.CandidateQuestionID,
		Questions:           append([]DraftQuestion(nil), in.Questions...),
	}
	mutate(&out)
	return out
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

func assertSingleLimitDiagnostic(t *testing.T, err error, limitName string) {
	t.Helper()
	semantic, ok := err.(*SemanticError)
	if !ok {
		t.Fatalf("error type = %T, want *SemanticError", err)
	}
	if len(semantic.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want exactly one", semantic.Diagnostics)
	}
	if semantic.Diagnostics[0].LimitName != limitName {
		t.Fatalf("limit name = %q, want %q", semantic.Diagnostics[0].LimitName, limitName)
	}
}
