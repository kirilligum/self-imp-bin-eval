package observability

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestStructuredLogFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LoggerOptions{
		Service:      "bin-eval-api",
		Env:          "test",
		ModelProfile: "checklist-evaluator",
		GitSHA:       "abc123",
		SecretValues: []string{"secret-token"},
	})

	err := logger.Log(Event{
		Time:         time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC),
		Level:        "info",
		RequestID:    "req-1",
		WorkflowID:   "workflow-1",
		EntityID:     "entity-1",
		ActivityType: "question_generation",
		PromptName:   "question_generation",
		Status:       "succeeded",
		ErrorClass:   "",
		DurationMS:   17,
		Fields: map[string]string{
			"safe_detail":     "uses secret-token internally",
			"task":            "raw task must not be logged",
			"context":         "raw context must not be logged",
			"model_answer":    "raw answer must not be logged",
			"prompt_request":  "raw request must not be logged",
			"prompt_response": "raw response must not be logged",
		},
	})
	if err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("logged invalid JSON: %v\n%s", err, buf.String())
	}

	for _, key := range []string{
		"ts", "level", "service", "env", "request_id", "workflow_id",
		"entity_id", "activity_type", "prompt_name", "model_profile",
		"status", "error_class", "duration_ms", "git_sha",
	} {
		if _, ok := got[key]; !ok {
			t.Fatalf("missing log key %s in %#v", key, got)
		}
	}
	for _, rawKey := range []string{"task", "context", "model_answer", "prompt_request", "prompt_response"} {
		if _, ok := got[rawKey]; ok {
			t.Fatalf("raw payload key %s was logged", rawKey)
		}
	}
	if strings.Contains(buf.String(), "secret-token") {
		t.Fatalf("secret value leaked in log: %s", buf.String())
	}
	safeDetail, ok := got["safe_detail"].(string)
	if !ok {
		t.Fatalf("safe_detail missing or non-string: %#v", got["safe_detail"])
	}
	if !strings.Contains(safeDetail, RedactionToken) {
		t.Fatalf("safe_detail redaction = %#v", got["safe_detail"])
	}
}
