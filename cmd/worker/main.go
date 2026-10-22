package main

import (
	"context"
	"log"

	sdkworker "go.temporal.io/sdk/worker"
	"go.uber.org/zap"

	"github.com/swagswa/alfa-credit-cabinet/internal/config"
	dbq "github.com/swagswa/alfa-credit-cabinet/internal/db"
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

	cfg, err := config.Load[config.Worker]()
	if err != nil {
		log.Fatal(err)
	}
	lg, err := logger.New(cfg.Level)
	if err != nil {
		log.Fatal(err)
	}

	// миграции накатывает api, воркер только читает и пишет
	db := postgres.New(lg.Zap, cfg.Postgres.Client())
	tc := temporalsvc.NewClient(lg.Zap, cfg.Temporal.Client())
	apps := usecase.NewApplication(db, repo.NewApplication(dbq.New(db)), nil, tc)
	appActs := activity.NewApplicationActivities(apps)

	w := temporalsvc.NewWorker(tc, usecase.QueueCredit, sdkworker.Options{}, func(r sdkworker.Registry) {
		r.RegisterWorkflow(workflow.ApplicationWorkflow)
		r.RegisterActivity(appActs)
	})

	if err := svc.Run(ctx, lg.Zap, lg, db, tc, w); err != nil {
		lg.Zap.Fatal("worker stopped", zap.Error(err))
	}
}
