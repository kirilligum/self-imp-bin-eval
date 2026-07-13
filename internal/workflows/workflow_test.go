package workflows

import (
	"context"
	"strings"
	"testing"

	"github.com/kirilligum/self-imp-bin-eval/internal/activities"
	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
	"github.com/kirilligum/self-imp-bin-eval/internal/failure"
	"github.com/kirilligum/self-imp-bin-eval/internal/llm"
	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
)

func TestP06ActivityRuntimeBudget(t *testing.T) {
	if activityStartToCloseTimeout <= llm.DefaultRequestTimeout {
		t.Fatalf("activity timeout %s must exceed LLM request timeout %s", activityStartToCloseTimeout, llm.DefaultRequestTimeout)
	}
}

func TestP06StructuredWorkflowFailures(t *testing.T) {
	const rawSentinel = "RAW_PROVIDER_OUTPUT_MUST_NOT_LEAK"
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.SetStartWorkflowOptions(client.StartWorkflowOptions{ID: "checklist-workflow-structured"})
	env.RegisterWorkflow(CreateChecklistWorkflow)
	registerActivityNames(env)

	carried := failure.Details{
		ErrorClass:         failure.ClassModelOutputInvalid,
		ErrorCode:          string(evalcore.CodeInvalidFinalChecklist),
		Message:            "final question budget exceeded",
		AttemptCount:       3,
		Diagnostics:        []evalcore.LimitDiagnostic{{LimitName: "max_final_questions", ConfiguredLimit: 64, ObservedCount: 65}},
		ArtifactReferences: []string{"checklists/checklist-structured/llm/weight_assignment/attempt-3/response.json"},
	}
	activityErr := temporal.NewApplicationErrorWithOptions(rawSentinel, failure.ClassModelOutputInvalid, temporal.ApplicationErrorOptions{
		NonRetryable: true,
		Details:      []any{carried},
	})
	env.OnActivity(activities.ActivityWriteChecklistInputs, mock.Anything, mock.Anything).Return(activityErr).Once()
	env.OnActivity(activities.ActivityFailChecklist, mock.Anything, mock.MatchedBy(func(in activities.FailChecklistInput) bool {
		encoded := in.Failure.Message + strings.Join(in.Failure.ArtifactReferences, " ")
		return in.ChecklistID == "checklist-structured" &&
			in.Failure.WorkflowID == "checklist-workflow-structured" &&
			in.Failure.Stage == "write_checklist_inputs" &&
			in.Failure.AttemptCount == 3 &&
			len(in.Failure.Diagnostics) == 1 &&
			!strings.Contains(encoded, rawSentinel)
	})).Return(nil).Once()

	env.ExecuteWorkflow(CreateChecklistWorkflow, CreateChecklistInput{
		ChecklistID: "checklist-structured",
		Task:        "task",
		Context:     "context",
		Limits:      evalcore.DefaultChecklistLimits(),
	})
	if env.GetWorkflowError() == nil {
		t.Fatal("expected workflow failure")
	}
	env.AssertExpectations(t)
}

func TestP06TemporalIdempotency(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(CreateChecklistWorkflow)
	registerActivityNames(env)
	original := temporal.NewNonRetryableApplicationError("original failure", activities.ErrorClassModelOutputInvalid, nil)
	persistence := temporal.NewNonRetryableApplicationError("failure persistence unavailable", activities.ErrorClassInfraNonRetryable, nil)
	env.OnActivity(activities.ActivityWriteChecklistInputs, mock.Anything, mock.Anything).Return(original).Once()
	env.OnActivity(activities.ActivityFailChecklist, mock.Anything, mock.Anything).Return(persistence).Once()

	env.ExecuteWorkflow(CreateChecklistWorkflow, CreateChecklistInput{
		ChecklistID: "checklist-failure-write",
		Task:        "task",
		Context:     "context",
		Limits:      evalcore.DefaultChecklistLimits(),
	})
	err := env.GetWorkflowError()
	if err == nil || !strings.Contains(err.Error(), "failure persistence unavailable") {
		t.Fatalf("workflow error = %v, want failure persistence error", err)
	}
	env.AssertExpectations(t)
}

func registerActivityNames(env *testsuite.TestWorkflowEnvironment) {
	env.RegisterActivityWithOptions(func(context.Context, activities.WriteChecklistInputsInput) error { return nil }, activity.RegisterOptions{Name: activities.ActivityWriteChecklistInputs})
	env.RegisterActivityWithOptions(func(context.Context, activities.WriteEvaluationInputInput) error { return nil }, activity.RegisterOptions{Name: activities.ActivityWriteEvaluationInput})
	env.RegisterActivityWithOptions(func(context.Context, activities.AnalyzeDimensionsInput) (activities.AnalyzeDimensionsResult, error) {
		return activities.AnalyzeDimensionsResult{}, nil
	}, activity.RegisterOptions{Name: activities.ActivityAnalyzeDimensions})
	env.RegisterActivityWithOptions(func(context.Context, activities.GenerateQuestionsForDimensionInput) (activities.GenerateQuestionsForDimensionResult, error) {
		return activities.GenerateQuestionsForDimensionResult{}, nil
	}, activity.RegisterOptions{Name: activities.ActivityGenerateQuestionsForDimension})
	env.RegisterActivityWithOptions(func(context.Context, activities.AssignWeightsInput) (activities.AssignWeightsResult, error) {
		return activities.AssignWeightsResult{}, nil
	}, activity.RegisterOptions{Name: activities.ActivityAssignWeights})
	env.RegisterActivityWithOptions(func(context.Context, activities.SplitQuestionInput) (activities.SplitQuestionResult, error) {
		return activities.SplitQuestionResult{}, nil
	}, activity.RegisterOptions{Name: activities.ActivitySplitQuestion})
	env.RegisterActivityWithOptions(func(context.Context, activities.JudgeAnswerInput) (activities.JudgeAnswerResult, error) {
		return activities.JudgeAnswerResult{}, nil
	}, activity.RegisterOptions{Name: activities.ActivityJudgeAnswer})
	env.RegisterActivityWithOptions(func(context.Context, activities.LoadChecklistInput) (activities.LoadChecklistResult, error) {
		return activities.LoadChecklistResult{}, nil
	}, activity.RegisterOptions{Name: activities.ActivityLoadChecklist})
	env.RegisterActivityWithOptions(func(context.Context, activities.SucceedChecklistInput) error { return nil }, activity.RegisterOptions{Name: activities.ActivitySucceedChecklist})
	env.RegisterActivityWithOptions(func(context.Context, activities.FailChecklistInput) error { return nil }, activity.RegisterOptions{Name: activities.ActivityFailChecklist})
	env.RegisterActivityWithOptions(func(context.Context, activities.SucceedEvaluationInput) error { return nil }, activity.RegisterOptions{Name: activities.ActivitySucceedEvaluation})
	env.RegisterActivityWithOptions(func(context.Context, activities.FailEvaluationInput) error { return nil }, activity.RegisterOptions{Name: activities.ActivityFailEvaluation})
}
