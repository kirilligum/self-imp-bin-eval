package workflows

import (
	"time"

	"github.com/kirilligum/self-imp-bin-eval/internal/activities"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const activityStartToCloseTimeout = 5 * time.Minute

func withActivityOptions(ctx workflow.Context) workflow.Context {
	return workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: activityStartToCloseTimeout,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:        time.Second,
			BackoffCoefficient:     2,
			MaximumInterval:        10 * time.Second,
			MaximumAttempts:        3,
			NonRetryableErrorTypes: []string{activities.ErrorClassModelOutputInvalid, activities.ErrorClassInfraNonRetryable},
		},
	})
}
