package observability

import (
	"encoding/json"
	"io"
	"strings"
	"time"
)

const RedactionToken = "[REDACTED]"

var rawPayloadFieldNames = map[string]struct{}{
	"task":            {},
	"context":         {},
	"model_answer":    {},
	"prompt_request":  {},
	"prompt_response": {},
}

type LoggerOptions struct {
	Service      string
	Env          string
	ModelProfile string
	GitSHA       string
	SecretValues []string
}

type Logger struct {
	w    io.Writer
	opts LoggerOptions
}

type Event struct {
	Time         time.Time
	Level        string
	RequestID    string
	WorkflowID   string
	EntityID     string
	ActivityType string
	PromptName   string
	ModelProfile string
	Status       string
	ErrorClass   string
	DurationMS   int64
	Fields       map[string]string
}

func NewLogger(w io.Writer, opts LoggerOptions) *Logger {
	return &Logger{w: w, opts: opts}
}

func (l *Logger) Log(event Event) error {
	ts := event.Time
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	level := event.Level
	if level == "" {
		level = "info"
	}
	modelProfile := event.ModelProfile
	if modelProfile == "" {
		modelProfile = l.opts.ModelProfile
	}
	record := map[string]any{
		"ts":            ts.Format(time.RFC3339Nano),
		"level":         level,
		"service":       l.opts.Service,
		"env":           l.opts.Env,
		"request_id":    event.RequestID,
		"workflow_id":   event.WorkflowID,
		"entity_id":     event.EntityID,
		"activity_type": event.ActivityType,
		"prompt_name":   event.PromptName,
		"model_profile": modelProfile,
		"status":        event.Status,
		"error_class":   event.ErrorClass,
		"duration_ms":   event.DurationMS,
		"git_sha":       l.opts.GitSHA,
	}
	for key, value := range event.Fields {
		if _, raw := rawPayloadFieldNames[key]; raw {
			continue
		}
		record[key] = l.redact(value)
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = l.w.Write(append(encoded, '\n'))
	return err
}

func (l *Logger) redact(s string) string {
	redacted := s
	for _, secret := range l.opts.SecretValues {
		if secret == "" {
			continue
		}
		redacted = strings.ReplaceAll(redacted, secret, RedactionToken)
	}
	return redacted
}
