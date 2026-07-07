package evalcore

import "testing"

// TEST-002
func TestAssignQuestionIDs(t *testing.T) {
	drafts := []DraftQuestion{
		{Rationale: "first rationale", Question: "Does it mention alpha?"},
		{Rationale: "second rationale", Question: "Does it mention beta?"},
	}

	got := AssignQuestionIDs(drafts)
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	assertQuestion(t, got[0], "q1", 1, drafts[0].Rationale, drafts[0].Question)
	assertQuestion(t, got[1], "q2", 2, drafts[1].Rationale, drafts[1].Question)
}

// TEST-002
func TestBuildActiveChecklist(t *testing.T) {
	questions := AssignQuestionIDs([]DraftQuestion{
		{Rationale: "r1", Question: "Question 1?"},
		{Rationale: "r2", Question: "Question 2?"},
		{Rationale: "r3", Question: "Question 3?"},
	})

	t.Run("excludes weight zero and preserves stable ids", func(t *testing.T) {
		active, err := BuildActiveChecklist(questions, []Weight{
			{QuestionID: "q1", Rationale: "excluded duplicate", Weight: 0},
			{QuestionID: "q2", Rationale: "important", Weight: 4},
			{QuestionID: "q3", Rationale: "useful", Weight: 1},
		})
		if err != nil {
			t.Fatalf("BuildActiveChecklist() error = %v", err)
		}
		if len(active) != 2 {
			t.Fatalf("active len = %d", len(active))
		}
		if active[0].ID != "q2" || active[0].Weight != 4 {
			t.Fatalf("active[0] = %#v", active[0])
		}
		if active[1].ID != "q3" || active[1].Weight != 1 {
			t.Fatalf("active[1] = %#v", active[1])
		}
	})

	for _, tc := range []struct {
		name    string
		qs      []CandidateQuestion
		weights []Weight
	}{
		{
			name: "duplicate question id",
			qs: []CandidateQuestion{
				{ID: "q1", Ordinal: 1, Question: "A?"},
				{ID: "q1", Ordinal: 2, Question: "B?"},
			},
			weights: []Weight{{QuestionID: "q1", Weight: 1}},
		},
		{
			name:    "duplicate weight",
			qs:      questions,
			weights: []Weight{{QuestionID: "q1", Weight: 1}, {QuestionID: "q1", Weight: 2}, {QuestionID: "q2", Weight: 1}, {QuestionID: "q3", Weight: 1}},
		},
		{
			name:    "unknown weight id",
			qs:      questions,
			weights: []Weight{{QuestionID: "q1", Weight: 1}, {QuestionID: "q2", Weight: 1}, {QuestionID: "q3", Weight: 1}, {QuestionID: "q999", Weight: 1}},
		},
		{
			name:    "missing weight",
			qs:      questions,
			weights: []Weight{{QuestionID: "q1", Weight: 1}, {QuestionID: "q2", Weight: 1}},
		},
		{
			name:    "negative weight",
			qs:      questions,
			weights: []Weight{{QuestionID: "q1", Weight: -1}, {QuestionID: "q2", Weight: 1}, {QuestionID: "q3", Weight: 1}},
		},
		{
			name:    "too large weight",
			qs:      questions,
			weights: []Weight{{QuestionID: "q1", Weight: 5}, {QuestionID: "q2", Weight: 1}, {QuestionID: "q3", Weight: 1}},
		},
		{
			name:    "all zero",
			qs:      questions,
			weights: []Weight{{QuestionID: "q1", Weight: 0}, {QuestionID: "q2", Weight: 0}, {QuestionID: "q3", Weight: 0}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildActiveChecklist(tc.qs, tc.weights)
			assertSemanticError(t, err, CodeInvalidWeights)
		})
	}
}

func assertQuestion(t *testing.T, got CandidateQuestion, id string, ordinal int, rationale string, question string) {
	t.Helper()
	if got.ID != id || got.Ordinal != ordinal || got.Rationale != rationale || got.Question != question {
		t.Fatalf("question = %#v", got)
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
