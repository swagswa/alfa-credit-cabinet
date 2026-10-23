package main

import (
	"context"
	"log"

	sdkworker "go.temporal.io/sdk/worker"
	"go.uber.org/zap"

	"github.com/swagswa/alfa-credit-cabinet/internal/config"
	"github.com/swagswa/alfa-credit-cabinet/internal/integration/ocr"
	"github.com/swagswa/alfa-credit-cabinet/internal/integration/ollama"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
	"github.com/swagswa/alfa-credit-cabinet/internal/worker/activity"
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
	text := ocr.New(cfg.Tesseract, cfg.OCRTimeout)
	llm, err := ollama.New(cfg.OllamaURL, cfg.OllamaModel, cfg.OllamaTimeout)
	if err != nil {
		lg.Zap.Fatal("docs-worker", zap.Error(err))
	}
	w := temporalsvc.NewWorker(tc, usecase.QueueDocs, opts, func(r sdkworker.Registry) {
		// регистрация идёт в Init воркера, к этому моменту пул postgres уже поднят
		docs := usecase.NewDocument(repo.NewDocument(db.Pool), nil, nil, text, llm, cfg.AIEnabled)
		r.RegisterWorkflow(workflow.RecognizeWorkflow)
		r.RegisterActivity(activity.NewDocumentActivities(docs))
	})

	if err := svc.Run(ctx, lg.Zap, lg, db, tc, w); err != nil {
		lg.Zap.Fatal("docs-worker stopped", zap.Error(err))
	}
}
