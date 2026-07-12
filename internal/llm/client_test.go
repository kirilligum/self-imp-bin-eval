package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
)

// TEST-007
func TestGenerateJSONClient(t *testing.T) {
	t.Run("sends schema request and decodes valid content", func(t *testing.T) {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			if r.URL.Path != "/v1/responses" {
				t.Fatalf("path = %s", r.URL.Path)
			}
			if r.Header.Get("Authorization") != "Bearer test-key" {
				t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("request body decode error = %v", err)
			}
			if body["model"] != "checklist-evaluator" {
				t.Fatalf("model = %#v", body["model"])
			}
			if body["stream"] != true {
				t.Fatalf("stream = %#v", body["stream"])
			}
			text, ok := body["text"].(map[string]any)
			if !ok {
				t.Fatalf("text format missing: %#v", body)
			}
			format, ok := text["format"].(map[string]any)
			if !ok || format["type"] != "json_schema" {
				t.Fatalf("text.format = %#v", text["format"])
			}
			writeResponsesContent(t, w, `{"questions":[{"rationale":"r","question":"q?"}]}`)
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "test-key", server.Client())
		var out QuestionGenerationOutput
		err := client.GenerateJSON(context.Background(), testQuestionGenerationRequest("checklist-evaluator"), &out)
		if err != nil {
			t.Fatalf("GenerateJSON() error = %v", err)
		}
		if calls.Load() != 1 {
			t.Fatalf("calls = %d", calls.Load())
		}
		if len(out.Questions) != 1 || out.Questions[0].Question != "q?" {
			t.Fatalf("out = %#v", out)
		}
	})

	t.Run("invalid JSON is non retryable model output", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeResponsesContent(t, w, `{not-json`)
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "", server.Client())
		var out QuestionGenerationOutput
		err := client.GenerateJSON(context.Background(), testQuestionGenerationRequest("model"), &out)
		assertModelOutputError(t, err)
	})

	t.Run("schema violation is non retryable model output", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeResponsesContent(t, w, `{"questions":[]}`)
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "", server.Client())
		var out QuestionGenerationOutput
		err := client.GenerateJSON(context.Background(), testQuestionGenerationRequest("model"), &out)
		assertModelOutputError(t, err)
	})

	t.Run("transient HTTP failure is retryable infra", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "temporary", http.StatusServiceUnavailable)
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "", server.Client())
		var out QuestionGenerationOutput
		err := client.GenerateJSON(context.Background(), testQuestionGenerationRequest("model"), &out)
		if !IsRetryableInfraError(err) {
			t.Fatalf("error = %T %v, want retryable infra", err, err)
		}
		if IsModelOutputInvalid(err) {
			t.Fatalf("infra error classified as model output: %v", err)
		}
	})
}

func writeResponsesContent(t *testing.T, w http.ResponseWriter, content string) {
	t.Helper()
	w.Header().Set("Content-Type", "text/event-stream")
	eventBytes, err := json.Marshal(map[string]any{
		"type": "response.output_text.done",
		"text": content,
	})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	_, _ = w.Write([]byte("data: " + string(eventBytes) + "\n\n"))
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
}

func assertModelOutputError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsModelOutputInvalid(err) {
		t.Fatalf("error = %T %v, want invalid model output", err, err)
	}
	if IsRetryableInfraError(err) {
		t.Fatalf("model output error classified as retryable: %v", err)
	}
	if strings.Contains(err.Error(), "test-key") {
		t.Fatalf("error leaked secret: %v", err)
	}
}

var _ = evalcore.AnswerYes

func testQuestionGenerationRequest(model string) GenerateRequest {
	return BuildQuestionGenerationRequest("task", "context", model, evalcore.Dimension{
		ID:        "d1",
		Ordinal:   1,
		Name:      "Correctness",
		Rubric:    "Check correctness.",
		Rationale: "Core dimension.",
	}, evalcore.DefaultChecklistLimits())
}
