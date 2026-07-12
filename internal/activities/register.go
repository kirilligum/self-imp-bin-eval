package activities

import (
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/worker"
)

func Register(w worker.Worker, acts *Activities) {
	w.RegisterActivityWithOptions(acts.WriteChecklistInputs, activity.RegisterOptions{Name: ActivityWriteChecklistInputs})
	w.RegisterActivityWithOptions(acts.WriteEvaluationInput, activity.RegisterOptions{Name: ActivityWriteEvaluationInput})
	w.RegisterActivityWithOptions(acts.AnalyzeDimensions, activity.RegisterOptions{Name: ActivityAnalyzeDimensions})
	w.RegisterActivityWithOptions(acts.GenerateQuestionsForDimension, activity.RegisterOptions{Name: ActivityGenerateQuestionsForDimension})
	w.RegisterActivityWithOptions(acts.AssignWeights, activity.RegisterOptions{Name: ActivityAssignWeights})
	w.RegisterActivityWithOptions(acts.SplitQuestion, activity.RegisterOptions{Name: ActivitySplitQuestion})
	w.RegisterActivityWithOptions(acts.JudgeAnswer, activity.RegisterOptions{Name: ActivityJudgeAnswer})
	w.RegisterActivityWithOptions(acts.LoadChecklist, activity.RegisterOptions{Name: ActivityLoadChecklist})
	w.RegisterActivityWithOptions(acts.SucceedChecklist, activity.RegisterOptions{Name: ActivitySucceedChecklist})
	w.RegisterActivityWithOptions(acts.FailChecklist, activity.RegisterOptions{Name: ActivityFailChecklist})
	w.RegisterActivityWithOptions(acts.SucceedEvaluation, activity.RegisterOptions{Name: ActivitySucceedEvaluation})
	w.RegisterActivityWithOptions(acts.FailEvaluation, activity.RegisterOptions{Name: ActivityFailEvaluation})
}
