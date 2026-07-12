package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/kirilligum/self-imp-bin-eval/internal/evalcore"
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
	"BIN_EVAL_LISTEN_ADDR",
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
	ListenAddr      string
	GitSHA          string
	ChecklistLimits evalcore.ChecklistLimits
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
	getAlias := func(name, alias string) string {
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" || value == "replace-with-local-llm-key" {
			value = strings.TrimSpace(os.Getenv(alias))
		}
		if value == "" {
			missing = append(missing, name)
		}
		return value
	}

	limits, err := loadChecklistLimits()
	if err != nil {
		return Config{}, err
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
		LLMAPIKey:       getAlias("BIN_EVAL_LLM_API_KEY", "LITELLM_MASTER_KEY"),
		ModelProfile:    get("BIN_EVAL_MODEL_PROFILE"),
		URL:             get("BIN_EVAL_URL"),
		ListenAddr:      get("BIN_EVAL_LISTEN_ADDR"),
		GitSHA:          strings.TrimSpace(os.Getenv("BIN_EVAL_GIT_SHA")),
		ChecklistLimits: limits,
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	if err := validateListenAddr(cfg.ListenAddr); err != nil {
		return Config{}, err
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

func loadChecklistLimits() (evalcore.ChecklistLimits, error) {
	limits := evalcore.DefaultChecklistLimits()
	var err error
	if limits.MaxDimensions, err = getenvPositiveInt("BIN_EVAL_MAX_DIMENSIONS", limits.MaxDimensions); err != nil {
		return evalcore.ChecklistLimits{}, err
	}
	if limits.MaxCandidatesPerDimension, err = getenvPositiveInt("BIN_EVAL_MAX_CANDIDATES_PER_DIMENSION", limits.MaxCandidatesPerDimension); err != nil {
		return evalcore.ChecklistLimits{}, err
	}
	if limits.MaxSplitCount, err = getenvPositiveInt("BIN_EVAL_MAX_SPLIT_COUNT", limits.MaxSplitCount); err != nil {
		return evalcore.ChecklistLimits{}, err
	}
	if limits.MaxFinalQuestions, err = getenvPositiveInt("BIN_EVAL_MAX_FINAL_QUESTIONS", limits.MaxFinalQuestions); err != nil {
		return evalcore.ChecklistLimits{}, err
	}
	return limits, nil
}

func getenvPositiveInt(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}

func validateListenAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid BIN_EVAL_LISTEN_ADDR: %w", err)
	}
	if isLocalBindHost(host) || strings.EqualFold(os.Getenv("BIN_EVAL_ALLOW_PUBLIC_BIND"), "true") {
		return nil
	}
	return fmt.Errorf("BIN_EVAL_LISTEN_ADDR must bind localhost unless BIN_EVAL_ALLOW_PUBLIC_BIND=true")
}

func isLocalBindHost(host string) bool {
	switch strings.ToLower(strings.Trim(host, "[]")) {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}

func IsMissingEnv(err error) bool {
	return err != nil && errors.Is(err, errMissingEnv{})
}

type errMissingEnv struct{}

func (errMissingEnv) Error() string { return "missing environment" }
