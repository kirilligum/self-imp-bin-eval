package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kirilligum/self-imp-bin-eval/internal/artifacts"
)

type prefixesFlag []string

func (p *prefixesFlag) String() string { return strings.Join(*p, ",") }

func (p *prefixesFlag) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("prefix cannot be blank")
	}
	*p = append(*p, value)
	return nil
}

type evidenceManifest struct {
	GitSHA         string                     `json:"git_sha"`
	EndpointClass  string                     `json:"endpoint_class"`
	FixtureVersion string                     `json:"fixture_version"`
	ModelProfile   string                     `json:"model_profile"`
	Objects        []artifacts.ExportedObject `json:"objects"`
}

func main() {
	output := flag.String("output", "", "directory for exact Garage artifacts")
	var prefixes prefixesFlag
	flag.Var(&prefixes, "prefix", "Garage object prefix to export; repeat for each entity")
	flag.Parse()
	if *output == "" || len(prefixes) == 0 {
		fatal("output and at least one prefix are required")
	}

	writer, err := artifacts.NewGarageWriter(
		requireEnv("BIN_EVAL_GARAGE_ENDPOINT"),
		requireEnv("BIN_EVAL_GARAGE_ACCESS_KEY"),
		requireEnv("BIN_EVAL_GARAGE_SECRET_KEY"),
		requireEnv("BIN_EVAL_ARTIFACT_BUCKET"),
	)
	if err != nil {
		fatal(err.Error())
	}
	objects, err := writer.Export(context.Background(), *output, prefixes)
	if err != nil {
		fatal(err.Error())
	}
	manifest := evidenceManifest{
		GitSHA:         envDefault("BIN_EVAL_GIT_SHA", "unknown"),
		EndpointClass:  envDefault("BIN_EVAL_ENDPOINT_CLASS", "local"),
		FixtureVersion: envDefault("BIN_EVAL_FIXTURE_VERSION", "not-applicable"),
		ModelProfile:   envDefault("BIN_EVAL_MODEL_PROFILE", "unknown"),
		Objects:        objects,
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		fatal(err.Error())
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(filepath.Join(*output, "manifest.json"), payload, 0o600); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("captured %d exact Garage artifacts under %s\n", len(objects), *output)
}

func requireEnv(name string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		fatal(name + " is required")
	}
	return value
}

func envDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
