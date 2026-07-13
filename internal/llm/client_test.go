package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
)

func TestP06ExactLLMArtifacts(t *testing.T) {
	t.Run("success returns byte-identical request and response bodies", func(t *testing.T) {
		responseBody := []byte("event: response.function_call_arguments.done\n" +
			"data: " + streamEvent(t, map[string]any{"type": "response.function_call_arguments.done", "arguments": `{"questions":[{"rationale":"r","question":"q?"}]}`}) + "\n\n" +
			"data: [DONE]\n\n")
		var receivedRequest []byte
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var err error
			receivedRequest, err = io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request error = %v", err)
			}
			_, _ = w.Write(responseBody)
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "secret-not-in-trace", server.Client())
		var out QuestionGenerationOutput
		trace, err := client.GenerateJSON(context.Background(), testQuestionGenerationRequest("model"), &out)
		if err != nil {
			t.Fatalf("GenerateJSON() error = %v", err)
		}
		if !bytes.Equal(trace.RequestBody, receivedRequest) || !bytes.Equal(trace.ResponseBody, responseBody) {
			t.Fatalf("trace changed transport bytes: trace=%#v", trace)
		}
		if strings.Contains(string(trace.RequestBody), "secret-not-in-trace") {
			t.Fatal("trace captured authorization secret")
		}
	})

	t.Run("invalid output and HTTP errors retain bodies without leaking them in errors", func(t *testing.T) {
		for _, tc := range []struct {
			name      string
			status    int
			body      []byte
			wantModel bool
		}{
			{name: "invalid structured output", status: http.StatusOK, body: []byte("data: " + streamEvent(t, map[string]any{"type": "response.function_call_arguments.done", "arguments": "RAW_INVALID_JSON"}) + "\n\n"), wantModel: true},
			{name: "HTTP error body", status: http.StatusServiceUnavailable, body: []byte("RAW_HTTP_ERROR_BODY")},
			{name: "streamed provider error", status: http.StatusOK, body: []byte("data: " + streamEvent(t, map[string]any{"type": "error", "message": "RAW_STREAM_ERROR"}) + "\n\n")},
		} {
			t.Run(tc.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(tc.status)
					_, _ = w.Write(tc.body)
				}))
				defer server.Close()
				client := NewHTTPClient(server.URL, "", server.Client())
				var out QuestionGenerationOutput
				trace, err := client.GenerateJSON(context.Background(), testQuestionGenerationRequest("model"), &out)
				if err == nil {
					t.Fatal("expected error")
				}
				if !bytes.Equal(trace.ResponseBody, tc.body) {
					t.Fatalf("response trace = %q, want %q", trace.ResponseBody, tc.body)
				}
				if strings.Contains(err.Error(), "RAW_") {
					t.Fatalf("error leaked raw body: %v", err)
				}
				if tc.wantModel && !IsModelOutputInvalid(err) {
					t.Fatalf("error = %T, want model output error", err)
				}
			})
		}
	})

	t.Run("response overflow is bounded and explicit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(strings.Repeat("x", 65)))
		}))
		defer server.Close()
		client := NewHTTPClient(server.URL, "", server.Client())
		client.maxResponseBytes = 64
		var out QuestionGenerationOutput
		trace, err := client.GenerateJSON(context.Background(), testQuestionGenerationRequest("model"), &out)
		var sizeErr *ResponseSizeError
		if !errors.As(err, &sizeErr) || !trace.ResponseTruncated || len(trace.ResponseBody) != 64 || trace.ResponseBytesRead != 65 {
			t.Fatalf("overflow trace/error = %#v / %T %v", trace, err, err)
		}
	})
}

func TestGenerateJSONClient(t *testing.T) {
	t.Run("default client has a bounded request timeout", func(t *testing.T) {
		client := NewHTTPClient("http://127.0.0.1", "", nil)
		if client.httpClient.Timeout != DefaultRequestTimeout {
			t.Fatalf("HTTP timeout = %s, want %s", client.httpClient.Timeout, DefaultRequestTimeout)
		}
	})

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
			tools, ok := body["tools"].([]any)
			if !ok || len(tools) != 1 {
				t.Fatalf("strict function tool missing: %#v", body)
			}
			tool, ok := tools[0].(map[string]any)
			if !ok || tool["type"] != "function" || tool["name"] != "question_generation" || tool["strict"] != true || tool["parameters"] == nil {
				t.Fatalf("tool = %#v", tools[0])
			}
			choice, ok := body["tool_choice"].(map[string]any)
			if !ok || choice["type"] != "function" || choice["name"] != "question_generation" {
				t.Fatalf("tool_choice = %#v", body["tool_choice"])
			}
			if _, legacy := body["text"]; legacy {
				t.Fatalf("request contains unsupported text.format: %#v", body["text"])
			}
			writeResponsesContent(t, w, `{"questions":[{"rationale":"r","question":"q?"}]}`)
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "test-key", server.Client())
		var out QuestionGenerationOutput
		_, err := client.GenerateJSON(context.Background(), testQuestionGenerationRequest("checklist-evaluator"), &out)
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
		_, err := client.GenerateJSON(context.Background(), testQuestionGenerationRequest("model"), &out)
		assertModelOutputError(t, err)
	})

	t.Run("schema violation is non retryable model output", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeResponsesContent(t, w, `{"questions":[]}`)
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "", server.Client())
		var out QuestionGenerationOutput
		_, err := client.GenerateJSON(context.Background(), testQuestionGenerationRequest("model"), &out)
		assertModelOutputError(t, err)
	})

	t.Run("transient HTTP failure is retryable infra", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "temporary", http.StatusServiceUnavailable)
		}))
		defer server.Close()

		client := NewHTTPClient(server.URL, "", server.Client())
		var out QuestionGenerationOutput
		_, err := client.GenerateJSON(context.Background(), testQuestionGenerationRequest("model"), &out)
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
				_, err := client.GenerateJSON(context.Background(), testQuestionGenerationRequest("model"), &out)
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
		_, err := client.GenerateJSON(context.Background(), testQuestionGenerationRequest("model"), &out)
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
		if _, err := client.GenerateJSON(context.Background(), testQuestionGenerationRequest("model"), &out); err != nil {
			t.Fatalf("GenerateJSON() error = %v", err)
		}
	})

	t.Run("invalid base URL is non retryable infra", func(t *testing.T) {
		client := NewHTTPClient("://bad-url", "", nil)
		var out QuestionGenerationOutput
		_, err := client.GenerateJSON(context.Background(), testQuestionGenerationRequest("model"), &out)
		var infra *InfraError
		if !errors.As(err, &infra) {
			t.Fatalf("error = %T %v, want infra", err, err)
		}
		if infra.Retryable {
			t.Fatalf("invalid URL was retryable: %v", infra)
		}
	})
}

