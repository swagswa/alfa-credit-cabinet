// Package alfaid: обмен кода Альфа ID на профиль клиента, сервер-сервер.
// На стенде отвечает симулятор в mocks, на проде нужен client_secret по договору с банком.
package alfaid

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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
		url:  baseURL + "/alfa-id/token",
		http: &http.Client{Timeout: timeout, Transport: otelhttp.NewTransport(http.DefaultTransport)},
	}
}

// Profile: 400 это протухший или уже потраченный код, для входа ErrUnauthorized
func (c *Client) Profile(ctx context.Context, code string) (domain.AlfaProfile, error) {
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, strings.NewReader(form.Encode()))
	if err != nil {
		return domain.AlfaProfile{}, fmt.Errorf("alfa id: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return domain.AlfaProfile{}, fmt.Errorf("alfa id: %w: %w", domain.ErrUnavailable, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusBadRequest:
		return domain.AlfaProfile{}, fmt.Errorf("alfa id code: %w", domain.ErrUnauthorized)
	case resp.StatusCode != http.StatusOK:
		return domain.AlfaProfile{}, fmt.Errorf("alfa id: %d: %w", resp.StatusCode, domain.ErrUnavailable)
	}
	var p struct {
		Login string `json:"login"`
		Phone string `json:"phone"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return domain.AlfaProfile{}, fmt.Errorf("alfa id: %w: %w", domain.ErrUnavailable, err)
	}
	return domain.AlfaProfile{Login: p.Login, Phone: p.Phone, Email: p.Email}, nil
}
