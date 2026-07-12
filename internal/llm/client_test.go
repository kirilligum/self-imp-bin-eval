package llm

import (
	"context"
	"encoding/json"
	"errors"
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

	t.Run("HTTP status classification", func(t *testing.T) {
		for _, tc := range []struct {
			status    int
			retryable bool
		}{
			{status: http.StatusBadRequest, retryable: false},
			{status: http.StatusUnauthorized, retryable: false},
			{status: http.StatusForbidden, retryable: false},
			{status: http.StatusTooManyRequests, retryable: true},
			{status: http.StatusInternalServerError, retryable: true},
		} {
			t.Run(http.StatusText(tc.status), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "provider error", tc.status)
				}))
				defer server.Close()

				client := NewHTTPClient(server.URL, "", server.Client())
				var out QuestionGenerationOutput
				err := client.GenerateJSON(context.Background(), testQuestionGenerationRequest("model"), &out)
				var infra *InfraError
				if !errors.As(err, &infra) {
					t.Fatalf("error = %T %v, want infra", err, err)
				}
				if infra.Retryable != tc.retryable {
					t.Fatalf("retryable = %v, want %v for HTTP %d", infra.Retryable, tc.retryable, tc.status)
				}
			})
		}
	})

	t.Run("empty streamed content is non retryable model output", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeResponsesContent(t, w, "")
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "", server.Client())
		var out QuestionGenerationOutput
		err := client.GenerateJSON(context.Background(), testQuestionGenerationRequest("model"), &out)
		assertModelOutputError(t, err)
	})

	t.Run("omits authorization header when api key is empty", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "" {
				t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
			}
			writeResponsesContent(t, w, `{"questions":[{"rationale":"r","question":"q?"}]}`)
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "", server.Client())
		var out QuestionGenerationOutput
		if err := client.GenerateJSON(context.Background(), testQuestionGenerationRequest("model"), &out); err != nil {
			t.Fatalf("GenerateJSON() error = %v", err)
		}
	})

	t.Run("invalid base URL is non retryable infra", func(t *testing.T) {
		client := NewHTTPClient("://bad-url", "", nil)
		var out QuestionGenerationOutput
		err := client.GenerateJSON(context.Background(), testQuestionGenerationRequest("model"), &out)
		var infra *InfraError
		if !errors.As(err, &infra) {
			t.Fatalf("error = %T %v, want infra", err, err)
		}
		if infra.Retryable {
			t.Fatalf("invalid URL was retryable: %v", infra)
		}
	})
}

// TEST-007
func TestReadResponsesStream(t *testing.T) {
	t.Run("concatenates delta chunks and ignores non data lines", func(t *testing.T) {
		content, err := readResponsesStream(strings.NewReader(
			"event: response.output_text.delta\n" +
				"data: " + streamEvent(t, map[string]any{"type": "response.output_text.delta", "delta": "hel"}) + "\n\n" +
				": keepalive\n" +
				"data: " + streamEvent(t, map[string]any{"type": "response.output_text.delta", "delta": "lo"}) + "\n\n" +
				"data: [DONE]\n\n",
		))
		if err != nil {
			t.Fatalf("readResponsesStream() error = %v", err)
		}
		if content != "hello" {
			t.Fatalf("content = %q", content)
		}
	})

	t.Run("done event text wins over deltas", func(t *testing.T) {
		content, err := readResponsesStream(strings.NewReader(
			"data: " + streamEvent(t, map[string]any{"type": "response.output_text.delta", "delta": "partial"}) + "\n\n" +
				"data: " + streamEvent(t, map[string]any{"type": "response.output_text.done", "text": "complete"}) + "\n\n",
		))
		if err != nil {
			t.Fatalf("readResponsesStream() error = %v", err)
		}
		if content != "complete" {
			t.Fatalf("content = %q", content)
		}
	})

	t.Run("invalid event JSON is an error", func(t *testing.T) {
		_, err := readResponsesStream(strings.NewReader("data: {not-json\n\n"))
		if err == nil {
			t.Fatal("expected stream decode error")
		}
	})

	t.Run("failed event reports provider message", func(t *testing.T) {
		_, err := readResponsesStream(strings.NewReader(
			"data: " + streamEvent(t, map[string]any{"type": "response.failed", "response": map[string]any{"error": map[string]any{"message": "rate limited"}}}) + "\n\n",
		))
		if err == nil || !strings.Contains(err.Error(), "rate limited") {
			t.Fatalf("error = %v, want provider message", err)
		}
	})

	t.Run("error event falls back to code", func(t *testing.T) {
		_, err := readResponsesStream(strings.NewReader(
			"data: " + streamEvent(t, map[string]any{"type": "error", "error": map[string]any{"code": "bad_request"}}) + "\n\n",
		))
		if err == nil || !strings.Contains(err.Error(), "bad_request") {
			t.Fatalf("error = %v, want code", err)
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

func streamEvent(t *testing.T, event map[string]any) string {
	t.Helper()
	eventBytes, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return string(eventBytes)
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
