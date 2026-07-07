package main

import (
	"context"
	"log"
	"net/http"
	"os"

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

	addr := os.Getenv("BIN_EVAL_LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	handler := api.NewRouter(api.Dependencies{
		Store: store,
		Starter: api.TemporalStarter{
			Client:    temporalClient,
			TaskQueue: cfg.TemporalTaskQ,
		},
	})
	log.Printf("bin-eval-api listening on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}
