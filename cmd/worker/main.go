package main

import (
	"context"
	"log"
	"path/filepath"

	sdkworker "go.temporal.io/sdk/worker"
	"go.uber.org/zap"

	"github.com/swagswa/alfa-credit-cabinet/internal/config"
	dbq "github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/integration/source"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/internal/rules"
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
	q := dbq.New(db)
	appRepo := repo.NewApplication(q)
	sources := source.New(source.URLs{
		EGRUL: cfg.EGRULURL, FNS: cfg.FNSURL, BKI: cfg.BKIURL, Bank: cfg.BankURL,
	}, cfg.Sources.Timeout)
	// кривой yaml роняет старт: решать по неполным правилам хуже, чем не решать
	catalog, err := rules.Products(filepath.Join(cfg.RulesDir, "products.yaml"))
	if err != nil {
		lg.Zap.Fatal("rules", zap.Error(err))
	}
	scoring, err := rules.Scoring(filepath.Join(cfg.RulesDir, "scoring.yaml"))
	if err != nil {
		lg.Zap.Fatal("rules", zap.Error(err))
	}
	policy, err := rules.Policy(filepath.Join(cfg.RulesDir, "policy.yaml"))
	if err != nil {
		lg.Zap.Fatal("rules", zap.Error(err))
	}

	deadlines := domain.Deadlines{NeedInfo: cfg.NeedInfoTTL, Review: cfg.ReviewSLA, Offer: cfg.OfferTTL}
	appActs := activity.NewApplicationActivities(
		usecase.NewApplication(db, appRepo, nil, tc, catalog),
		usecase.NewDecision(catalog, scoring, policy),
		deadlines,
	)
	checkActs := activity.NewCheckActivities(usecase.NewCheck(appRepo, repo.NewCheck(q), sources))

	w := temporalsvc.NewWorker(tc, usecase.QueueCredit, sdkworker.Options{}, func(r sdkworker.Registry) {
		r.RegisterWorkflow(workflow.ApplicationWorkflow)
		r.RegisterActivity(appActs)
		r.RegisterActivity(checkActs)
	})

	if err := svc.Run(ctx, lg.Zap, lg, db, tc, w); err != nil {
		lg.Zap.Fatal("worker stopped", zap.Error(err))
	}
}
