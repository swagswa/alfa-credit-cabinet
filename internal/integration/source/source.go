// Package source: запросы в источники проверок заявки (ЕГРЮЛ, ФНС, БКИ,
// внутренние данные банка). Ответ не разбираем: он ложится в check_results
// как есть, факты из него собирает решение.
package source

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

// URLs базовые адреса источников, на стенде все смотрят в mocks.
type URLs struct {
	EGRUL, FNS, BKI, Bank string
}

type Client struct {
	urls    URLs
	http    *http.Client
	timeout time.Duration
}

func New(urls URLs, timeout time.Duration) *Client {
	return &Client{urls: urls, http: &http.Client{Timeout: timeout}, timeout: timeout}
}

// Fetch отдаёт ответ источника. 404 это ErrNotFound, сеть, таймаут и 5xx это
// ErrUnavailable (их стоит повторить), 403 это ErrForbidden, прочие 4xx ErrInvalid.
func (c *Client) Fetch(
	ctx context.Context,
	src domain.Source,
	inn string,
	consentID uuid.UUID,
) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := c.request(ctx, src, inn, consentID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", src, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w: %w", src, inn, domain.ErrUnavailable, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("%s %s: %w", src, inn, domain.ErrNotFound)
	case resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("%s %s: %w", src, inn, domain.ErrForbidden)
	case resp.StatusCode >= http.StatusInternalServerError:
		return nil, fmt.Errorf("%s %s: %d: %w", src, inn, resp.StatusCode, domain.ErrUnavailable)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%s %s: %d: %w", src, inn, resp.StatusCode, domain.ErrInvalid)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w: %w", src, inn, domain.ErrUnavailable, err)
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("%s %s: ответ не json: %w", src, inn, domain.ErrInvalid)
	}
	return body, nil
}

func (c *Client) request(ctx context.Context, src domain.Source, inn string, consent uuid.UUID) (*http.Request, error) {
	path := url.PathEscape(inn)
	method, body := http.MethodGet, io.Reader(http.NoBody)
	var target string
	switch src {
	case domain.SourceEGRUL:
		target = c.urls.EGRUL + "/egrul/" + path
	case domain.SourceFNS:
		target = c.urls.FNS + "/fns/" + path
	case domain.SourceInternal:
		target = c.urls.Bank + "/internal/" + path
	case domain.SourceBKI:
		// бюро отвечает только по согласию, оно дано в анкете заявки
		raw, err := json.Marshal(bkiRequest{INN: inn, ConsentID: consent})
		if err != nil {
			return nil, fmt.Errorf("bki body: %w", err)
		}
		method, target, body = http.MethodPost, c.urls.BKI+"/bki/report", bytes.NewReader(raw)
	default:
		return nil, fmt.Errorf("unknown source %q", src)
	}

	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

type bkiRequest struct {
	INN       string    `json:"inn"`
	ConsentID uuid.UUID `json:"consent_id"`
}
