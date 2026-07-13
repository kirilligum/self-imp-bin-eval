package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponsesEndpointAcceptsOpenAICompatibleRequest(t *testing.T) {
	body := `{
		"model":"deterministic-fixture",
		"stream":true,
		"input":[{"role":"user","content":[{"type":"input_text","text":"{\"task\":\"test\"}"}]}],
		"max_output_tokens":16000,
		"tools":[{"type":"function","name":"dimension_analysis","strict":true,"parameters":{"type":"object"}}],
		"tool_choice":{"type":"function","name":"dimension_analysis"}
	}`
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	response := httptest.NewRecorder()

	handleResponses(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", contentType)
	}
	if version := response.Header().Get("X-Bin-Eval-Fixture-Version"); version != fixtureVersion {
		t.Fatalf("fixture version = %q, want %q", version, fixtureVersion)
	}
	line := strings.TrimPrefix(strings.Split(response.Body.String(), "\n")[0], "data: ")
	var event struct {
		Type      string `json:"type"`
		Arguments string `json:"arguments"`
	}
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		t.Fatalf("decode response event: %v", err)
	}
	if event.Type != "response.function_call_arguments.done" {
		t.Fatalf("event type = %q", event.Type)
	}
	var output struct {
		Dimensions []map[string]string `json:"dimensions"`
	}
	if err := json.Unmarshal([]byte(event.Arguments), &output); err != nil {
		t.Fatalf("decode fixture output: %v", err)
	}
	if len(output.Dimensions) != 2 {
		t.Fatalf("dimension count = %d, want 2", len(output.Dimensions))
	}
}

func TestResponsesEndpointRejectsTrailingJSON(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"tools":[{"type":"function","name":"dimension_analysis","strict":true,"parameters":{"type":"object"}}],"tool_choice":{"type":"function","name":"dimension_analysis"}}{}`))
	response := httptest.NewRecorder()

	handleResponses(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}
