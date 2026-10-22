// Package redisdb держит клиент Redis как svc.Service.
package redisdb

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type Config struct {
	Addr     string
	Password string
	DB       int
}

// Client отдаёт команды встроенного клиента, который появляется только в Init:
// репозитории можно собрать заранее, а вызывать после старта.
type Client struct {
	*redis.Client

	cfg Config
}

func New(cfg Config) *Client {
	return &Client{cfg: cfg}
}

func (c *Client) Name() string { return "redis" }

func (c *Client) Init(ctx context.Context) error {
	c.Client = redis.NewClient(&redis.Options{Addr: c.cfg.Addr, Password: c.cfg.Password, DB: c.cfg.DB})
	if err := c.Check(ctx); err != nil {
		_ = c.Close()
		return err
	}
	return nil
}

func (c *Client) Run(context.Context) error { return nil }

func (c *Client) Stop(context.Context) error {
	if c.Client == nil {
		return nil
	}
	if err := c.Close(); err != nil {
		return fmt.Errorf("redis close: %w", err)
	}
	return nil
}

// Check для health. Не Ping: тот уже есть у встроенного клиента с другой
// сигнатурой, и с ним Client перестал бы подходить под redis.Cmdable.
func (c *Client) Check(ctx context.Context) error {
	if err := c.Client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	return nil
}
