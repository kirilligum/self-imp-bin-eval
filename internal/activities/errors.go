package activities

import (
	"context"
	"errors"

	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
	"github.com/kirilligum/self-imp-bin-eval/internal/failure"
	"github.com/kirilligum/self-imp-bin-eval/internal/llm"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
)

const (
	ErrorClassModelOutputInvalid = llm.ErrorClassModelOutputInvalid
	ErrorClassInfraRetryable     = llm.ErrorClassInfraRetryable
	ErrorClassInfraNonRetryable  = llm.ErrorClassInfraNonRetryable
)

func ToTemporalError(ctx context.Context, err error, artifactReferences ...string) error {
	if err == nil {
		return nil
	}
	details := FailureDetails(err, activityAttempt(ctx))
	details.ArtifactReferences = append([]string(nil), artifactReferences...)
	return temporal.NewApplicationErrorWithOptions(details.Message, details.ErrorClass, temporal.ApplicationErrorOptions{
		NonRetryable: !details.Retryable,
		Details:      []any{details},
	})
}

func FailureDetails(err error, attemptCount int) failure.Details {
	if attemptCount <= 0 {
		attemptCount = 1
	}
	var modelOutput *llm.ModelOutputError
	var semantic *evalcore.SemanticError
	if errors.As(err, &modelOutput) {
		return failure.Details{
			ErrorClass:   ErrorClassModelOutputInvalid,
			ErrorCode:    "model_output_invalid",
			Message:      modelOutput.Message,
			AttemptCount: attemptCount,
		}
	}
	if errors.As(err, &semantic) {
		return failure.Details{
			ErrorClass:   ErrorClassModelOutputInvalid,
			ErrorCode:    string(semantic.Code),
			Message:      semantic.Message,
			AttemptCount: attemptCount,
			Diagnostics:  semantic.Diagnostics,
		}
	}
	var infra *llm.InfraError
	if errors.As(err, &infra) {
		details := failure.Details{
			ErrorCode:    "llm_infrastructure_error",
			Message:      infra.Message,
			Retryable:    infra.Retryable,
			AttemptCount: attemptCount,
		}
		if infra.Retryable {
			details.ErrorClass = ErrorClassInfraRetryable
			return details
		}
		details.ErrorClass = ErrorClassInfraNonRetryable
		return details
	}
	var responseSize *llm.ResponseSizeError
	if errors.As(err, &responseSize) {
		return failure.Details{
			ErrorClass:   ErrorClassInfraNonRetryable,
			ErrorCode:    "llm_response_too_large",
			Message:      responseSize.Error(),
			AttemptCount: attemptCount,
			Diagnostics: []evalcore.LimitDiagnostic{{
				LimitName:       "max_llm_response_bytes",
				ConfiguredLimit: responseSize.ConfiguredLimit,
				ObservedCount:   responseSize.ObservedCount,
			}},
		}
	}
	return failure.Details{
		ErrorClass:   ErrorClassInfraRetryable,
		ErrorCode:    "internal_operation_failed",
		Message:      "internal operation failed",
		Retryable:    true,
		AttemptCount: attemptCount,
	}
}

func activityAttempt(ctx context.Context) int {
	if activity.IsActivity(ctx) {
		return int(activity.GetInfo(ctx).Attempt)
	}
	return 1
}
