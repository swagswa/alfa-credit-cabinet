// Package faults: ручка сбоев мока источников (/mocks/admin/faults). Через неё
// admin с пульта роняет ЕГРЮЛ, ФНС, БКИ или данные банка на стенде.
package faults

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

type Client struct {
	url  string
	http *http.Client
}

func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		url:  baseURL + "/mocks/admin/faults",
		http: &http.Client{Timeout: timeout, Transport: otelhttp.NewTransport(http.DefaultTransport)},
	}
}

type fault struct {
	Source domain.Source `json:"source"`
	INN    string        `json:"inn,omitempty"`
	Mode   string        `json:"mode"`
	Left   int           `json:"left,omitempty"`
	N      int           `json:"n,omitempty"`
}

func (c *Client) List(ctx context.Context) ([]domain.Fault, error) {
	var fs []fault
	if err := c.do(ctx, http.MethodGet, nil, &fs); err != nil {
		return nil, err
	}
	out := make([]domain.Fault, len(fs))
	for i, f := range fs {
		out[i] = domain.Fault{Source: f.Source, INN: f.INN, Mode: f.Mode, Left: f.Left}
	}
	return out, nil
}

func (c *Client) Set(ctx context.Context, f domain.Fault) error {
	return c.do(ctx, http.MethodPut, fault{Source: f.Source, INN: f.INN, Mode: f.Mode, N: f.Left}, nil)
}

// do: мок не отвечает или отвечает не 2xx, для пульта это одно и то же, стенд сломан
func (c *Client) do(ctx context.Context, method string, in, out any) error {
	var body bytes.Buffer
	if in != nil {
		if err := json.NewEncoder(&body).Encode(in); err != nil {
			return fmt.Errorf("faults: %w", err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.url, &body)
	if err != nil {
		return fmt.Errorf("faults: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("faults: %w: %w", domain.ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("faults: %d: %w", resp.StatusCode, domain.ErrUnavailable)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("faults: %w: %w", domain.ErrUnavailable, err)
	}
	return nil
}
