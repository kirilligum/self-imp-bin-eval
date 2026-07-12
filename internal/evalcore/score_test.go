package evalcore

import "testing"

// TEST-002
func TestEvalcoreRubricRefinement(t *testing.T) {
	dimensions, candidates := validRubricInputs()
	weights := []Weight{
		{CandidateQuestionID: "c1", Rationale: "duplicate", Weight: 0},
		{CandidateQuestionID: "c2", Rationale: "normal", Weight: 1},
		{CandidateQuestionID: "c3", Rationale: "split into details", Weight: 2},
		{CandidateQuestionID: "c4", Rationale: "split into more details", Weight: 4},
	}
	splits := []SplitQuestions{
		{CandidateQuestionID: "c3", Questions: []DraftQuestion{
			{Rationale: "alpha detail", Question: "Does it mention alpha?"},
			{Rationale: "beta detail", Question: "Does it mention beta?"},
		}},
		{CandidateQuestionID: "c4", Questions: []DraftQuestion{
			{Rationale: "one", Question: "Does it cover one?"},
			{Rationale: "two", Question: "Does it cover two?"},
			{Rationale: "three", Question: "Does it cover three?"},
			{Rationale: "four", Question: "Does it cover four?"},
		}},
	}

	final, err := BuildFinalChecklist(dimensions, candidates, weights, splits, DefaultChecklistLimits())
	if err != nil {
		t.Fatalf("BuildFinalChecklist() error = %v", err)
	}
	if len(final) != 7 {
		t.Fatalf("final len = %d: %#v", len(final), final)
	}
	if final[0].ID != "q1" || final[0].SourceCandidateID != "c2" || final[0].Question != "Does it satisfy normal requirement?" {
		t.Fatalf("kept final question = %#v", final[0])
	}
	if final[1].ID != "q2" || final[1].SourceCandidateID != "c3" || final[1].Question != "Does it mention alpha?" {
		t.Fatalf("split final question = %#v", final[1])
	}
	if final[6].ID != "q7" || final[6].SourceCandidateID != "c4" {
		t.Fatalf("last final question = %#v", final[6])
	}

	score, err := ScoreChecklist(final, []Judgment{
		{QuestionID: "q1", Evidence: "normal ok", Answer: AnswerYes},
		{QuestionID: "q2", Evidence: "alpha ok", Answer: AnswerYes},
		{QuestionID: "q3", Evidence: "beta missing", Answer: AnswerNo},
		{QuestionID: "q4", Evidence: "one ok", Answer: AnswerYes},
		{QuestionID: "q5", Evidence: "two missing", Answer: AnswerNo},
		{QuestionID: "q6", Evidence: "three ok", Answer: AnswerYes},
		{QuestionID: "q7", Evidence: "four ok", Answer: AnswerYes},
	})
	if err != nil {
		t.Fatalf("ScoreChecklist() error = %v", err)
	}
	if score.SatisfiedPoints != 5 || score.TotalPossiblePoints != 7 {
		t.Fatalf("score = %#v", score)
	}
	if score.ChecklistPassRate != float64(5)/float64(7) {
		t.Fatalf("pass rate = %v", score.ChecklistPassRate)
	}
	if len(score.FailedQuestionIDs) != 2 || score.FailedQuestionIDs[0] != "q3" || score.FailedQuestionIDs[1] != "q5" {
		t.Fatalf("failed ids = %#v", score.FailedQuestionIDs)
	}
}

// TEST-002
func TestEvalcoreRubricRefinementValidation(t *testing.T) {
	dimensions, candidates := validRubricInputs()
	validWeights := []Weight{
		{CandidateQuestionID: "c1", Rationale: "keep", Weight: 1},
		{CandidateQuestionID: "c2", Rationale: "keep", Weight: 1},
		{CandidateQuestionID: "c3", Rationale: "split", Weight: 2},
		{CandidateQuestionID: "c4", Rationale: "delete", Weight: 0},
	}
	validSplits := []SplitQuestions{{CandidateQuestionID: "c3", Questions: []DraftQuestion{
		{Rationale: "a", Question: "A?"},
		{Rationale: "b", Question: "B?"},
	}}}

	for _, tc := range []struct {
		name    string
		weights []Weight
		splits  []SplitQuestions
		code    ErrorCode
	}{
		{
			name:    "all deleted",
			weights: []Weight{{CandidateQuestionID: "c1", Rationale: "x", Weight: 0}, {CandidateQuestionID: "c2", Rationale: "x", Weight: 0}, {CandidateQuestionID: "c3", Rationale: "x", Weight: 0}, {CandidateQuestionID: "c4", Rationale: "x", Weight: 0}},
			code:    CodeInvalidFinalChecklist,
		},
		{
			name:    "missing split",
			weights: validWeights,
			splits:  nil,
			code:    CodeInvalidSplits,
		},
		{
			name:    "wrong split count",
			weights: validWeights,
			splits:  []SplitQuestions{{CandidateQuestionID: "c3", Questions: []DraftQuestion{{Rationale: "a", Question: "A?"}}}},
			code:    CodeInvalidSplits,
		},
		{
			name:    "unknown split",
			weights: validWeights,
			splits:  append(validSplits, SplitQuestions{CandidateQuestionID: "c999", Questions: []DraftQuestion{{Rationale: "x", Question: "X?"}}}),
			code:    CodeInvalidSplits,
		},
		{
			name:    "duplicate weight",
			weights: append(validWeights, Weight{CandidateQuestionID: "c1", Rationale: "duplicate", Weight: 1}),
			splits:  validSplits,
			code:    CodeInvalidWeights,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildFinalChecklist(dimensions, candidates, tc.weights, tc.splits, DefaultChecklistLimits())
			assertSemanticError(t, err, tc.code)
		})
	}

	t.Run("over budget includes limit diagnostics", func(t *testing.T) {
		_, err := BuildFinalChecklist(dimensions, candidates, validWeights, validSplits, ChecklistLimits{
			MaxDimensions:             6,
			MaxCandidatesPerDimension: 8,
			MaxSplitCount:             4,
			MaxFinalQuestions:         2,
		})
		assertSemanticError(t, err, CodeInvalidFinalChecklist)
		semantic := err.(*SemanticError)
		if len(semantic.Diagnostics) != 1 || semantic.Diagnostics[0].LimitName != "max_final_questions" || semantic.Diagnostics[0].ConfiguredLimit != 2 {
			t.Fatalf("diagnostics = %#v", semantic.Diagnostics)
		}
	})
}

func validRubricInputs() ([]Dimension, []CandidateQuestion) {
	dimensions := []Dimension{
		{ID: "d1", Ordinal: 1, Name: "Correctness", Rubric: "Check correctness.", Rationale: "Core."},
	}
	candidates := []CandidateQuestion{
		{ID: "c1", DimensionID: "d1", Ordinal: 1, Rationale: "duplicate", Question: "Does it repeat duplicate detail?"},
		{ID: "c2", DimensionID: "d1", Ordinal: 2, Rationale: "normal", Question: "Does it satisfy normal requirement?"},
		{ID: "c3", DimensionID: "d1", Ordinal: 3, Rationale: "broad", Question: "Does it cover alpha and beta?"},
		{ID: "c4", DimensionID: "d1", Ordinal: 4, Rationale: "very broad", Question: "Does it cover four details?"},
	}
	return dimensions, candidates
}
