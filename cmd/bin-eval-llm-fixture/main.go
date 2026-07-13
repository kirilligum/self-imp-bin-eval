package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

type responsesRequest struct {
	Input []struct {
		Role    string `json:"role"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"input"`
	Tools []struct {
		Type       string         `json:"type"`
		Name       string         `json:"name"`
		Strict     bool           `json:"strict"`
		Parameters map[string]any `json:"parameters"`
	} `json:"tools"`
	ToolChoice struct {
		Type string `json:"type"`
		Name string `json:"name"`
	} `json:"tool_choice"`
}

const fixtureVersion = "v2"

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Bin-Eval-Fixture-Version", fixtureVersion)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /v1/responses", handleResponses)
	log.Print("deterministic LLM fixture listening on :4000")
	log.Fatal(http.ListenAndServe(":4000", mux))
}

func handleResponses(w http.ResponseWriter, r *http.Request) {
	var request responsesRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := validateStructuredToolRequest(request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload, err := fixtureOutput(request.ToolChoice.Name, userPayload(request.Input))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "encode fixture output", http.StatusInternalServerError)
		return
	}
	event, err := json.Marshal(map[string]any{
		"type":      "response.function_call_arguments.done",
		"name":      request.ToolChoice.Name,
		"arguments": string(encoded),
	})
	if err != nil {
		http.Error(w, "encode fixture event", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Bin-Eval-Fixture-Version", fixtureVersion)
	_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", event)
}

func validateStructuredToolRequest(request responsesRequest) error {
	if request.ToolChoice.Type != "function" || request.ToolChoice.Name == "" {
		return fmt.Errorf("a forced function tool is required")
	}
	for _, tool := range request.Tools {
		if tool.Type == "function" && tool.Name == request.ToolChoice.Name && tool.Strict && len(tool.Parameters) > 0 {
			return nil
		}
	}
	return fmt.Errorf("the forced function tool must have strict schema parameters")
}

func userPayload(input []struct {
	Role    string `json:"role"`
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}) map[string]any {
	for i := len(input) - 1; i >= 0; i-- {
		if input[i].Role != "user" || len(input[i].Content) == 0 {
			continue
		}
		var payload map[string]any
		if json.Unmarshal([]byte(input[i].Content[0].Text), &payload) == nil {
			return payload
		}
	}
	return map[string]any{}
}

func fixtureOutput(schemaName string, payload map[string]any) (any, error) {
	switch schemaName {
	case "dimension_analysis":
		return map[string]any{"dimensions": []map[string]string{
			{"name": "Required content", "rubric": "Check required facts and actions.", "rationale": "Covers positive requirements."},
			{"name": "Safety and exclusions", "rubric": "Check unsupported or prohibited claims.", "rationale": "Covers negative requirements."},
		}}, nil
	case "question_generation":
		return map[string]any{"questions": []map[string]string{
			{"rationale": "first requirement", "question": "Does the answer satisfy requirement one?"},
			{"rationale": "compound requirement", "question": "Does the answer satisfy requirements two and three?"},
			{"rationale": "third requirement", "question": "Does the answer satisfy requirement four?"},
			{"rationale": "fourth requirement", "question": "Does the answer satisfy requirement five?"},
		}}, nil
	case "weight_assignment":
		questions, _ := payload["questions"].([]any)
		weights := make([]map[string]any, 0, len(questions))
		for index, value := range questions {
			question, _ := value.(map[string]any)
			weight := 1
			rationale := "atomic requirement"
			if index == 0 {
				weight = 0
				rationale = "semantic duplicate"
			} else if index == 1 {
				weight = 2
				rationale = "two independently judgeable requirements"
			}
			weights = append(weights, map[string]any{"candidate_question_id": question["id"], "rationale": rationale, "weight": weight})
		}
		return map[string]any{"weights": weights}, nil
	case "question_splitting":
		weight, _ := payload["weight"].(map[string]any)
		count, _ := weight["weight"].(float64)
		questions := make([]map[string]string, 0, int(count))
		for index := 1; index <= int(count); index++ {
			questions = append(questions, map[string]string{"rationale": fmt.Sprintf("atomic part %d", index), "question": fmt.Sprintf("Does the answer satisfy compound requirement part %d?", index)})
		}
		return map[string]any{"questions": questions}, nil
	case "binary_judging":
		questions, _ := payload["questions"].([]any)
		modelAnswer, _ := payload["model_answer"].(string)
		answer := "no"
		evidence := "The answer omits the required concrete details."
		if len(strings.TrimSpace(modelAnswer)) >= 300 {
			answer = "yes"
			evidence = "The answer contains the required concrete details."
		}
		judgments := make([]map[string]any, 0, len(questions))
		for _, value := range questions {
			question, _ := value.(map[string]any)
			judgments = append(judgments, map[string]any{"question_id": question["id"], "evidence": evidence, "answer": answer})
		}
		return map[string]any{"judgments": judgments}, nil
	default:
		return nil, fmt.Errorf("unsupported schema %q", schemaName)
	}
}
