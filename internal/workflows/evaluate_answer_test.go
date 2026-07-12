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

	finalQuestions := []evalcore.FinalQuestion{
		{ID: "q1", Ordinal: 1, DimensionID: "d1", SourceCandidateID: "c1", Rationale: "normal", Question: "Normal?"},
		{ID: "q2", Ordinal: 2, DimensionID: "d2", SourceCandidateID: "c2", Rationale: "detail", Question: "Specific?"},
	}
	weights := []evalcore.Weight{{CandidateQuestionID: "c1", Rationale: "normal", Weight: 1}, {CandidateQuestionID: "c2", Rationale: "split", Weight: 2}}
	judgments := []evalcore.Judgment{
		{QuestionID: "q1", Evidence: "Satisfied.", Answer: evalcore.AnswerYes},
		{QuestionID: "q2", Evidence: "Missing.", Answer: evalcore.AnswerNo},
	}
	checklist := db.Checklist{ID: "checklist-1", Status: db.StatusSucceeded, Questions: finalQuestions, Weights: weights}

	input := EvaluateAnswerInput{EvaluationID: "evaluation-1", ChecklistID: "checklist-1", ModelAnswer: "answer"}
	env.OnActivity(activities.ActivityWriteEvaluationInput, mock.Anything, activities.WriteEvaluationInputInput{
		EvaluationID: "evaluation-1", ModelAnswer: "answer",
	}).Return(nil).Once()
	env.OnActivity(activities.ActivityLoadChecklist, mock.Anything, activities.LoadChecklistInput{ChecklistID: "checklist-1"}).
		Return(activities.LoadChecklistResult{Checklist: checklist, Task: "task", Context: "context"}, nil).Once()
	env.OnActivity(activities.ActivityJudgeAnswer, mock.Anything, mock.MatchedBy(func(in activities.JudgeAnswerInput) bool {
		return len(in.Questions) == 2 && in.Questions[0].ID == "q1" && in.Questions[1].ID == "q2"
	})).Return(activities.JudgeAnswerResult{Judgments: judgments}, nil).Once()
	env.OnActivity(activities.ActivitySucceedEvaluation, mock.Anything, mock.MatchedBy(func(in activities.SucceedEvaluationInput) bool {
		return in.EvaluationID == "evaluation-1" &&
			in.Score.SatisfiedPoints == 1 &&
			in.Score.TotalPossiblePoints == 2 &&
			len(in.Score.FailedQuestionIDs) == 1 &&
			in.Score.FailedQuestionIDs[0] == "q2"
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
