package workflows

import (
	"errors"

	"github.com/kirilligum/self-imp-bin-eval/internal/activities"
	"github.com/kirilligum/self-imp-bin-eval/internal/failure"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

func workflowFailureDetails(ctx workflow.Context, stage string, cause error) failure.Details {
	details := activities.FailureDetails(cause, 1)
	var applicationError *temporal.ApplicationError
	if errors.As(cause, &applicationError) && applicationError.HasDetails() {
		var carried failure.Details
		if err := applicationError.Details(&carried); err == nil {
			details = carried
		}
	}
	details.WorkflowID = workflow.GetInfo(ctx).WorkflowExecution.ID
	details.Stage = stage
	if details.AttemptCount <= 0 {
		details.AttemptCount = 1
	}
	return details
}
