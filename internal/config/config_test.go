package config

import (
	"strings"
	"testing"
)

// TEST-009
func TestConfigValidation(t *testing.T) {
	t.Run("reports missing names without values", func(t *testing.T) {
		t.Setenv("LITELLM_MASTER_KEY", "")
		_, err := Load()
		if err == nil {
			t.Fatal("expected missing env error")
		}
		msg := err.Error()
		for _, name := range RequiredEnvNames {
			if !strings.Contains(msg, name) {
				t.Fatalf("missing env name %s in error %q", name, msg)
			}
		}
		if strings.Contains(msg, "test-secret") {
			t.Fatalf("config error leaked a secret value: %q", msg)
		}
	})

	t.Run("loads required values and exposes secret redaction set", func(t *testing.T) {
		setRequiredEnv(t)

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.Env != "test" {
			t.Fatalf("Env = %q", cfg.Env)
		}
		if cfg.ArtifactBucket != "bin-eval-artifacts" {
			t.Fatalf("ArtifactBucket = %q", cfg.ArtifactBucket)
		}
		if cfg.ListenAddr != "127.0.0.1:8080" {
			t.Fatalf("ListenAddr = %q", cfg.ListenAddr)
		}
		redacted := cfg.RedactSecrets("prefix garage-secret-value and llm-secret-value suffix")
		if strings.Contains(redacted, "garage-secret-value") || strings.Contains(redacted, "llm-secret-value") {
			t.Fatalf("secret values were not redacted: %q", redacted)
		}
		if !strings.Contains(redacted, RedactionToken) {
			t.Fatalf("redaction token missing from %q", redacted)
		}
	})

	t.Run("reuses LiteLLM master key when bin-eval key is placeholder", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("BIN_EVAL_LLM_API_KEY", "replace-with-local-llm-key")
		t.Setenv("LITELLM_MASTER_KEY", "litellm-secret-value")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.LLMAPIKey != "litellm-secret-value" {
			t.Fatalf("LLMAPIKey = %q", cfg.LLMAPIKey)
		}
		if strings.Contains(cfg.RedactSecrets("prefix litellm-secret-value suffix"), "litellm-secret-value") {
			t.Fatal("LiteLLM fallback key was not redacted")
		}
	})

	t.Run("rejects public listen address by default", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("BIN_EVAL_LISTEN_ADDR", "0.0.0.0:8080")

		_, err := Load()
		if err == nil {
			t.Fatal("expected public bind error")
		}
		if !strings.Contains(err.Error(), "BIN_EVAL_LISTEN_ADDR") {
			t.Fatalf("error = %v, want listen address context", err)
		}
	})
}

func setRequiredEnv(t *testing.T) {
	t.Helper()
	values := map[string]string{
		"BIN_EVAL_ENV":               "test",
		"BIN_EVAL_DATABASE_URL":      "postgres://bin_eval:bin_eval@127.0.0.1:54329/bin_eval?sslmode=disable",
		"BIN_EVAL_TEMPORAL_ADDRESS":  "127.0.0.1:7233",
		"BIN_EVAL_GARAGE_ENDPOINT":   "http://127.0.0.1:3900",
		"BIN_EVAL_GARAGE_ACCESS_KEY": "garage-access-value",
		"BIN_EVAL_GARAGE_SECRET_KEY": "garage-secret-value",
		"BIN_EVAL_ARTIFACT_BUCKET":   "bin-eval-artifacts",
		"BIN_EVAL_LLM_BASE_URL":      "http://127.0.0.1:4000",
		"BIN_EVAL_LLM_API_KEY":       "llm-secret-value",
		"BIN_EVAL_MODEL_PROFILE":     "checklist-evaluator",
		"BIN_EVAL_URL":               "http://127.0.0.1:8080",
		"BIN_EVAL_LISTEN_ADDR":       "127.0.0.1:8080",
	}
	for name, value := range values {
		t.Setenv(name, value)
	}
}
