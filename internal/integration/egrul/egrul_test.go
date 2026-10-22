package egrul_test

import (
	"cmp"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/integration/egrul"
)

const coffeeShop = `{
  "inn": "0000100012", "ogrn": "1230000012345", "kpp": "000101001", "kind": "UL",
  "full_name": "Общество с ограниченной ответственностью «Зерно и пар»", "short_name": "ООО «Зерно и пар»",
  "status": "active", "registered_at": "2023-06-14",
  "address": {"region_code": "77", "text": "г. Москва, ул. Садовая, д. 12"},
  "okved_main": {"code": "56.30", "name": "Подача напитков"}, "okved_extra": [],
  "director": {"fio": "Смирнова Анна Сергеевна", "inn": "000000000000", "position": "Генеральный директор", "since": "2023-06-14"},
  "founders": [{"name": "Смирнова Анна Сергеевна", "inn": "000000000000", "share_pct": 100, "is_foreign": false}],
  "capital_rub": 10000, "unreliable": []
}`

const generatedIP = `{
  "inn": "770123456703", "ogrn": "323770000123456", "kind": "IP",
  "full_name": "Индивидуальный предприниматель Петров Пётр Петрович", "short_name": "Петров Пётр Петрович",
  "status": "active", "registered_at": "2021-02-01",
  "address": {"region_code": "77", "text": "г. Москва"},
  "okved_main": {"code": "47.11", "name": "Торговля розничная"},
  "generator_version": "1"
}`

func TestCompany(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		delay   time.Duration
		want    domain.Company
		wantErr error
	}{
		{
			name: "юрлицо из фикстуры", status: 200, body: coffeeShop,
			want: domain.Company{
				INN: "0000100012", OGRN: "1230000012345", KPP: "000101001", Kind: domain.KindUL,
				FullName:     "Общество с ограниченной ответственностью «Зерно и пар»",
				ShortName:    "ООО «Зерно и пар»",
				Status:       "active",
				RegisteredAt: time.Date(2023, 6, 14, 0, 0, 0, 0, time.UTC),
				Address:      "г. Москва, ул. Садовая, д. 12",
				Okved:        domain.Okved{Code: "56.30", Name: "Подача напитков"},
				Director:     &domain.Director{FIO: "Смирнова Анна Сергеевна", Position: "Генеральный директор"},
				Founders:     []domain.Founder{{Name: "Смирнова Анна Сергеевна", SharePct: 100}},
			},
		},
		{
			name: "ип из генератора", status: 200, body: generatedIP,
			want: domain.Company{
				INN: "770123456703", OGRN: "323770000123456", Kind: domain.KindIP,
				FullName:     "Индивидуальный предприниматель Петров Пётр Петрович",
				ShortName:    "Петров Пётр Петрович",
				Status:       "active",
				RegisteredAt: time.Date(2021, 2, 1, 0, 0, 0, 0, time.UTC),
				Address:      "г. Москва",
				Okved:        domain.Okved{Code: "47.11", Name: "Торговля розничная"},
				Simulated:    true,
			},
		},
		{name: "нет в реестре", status: 404, wantErr: domain.ErrNotFound},
		{name: "источник лежит", status: 503, wantErr: domain.ErrUnavailable},
		{name: "таймаут", status: 200, body: coffeeShop, delay: 300 * time.Millisecond, wantErr: domain.ErrUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inn := cmp.Or(tt.want.INN, "0000100012")
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/egrul/"+inn, r.URL.Path)
				time.Sleep(tt.delay)
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			got, err := egrul.New(srv.URL, 100*time.Millisecond).Company(t.Context(), inn)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
