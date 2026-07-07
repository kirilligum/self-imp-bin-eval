package main

import (
	"context"
	"log"
	"net/http"

	"github.com/kirilligum/self-imp-bin-eval/internal/activities"
	"github.com/kirilligum/self-imp-bin-eval/internal/artifacts"
	"github.com/kirilligum/self-imp-bin-eval/internal/config"
	"github.com/kirilligum/self-imp-bin-eval/internal/db"
	"github.com/kirilligum/self-imp-bin-eval/internal/llm"
	"github.com/kirilligum/self-imp-bin-eval/internal/workflows"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	store, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	artifactWriter, err := artifacts.NewGarageWriter(cfg.GarageEndpoint, cfg.GarageAccessKey, cfg.GarageSecretKey, cfg.ArtifactBucket)
	if err != nil {
		log.Fatal(err)
	}
	llmClient := llm.NewHTTPClient(cfg.LLMBaseURL, cfg.LLMAPIKey, http.DefaultClient)
	temporalClient, err := client.Dial(client.Options{HostPort: cfg.TemporalAddress})
	if err != nil {
		log.Fatal(err)
	}
	defer temporalClient.Close()

	w := worker.New(temporalClient, cfg.TemporalTaskQ, worker.Options{})
	w.RegisterWorkflow(workflows.CreateChecklistWorkflow)
	w.RegisterWorkflow(workflows.EvaluateAnswerWorkflow)
	activities.Register(w, activities.New(activities.Dependencies{
		Artifacts:    artifactWriter,
		LLM:          llmClient,
		Store:        store,
		ModelProfile: cfg.ModelProfile,
	}))
	if err := w.Run(worker.InterruptCh()); err != nil {
		log.Fatal(err)
	}
}
