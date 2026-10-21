package main

import (
	"context"
	"log"

	sdkworker "go.temporal.io/sdk/worker"
	"go.uber.org/zap"

	"github.com/swagswa/alfa-credit-cabinet/internal/config"
	"github.com/swagswa/alfa-credit-cabinet/internal/worker/workflow"
	"github.com/swagswa/alfa-credit-cabinet/pkg/logger"
	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
	"github.com/swagswa/alfa-credit-cabinet/pkg/svc"
	"github.com/swagswa/alfa-credit-cabinet/pkg/temporalsvc"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load[config.DocsWorker]()
	if err != nil {
		log.Fatal(err)
	}
	lg, err := logger.New(cfg.Level)
	if err != nil {
		log.Fatal(err)
	}

	db := postgres.New(lg.Zap, cfg.Postgres.Client())
	tc := temporalsvc.NewClient(lg.Zap, cfg.Temporal.Client())
	// модель на CPU занимает все ядра, две генерации параллельно только мешают друг другу
	opts := sdkworker.Options{MaxConcurrentActivityExecutionSize: 1}
	w := temporalsvc.NewWorker(tc, workflow.QueueDocs, opts, func(r sdkworker.Registry) {
		r.RegisterWorkflow(workflow.RecognizeWorkflow)
	})

	if err := svc.Run(ctx, lg.Zap, lg, db, tc, w); err != nil {
		lg.Zap.Fatal("docs-worker stopped", zap.Error(err))
	}
}
