package activities

import (
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/worker"
)

func Register(w worker.Worker, acts *Activities) {
	w.RegisterActivityWithOptions(acts.WriteChecklistInputs, activity.RegisterOptions{Name: ActivityWriteChecklistInputs})
	w.RegisterActivityWithOptions(acts.WriteEvaluationInput, activity.RegisterOptions{Name: ActivityWriteEvaluationInput})
	w.RegisterActivityWithOptions(acts.GenerateQuestions, activity.RegisterOptions{Name: ActivityGenerateQuestions})
	w.RegisterActivityWithOptions(acts.AssignWeights, activity.RegisterOptions{Name: ActivityAssignWeights})
	w.RegisterActivityWithOptions(acts.JudgeAnswer, activity.RegisterOptions{Name: ActivityJudgeAnswer})
	w.RegisterActivityWithOptions(acts.LoadChecklist, activity.RegisterOptions{Name: ActivityLoadChecklist})
	w.RegisterActivityWithOptions(acts.SucceedChecklist, activity.RegisterOptions{Name: ActivitySucceedChecklist})
	w.RegisterActivityWithOptions(acts.FailChecklist, activity.RegisterOptions{Name: ActivityFailChecklist})
	w.RegisterActivityWithOptions(acts.SucceedEvaluation, activity.RegisterOptions{Name: ActivitySucceedEvaluation})
	w.RegisterActivityWithOptions(acts.FailEvaluation, activity.RegisterOptions{Name: ActivityFailEvaluation})
}
