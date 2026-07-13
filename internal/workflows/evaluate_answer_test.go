package workflows

import (
	"testing"

	"github.com/kirilligum/self-imp-bin-eval/internal/activities"
	"github.com/kirilligum/self-imp-bin-eval/internal/db"
	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/testsuite"
)

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
	checklist := db.Checklist{ID: "checklist-1", Status: db.StatusSucceeded, EvaluationRuns: 1, Questions: finalQuestions, Weights: weights}

	input := EvaluateAnswerInput{EvaluationID: "evaluation-1", ChecklistID: "checklist-1", ModelAnswer: "answer"}
	env.OnActivity(activities.ActivityWriteEvaluationInput, mock.Anything, activities.WriteEvaluationInputInput{
		EvaluationID: "evaluation-1", ModelAnswer: "answer",
	}).Return(nil).Once()
	env.OnActivity(activities.ActivityLoadChecklist, mock.Anything, activities.LoadChecklistInput{ChecklistID: "checklist-1"}).
		Return(activities.LoadChecklistResult{Checklist: checklist, Task: "task", Context: "context"}, nil).Once()
	env.OnActivity(activities.ActivityJudgeAnswer, mock.Anything, mock.MatchedBy(func(in activities.JudgeAnswerInput) bool {
		return in.RunIndex == 1 && len(in.Questions) == 2 && in.Questions[0].ID == "q1" && in.Questions[1].ID == "q2"
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

func TestP06RepeatedEvaluation(t *testing.T) {
	env := newEvaluateAnswerTestEnv()
	questions := []evalcore.FinalQuestion{
		{ID: "q1", Ordinal: 1, DimensionID: "d1", SourceCandidateID: "c1", Rationale: "r", Question: "Q1?"},
		{ID: "q2", Ordinal: 2, DimensionID: "d1", SourceCandidateID: "c2", Rationale: "r", Question: "Q2?"},
	}
	checklist := db.Checklist{ID: "checklist-runs", Status: db.StatusSucceeded, EvaluationRuns: 3, Questions: questions}
	input := EvaluateAnswerInput{EvaluationID: "evaluation-runs", ChecklistID: checklist.ID, ModelAnswer: "answer"}
	env.OnActivity(activities.ActivityWriteEvaluationInput, mock.Anything, mock.Anything).Return(nil).Once()
	env.OnActivity(activities.ActivityLoadChecklist, mock.Anything, mock.Anything).
		Return(activities.LoadChecklistResult{Checklist: checklist, Task: "task", Context: "context"}, nil).Once()
	answers := map[int][]evalcore.Judgment{
		1: {{QuestionID: "q1", Evidence: "run 1 yes", Answer: evalcore.AnswerYes}, {QuestionID: "q2", Evidence: "run 1 no", Answer: evalcore.AnswerNo}},
		2: {{QuestionID: "q1", Evidence: "run 2 yes", Answer: evalcore.AnswerYes}, {QuestionID: "q2", Evidence: "run 2 yes", Answer: evalcore.AnswerYes}},
		3: {{QuestionID: "q1", Evidence: "run 3 no", Answer: evalcore.AnswerNo}, {QuestionID: "q2", Evidence: "run 3 no", Answer: evalcore.AnswerNo}},
	}
	for runIndex := 1; runIndex <= 3; runIndex++ {
		env.OnActivity(activities.ActivityJudgeAnswer, mock.Anything, mock.MatchedBy(func(in activities.JudgeAnswerInput) bool {
			return in.EvaluationID == input.EvaluationID && in.RunIndex == runIndex
		})).Return(activities.JudgeAnswerResult{Judgments: answers[runIndex]}, nil).Once()
	}
	env.OnActivity(activities.ActivitySucceedEvaluation, mock.Anything, mock.MatchedBy(func(in activities.SucceedEvaluationInput) bool {
		return in.EvaluationID == input.EvaluationID && len(in.RunJudgments) == 6 &&
			in.Score.SatisfiedPoints == 1 && len(in.Score.FailedQuestionIDs) == 1 && in.Score.FailedQuestionIDs[0] == "q2"
	})).Return(nil).Once()

	env.ExecuteWorkflow(EvaluateAnswerWorkflow, input)
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error = %v", err)
	}
	env.AssertExpectations(t)
}

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
		return in.EvaluationID == "evaluation-fail" && in.ChecklistID == "checklist-1" && in.Failure.Message != ""
	})).Return(nil).Once()

	env.ExecuteWorkflow(EvaluateAnswerWorkflow, input)
	if env.GetWorkflowError() == nil {
		t.Fatal("expected workflow failure")
	}
	env.AssertExpectations(t)
}

