package workflows

import (
	"testing"

	"github.com/kirilligum/self-imp-bin-eval/internal/activities"
	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
)

func TestCreateChecklistWorkflow(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(CreateChecklistWorkflow)
	registerActivityNames(env)

	limits := evalcore.DefaultChecklistLimits()
	input := CreateChecklistInput{ChecklistID: "checklist-1", Task: "task", Context: "context", Limits: limits}
	dimensions := []evalcore.Dimension{
		{ID: "d1", Ordinal: 1, Name: "Correctness", Rubric: "Check correctness.", Rationale: "Core."},
		{ID: "d2", Ordinal: 2, Name: "Evidence", Rubric: "Check evidence.", Rationale: "Support."},
	}
	d1Drafts := []evalcore.DraftQuestion{{Rationale: "normal", Question: "Normal?"}}
	d2Drafts := []evalcore.DraftQuestion{{Rationale: "broad", Question: "Broad?"}}
	candidates := []evalcore.CandidateQuestion{
		{ID: "c1", DimensionID: "d1", Ordinal: 1, Rationale: "normal", Question: "Normal?"},
		{ID: "c2", DimensionID: "d2", Ordinal: 2, Rationale: "broad", Question: "Broad?"},
	}
	weights := []evalcore.Weight{
		{CandidateQuestionID: "c1", Weight: 1, Rationale: "normal"},
		{CandidateQuestionID: "c2", Weight: 2, Rationale: "split"},
	}
	split := evalcore.SplitQuestions{CandidateQuestionID: "c2", Questions: []evalcore.DraftQuestion{
		{Rationale: "detail a", Question: "Specific A?"},
		{Rationale: "detail b", Question: "Specific B?"},
	}}
	finalQuestions := []evalcore.FinalQuestion{
		{ID: "q1", Ordinal: 1, DimensionID: "d1", SourceCandidateID: "c1", Rationale: "normal", Question: "Normal?"},
		{ID: "q2", Ordinal: 2, DimensionID: "d2", SourceCandidateID: "c2", Rationale: "detail a", Question: "Specific A?"},
		{ID: "q3", Ordinal: 3, DimensionID: "d2", SourceCandidateID: "c2", Rationale: "detail b", Question: "Specific B?"},
	}

	env.OnActivity(activities.ActivityWriteChecklistInputs, mock.Anything, activities.WriteChecklistInputsInput{
		ChecklistID: "checklist-1", Task: "task", Context: "context",
	}).Return(nil).Once()
	env.OnActivity(activities.ActivityAnalyzeDimensions, mock.Anything, activities.AnalyzeDimensionsInput{
		ChecklistID: "checklist-1", Task: "task", Context: "context", Limits: limits,
	}).Return(activities.AnalyzeDimensionsResult{Dimensions: dimensions}, nil).Once()
	env.OnActivity(activities.ActivityGenerateQuestionsForDimension, mock.Anything, activities.GenerateQuestionsForDimensionInput{
		ChecklistID: "checklist-1", Task: "task", Context: "context", Dimension: dimensions[0], Limits: limits,
	}).Return(activities.GenerateQuestionsForDimensionResult{Questions: d1Drafts}, nil).Once()
	env.OnActivity(activities.ActivityGenerateQuestionsForDimension, mock.Anything, activities.GenerateQuestionsForDimensionInput{
		ChecklistID: "checklist-1", Task: "task", Context: "context", Dimension: dimensions[1], Limits: limits,
	}).Return(activities.GenerateQuestionsForDimensionResult{Questions: d2Drafts}, nil).Once()
	env.OnActivity(activities.ActivityAssignWeights, mock.Anything, activities.AssignWeightsInput{
		ChecklistID: "checklist-1", Task: "task", Context: "context", CandidateQuestions: candidates, Limits: limits,
	}).Return(activities.AssignWeightsResult{Weights: weights}, nil).Once()
	env.OnActivity(activities.ActivitySplitQuestion, mock.Anything, activities.SplitQuestionInput{
		ChecklistID: "checklist-1", Task: "task", Context: "context", CandidateQuestion: candidates[1], Weight: weights[1], Limits: limits,
	}).Return(activities.SplitQuestionResult{Split: split}, nil).Once()
	env.OnActivity(activities.ActivitySucceedChecklist, mock.Anything, activities.SucceedChecklistInput{
		ChecklistID: "checklist-1", Dimensions: dimensions, CandidateQuestions: candidates, Weights: weights, Questions: finalQuestions, Limits: limits,
	}).Return(nil).Once()

	env.ExecuteWorkflow(CreateChecklistWorkflow, input)
	if !env.IsWorkflowCompleted() || env.GetWorkflowError() != nil {
		t.Fatalf("workflow error = %v", env.GetWorkflowError())
	}
	env.AssertExpectations(t)
}

