package workflows

import (
	"testing"

	"github.com/kirilligum/self-imp-bin-eval/internal/activities"
	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/testsuite"
)

// TEST-012
// TEST-019
func TestCreateChecklistWorkflow(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(CreateChecklistWorkflow)
	registerActivityNames(env)

	input := CreateChecklistInput{ChecklistID: "checklist-1", Task: "task", Context: "context"}
	questions := evalcore.AssignQuestionIDs([]evalcore.DraftQuestion{
		{Rationale: "excluded", Question: "Excluded?"},
		{Rationale: "active", Question: "Active?"},
	})
	weights := []evalcore.Weight{{QuestionID: "q1", Weight: 0, Rationale: "duplicate"}, {QuestionID: "q2", Weight: 4, Rationale: "important"}}

	env.OnActivity(activities.ActivityWriteChecklistInputs, mock.Anything, activities.WriteChecklistInputsInput{
		ChecklistID: "checklist-1", Task: "task", Context: "context",
	}).Return(nil).Once()
	env.OnActivity(activities.ActivityGenerateQuestions, mock.Anything, activities.GenerateQuestionsInput{
		ChecklistID: "checklist-1", Task: "task", Context: "context",
	}).Return(activities.GenerateQuestionsResult{Questions: questions}, nil).Once()
	env.OnActivity(activities.ActivityAssignWeights, mock.Anything, activities.AssignWeightsInput{
		ChecklistID: "checklist-1", Task: "task", Context: "context", Questions: questions,
	}).Return(activities.AssignWeightsResult{Weights: weights}, nil).Once()
	env.OnActivity(activities.ActivitySucceedChecklist, mock.Anything, activities.SucceedChecklistInput{
		ChecklistID: "checklist-1", Questions: questions, Weights: weights,
	}).Return(nil).Once()

	env.ExecuteWorkflow(CreateChecklistWorkflow, input)
	if !env.IsWorkflowCompleted() || env.GetWorkflowError() != nil {
		t.Fatalf("workflow error = %v", env.GetWorkflowError())
	}
	env.AssertExpectations(t)
}

// TEST-019
func TestWorkflowFailurePersistence(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(CreateChecklistWorkflow)
	registerActivityNames(env)

	input := CreateChecklistInput{ChecklistID: "checklist-fail", Task: "task", Context: "context"}
	questions := evalcore.AssignQuestionIDs([]evalcore.DraftQuestion{{Rationale: "r", Question: "Q?"}})
	allZero := []evalcore.Weight{{QuestionID: "q1", Rationale: "excluded", Weight: 0}}

	env.OnActivity(activities.ActivityWriteChecklistInputs, mock.Anything, mock.Anything).Return(nil).Once()
	env.OnActivity(activities.ActivityGenerateQuestions, mock.Anything, mock.Anything).Return(activities.GenerateQuestionsResult{Questions: questions}, nil).Once()
	env.OnActivity(activities.ActivityAssignWeights, mock.Anything, mock.Anything).Return(activities.AssignWeightsResult{Weights: allZero}, nil).Once()
	env.OnActivity(activities.ActivityFailChecklist, mock.Anything, mock.MatchedBy(func(in activities.FailChecklistInput) bool {
		return in.ChecklistID == "checklist-fail" && in.ErrorMessage != ""
	})).Return(nil).Once()

	env.ExecuteWorkflow(CreateChecklistWorkflow, input)
	if env.GetWorkflowError() == nil {
		t.Fatal("expected workflow failure")
	}
	env.AssertExpectations(t)
}
