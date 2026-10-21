package main

import (
	"context"
	"flag"
	"log"
	"os"

	"go.uber.org/zap"

	"github.com/swagswa/alfa-credit-cabinet/internal/config"
	"github.com/swagswa/alfa-credit-cabinet/internal/mocks"
	"github.com/swagswa/alfa-credit-cabinet/pkg/httpserver"
	"github.com/swagswa/alfa-credit-cabinet/pkg/logger"
	"github.com/swagswa/alfa-credit-cabinet/pkg/svc"
)

func main() {
	probe := flag.String("healthcheck", "", "проверить health по адресу и выйти, для healthcheck в compose")
	flag.Parse()
	ctx := context.Background()

	if *probe != "" {
		if err := httpserver.Probe(ctx, *probe); err != nil {
			log.Print(err)
			os.Exit(1)
		}
		return
	}

	cfg, err := config.Load[config.Mocks]()
	if err != nil {
		log.Fatal(err)
	}
	lg, err := logger.New(cfg.Level)
	if err != nil {
		log.Fatal(err)
	}

	if err := svc.Run(ctx, lg.Zap, lg, httpserver.New("http", cfg.Server(), mocks.New())); err != nil {
		lg.Zap.Fatal("mocks stopped", zap.Error(err))
	}
}
