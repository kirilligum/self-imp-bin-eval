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
	Limits      evalcore.ChecklistLimits
}

type splitFuture struct {
	future workflow.Future
}

func CreateChecklistWorkflow(ctx workflow.Context, in CreateChecklistInput) error {
	ctx = withActivityOptions(ctx)
	limits := in.Limits.WithDefaults()
	if err := workflow.ExecuteActivity(ctx, activities.ActivityWriteChecklistInputs, activities.WriteChecklistInputsInput{
		ChecklistID: in.ChecklistID,
		Task:        in.Task,
		Context:     in.Context,
	}).Get(ctx, nil); err != nil {
		return failChecklist(ctx, in.ChecklistID, err)
	}

	var analyzed activities.AnalyzeDimensionsResult
	if err := workflow.ExecuteActivity(ctx, activities.ActivityAnalyzeDimensions, activities.AnalyzeDimensionsInput{
		ChecklistID: in.ChecklistID,
		Task:        in.Task,
		Context:     in.Context,
		Limits:      limits,
	}).Get(ctx, &analyzed); err != nil {
		return failChecklist(ctx, in.ChecklistID, err)
	}
	if err := evalcore.ValidateDimensions(analyzed.Dimensions, limits); err != nil {
		return failChecklist(ctx, in.ChecklistID, err)
	}

	questionFutures := make([]workflow.Future, len(analyzed.Dimensions))
	for i, dimension := range analyzed.Dimensions {
		questionFutures[i] = workflow.ExecuteActivity(ctx, activities.ActivityGenerateQuestionsForDimension, activities.GenerateQuestionsForDimensionInput{
			ChecklistID: in.ChecklistID,
			Task:        in.Task,
			Context:     in.Context,
			Dimension:   dimension,
			Limits:      limits,
		})
	}
	candidates := make([]evalcore.CandidateQuestion, 0, len(analyzed.Dimensions)*limits.MaxCandidatesPerDimension)
	for i, future := range questionFutures {
		var generated activities.GenerateQuestionsForDimensionResult
		if err := future.Get(ctx, &generated); err != nil {
			return failChecklist(ctx, in.ChecklistID, err)
		}
		candidates = append(candidates, evalcore.AssignCandidateQuestionIDs(analyzed.Dimensions[i].ID, len(candidates)+1, generated.Questions)...)
	}
	if err := evalcore.ValidateCandidateQuestions(analyzed.Dimensions, candidates, limits); err != nil {
		return failChecklist(ctx, in.ChecklistID, err)
	}

	var weighted activities.AssignWeightsResult
	if err := workflow.ExecuteActivity(ctx, activities.ActivityAssignWeights, activities.AssignWeightsInput{
		ChecklistID:        in.ChecklistID,
		Task:               in.Task,
		Context:            in.Context,
		CandidateQuestions: candidates,
		Limits:             limits,
	}).Get(ctx, &weighted); err != nil {
		return failChecklist(ctx, in.ChecklistID, err)
	}
	if err := evalcore.ValidateWeights(candidates, weighted.Weights, limits); err != nil {
		return failChecklist(ctx, in.ChecklistID, err)
	}

	weightByCandidateID := make(map[string]evalcore.Weight, len(weighted.Weights))
	for _, weight := range weighted.Weights {
		weightByCandidateID[weight.CandidateQuestionID] = weight
	}
	splitFutures := make([]splitFuture, 0)
	for _, candidate := range candidates {
		weight := weightByCandidateID[candidate.ID]
		if weight.Weight <= 1 {
			continue
		}
		splitFutures = append(splitFutures, splitFuture{
			future: workflow.ExecuteActivity(ctx, activities.ActivitySplitQuestion, activities.SplitQuestionInput{
				ChecklistID:       in.ChecklistID,
				Task:              in.Task,
				Context:           in.Context,
				CandidateQuestion: candidate,
				Weight:            weight,
				Limits:            limits,
			}),
		})
	}
	splits := make([]evalcore.SplitQuestions, 0, len(splitFutures))
	for _, pending := range splitFutures {
		var split activities.SplitQuestionResult
		if err := pending.future.Get(ctx, &split); err != nil {
			return failChecklist(ctx, in.ChecklistID, err)
		}
		splits = append(splits, split.Split)
	}

	finalQuestions, err := evalcore.BuildFinalChecklist(analyzed.Dimensions, candidates, weighted.Weights, splits, limits)
	if err != nil {
		return failChecklist(ctx, in.ChecklistID, err)
	}
	if err := workflow.ExecuteActivity(ctx, activities.ActivitySucceedChecklist, activities.SucceedChecklistInput{
		ChecklistID:        in.ChecklistID,
		Dimensions:         analyzed.Dimensions,
		CandidateQuestions: candidates,
		Weights:            weighted.Weights,
		Questions:          finalQuestions,
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
