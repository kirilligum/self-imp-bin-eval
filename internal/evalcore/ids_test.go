package evalcore

import "testing"

func TestAssignDimensionAndCandidateIDs(t *testing.T) {
	dimensions := AssignDimensionIDs([]DraftDimension{
		{Name: "Correctness", Rubric: "Check correctness.", Rationale: "Core."},
		{Name: "Completeness", Rubric: "Check completeness.", Rationale: "Coverage."},
	})
	if len(dimensions) != 2 {
		t.Fatalf("dimension len = %d", len(dimensions))
	}
	if dimensions[0].ID != "d1" || dimensions[0].Ordinal != 1 || dimensions[1].ID != "d2" || dimensions[1].Ordinal != 2 {
		t.Fatalf("dimensions = %#v", dimensions)
	}

	candidates := AssignCandidateQuestionIDs("d2", 3, []DraftQuestion{
		{Rationale: "first rationale", Question: "Does it mention alpha?"},
		{Rationale: "second rationale", Question: "Does it mention beta?"},
	})
	if len(candidates) != 2 {
		t.Fatalf("candidate len = %d", len(candidates))
	}
	assertCandidate(t, candidates[0], "c3", "d2", 3, "first rationale", "Does it mention alpha?")
	assertCandidate(t, candidates[1], "c4", "d2", 4, "second rationale", "Does it mention beta?")
}

func assertCandidate(t *testing.T, got CandidateQuestion, id, dimensionID string, ordinal int, rationale, question string) {
	t.Helper()
	if got.ID != id || got.DimensionID != dimensionID || got.Ordinal != ordinal || got.Rationale != rationale || got.Question != question {
		t.Fatalf("candidate = %#v", got)
	}
}
