package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const RedactionToken = "[REDACTED]"

var RequiredEnvNames = []string{
	"BIN_EVAL_ENV",
	"BIN_EVAL_DATABASE_URL",
	"BIN_EVAL_TEMPORAL_ADDRESS",
	"BIN_EVAL_GARAGE_ENDPOINT",
	"BIN_EVAL_GARAGE_ACCESS_KEY",
	"BIN_EVAL_GARAGE_SECRET_KEY",
	"BIN_EVAL_ARTIFACT_BUCKET",
	"BIN_EVAL_LLM_BASE_URL",
	"BIN_EVAL_LLM_API_KEY",
	"BIN_EVAL_MODEL_PROFILE",
	"BIN_EVAL_URL",
}

type Config struct {
	Env             string
	DatabaseURL     string
	TemporalAddress string
	TemporalTaskQ   string
	GarageEndpoint  string
	GarageAccessKey string
	GarageSecretKey string
	ArtifactBucket  string
	LLMBaseURL      string
	LLMAPIKey       string
	ModelProfile    string
	URL             string
	GitSHA          string
}

func Load() (Config, error) {
	var missing []string
	get := func(name string) string {
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			missing = append(missing, name)
		}
		return value
	}

	cfg := Config{
		Env:             get("BIN_EVAL_ENV"),
		DatabaseURL:     get("BIN_EVAL_DATABASE_URL"),
		TemporalAddress: get("BIN_EVAL_TEMPORAL_ADDRESS"),
		TemporalTaskQ:   getenvDefault("BIN_EVAL_TEMPORAL_TASK_QUEUE", "bin-eval"),
		GarageEndpoint:  get("BIN_EVAL_GARAGE_ENDPOINT"),
		GarageAccessKey: get("BIN_EVAL_GARAGE_ACCESS_KEY"),
		GarageSecretKey: get("BIN_EVAL_GARAGE_SECRET_KEY"),
		ArtifactBucket:  get("BIN_EVAL_ARTIFACT_BUCKET"),
		LLMBaseURL:      get("BIN_EVAL_LLM_BASE_URL"),
		LLMAPIKey:       get("BIN_EVAL_LLM_API_KEY"),
		ModelProfile:    get("BIN_EVAL_MODEL_PROFILE"),
		URL:             get("BIN_EVAL_URL"),
		GitSHA:          strings.TrimSpace(os.Getenv("BIN_EVAL_GIT_SHA")),
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return cfg, nil
}

func (c Config) SecretValues() []string {
	candidates := []string{
		c.DatabaseURL,
		c.GarageAccessKey,
		c.GarageSecretKey,
		c.LLMAPIKey,
	}
	secrets := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) != "" {
			secrets = append(secrets, candidate)
		}
	}
	return secrets
}

func (c Config) RedactSecrets(s string) string {
	for _, secret := range c.SecretValues() {
		s = strings.ReplaceAll(s, secret, RedactionToken)
	}
	return s
}

func getenvDefault(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func IsMissingEnv(err error) bool {
	return err != nil && errors.Is(err, errMissingEnv{})
}

type errMissingEnv struct{}

func (errMissingEnv) Error() string { return "missing environment" }