func TestEvaluateAnswerWorkflowFailureMatrix(t *testing.T) {
	finalQuestions := []evalcore.FinalQuestion{
		{ID: "q1", Ordinal: 1, DimensionID: "d1", SourceCandidateID: "c1", Rationale: "normal", Question: "Normal?"},
		{ID: "q2", Ordinal: 2, DimensionID: "d1", SourceCandidateID: "c2", Rationale: "detail", Question: "Specific?"},
	}
	succeededChecklist := db.Checklist{ID: "checklist-1", Status: db.StatusSucceeded, EvaluationRuns: 1, Questions: finalQuestions}

	t.Run("load checklist activity failure persists failed evaluation", func(t *testing.T) {
		env := newEvaluateAnswerTestEnv()
		input := EvaluateAnswerInput{EvaluationID: "evaluation-load-fail", ChecklistID: "checklist-1", ModelAnswer: "answer"}
		env.OnActivity(activities.ActivityWriteEvaluationInput, mock.Anything, mock.Anything).Return(nil).Once()
		env.OnActivity(activities.ActivityLoadChecklist, mock.Anything, activities.LoadChecklistInput{ChecklistID: "checklist-1"}).
			Return(activities.LoadChecklistResult{}, nonRetryableActivityError("load failed")).Once()
		expectFailEvaluation(env, input.EvaluationID, input.ChecklistID)

		env.ExecuteWorkflow(EvaluateAnswerWorkflow, input)
		if env.GetWorkflowError() == nil {
			t.Fatal("expected workflow failure")
		}
		env.AssertExpectations(t)
	})

	t.Run("failed checklist status rejects before judging", func(t *testing.T) {
		env := newEvaluateAnswerTestEnv()
		input := EvaluateAnswerInput{EvaluationID: "evaluation-status-fail", ChecklistID: "checklist-1", ModelAnswer: "answer"}
		env.OnActivity(activities.ActivityWriteEvaluationInput, mock.Anything, mock.Anything).Return(nil).Once()
		env.OnActivity(activities.ActivityLoadChecklist, mock.Anything, activities.LoadChecklistInput{ChecklistID: "checklist-1"}).
			Return(activities.LoadChecklistResult{Checklist: db.Checklist{ID: "checklist-1", Status: db.StatusFailed}}, nil).Once()
		expectFailEvaluation(env, input.EvaluationID, input.ChecklistID)

		env.ExecuteWorkflow(EvaluateAnswerWorkflow, input)
		if env.GetWorkflowError() == nil {
			t.Fatal("expected workflow failure")
		}
		env.AssertExpectations(t)
	})

	t.Run("judge activity failure persists failed evaluation", func(t *testing.T) {
		env := newEvaluateAnswerTestEnv()
		input := EvaluateAnswerInput{EvaluationID: "evaluation-judge-fail", ChecklistID: "checklist-1", ModelAnswer: "answer"}
		env.OnActivity(activities.ActivityWriteEvaluationInput, mock.Anything, mock.Anything).Return(nil).Once()
		env.OnActivity(activities.ActivityLoadChecklist, mock.Anything, mock.Anything).
			Return(activities.LoadChecklistResult{Checklist: succeededChecklist, Task: "task", Context: "context"}, nil).Once()
		env.OnActivity(activities.ActivityJudgeAnswer, mock.Anything, mock.Anything).
			Return(activities.JudgeAnswerResult{}, nonRetryableActivityError("judge failed")).Once()
		expectFailEvaluation(env, input.EvaluationID, input.ChecklistID)

		env.ExecuteWorkflow(EvaluateAnswerWorkflow, input)
		if env.GetWorkflowError() == nil {
			t.Fatal("expected workflow failure")
		}
		env.AssertExpectations(t)
	})

	t.Run("invalid judgment output fails scoring and persists failed evaluation", func(t *testing.T) {
		env := newEvaluateAnswerTestEnv()
		input := EvaluateAnswerInput{EvaluationID: "evaluation-invalid-judgments", ChecklistID: "checklist-1", ModelAnswer: "answer"}
		env.OnActivity(activities.ActivityWriteEvaluationInput, mock.Anything, mock.Anything).Return(nil).Once()
		env.OnActivity(activities.ActivityLoadChecklist, mock.Anything, mock.Anything).
			Return(activities.LoadChecklistResult{Checklist: succeededChecklist, Task: "task", Context: "context"}, nil).Once()
		env.OnActivity(activities.ActivityJudgeAnswer, mock.Anything, mock.Anything).
			Return(activities.JudgeAnswerResult{Judgments: []evalcore.Judgment{{QuestionID: "q1", Evidence: "Only one.", Answer: evalcore.AnswerYes}}}, nil).Once()
		expectFailEvaluation(env, input.EvaluationID, input.ChecklistID)

		env.ExecuteWorkflow(EvaluateAnswerWorkflow, input)
		if env.GetWorkflowError() == nil {
			t.Fatal("expected workflow failure")
		}
		env.AssertExpectations(t)
	})

	t.Run("succeed evaluation activity failure is persisted as failed", func(t *testing.T) {
		env := newEvaluateAnswerTestEnv()
		input := EvaluateAnswerInput{EvaluationID: "evaluation-succeed-fail", ChecklistID: "checklist-1", ModelAnswer: "answer"}
		judgments := []evalcore.Judgment{
			{QuestionID: "q1", Evidence: "Satisfied.", Answer: evalcore.AnswerYes},
			{QuestionID: "q2", Evidence: "Missing.", Answer: evalcore.AnswerNo},
		}
		env.OnActivity(activities.ActivityWriteEvaluationInput, mock.Anything, mock.Anything).Return(nil).Once()
		env.OnActivity(activities.ActivityLoadChecklist, mock.Anything, mock.Anything).
			Return(activities.LoadChecklistResult{Checklist: succeededChecklist, Task: "task", Context: "context"}, nil).Once()
		env.OnActivity(activities.ActivityJudgeAnswer, mock.Anything, mock.Anything).
			Return(activities.JudgeAnswerResult{Judgments: judgments}, nil).Once()
		env.OnActivity(activities.ActivitySucceedEvaluation, mock.Anything, mock.Anything).
			Return(nonRetryableActivityError("terminal write failed")).Once()
		expectFailEvaluation(env, input.EvaluationID, input.ChecklistID)

		env.ExecuteWorkflow(EvaluateAnswerWorkflow, input)
		if env.GetWorkflowError() == nil {
			t.Fatal("expected workflow failure")
		}
		env.AssertExpectations(t)
	})
}

func newEvaluateAnswerTestEnv() *testsuite.TestWorkflowEnvironment {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(EvaluateAnswerWorkflow)
	registerActivityNames(env)
	return env
}

func expectFailEvaluation(env *testsuite.TestWorkflowEnvironment, evaluationID, checklistID string) {
	env.OnActivity(activities.ActivityFailEvaluation, mock.Anything, mock.MatchedBy(func(in activities.FailEvaluationInput) bool {
		return in.EvaluationID == evaluationID && in.ChecklistID == checklistID && in.Failure.Message != ""
	})).Return(nil).Once()
}