func TestWorkflowFailurePersistence(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(CreateChecklistWorkflow)
	registerActivityNames(env)

	limits := evalcore.DefaultChecklistLimits()
	input := CreateChecklistInput{ChecklistID: "checklist-fail", Task: "task", Context: "context", Limits: limits}
	dimensions := []evalcore.Dimension{{ID: "d1", Ordinal: 1, Name: "Correctness", Rubric: "Check correctness.", Rationale: "Core."}}
	drafts := []evalcore.DraftQuestion{{Rationale: "r", Question: "Q?"}}
	allZero := []evalcore.Weight{{CandidateQuestionID: "c1", Rationale: "excluded", Weight: 0}}

	env.OnActivity(activities.ActivityWriteChecklistInputs, mock.Anything, mock.Anything).Return(nil).Once()
	env.OnActivity(activities.ActivityAnalyzeDimensions, mock.Anything, mock.Anything).Return(activities.AnalyzeDimensionsResult{Dimensions: dimensions}, nil).Once()
	env.OnActivity(activities.ActivityGenerateQuestionsForDimension, mock.Anything, mock.Anything).Return(activities.GenerateQuestionsForDimensionResult{Questions: drafts}, nil).Once()
	env.OnActivity(activities.ActivityAssignWeights, mock.Anything, mock.Anything).Return(activities.AssignWeightsResult{Weights: allZero}, nil).Once()
	env.OnActivity(activities.ActivityFailChecklist, mock.Anything, mock.MatchedBy(func(in activities.FailChecklistInput) bool {
		return in.ChecklistID == "checklist-fail" && in.Failure.Message != ""
	})).Return(nil).Once()

	env.ExecuteWorkflow(CreateChecklistWorkflow, input)
	if env.GetWorkflowError() == nil {
		t.Fatal("expected workflow failure")
	}
	env.AssertExpectations(t)
}

