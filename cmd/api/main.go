package main

import (
	"context"
	"flag"
	"log"
	"os"

	"go.uber.org/zap"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/handler"
	"github.com/swagswa/alfa-credit-cabinet/internal/api/server"
	"github.com/swagswa/alfa-credit-cabinet/internal/config"
	"github.com/swagswa/alfa-credit-cabinet/internal/rbac"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
	"github.com/swagswa/alfa-credit-cabinet/migrations"
	"github.com/swagswa/alfa-credit-cabinet/pkg/httpserver"
	"github.com/swagswa/alfa-credit-cabinet/pkg/logger"
	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
	"github.com/swagswa/alfa-credit-cabinet/pkg/svc"
	"github.com/swagswa/alfa-credit-cabinet/pkg/temporalsvc"
	"github.com/swagswa/alfa-credit-cabinet/pkg/token"
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

	cfg, err := config.Load[config.API]()
	if err != nil {
		log.Fatal(err)
	}
	lg, err := logger.New(cfg.Level)
	if err != nil {
		log.Fatal(err)
	}

	db := postgres.New(lg.Zap, cfg.Postgres.Client(), postgres.Migrate(migrations.FS))
	tc := temporalsvc.NewClient(lg.Zap, cfg.Temporal.Client())

	enf, err := rbac.New()
	if err != nil {
		log.Fatal(err)
	}
	health := usecase.NewHealth(map[string]usecase.Check{"db": db.Ping, "temporal": tc.Ping})

	e, err := server.New(server.Deps{
		Log:        lg.Zap,
		Handler:    handler.New(health),
		Tokens:     token.New(cfg.Secret, cfg.TTL),
		RBAC:       enf,
		CORSOrigin: cfg.CORSOrigin,
	})
	if err != nil {
		log.Fatal(err)
	}

	err = svc.Run(ctx, lg.Zap, lg, db, tc, httpserver.New("http", cfg.Server(), e))
	if err != nil {
		lg.Zap.Fatal("api stopped", zap.Error(err))
	}
}
