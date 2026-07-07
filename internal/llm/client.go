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
	GenerateJSON(ctx context.Context, req GenerateRequest, out any) error
}

type HTTPClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewHTTPClient(baseURL, apiKey string, httpClient *http.Client) *HTTPClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &HTTPClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: httpClient,
	}
}

func (c *HTTPClient) GenerateJSON(ctx context.Context, req GenerateRequest, out any) error {
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
		"model":  req.ModelProfile,
		"stream": true,
		"input":  input,
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   req.SchemaName,
				"strict": true,
				"schema": req.Schema,
			},
		},
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return &ModelOutputError{Message: "failed to marshal LLM request", Cause: err}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/responses", bytes.NewReader(bodyBytes))
	if err != nil {
		return &InfraError{Message: "failed to build LLM request", Retryable: false, Cause: err}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return &InfraError{Message: "LLM request failed", Retryable: true, Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return &InfraError{
			Message:   fmt.Sprintf("LLM endpoint returned HTTP %d", resp.StatusCode),
			Retryable: resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500,
		}
	}

	content, err := readResponsesStream(resp.Body)
	if err != nil {
		return &InfraError{Message: "failed to decode LLM response stream", Retryable: true, Cause: err}
	}
	if strings.TrimSpace(content) == "" {
		return &ModelOutputError{Message: "LLM response contained no JSON content"}
	}
	if err := json.Unmarshal([]byte(content), out); err != nil {
		return &ModelOutputError{Message: "LLM returned invalid JSON content", Cause: err}
	}
	if validatable, ok := out.(Validatable); ok {
		if err := validatable.Validate(); err != nil {
			return &ModelOutputError{Message: "LLM output failed schema validation", Cause: err}
		}
	}
	return nil
}

func readResponsesStream(r io.Reader) (string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
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
		var event struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
			Text  string `json:"text"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return "", err
		}
		switch event.Type {
		case "response.output_text.delta":
			content.WriteString(event.Delta)
		case "response.output_text.done":
			if event.Text != "" {
				return event.Text, nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return content.String(), nil
}
