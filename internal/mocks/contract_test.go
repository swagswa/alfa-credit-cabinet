package mocks_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
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
	} {
		t.Run(inn, func(t *testing.T) {
			c, err := reg.Company(t.Context(), inn)
			require.NoError(t, err)
			require.Equal(t, name, c.ShortName)
			require.False(t, c.Simulated, "фикстура, а не генератор")

			for _, s := range domain.Sources {
				_, err := src.Fetch(t.Context(), s, inn, uuid.New())
				if s == domain.SourceInternal && inn == "0000300011" {
					require.ErrorIs(t, err, domain.ErrNotFound)
					continue
				}
				require.NoError(t, err, s)
			}
		})
	}

	_, err := reg.Company(t.Context(), "0000500010")
	require.ErrorIs(t, err, domain.ErrUnavailable)
}
