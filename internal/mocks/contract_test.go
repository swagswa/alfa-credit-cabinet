package mocks_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/integration/alfaid"
	"github.com/swagswa/alfa-credit-cabinet/internal/integration/egrul"
	"github.com/swagswa/alfa-credit-cabinet/internal/integration/source"
	"github.com/swagswa/alfa-credit-cabinet/internal/mocks"
)

// клиенты кабинета и воркера разбирают то, что отдаёт мок; названия как в демо-сиде
func TestClientsReadMocks(t *testing.T) {
	srv := httptest.NewServer(mocks.New())
	defer srv.Close()
	reg := egrul.New(srv.URL, time.Second)
	src := source.New(source.URLs{EGRUL: srv.URL, FNS: srv.URL, BKI: srv.URL, Bank: srv.URL}, time.Second)

	for inn, name := range map[string]string{
		"0000100012":   "ООО «Тёплое зерно»",
		"000020001010": "ИП Лебедев С. Н.",
		"0000300011":   "ООО «Кодовая база»",
		"0000400016":   "ООО «Стройснаб»",
		"0000700010":   "ООО «Мотор-Сервис»",
	} {
		t.Run(inn, func(t *testing.T) {
			c, err := reg.Company(t.Context(), inn)
			require.NoError(t, err)
			require.Equal(t, name, c.ShortName)
			require.False(t, c.Simulated, "фикстура, а не генератор")

			consent := domain.BKIConsent{ID: uuid.New(), GivenAt: time.Now().AddDate(0, -1, 0)}
			for _, s := range domain.Sources {
				_, err := src.Fetch(t.Context(), s, inn, consent)
				if s == domain.SourceInternal && inn == "0000300011" {
					require.ErrorIs(t, err, domain.ErrNotFound)
					continue
				}
				require.NoError(t, err, s)
			}
		})
	}

	c, err := reg.Company(t.Context(), "7701234560")
	require.NoError(t, err)
	require.True(t, c.Simulated, "не демо, компания из генератора")

	_, err = reg.Company(t.Context(), "0000500010")
	require.ErrorIs(t, err, domain.ErrUnavailable)
}

func TestAlfaIDClientReadsMock(t *testing.T) {
	srv := httptest.NewServer(mocks.New())
	defer srv.Close()
	// редирект с кодом кабинету не нужен, код берём из Location
	hc := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := hc.PostForm(srv.URL+"/alfa-id/authorize", url.Values{"redirect_uri": {"/cb"}, "login": {"coffee"}})
	require.NoError(t, err)
	resp.Body.Close()
	back, err := url.Parse(resp.Header.Get("Location"))
	require.NoError(t, err)

	c := alfaid.New(srv.URL, time.Second)
	p, err := c.Profile(t.Context(), back.Query().Get("code"))
	require.NoError(t, err)
	require.Equal(t, domain.AlfaProfile{Login: "coffee", Phone: "+79161000001", Email: "sokolova.av@alfa-id.demo"}, p)

	_, err = c.Profile(t.Context(), back.Query().Get("code"))
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}
