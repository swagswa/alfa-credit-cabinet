package main

import (
	"context"
	"log"
	"path/filepath"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/handler"
	"github.com/swagswa/alfa-credit-cabinet/internal/api/server"
	"github.com/swagswa/alfa-credit-cabinet/internal/config"
	dbq "github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/integration/egrul"
	"github.com/swagswa/alfa-credit-cabinet/internal/rbac"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/internal/rules"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
	"github.com/swagswa/alfa-credit-cabinet/migrations"
	"github.com/swagswa/alfa-credit-cabinet/pkg/httpserver"
	"github.com/swagswa/alfa-credit-cabinet/pkg/logger"
	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
	"github.com/swagswa/alfa-credit-cabinet/pkg/redisdb"
	"github.com/swagswa/alfa-credit-cabinet/pkg/svc"
	"github.com/swagswa/alfa-credit-cabinet/pkg/temporalsvc"
	"github.com/swagswa/alfa-credit-cabinet/pkg/token"
)

func main() {
	ctx := context.Background()
	httpserver.ProbeMode(ctx)

	cfg, err := config.Load[config.API]()
	if err != nil {
		log.Fatal(err)
	}
	lg, err := logger.New(cfg.Level)
	if err != nil {
		log.Fatal(err)
	}

	hooks := []postgres.Hook{postgres.Migrate(migrations.FS)}
	if cfg.SeedDemo {
		hooks = append(hooks, postgres.Exec(migrations.SeedDemo))
	}
	db := postgres.New(lg.Zap, cfg.Postgres.Client(), hooks...)
	tc := temporalsvc.NewClient(lg.Zap, cfg.Temporal.Client())
	rdb := redisdb.New(cfg.Redis.Client())

	health := usecase.NewHealth(map[string]usecase.Check{"db": db.Ping, "temporal": tc.Ping, "redis": rdb.Check})
	tokens := token.New(cfg.Secret, cfg.TTL)
	limit := usecase.LoginLimit{Attempts: cfg.LoginAttempts, Window: cfg.LoginWindow}
	users := repo.NewUser(dbq.New(db))
	auth := usecase.NewAuth(users, repo.NewSession(rdb), tokens, limit)
	catalog, err := rules.Products(filepath.Join(cfg.RulesDir, "products.yaml"))
	if err != nil {
		lg.Zap.Fatal("rules", zap.Error(err))
	}
	companies := usecase.NewCompany(egrul.New(cfg.EGRULURL, cfg.Timeout))
	apps := usecase.NewApplication(db, repo.NewApplication(dbq.New(db)), companies, tc, catalog)
	events := repo.NewEvent(lg.Zap, db)
	docs := usecase.NewDocument(repo.NewDocument(db), repo.NewApplication(dbq.New(db)), tc, nil, nil, cfg.AIEnabled)

	e, err := newServer(&cfg, lg.Zap, health, auth, usecase.NewProduct(catalog), companies, apps,
		usecase.NewEvent(events, users), docs)
	if err != nil {
		lg.Zap.Fatal("api", zap.Error(err))
	}

	err = svc.Run(ctx, lg.Zap, lg, db, rdb, tc, events, httpserver.New("http", cfg.Server(), e))
	if err != nil {
		lg.Zap.Fatal("api stopped", zap.Error(err))
	}
}

func newServer(
	cfg *config.API,
	lg *zap.Logger,
	health usecase.HealthUseCase,
	auth usecase.AuthUseCase,
	products usecase.ProductUseCase,
	companies usecase.CompanyUseCase,
	apps usecase.ApplicationUseCase,
	events usecase.EventUseCase,
	docs usecase.DocumentUseCase,
) (*echo.Echo, error) {
	enf, err := rbac.New()
	if err != nil {
		return nil, err
	}
	return server.New(server.Deps{
		Log: lg,
		Handler: handler.New(handler.Deps{
			Health:       health,
			Products:     products,
			Companies:    companies,
			Applications: apps,
			Auth:         auth,
			Events:       events,
			Documents:    docs,
			SessionTTL:   cfg.TTL,
			CookieSecure: cfg.CookieSecure,
		}),
		Auth:       auth,
		RBAC:       enf,
		CORSOrigin: cfg.CORSOrigin,
	})
}
