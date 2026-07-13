package failure

import (
	"fmt"
	"path"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
)

const (
	ClassModelOutputInvalid = "model_output_invalid"
	ClassInfraRetryable     = "infra_retryable"
	ClassInfraNonRetryable  = "infra_non_retryable"
)

type Details struct {
	WorkflowID         string                     `json:"workflow_id"`
	Stage              string                     `json:"stage"`
	ErrorClass         string                     `json:"error_class"`
	ErrorCode          string                     `json:"error_code"`
	Message            string                     `json:"message"`
	Retryable          bool                       `json:"retryable"`
	AttemptCount       int                        `json:"attempt_count"`
	Diagnostics        []evalcore.LimitDiagnostic `json:"diagnostics"`
	ArtifactReferences []string                   `json:"artifact_references"`
}

type Record struct {
	ID           string  `json:"id"`
	ChecklistID  *string `json:"-"`
	EvaluationID *string `json:"-"`
	Details
	CreatedAt time.Time `json:"created_at"`
}

func (d Details) Validate() error {
	for name, value := range map[string]string{
		"workflow_id": d.WorkflowID,
		"stage":       d.Stage,
		"error_class": d.ErrorClass,
		"error_code":  d.ErrorCode,
		"message":     d.Message,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("failure %s is required", name)
		}
	}
	switch d.ErrorClass {
	case ClassModelOutputInvalid, ClassInfraRetryable, ClassInfraNonRetryable:
	default:
		return fmt.Errorf("unsupported failure error_class %q", d.ErrorClass)
	}
	if d.AttemptCount <= 0 {
		return fmt.Errorf("failure attempt_count must be positive")
	}
	for _, reference := range d.ArtifactReferences {
		if err := validateArtifactReference(reference); err != nil {
			return err
		}
	}
	return nil
}

func (d Details) Equal(other Details) bool {
	return d.WorkflowID == other.WorkflowID &&
		d.Stage == other.Stage &&
		d.ErrorClass == other.ErrorClass &&
		d.ErrorCode == other.ErrorCode &&
		d.Message == other.Message &&
		d.Retryable == other.Retryable &&
		d.AttemptCount == other.AttemptCount &&
		slices.Equal(d.Diagnostics, other.Diagnostics) &&
		slices.Equal(d.ArtifactReferences, other.ArtifactReferences)
}

func validateArtifactReference(reference string) error {
	if strings.TrimSpace(reference) == "" || path.IsAbs(reference) || path.Clean(reference) != reference || strings.HasPrefix(reference, "../") {
		return fmt.Errorf("invalid failure artifact reference %q", reference)
	}
	for _, r := range reference {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return fmt.Errorf("invalid failure artifact reference %q", reference)
		}
	}
	return nil
}
