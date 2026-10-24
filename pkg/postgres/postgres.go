// Package postgres держит пул pgx как svc.Service и даёт транзакции с повтором.
package postgres

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type Config struct {
	URL            string
	MaxConns       int32
	ConnectTimeout time.Duration
}

// Hook выполняется после подключения, до старта остальных сервисов: миграции и т.п.
type Hook func(ctx context.Context, pool *pgxpool.Pool) error

type Client struct {
	Pool *pgxpool.Pool

	log   *zap.Logger
	cfg   Config
	hooks []Hook
}

func New(log *zap.Logger, cfg Config, hooks ...Hook) *Client {
	return &Client{log: log, cfg: cfg, hooks: hooks}
}

func (c *Client) Name() string { return "postgres" }

func (c *Client) Init(ctx context.Context) error {
	pc, err := pgxpool.ParseConfig(c.cfg.URL)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	if c.cfg.MaxConns > 0 {
		pc.MaxConns = c.cfg.MaxConns
	}
	if c.cfg.ConnectTimeout > 0 {
		pc.ConnConfig.ConnectTimeout = c.cfg.ConnectTimeout
	}
	// запросы спанами в трейсе; без экспортёра провайдер otel ничего не пишет. Взятие
	// соединения из пула спаном не пишем: на каждый запрос лишняя строка в трейсе
	pc.ConnConfig.Tracer = otelpgx.NewTracer(otelpgx.WithSpanNameCtxFunc(spanName),
		otelpgx.WithMetricOperationName(spanName), otelpgx.WithDisableAcquireTracer())

	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return fmt.Errorf("pool: %w", err)
	}
	if err := otelpgx.RecordStats(pool); err != nil {
		pool.Close()
		return fmt.Errorf("pool stats: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("ping: %w", err)
	}
	for _, h := range c.hooks {
		if err := h(ctx, pool); err != nil {
			pool.Close()
			return err
		}
	}

	c.Pool = pool
	c.log.Info("postgres connected")
	return nil
}

func (c *Client) Run(context.Context) error { return nil }

// sqlc кладёт в начало запроса "-- name: ApplicationCard :one", по нему спан и называем,
// иначе в трейсе одни SELECT
var queryName = regexp.MustCompile(`^-- name: (\w+)`)

func spanName(_ context.Context, stmt string) string {
	if m := queryName.FindStringSubmatch(stmt); m != nil {
		return m[1]
	}
	stmt = strings.TrimSpace(stmt)
	// у запросов, написанных руками, бывает комментарий перед SQL
	for strings.HasPrefix(stmt, "--") {
		_, stmt, _ = strings.Cut(stmt, "\n")
		stmt = strings.TrimSpace(stmt)
	}
	op, _, _ := strings.Cut(stmt, " ")
	return op
}

func (c *Client) Stop(context.Context) error {
	if c.Pool != nil {
		c.Pool.Close()
	}
	return nil
}

func (c *Client) Ping(ctx context.Context) error {
	if err := c.Pool.Ping(ctx); err != nil {
		return fmt.Errorf("postgres ping: %w", err)
	}
	return nil
}

// Exec вместе с Query и QueryRow делает Client подходящим для db.New: репозитории
// собираются до старта, а пул появляется только в Init.
func (c *Client) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return c.Pool.Exec(ctx, sql, args...)
}

func (c *Client) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return c.Pool.Query(ctx, sql, args...)
}

func (c *Client) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return c.Pool.QueryRow(ctx, sql, args...)
}
