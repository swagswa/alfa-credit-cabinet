// Package egrul: клиент ЕГРЮЛ/ЕГРИП. На стенде ходит в mocks, контракт
// ответа повторяет смысловые поля выписки, в кабинет они уходят через
// GET /companies/{inn} (api/openapi.yaml).
package egrul

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

type Client struct {
	base    string
	http    *http.Client
	timeout time.Duration
}

func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		base:    baseURL,
		http:    &http.Client{Timeout: timeout, Transport: otelhttp.NewTransport(http.DefaultTransport)},
		timeout: timeout,
	}
}

type response struct {
	INN          string `json:"inn"`
	OGRN         string `json:"ogrn"`
	KPP          string `json:"kpp"`
	Kind         string `json:"kind"`
	FullName     string `json:"full_name"`
	ShortName    string `json:"short_name"`
	Status       string `json:"status"`
	RegisteredAt string `json:"registered_at"`
	Address      struct {
		Text string `json:"text"`
	} `json:"address"`
	OkvedMain struct {
		Code string `json:"code"`
		Name string `json:"name"`
	} `json:"okved_main"`
	Director *struct {
		FIO      string `json:"fio"`
		Position string `json:"position"`
	} `json:"director"`
	Founders []struct {
		Name     string  `json:"name"`
		SharePct float64 `json:"share_pct"`
	} `json:"founders"`
	// у фикстур версии нет, её ставит только генератор
	GeneratorVersion string `json:"generator_version"`
}

// Company отдаёт выписку. 404 источника это domain.ErrNotFound, таймаут и 5xx это domain.ErrUnavailable.
func (c *Client) Company(ctx context.Context, inn string) (domain.Company, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/egrul/"+url.PathEscape(inn), http.NoBody)
	if err != nil {
		return domain.Company{}, fmt.Errorf("egrul: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return domain.Company{}, fmt.Errorf("egrul %s: %w: %w", inn, domain.ErrUnavailable, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return domain.Company{}, fmt.Errorf("egrul %s: %w", inn, domain.ErrNotFound)
	case resp.StatusCode >= http.StatusInternalServerError:
		return domain.Company{}, fmt.Errorf("egrul %s: %d: %w", inn, resp.StatusCode, domain.ErrUnavailable)
	case resp.StatusCode != http.StatusOK:
		return domain.Company{}, fmt.Errorf("egrul %s: unexpected status %d", inn, resp.StatusCode)
	}

	var r response
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return domain.Company{}, fmt.Errorf("egrul %s: decode: %w", inn, err)
	}
	return r.company()
}

func (r *response) company() (domain.Company, error) {
	reg, err := time.Parse(time.DateOnly, r.RegisteredAt)
	if err != nil {
		return domain.Company{}, fmt.Errorf("egrul %s: registered_at: %w", r.INN, err)
	}
	if r.Kind != string(domain.KindUL) && r.Kind != string(domain.KindIP) {
		return domain.Company{}, fmt.Errorf("egrul %s: unknown kind %q", r.INN, r.Kind)
	}
	c := domain.Company{
		INN:          r.INN,
		OGRN:         r.OGRN,
		KPP:          r.KPP,
		Kind:         domain.CompanyKind(r.Kind),
		FullName:     r.FullName,
		ShortName:    r.ShortName,
		Status:       r.Status,
		RegisteredAt: reg,
		Address:      r.Address.Text,
		Okved:        domain.Okved{Code: r.OkvedMain.Code, Name: r.OkvedMain.Name},
		Simulated:    r.GeneratorVersion != "",
	}
	if r.Director != nil {
		c.Director = &domain.Director{FIO: r.Director.FIO, Position: r.Director.Position}
	}
	for _, f := range r.Founders {
		c.Founders = append(c.Founders, domain.Founder{Name: f.Name, SharePct: f.SharePct})
	}
	return c, nil
}
