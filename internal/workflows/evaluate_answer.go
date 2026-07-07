package workflows

import (
	"fmt"

	"github.com/kirilligum/self-imp-bin-eval/internal/activities"
	"github.com/kirilligum/self-imp-bin-eval/internal/db"
	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
	"go.temporal.io/sdk/workflow"
)

type EvaluateAnswerInput struct {
	EvaluationID string
	ChecklistID  string
	ModelAnswer  string
}

func EvaluateAnswerWorkflow(ctx workflow.Context, in EvaluateAnswerInput) error {
	ctx = withActivityOptions(ctx)
	if err := workflow.ExecuteActivity(ctx, activities.ActivityWriteEvaluationInput, activities.WriteEvaluationInputInput{
		EvaluationID: in.EvaluationID,
		ModelAnswer:  in.ModelAnswer,
	}).Get(ctx, nil); err != nil {
		return failEvaluation(ctx, in.EvaluationID, in.ChecklistID, err)
	}

	var loaded activities.LoadChecklistResult
	if err := workflow.ExecuteActivity(ctx, activities.ActivityLoadChecklist, activities.LoadChecklistInput{ChecklistID: in.ChecklistID}).Get(ctx, &loaded); err != nil {
		return failEvaluation(ctx, in.EvaluationID, in.ChecklistID, err)
	}
	if loaded.Checklist.Status != db.StatusSucceeded {
		return failEvaluation(ctx, in.EvaluationID, in.ChecklistID, fmt.Errorf("checklist %s is not succeeded", in.ChecklistID))
	}

	var judged activities.JudgeAnswerResult
	if err := workflow.ExecuteActivity(ctx, activities.ActivityJudgeAnswer, activities.JudgeAnswerInput{
		EvaluationID: in.EvaluationID,
		Task:         loaded.Task,
		Context:      loaded.Context,
		ModelAnswer:  in.ModelAnswer,
		Questions:    loaded.Checklist.Questions,
		Weights:      loaded.Checklist.Weights,
	}).Get(ctx, &judged); err != nil {
		return failEvaluation(ctx, in.EvaluationID, in.ChecklistID, err)
	}

	score, err := evalcore.ScoreChecklist(loaded.Checklist.Questions, loaded.Checklist.Weights, judged.Judgments)
	if err != nil {
		return failEvaluation(ctx, in.EvaluationID, in.ChecklistID, err)
	}

	if err := workflow.ExecuteActivity(ctx, activities.ActivitySucceedEvaluation, activities.SucceedEvaluationInput{
		EvaluationID: in.EvaluationID,
		ChecklistID:  in.ChecklistID,
		Judgments:    judged.Judgments,
		Score:        score,
	}).Get(ctx, nil); err != nil {
		return failEvaluation(ctx, in.EvaluationID, in.ChecklistID, err)
	}
	return nil
}

func failEvaluation(ctx workflow.Context, evaluationID, checklistID string, cause error) error {
	disconnected, _ := workflow.NewDisconnectedContext(ctx)
	_ = workflow.ExecuteActivity(disconnected, activities.ActivityFailEvaluation, activities.FailEvaluationInput{
		EvaluationID: evaluationID,
		ChecklistID:  checklistID,
		ErrorMessage: cause.Error(),
	}).Get(disconnected, nil)
	return cause
}
