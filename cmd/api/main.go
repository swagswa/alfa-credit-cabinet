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
	"github.com/swagswa/alfa-credit-cabinet/internal/integration/egrul"
	"github.com/swagswa/alfa-credit-cabinet/internal/rbac"
	"github.com/swagswa/alfa-credit-cabinet/internal/rules"
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

	health := usecase.NewHealth(map[string]usecase.Check{"db": db.Ping, "temporal": tc.Ping})
	e, err := newServer(&cfg, lg.Zap, health)
	if err != nil {
		lg.Zap.Fatal("api", zap.Error(err))
	}

	err = svc.Run(ctx, lg.Zap, lg, db, tc, httpserver.New("http", cfg.Server(), e))
	if err != nil {
		lg.Zap.Fatal("api stopped", zap.Error(err))
	}
}

func newServer(cfg *config.API, lg *zap.Logger, health usecase.HealthUseCase) (*echo.Echo, error) {
	enf, err := rbac.New()
	if err != nil {
		return nil, err //nolint:wrapcheck // rbac уже подписал ошибку
	}
	catalog, err := rules.Products(filepath.Join(cfg.RulesDir, "products.yaml"))
	if err != nil {
		return nil, err //nolint:wrapcheck // в ошибке уже путь к файлу
	}
	products := usecase.NewProduct(catalog)
	companies := usecase.NewCompany(egrul.New(cfg.EGRULURL, cfg.Timeout))

	return server.New(server.Deps{ //nolint:wrapcheck // там же собирается сервер, контекст не нужен
		Log:        lg,
		Handler:    handler.New(health, products, companies),
		Tokens:     token.New(cfg.Secret, cfg.TTL),
		RBAC:       enf,
		CORSOrigin: cfg.CORSOrigin,
	})
}
