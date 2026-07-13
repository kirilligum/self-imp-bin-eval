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

type judgeFuture struct {
	runIndex int
	future   workflow.Future
}

func EvaluateAnswerWorkflow(ctx workflow.Context, in EvaluateAnswerInput) error {
	ctx = withActivityOptions(ctx)
	if err := workflow.ExecuteActivity(ctx, activities.ActivityWriteEvaluationInput, activities.WriteEvaluationInputInput{
		EvaluationID: in.EvaluationID,
		ModelAnswer:  in.ModelAnswer,
	}).Get(ctx, nil); err != nil {
		return failEvaluation(ctx, in.EvaluationID, in.ChecklistID, "write_evaluation_input", err)
	}

	var loaded activities.LoadChecklistResult
	if err := workflow.ExecuteActivity(ctx, activities.ActivityLoadChecklist, activities.LoadChecklistInput{ChecklistID: in.ChecklistID}).Get(ctx, &loaded); err != nil {
		return failEvaluation(ctx, in.EvaluationID, in.ChecklistID, "load_checklist", err)
	}
	if loaded.Checklist.Status != db.StatusSucceeded {
		return failEvaluation(ctx, in.EvaluationID, in.ChecklistID, "load_checklist", fmt.Errorf("checklist %s is not succeeded", in.ChecklistID))
	}
	if err := evalcore.ValidateEvaluationRuns(loaded.Checklist.EvaluationRuns, loaded.Checklist.EvaluationRuns); err != nil {
		return failEvaluation(ctx, in.EvaluationID, in.ChecklistID, "load_checklist", err)
	}

	judgeFutures := make([]judgeFuture, 0, loaded.Checklist.EvaluationRuns)
	for runIndex := 1; runIndex <= loaded.Checklist.EvaluationRuns; runIndex++ {
		judgeFutures = append(judgeFutures, judgeFuture{
			runIndex: runIndex,
			future: workflow.ExecuteActivity(ctx, activities.ActivityJudgeAnswer, activities.JudgeAnswerInput{
				EvaluationID: in.EvaluationID,
				RunIndex:     runIndex,
				Task:         loaded.Task,
				Context:      loaded.Context,
				ModelAnswer:  in.ModelAnswer,
				Questions:    loaded.Checklist.Questions,
			}),
		})
	}
	runJudgments := make([]evalcore.RunJudgment, 0, loaded.Checklist.EvaluationRuns*len(loaded.Checklist.Questions))
	for _, pending := range judgeFutures {
		var judged activities.JudgeAnswerResult
		if err := pending.future.Get(ctx, &judged); err != nil {
			return failEvaluation(ctx, in.EvaluationID, in.ChecklistID, "binary_judging", err)
		}
		for _, judgment := range judged.Judgments {
			runJudgments = append(runJudgments, evalcore.RunJudgment{
				RunIndex:   pending.runIndex,
				QuestionID: judgment.QuestionID,
				Evidence:   judgment.Evidence,
				Answer:     judgment.Answer,
			})
		}
	}

	aggregated, err := evalcore.AggregateJudgments(loaded.Checklist.Questions, runJudgments, loaded.Checklist.EvaluationRuns)
	if err != nil {
		return failEvaluation(ctx, in.EvaluationID, in.ChecklistID, "score_checklist", err)
	}

	if err := workflow.ExecuteActivity(ctx, activities.ActivitySucceedEvaluation, activities.SucceedEvaluationInput{
		EvaluationID: in.EvaluationID,
		ChecklistID:  in.ChecklistID,
		RunJudgments: runJudgments,
		Score:        aggregated.Score,
	}).Get(ctx, nil); err != nil {
		return failEvaluation(ctx, in.EvaluationID, in.ChecklistID, "succeed_evaluation", err)
	}
	return nil
}

func failEvaluation(ctx workflow.Context, evaluationID, checklistID, stage string, cause error) error {
	disconnected, _ := workflow.NewDisconnectedContext(ctx)
	if err := workflow.ExecuteActivity(disconnected, activities.ActivityFailEvaluation, activities.FailEvaluationInput{
		EvaluationID: evaluationID,
		ChecklistID:  checklistID,
		Failure:      workflowFailureDetails(ctx, stage, cause),
	}).Get(disconnected, nil); err != nil {
		return err
	}
	return cause
}
