package llm

import "fmt"

const (
	ErrorClassModelOutputInvalid = "model_output_invalid"
	ErrorClassInfraRetryable     = "infra_retryable"
	ErrorClassInfraNonRetryable  = "infra_non_retryable"
)

type ModelOutputError struct {
	Message string
	Cause   error
}

func (e *ModelOutputError) Error() string {
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e *ModelOutputError) Unwrap() error { return e.Cause }

type InfraError struct {
	Message   string
	Retryable bool
	Cause     error
}

func (e *InfraError) Error() string {
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
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
