package activities

import "context"

func (a *Activities) LoadChecklist(ctx context.Context, in LoadChecklistInput) (LoadChecklistResult, error) {
	checklist, err := a.store.GetChecklist(ctx, in.ChecklistID)
	if err != nil {
		return LoadChecklistResult{}, ToTemporalError(ctx, err)
	}
	result := LoadChecklistResult{Checklist: checklist}
	if checklist.Status == "succeeded" {
		task, err := a.artifacts.Read(ctx, checklist.TaskArtifactKey)
		if err != nil {
			return LoadChecklistResult{}, ToTemporalError(ctx, err)
		}
		contextText, err := a.artifacts.Read(ctx, checklist.ContextArtifactKey)
		if err != nil {
			return LoadChecklistResult{}, ToTemporalError(ctx, err)
		}
		result.Task = string(task)
		result.Context = string(contextText)
	}
	return result, nil
}

func (a *Activities) SucceedChecklist(ctx context.Context, in SucceedChecklistInput) error {
	return ToTemporalError(ctx, a.store.SucceedChecklist(ctx, in.ChecklistID, in.Dimensions, in.CandidateQuestions, in.Weights, in.Questions, in.Limits))
}

func (a *Activities) FailChecklist(ctx context.Context, in FailChecklistInput) error {
	return ToTemporalError(ctx, a.store.FailChecklist(ctx, in.ChecklistID, in.Failure))
}

func (a *Activities) SucceedEvaluation(ctx context.Context, in SucceedEvaluationInput) error {
	return ToTemporalError(ctx, a.store.SucceedEvaluation(ctx, in.EvaluationID, in.ChecklistID, in.RunJudgments, in.Score))
}

func (a *Activities) FailEvaluation(ctx context.Context, in FailEvaluationInput) error {
	return ToTemporalError(ctx, a.store.FailEvaluation(ctx, in.EvaluationID, in.ChecklistID, in.Failure))
}
