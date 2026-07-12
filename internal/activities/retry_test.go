package activities

import (
	"errors"
	"testing"

	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
	"github.com/kirilligum/self-imp-bin-eval/internal/llm"
	"go.temporal.io/sdk/temporal"
)

// TEST-018
func TestRetryClassification(t *testing.T) {
	if err := ToTemporalError(nil); err != nil {
		t.Fatalf("nil error classified as %v", err)
	}

	infra := ToTemporalError(&llm.InfraError{Message: "temporary", Retryable: true})
	var infraApp *temporal.ApplicationError
	if !errors.As(infra, &infraApp) {
		t.Fatalf("infra error type = %T", infra)
	}
	if infraApp.Type() != ErrorClassInfraRetryable || infraApp.NonRetryable() {
		t.Fatalf("infra app error type=%s nonRetryable=%v", infraApp.Type(), infraApp.NonRetryable())
	}

	model := ToTemporalError(&llm.ModelOutputError{Message: "invalid"})
	var modelApp *temporal.ApplicationError
	if !errors.As(model, &modelApp) {
		t.Fatalf("model error type = %T", model)
	}
	if modelApp.Type() != ErrorClassModelOutputInvalid || !modelApp.NonRetryable() {
		t.Fatalf("model app error type=%s nonRetryable=%v", modelApp.Type(), modelApp.NonRetryable())
	}

	semantic := ToTemporalError(&evalcore.SemanticError{Code: evalcore.CodeInvalidWeights, Message: "bad weights"})
	var semanticApp *temporal.ApplicationError
	if !errors.As(semantic, &semanticApp) {
		t.Fatalf("semantic error type = %T", semantic)
	}
	if semanticApp.Type() != ErrorClassModelOutputInvalid || !semanticApp.NonRetryable() {
		t.Fatalf("semantic app error type=%s nonRetryable=%v", semanticApp.Type(), semanticApp.NonRetryable())
	}

	nonRetryableInfra := ToTemporalError(&llm.InfraError{Message: "bad request", Retryable: false})
	var nonRetryableInfraApp *temporal.ApplicationError
	if !errors.As(nonRetryableInfra, &nonRetryableInfraApp) {
		t.Fatalf("nonretryable infra error type = %T", nonRetryableInfra)
	}
	if nonRetryableInfraApp.Type() != ErrorClassInfraNonRetryable || !nonRetryableInfraApp.NonRetryable() {
		t.Fatalf("nonretryable infra app error type=%s nonRetryable=%v", nonRetryableInfraApp.Type(), nonRetryableInfraApp.NonRetryable())
	}

	unknown := ToTemporalError(errors.New("postgres connection reset"))
	var unknownApp *temporal.ApplicationError
	if !errors.As(unknown, &unknownApp) {
		t.Fatalf("unknown error type = %T", unknown)
	}
	if unknownApp.Type() != ErrorClassInfraRetryable || unknownApp.NonRetryable() {
		t.Fatalf("unknown app error type=%s nonRetryable=%v", unknownApp.Type(), unknownApp.NonRetryable())
	}
}
