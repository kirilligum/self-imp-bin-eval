//go:build integration

package artifacts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGarageArtifactWriterAndKeyLayout(t *testing.T) {
	checklistID := "00000000-0000-0000-0000-000000000001"
	evaluationID := "00000000-0000-0000-0000-000000000002"

	expected := []string{
		"checklists/" + checklistID + "/inputs/task.txt",
		"checklists/" + checklistID + "/inputs/context.txt",
		"checklists/" + checklistID + "/llm/dimension_analysis/attempt-1/request.json",
		"checklists/" + checklistID + "/llm/dimension_analysis/attempt-1/response.body",
		"checklists/" + checklistID + "/llm/weight_assignment/attempt-1/request.json",
		"checklists/" + checklistID + "/llm/weight_assignment/attempt-1/response.body",
		"evaluations/" + evaluationID + "/inputs/model_answer.txt",
		"evaluations/" + evaluationID + "/llm/binary_judging/run-1/attempt-1/request.json",
		"evaluations/" + evaluationID + "/llm/binary_judging/run-1/attempt-1/response.body",
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
	key := ChecklistLLMResponseKey(checklistID, PromptQuestionGeneration+"/d1", 1)

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

func TestP06ExactLLMArtifacts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	writer := newTestGarageWriter(t)
	checklistID := "00000000-0000-0000-0000-000000000003"
	requestKey1 := ChecklistLLMRequestKey(checklistID, PromptDimensionAnalysis, 1)
	responseKey1 := ChecklistLLMResponseKey(checklistID, PromptDimensionAnalysis, 1)
	requestKey2 := ChecklistLLMRequestKey(checklistID, PromptDimensionAnalysis, 2)
	responseKey2 := ChecklistLLMResponseKey(checklistID, PromptDimensionAnalysis, 2)
	keys := []string{requestKey1, responseKey1, requestKey2, responseKey2}
	payloads := make(map[string][]byte, len(keys))
	for i, key := range keys {
		payload := []byte{0, byte(i), '\n', 0xff}
		payloads[key] = payload
		waitForGarage(t, ctx, func() error { return writer.Write(ctx, key, payload) })
		got, err := writer.Read(ctx, key)
		if err != nil {
			t.Fatalf("Read(%q) error = %v", key, err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("Read(%q) = %v, want %v", key, got, payload)
		}
	}
	if requestKey1 == requestKey2 || responseKey1 == responseKey2 {
		t.Fatalf("attempt keys collided: %q %q %q %q", requestKey1, responseKey1, requestKey2, responseKey2)
	}

	outputDir := t.TempDir()
	exported, err := writer.Export(ctx, outputDir, []string{"checklists/" + checklistID + "/"})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	if len(exported) < len(keys) {
		t.Fatalf("Export() object count = %d, want at least %d", len(exported), len(keys))
	}
	for _, object := range exported {
		want, ok := payloads[object.Key]
		if !ok {
			continue
		}
		got, err := os.ReadFile(filepath.Join(outputDir, filepath.FromSlash(object.Key)))
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", object.Key, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("exported %q = %v, want %v", object.Key, got, want)
		}
		digest := sha256.Sum256(want)
		if object.SHA256 != hex.EncodeToString(digest[:]) {
			t.Fatalf("exported %q SHA256 = %q", object.Key, object.SHA256)
		}
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
