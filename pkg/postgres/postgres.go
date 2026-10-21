// Package postgres держит пул pgx как svc.Service и даёт транзакции с повтором.
package postgres

import (
	"context"
	"fmt"
	"time"

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

	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return fmt.Errorf("pool: %w", err)
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
