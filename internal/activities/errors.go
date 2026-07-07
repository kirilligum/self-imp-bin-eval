package activities

import (
	"errors"

	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
	"github.com/kirilligum/self-imp-bin-eval/internal/llm"
	"go.temporal.io/sdk/temporal"
)

const (
	ErrorClassModelOutputInvalid = llm.ErrorClassModelOutputInvalid
	ErrorClassInfraRetryable     = llm.ErrorClassInfraRetryable
	ErrorClassInfraNonRetryable  = llm.ErrorClassInfraNonRetryable
)

func ToTemporalError(err error) error {
	if err == nil {
		return nil
	}
	var modelOutput *llm.ModelOutputError
	var semantic *evalcore.SemanticError
	if errors.As(err, &modelOutput) || errors.As(err, &semantic) {
		return temporal.NewNonRetryableApplicationError(err.Error(), ErrorClassModelOutputInvalid, err)
	}
	var infra *llm.InfraError
	if errors.As(err, &infra) {
		if infra.Retryable {
			return temporal.NewApplicationError(err.Error(), ErrorClassInfraRetryable)
		}
		return temporal.NewNonRetryableApplicationError(err.Error(), ErrorClassInfraNonRetryable, err)
	}
	return temporal.NewApplicationError(err.Error(), ErrorClassInfraRetryable)
}