func TestCreateChecklistWorkflowFailureMatrix(t *testing.T) {
	t.Run("invalid analyzed dimensions fail before question generation", func(t *testing.T) {
		env := newCreateChecklistTestEnv()
		limits := evalcore.DefaultChecklistLimits()
		input := CreateChecklistInput{ChecklistID: "checklist-invalid-dimensions", Task: "task", Context: "context", Limits: limits}

		env.OnActivity(activities.ActivityWriteChecklistInputs, mock.Anything, mock.Anything).Return(nil).Once()
		env.OnActivity(activities.ActivityAnalyzeDimensions, mock.Anything, mock.Anything).Return(activities.AnalyzeDimensionsResult{}, nil).Once()
		expectFailChecklist(env, input.ChecklistID)

		env.ExecuteWorkflow(CreateChecklistWorkflow, input)
		if env.GetWorkflowError() == nil {
			t.Fatal("expected workflow failure")
		}
		env.AssertExpectations(t)
	})

	t.Run("question generation activity failure persists failed checklist", func(t *testing.T) {
		env := newCreateChecklistTestEnv()
		limits := evalcore.DefaultChecklistLimits()
		input := CreateChecklistInput{ChecklistID: "checklist-generate-fail", Task: "task", Context: "context", Limits: limits}
		dimensions := []evalcore.Dimension{{ID: "d1", Ordinal: 1, Name: "Correctness", Rubric: "Check correctness.", Rationale: "Core."}}

		env.OnActivity(activities.ActivityWriteChecklistInputs, mock.Anything, mock.Anything).Return(nil).Once()
		env.OnActivity(activities.ActivityAnalyzeDimensions, mock.Anything, mock.Anything).Return(activities.AnalyzeDimensionsResult{Dimensions: dimensions}, nil).Once()
		env.OnActivity(activities.ActivityGenerateQuestionsForDimension, mock.Anything, mock.Anything).Return(activities.GenerateQuestionsForDimensionResult{}, nonRetryableActivityError("generation failed")).Once()
		expectFailChecklist(env, input.ChecklistID)

		env.ExecuteWorkflow(CreateChecklistWorkflow, input)
		if env.GetWorkflowError() == nil {
			t.Fatal("expected workflow failure")
		}
		env.AssertExpectations(t)
	})

	t.Run("split activity failure persists failed checklist", func(t *testing.T) {
		env := newCreateChecklistTestEnv()
		limits := evalcore.DefaultChecklistLimits()
		input := CreateChecklistInput{ChecklistID: "checklist-split-fail", Task: "task", Context: "context", Limits: limits}
		dimensions := []evalcore.Dimension{{ID: "d1", Ordinal: 1, Name: "Correctness", Rubric: "Check correctness.", Rationale: "Core."}}
		drafts := []evalcore.DraftQuestion{{Rationale: "broad", Question: "Broad?"}}
		candidates := []evalcore.CandidateQuestion{{ID: "c1", DimensionID: "d1", Ordinal: 1, Rationale: "broad", Question: "Broad?"}}
		weights := []evalcore.Weight{{CandidateQuestionID: "c1", Rationale: "split", Weight: 2}}

		env.OnActivity(activities.ActivityWriteChecklistInputs, mock.Anything, mock.Anything).Return(nil).Once()
		env.OnActivity(activities.ActivityAnalyzeDimensions, mock.Anything, mock.Anything).Return(activities.AnalyzeDimensionsResult{Dimensions: dimensions}, nil).Once()
		env.OnActivity(activities.ActivityGenerateQuestionsForDimension, mock.Anything, mock.Anything).Return(activities.GenerateQuestionsForDimensionResult{Questions: drafts}, nil).Once()
		env.OnActivity(activities.ActivityAssignWeights, mock.Anything, mock.Anything).Return(activities.AssignWeightsResult{Weights: weights}, nil).Once()
		env.OnActivity(activities.ActivitySplitQuestion, mock.Anything, activities.SplitQuestionInput{
			ChecklistID:       input.ChecklistID,
			Task:              input.Task,
			Context:           input.Context,
			CandidateQuestion: candidates[0],
			Weight:            weights[0],
			Limits:            limits,
		}).Return(activities.SplitQuestionResult{}, nonRetryableActivityError("split failed")).Once()
		expectFailChecklist(env, input.ChecklistID)

		env.ExecuteWorkflow(CreateChecklistWorkflow, input)
		if env.GetWorkflowError() == nil {
			t.Fatal("expected workflow failure")
		}
		env.AssertExpectations(t)
	})

	t.Run("succeed checklist activity failure is persisted as failed", func(t *testing.T) {
		env := newCreateChecklistTestEnv()
		limits := evalcore.DefaultChecklistLimits()
		input := CreateChecklistInput{ChecklistID: "checklist-succeed-fail", Task: "task", Context: "context", Limits: limits}
		dimensions := []evalcore.Dimension{{ID: "d1", Ordinal: 1, Name: "Correctness", Rubric: "Check correctness.", Rationale: "Core."}}
		drafts := []evalcore.DraftQuestion{{Rationale: "normal", Question: "Normal?"}}
		weights := []evalcore.Weight{{CandidateQuestionID: "c1", Rationale: "normal", Weight: 1}}

		env.OnActivity(activities.ActivityWriteChecklistInputs, mock.Anything, mock.Anything).Return(nil).Once()
		env.OnActivity(activities.ActivityAnalyzeDimensions, mock.Anything, mock.Anything).Return(activities.AnalyzeDimensionsResult{Dimensions: dimensions}, nil).Once()
		env.OnActivity(activities.ActivityGenerateQuestionsForDimension, mock.Anything, mock.Anything).Return(activities.GenerateQuestionsForDimensionResult{Questions: drafts}, nil).Once()
		env.OnActivity(activities.ActivityAssignWeights, mock.Anything, mock.Anything).Return(activities.AssignWeightsResult{Weights: weights}, nil).Once()
		env.OnActivity(activities.ActivitySucceedChecklist, mock.Anything, mock.Anything).Return(nonRetryableActivityError("terminal write failed")).Once()
		expectFailChecklist(env, input.ChecklistID)

		env.ExecuteWorkflow(CreateChecklistWorkflow, input)
		if env.GetWorkflowError() == nil {
			t.Fatal("expected workflow failure")
		}
		env.AssertExpectations(t)
	})
}

