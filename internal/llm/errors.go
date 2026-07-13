package llm

import (
	"github.com/kirilligum/self-imp-bin-eval/internal/failure"
)

const (
	ErrorClassModelOutputInvalid = failure.ClassModelOutputInvalid
	ErrorClassInfraRetryable     = failure.ClassInfraRetryable
	ErrorClassInfraNonRetryable  = failure.ClassInfraNonRetryable
)

type ModelOutputError struct {
	Message string
	Cause   error
}

func (e *ModelOutputError) Error() string { return e.Message }

func (e *ModelOutputError) Unwrap() error { return e.Cause }

type InfraError struct {
	Message   string
	Retryable bool
	Cause     error
}

func (e *InfraError) Error() string {
	return e.Message
}

func (e *InfraError) Unwrap() error { return e.Cause }

func IsModelOutputInvalid(err error) bool {
	_, ok := err.(*ModelOutputError)
	return ok
}

func IsRetryableInfraError(err error) bool {
	infra, ok := err.(*InfraError)
	return ok && infra.Retryable
}
