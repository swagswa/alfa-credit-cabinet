package main

import (
	"context"
	"log"

	"go.uber.org/zap"

	"github.com/swagswa/alfa-credit-cabinet/internal/config"
	"github.com/swagswa/alfa-credit-cabinet/internal/mocks"
	"github.com/swagswa/alfa-credit-cabinet/pkg/httpserver"
	"github.com/swagswa/alfa-credit-cabinet/pkg/logger"
	"github.com/swagswa/alfa-credit-cabinet/pkg/svc"
	"github.com/swagswa/alfa-credit-cabinet/pkg/telemetry"
)

func main() {
	ctx := context.Background()
	httpserver.ProbeMode(ctx)

	cfg, err := config.Load[config.Mocks]()
	if err != nil {
		log.Fatal(err)
	}
	lg, err := logger.New(cfg.Level)
	if err != nil {
		log.Fatal(err)
	}

	if err := svc.Run(ctx, lg.Zap, telemetry.New("mocks", cfg.MetricsAddr), lg,
		httpserver.New("http", cfg.Server(), mocks.New())); err != nil {
		lg.Zap.Fatal("mocks stopped", zap.Error(err))
	}
}
