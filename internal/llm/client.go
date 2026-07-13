package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type LLMClient interface {
	GenerateJSON(ctx context.Context, req GenerateRequest, out any) (Trace, error)
}

const (
	DefaultMaxResponseBytes = 8 << 20
	DefaultRequestTimeout   = 4 * time.Minute
)

type Trace struct {
	RequestBody       []byte
	ResponseBody      []byte
	HTTPStatus        int
	ResponseBytesRead int
	ResponseTruncated bool
}

type ResponseSizeError struct {
	ConfiguredLimit int
	ObservedCount   int
}

func (e *ResponseSizeError) Error() string {
	return fmt.Sprintf("LLM response exceeded %d byte capture limit", e.ConfiguredLimit)
}

type HTTPClient struct {
	baseURL          string
	apiKey           string
	httpClient       *http.Client
	maxResponseBytes int
}

func NewHTTPClient(baseURL, apiKey string, httpClient *http.Client) *HTTPClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultRequestTimeout}
	}
	return &HTTPClient{
		baseURL:          strings.TrimRight(baseURL, "/"),
		apiKey:           apiKey,
		httpClient:       httpClient,
		maxResponseBytes: DefaultMaxResponseBytes,
	}
}

func (c *HTTPClient) GenerateJSON(ctx context.Context, req GenerateRequest, out any) (Trace, error) {
	var trace Trace
	input := make([]map[string]any, 0, len(req.Messages))
	for _, message := range req.Messages {
		input = append(input, map[string]any{
			"role": message.Role,
			"content": []map[string]string{
				{"type": "input_text", "text": message.Content},
			},
		})
	}
	body := map[string]any{
		"model":             req.ModelProfile,
		"stream":            true,
		"input":             input,
		"max_output_tokens": 16000,
		"tools": []map[string]any{{
			"type":        "function",
			"name":        req.SchemaName,
			"description": "Provide the structured result for this operation.",
			"parameters":  req.Schema,
			"strict":      true,
		}},
		"tool_choice": map[string]any{
			"type": "function",
			"name": req.SchemaName,
		},
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return trace, &ModelOutputError{Message: "failed to marshal LLM request", Cause: err}
	}
	trace.RequestBody = bodyBytes

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/responses", bytes.NewReader(bodyBytes))
	if err != nil {
		return trace, &InfraError{Message: "failed to build LLM request", Retryable: false, Cause: err}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return trace, &InfraError{Message: "LLM request failed", Retryable: true, Cause: err}
	}
	defer resp.Body.Close()
	trace.HTTPStatus = resp.StatusCode
	responseBody, observed, truncated, err := readBoundedResponse(resp.Body, c.maxResponseBytes)
	trace.ResponseBody = responseBody
	trace.ResponseBytesRead = observed
	trace.ResponseTruncated = truncated
	if err != nil {
		return trace, &InfraError{Message: "failed to read LLM response", Retryable: true, Cause: err}
	}
	if truncated {
		return trace, &ResponseSizeError{ConfiguredLimit: c.maxResponseBytes, ObservedCount: observed}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return trace, &InfraError{
			Message:   fmt.Sprintf("LLM endpoint returned HTTP %d", resp.StatusCode),
			Retryable: resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500,
		}
	}

	content, err := readStructuredResponsesStream(bytes.NewReader(responseBody), req.SchemaName)
	if err != nil {
		return trace, &InfraError{Message: "failed to decode LLM response stream", Retryable: true, Cause: err}
	}
	if strings.TrimSpace(content) == "" {
		return trace, &ModelOutputError{Message: "LLM response contained no JSON content"}
	}
	if err := json.Unmarshal([]byte(content), out); err != nil {
		return trace, &ModelOutputError{Message: "LLM returned invalid JSON content", Cause: err}
	}
	if validatable, ok := out.(Validatable); ok {
		if err := validatable.Validate(); err != nil {
			return trace, &ModelOutputError{Message: "LLM output failed structural validation", Cause: err}
		}
	}
	return trace, nil
}

func readBoundedResponse(r io.Reader, limit int) ([]byte, int, bool, error) {
	if limit <= 0 {
		limit = DefaultMaxResponseBytes
	}
	read, err := io.ReadAll(io.LimitReader(r, int64(limit)+1))
	if err != nil {
		return nil, len(read), false, err
	}
	if len(read) > limit {
		return read[:limit], len(read), true, nil
	}
	return read, len(read), false, nil
}

func readStructuredResponsesStream(r io.Reader, expectedToolName string) (string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), DefaultMaxResponseBytes+1)
	var content strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			break
		}
		var event responsesStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return "", err
		}
		switch event.Type {
		case "response.function_call_arguments.delta":
			content.WriteString(event.Delta)
		case "response.function_call_arguments.done":
			if event.Arguments != "" {
				return event.Arguments, nil
			}
		case "response.output_item.done":
			if event.Item != nil && event.Item.Type == "function_call" && event.Item.Name == expectedToolName && event.Item.Arguments != "" {
				return event.Item.Arguments, nil
			}
		case "response.failed", "error":
			return "", fmt.Errorf("responses stream failed: %s", responseStreamError(event))
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return content.String(), nil
}

type responsesStreamEvent struct {
	Type      string                    `json:"type"`
	Delta     string                    `json:"delta"`
	Arguments string                    `json:"arguments"`
	Item      *responsesFunctionCall    `json:"item"`
	Message   string                    `json:"message"`
	Error     *responsesStreamErrorBody `json:"error"`
	Response  *struct {
		Error *responsesStreamErrorBody `json:"error"`
	} `json:"response"`
}

type responsesFunctionCall struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type responsesStreamErrorBody struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

func responseStreamError(event responsesStreamEvent) string {
	var responseError *responsesStreamErrorBody
	if event.Response != nil {
		responseError = event.Response.Error
	}
	for _, candidate := range []string{
		event.Message,
		errorText(event.Error),
		errorText(responseError),
		event.Type,
	} {
		if strings.TrimSpace(candidate) != "" {
			return candidate
		}
	}
	return "unknown error"
}

func errorText(err *responsesStreamErrorBody) string {
	if err == nil {
		return ""
	}
	if strings.TrimSpace(err.Message) != "" {
		return err.Message
	}
	return err.Code
}
