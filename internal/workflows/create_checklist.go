package workflows

import (
	"github.com/kirilligum/self-imp-bin-eval/internal/activities"
	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
	"go.temporal.io/sdk/workflow"
)

type CreateChecklistInput struct {
	ChecklistID string
	Task        string
	Context     string
}

func CreateChecklistWorkflow(ctx workflow.Context, in CreateChecklistInput) error {
	ctx = withActivityOptions(ctx)
	if err := workflow.ExecuteActivity(ctx, activities.ActivityWriteChecklistInputs, activities.WriteChecklistInputsInput{
		ChecklistID: in.ChecklistID,
		Task:        in.Task,
		Context:     in.Context,
	}).Get(ctx, nil); err != nil {
		return failChecklist(ctx, in.ChecklistID, err)
	}

	var questions activities.GenerateQuestionsResult
	if err := workflow.ExecuteActivity(ctx, activities.ActivityGenerateQuestions, activities.GenerateQuestionsInput{
		ChecklistID: in.ChecklistID,
		Task:        in.Task,
		Context:     in.Context,
	}).Get(ctx, &questions); err != nil {
		return failChecklist(ctx, in.ChecklistID, err)
	}

	var weights activities.AssignWeightsResult
	if err := workflow.ExecuteActivity(ctx, activities.ActivityAssignWeights, activities.AssignWeightsInput{
		ChecklistID: in.ChecklistID,
		Task:        in.Task,
		Context:     in.Context,
		Questions:   questions.Questions,
	}).Get(ctx, &weights); err != nil {
		return failChecklist(ctx, in.ChecklistID, err)
	}
	if err := evalcore.ValidateWeights(questions.Questions, weights.Weights); err != nil {
		return failChecklist(ctx, in.ChecklistID, err)
	}

	if err := workflow.ExecuteActivity(ctx, activities.ActivitySucceedChecklist, activities.SucceedChecklistInput{
		ChecklistID: in.ChecklistID,
		Questions:   questions.Questions,
		Weights:     weights.Weights,
	}).Get(ctx, nil); err != nil {
		return failChecklist(ctx, in.ChecklistID, err)
	}
	return nil
}

func failChecklist(ctx workflow.Context, checklistID string, cause error) error {
	disconnected, _ := workflow.NewDisconnectedContext(ctx)
	_ = workflow.ExecuteActivity(disconnected, activities.ActivityFailChecklist, activities.FailChecklistInput{
		ChecklistID:  checklistID,
		ErrorMessage: cause.Error(),
	}).Get(disconnected, nil)
	return cause
}
