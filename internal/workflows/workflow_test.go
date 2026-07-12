package workflows

import (
	"context"

	"github.com/kirilligum/self-imp-bin-eval/internal/activities"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

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
