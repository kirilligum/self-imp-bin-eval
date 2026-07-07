//go:build integration

package artifacts

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"
)

// TEST-011
func TestGarageArtifactWriterAndKeyLayout(t *testing.T) {
	checklistID := "00000000-0000-0000-0000-000000000001"
	evaluationID := "00000000-0000-0000-0000-000000000002"

	expected := []string{
		"checklists/" + checklistID + "/inputs/task.txt",
		"checklists/" + checklistID + "/inputs/context.txt",
		"checklists/" + checklistID + "/llm/question_generation/request.json",
		"checklists/" + checklistID + "/llm/question_generation/response.json",
		"checklists/" + checklistID + "/llm/weight_assignment/request.json",
		"checklists/" + checklistID + "/llm/weight_assignment/response.json",
		"evaluations/" + evaluationID + "/inputs/model_answer.txt",
		"evaluations/" + evaluationID + "/llm/binary_judging/request.json",
		"evaluations/" + evaluationID + "/llm/binary_judging/response.json",
	}
	got := RequiredKeys(checklistID, evaluationID)
	if len(got) != len(expected) {
		t.Fatalf("RequiredKeys len = %d", len(got))
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("RequiredKeys[%d] = %q, want %q", i, got[i], expected[i])
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	writer := newTestGarageWriter(t)
	payload := []byte("byte-preserving artifact payload\n")
	key := ChecklistLLMResponseKey(checklistID, PromptQuestionGeneration)

	waitForGarage(t, ctx, func() error {
		return writer.Write(ctx, key, payload)
	})
	read, err := writer.Read(ctx, key)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !bytes.Equal(read, payload) {
		t.Fatalf("artifact bytes changed: got %q want %q", read, payload)
	}
}

func newTestGarageWriter(t *testing.T) *GarageWriter {
	t.Helper()
	endpoint := getenvDefault("BIN_EVAL_GARAGE_ENDPOINT", "http://127.0.0.1:3900")
	accessKey := getenvDefault("BIN_EVAL_GARAGE_ACCESS_KEY", "GK0123456789abcdef0123456789abcdef")
	secretKey := getenvDefault("BIN_EVAL_GARAGE_SECRET_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	bucket := getenvDefault("BIN_EVAL_ARTIFACT_BUCKET", "bin-eval-artifacts")
	writer, err := NewGarageWriter(endpoint, accessKey, secretKey, bucket)
	if err != nil {
		t.Fatalf("NewGarageWriter() error = %v", err)
	}
	return writer
}

func waitForGarage(t *testing.T, ctx context.Context, fn func() error) {
	t.Helper()
	var last error
	for {
		last = fn()
		if last == nil {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("garage unavailable: %v", last)
		case <-time.After(time.Second):
		}
	}
}

func getenvDefault(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}