func TestP06CompositionalWeightsAndEffectiveLimits(t *testing.T) {
	env := newCreateChecklistTestEnv()
	limits := evalcore.ChecklistLimits{
		MaxDimensions:             6,
		MaxCandidatesPerDimension: 8,
		MaxSplitCount:             4,
		MaxFinalQuestions:         1,
	}
	input := CreateChecklistInput{ChecklistID: "checklist-projected-limit", Task: "task", Context: "context", Limits: limits}
	dimensions := []evalcore.Dimension{{ID: "d1", Ordinal: 1, Name: "Correctness", Rubric: "Check correctness.", Rationale: "Core."}}
	drafts := []evalcore.DraftQuestion{{Rationale: "compound", Question: "Does it identify and fix the issue?"}}
	weights := []evalcore.Weight{{CandidateQuestionID: "c1", Rationale: "two obligations", Weight: 2}}

	env.OnActivity(activities.ActivityWriteChecklistInputs, mock.Anything, mock.Anything).Return(nil).Once()
	env.OnActivity(activities.ActivityAnalyzeDimensions, mock.Anything, mock.Anything).Return(activities.AnalyzeDimensionsResult{Dimensions: dimensions}, nil).Once()
	env.OnActivity(activities.ActivityGenerateQuestionsForDimension, mock.Anything, mock.Anything).Return(activities.GenerateQuestionsForDimensionResult{Questions: drafts}, nil).Once()
	env.OnActivity(activities.ActivityAssignWeights, mock.Anything, mock.Anything).Return(activities.AssignWeightsResult{Weights: weights}, nil).Once()
	expectFailChecklist(env, input.ChecklistID)

	env.ExecuteWorkflow(CreateChecklistWorkflow, input)
	if env.GetWorkflowError() == nil {
		t.Fatal("expected projected final-count failure")
	}
	env.AssertExpectations(t)
}

func newCreateChecklistTestEnv() *testsuite.TestWorkflowEnvironment {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(CreateChecklistWorkflow)
	registerActivityNames(env)
	return env
}

func expectFailChecklist(env *testsuite.TestWorkflowEnvironment, checklistID string) {
	env.OnActivity(activities.ActivityFailChecklist, mock.Anything, mock.MatchedBy(func(in activities.FailChecklistInput) bool {
		return in.ChecklistID == checklistID && in.Failure.Message != ""
	})).Return(nil).Once()
}

func nonRetryableActivityError(message string) error {
	return temporal.NewNonRetryableApplicationError(message, activities.ErrorClassModelOutputInvalid, nil)
}
