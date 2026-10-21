// Package temporalsvc держит клиент и воркер Temporal как svc.Service.
package temporalsvc

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/client"
	"go.uber.org/zap"

	"github.com/swagswa/alfa-credit-cabinet/pkg/zapadapter"
)

type Config struct {
	HostPort  string
	Namespace string
}

type Client struct {
	client.Client

	log *zap.Logger
	cfg Config
}

func NewClient(log *zap.Logger, cfg Config) *Client {
	return &Client{log: log, cfg: cfg}
}

func (c *Client) Name() string { return "temporal-client" }

func (c *Client) Init(ctx context.Context) error {
	cl, err := client.DialContext(ctx, client.Options{
		HostPort:  c.cfg.HostPort,
		Namespace: c.cfg.Namespace,
		Logger:    zapadapter.New(c.log),
	})
	if err != nil {
		return fmt.Errorf("temporal dial %s: %w", c.cfg.HostPort, err)
	}
	c.Client = cl
	return nil
}

func (c *Client) Run(context.Context) error { return nil }

func (c *Client) Stop(context.Context) error {
	if c.Client != nil {
		c.Close()
	}
	return nil
}

func (c *Client) Ping(ctx context.Context) error {
	if _, err := c.CheckHealth(ctx, &client.CheckHealthRequest{}); err != nil {
		return fmt.Errorf("temporal health: %w", err)
	}
	return nil
}
