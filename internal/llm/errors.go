package llm

import (
	"fmt"
	"strings"
)

const (
	ErrorClassModelOutputInvalid = "model_output_invalid"
	ErrorClassInfraRetryable     = "infra_retryable"
	ErrorClassInfraNonRetryable  = "infra_non_retryable"
)

type ModelOutputError struct {
	Message    string
	Cause      error
	RawContent string
}

func (e *ModelOutputError) Error() string {
	msg := e.Message
	if e.Cause == nil {
		if strings.TrimSpace(e.RawContent) == "" {
			return msg
		}
		return fmt.Sprintf("%s: raw_content=%q", msg, truncateRawContent(e.RawContent))
	}
	msg = fmt.Sprintf("%s: %v", msg, e.Cause)
	if strings.TrimSpace(e.RawContent) == "" {
		return msg
	}
	return fmt.Sprintf("%s: raw_content=%q", msg, truncateRawContent(e.RawContent))
}

func (e *ModelOutputError) Unwrap() error { return e.Cause }

func truncateRawContent(raw string) string {
	raw = strings.TrimSpace(raw)
	const maxLen = 800
	if len(raw) <= maxLen {
		return raw
	}
	return raw[:maxLen] + "...[truncated]"
}

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