func TestReadStructuredResponsesStream(t *testing.T) {
	t.Run("concatenates delta chunks and ignores non data lines", func(t *testing.T) {
		content, err := readStructuredResponsesStream(strings.NewReader(
			"event: response.function_call_arguments.delta\n"+
				"data: "+streamEvent(t, map[string]any{"type": "response.function_call_arguments.delta", "delta": "hel"})+"\n\n"+
				": keepalive\n"+
				"data: "+streamEvent(t, map[string]any{"type": "response.function_call_arguments.delta", "delta": "lo"})+"\n\n"+
				"data: [DONE]\n\n",
		), "question_generation")
		if err != nil {
			t.Fatalf("readStructuredResponsesStream() error = %v", err)
		}
		if content != "hello" {
			t.Fatalf("content = %q", content)
		}
	})

	t.Run("done event arguments win over deltas", func(t *testing.T) {
		content, err := readStructuredResponsesStream(strings.NewReader(
			"data: "+streamEvent(t, map[string]any{"type": "response.function_call_arguments.delta", "delta": "partial"})+"\n\n"+
				"data: "+streamEvent(t, map[string]any{"type": "response.function_call_arguments.done", "arguments": "complete"})+"\n\n",
		), "question_generation")
		if err != nil {
			t.Fatalf("readStructuredResponsesStream() error = %v", err)
		}
		if content != "complete" {
			t.Fatalf("content = %q", content)
		}
	})

	t.Run("accepts the forced function output item", func(t *testing.T) {
		content, err := readStructuredResponsesStream(strings.NewReader(
			"data: "+streamEvent(t, map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "function_call", "name": "question_generation", "arguments": "complete"}})+"\n\n",
		), "question_generation")
		if err != nil || content != "complete" {
			t.Fatalf("content/error = %q / %v", content, err)
		}
	})

	t.Run("ignores a function output for another tool", func(t *testing.T) {
		content, err := readStructuredResponsesStream(strings.NewReader(
			"data: "+streamEvent(t, map[string]any{"type": "response.output_item.done", "item": map[string]any{"type": "function_call", "name": "other", "arguments": "unexpected"}})+"\n\n",
		), "question_generation")
		if err != nil || content != "" {
			t.Fatalf("content/error = %q / %v", content, err)
		}
	})

	t.Run("invalid event JSON is an error", func(t *testing.T) {
		_, err := readStructuredResponsesStream(strings.NewReader("data: {not-json\n\n"), "question_generation")
		if err == nil {
			t.Fatal("expected stream decode error")
		}
	})

	t.Run("failed event reports provider message", func(t *testing.T) {
		_, err := readStructuredResponsesStream(strings.NewReader(
			"data: "+streamEvent(t, map[string]any{"type": "response.failed", "response": map[string]any{"error": map[string]any{"message": "rate limited"}}})+"\n\n",
		), "question_generation")
		if err == nil || !strings.Contains(err.Error(), "rate limited") {
			t.Fatalf("error = %v, want provider message", err)
		}
	})

	t.Run("error event falls back to code", func(t *testing.T) {
		_, err := readStructuredResponsesStream(strings.NewReader(
			"data: "+streamEvent(t, map[string]any{"type": "error", "error": map[string]any{"code": "bad_request"}})+"\n\n",
		), "question_generation")
		if err == nil || !strings.Contains(err.Error(), "bad_request") {
			t.Fatalf("error = %v, want code", err)
		}
	})
}

func writeResponsesContent(t *testing.T, w http.ResponseWriter, content string) {
	t.Helper()
	w.Header().Set("Content-Type", "text/event-stream")
	eventBytes, err := json.Marshal(map[string]any{
		"type":      "response.function_call_arguments.done",
		"arguments": content,
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
