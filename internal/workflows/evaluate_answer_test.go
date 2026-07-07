package workflows

import (
	"testing"

	"github.com/kirilligum/self-imp-bin-eval/internal/activities"
	"github.com/kirilligum/self-imp-bin-eval/internal/db"
	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/testsuite"
)

// TEST-012
func TestEvaluateAnswerWorkflow(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(EvaluateAnswerWorkflow)
	registerActivityNames(env)

	questions := evalcore.AssignQuestionIDs([]evalcore.DraftQuestion{
		{Rationale: "excluded", Question: "Excluded?"},
		{Rationale: "active", Question: "Active?"},
	})
	weights := []evalcore.Weight{{QuestionID: "q1", Rationale: "duplicate", Weight: 0}, {QuestionID: "q2", Rationale: "important", Weight: 4}}
	judgments := []evalcore.Judgment{{QuestionID: "q2", Evidence: "Satisfied.", Answer: evalcore.AnswerYes}}
	checklist := db.Checklist{ID: "checklist-1", Status: db.StatusSucceeded, Questions: questions, Weights: weights}

	input := EvaluateAnswerInput{EvaluationID: "evaluation-1", ChecklistID: "checklist-1", ModelAnswer: "answer"}
	env.OnActivity(activities.ActivityWriteEvaluationInput, mock.Anything, activities.WriteEvaluationInputInput{
		EvaluationID: "evaluation-1", ModelAnswer: "answer",
	}).Return(nil).Once()
	env.OnActivity(activities.ActivityLoadChecklist, mock.Anything, activities.LoadChecklistInput{ChecklistID: "checklist-1"}).
		Return(activities.LoadChecklistResult{Checklist: checklist, Task: "task", Context: "context"}, nil).Once()
	env.OnActivity(activities.ActivityJudgeAnswer, mock.Anything, mock.MatchedBy(func(in activities.JudgeAnswerInput) bool {
		activePayload, err := evalcore.BuildActiveChecklist(in.Questions, in.Weights)
		return err == nil && len(activePayload) == 1 && activePayload[0].ID == "q2"
	})).Return(activities.JudgeAnswerResult{Judgments: judgments}, nil).Once()
	env.OnActivity(activities.ActivitySucceedEvaluation, mock.Anything, mock.MatchedBy(func(in activities.SucceedEvaluationInput) bool {
		return in.EvaluationID == "evaluation-1" &&
			in.Score.SatisfiedPoints == 4 &&
			in.Score.TotalPossiblePoints == 4 &&
			len(in.Score.FailedQuestionIDs) == 0
	})).Return(nil).Once()

	env.ExecuteWorkflow(EvaluateAnswerWorkflow, input)
	if !env.IsWorkflowCompleted() || env.GetWorkflowError() != nil {
		t.Fatalf("workflow error = %v", env.GetWorkflowError())
	}
	env.AssertExpectations(t)
}

// TEST-019
func TestEvaluateAnswerWorkflowFailurePersistence(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(EvaluateAnswerWorkflow)
	registerActivityNames(env)

	input := EvaluateAnswerInput{EvaluationID: "evaluation-fail", ChecklistID: "checklist-1", ModelAnswer: "answer"}
	env.OnActivity(activities.ActivityWriteEvaluationInput, mock.Anything, mock.Anything).Return(nil).Once()
	env.OnActivity(activities.ActivityLoadChecklist, mock.Anything, activities.LoadChecklistInput{ChecklistID: "checklist-1"}).
		Return(activities.LoadChecklistResult{Checklist: db.Checklist{ID: "checklist-1", Status: db.StatusRunning}}, nil).Once()
	env.OnActivity(activities.ActivityFailEvaluation, mock.Anything, mock.MatchedBy(func(in activities.FailEvaluationInput) bool {
		return in.EvaluationID == "evaluation-fail" && in.ChecklistID == "checklist-1" && in.ErrorMessage != ""
	})).Return(nil).Once()

	env.ExecuteWorkflow(EvaluateAnswerWorkflow, input)
	if env.GetWorkflowError() == nil {
		t.Fatal("expected workflow failure")
	}
	env.AssertExpectations(t)
}
