package main

import (
	"context"
	"log"
	"net/http"

	"github.com/kirilligum/self-imp-bin-eval/internal/api"
	"github.com/kirilligum/self-imp-bin-eval/internal/config"
	"github.com/kirilligum/self-imp-bin-eval/internal/db"
	"go.temporal.io/sdk/client"
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
	if err := store.ApplyMigrations(ctx, "migrations"); err != nil {
		log.Fatal(err)
	}
	temporalClient, err := client.Dial(client.Options{HostPort: cfg.TemporalAddress})
	if err != nil {
		log.Fatal(err)
	}
	defer temporalClient.Close()

	handler := api.NewRouter(api.Dependencies{
		Store:             store,
		MaxEvaluationRuns: cfg.MaxEvaluationRuns,
		Starter: api.TemporalStarter{
			Client:          temporalClient,
			TaskQueue:       cfg.TemporalTaskQ,
			ChecklistLimits: cfg.ChecklistLimits,
		},
	})
	log.Printf("bin-eval-api listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, handler); err != nil {
		log.Fatal(err)
	}
}
