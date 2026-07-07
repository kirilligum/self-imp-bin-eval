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
}
