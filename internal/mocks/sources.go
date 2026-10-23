package mocks

import (
	"embed"
	"fmt"
	"maps"
	"net/http"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"go.yaml.in/yaml/v3"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/mocks/synth"
)

// Фикстуры демо-компаний: по файлу на ИНН, в нём ответ каждого источника как есть
// и сбой по умолчанию. Любой другой ИНН отвечает компанией из synth.Sources в том же
// виде. Выручку и обороты досчитываем из synth, чтобы документы сходились с ФНС и банком.
//
//go:embed fixtures/*.yaml
var fixturesFS embed.FS

const (
	modeOK      = "ok"
	modeTimeout = "timeout"
	modeHTTP500 = "http_500"
	modeHTTP503 = "http_503"
	modeFlaky   = "flaky"
	modeMissing = "not_found"
)

var modes = []string{modeOK, modeTimeout, modeHTTP500, modeHTTP503, modeFlaky, modeMissing}

type fault struct {
	mode string
	left int // для flaky: сколько ещё вызовов падает
}

type faultKey struct {
	src domain.Source
	inn string // пусто: на весь источник
}

type sim struct {
	fixtures map[string]map[string]any

	mu     sync.Mutex
	faults map[faultKey]*fault
}

func newSim() (*sim, error) {
	files, err := fixturesFS.ReadDir("fixtures")
	if err != nil {
		return nil, fmt.Errorf("fixtures: %w", err)
	}
	s := &sim{fixtures: map[string]map[string]any{}, faults: map[faultKey]*fault{}}
	for _, f := range files {
		raw, err := fixturesFS.ReadFile(path.Join("fixtures", f.Name()))
		if err != nil {
			return nil, fmt.Errorf("fixture %s: %w", f.Name(), err)
		}
		var doc map[string]any
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("fixture %s: %w", f.Name(), err)
		}
		s.fixtures[strings.TrimSuffix(f.Name(), ".yaml")] = doc
	}
	return s, nil
}

func (s *sim) routes(e *echo.Echo) {
	for _, src := range []domain.Source{domain.SourceEGRUL, domain.SourceFNS, domain.SourceInternal} {
		e.GET("/"+string(src)+"/:inn", func(c echo.Context) error { return s.respond(c, src, c.Param("inn")) })
	}
	e.POST("/bki/report", s.bki)
	e.PUT("/mocks/admin/faults", s.setFault)
	e.GET("/mocks/admin/faults", s.listFaults)
}

func (s *sim) bki(c echo.Context) error {
	var req struct {
		INN       string    `json:"inn"`
		ConsentID uuid.UUID `json:"consent_id"`
		GivenAt   time.Time `json:"consent_given_at"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	// бюро без действующего согласия субъекта отчёт не отдаёт (218-ФЗ, ст. 6)
	if !(domain.BKIConsent{ID: req.ConsentID, GivenAt: req.GivenAt}).Valid(time.Now()) {
		return echo.NewHTTPError(http.StatusForbidden,
			fmt.Sprintf("нет действующего согласия на запрос в бюро, оно живёт %d месяцев", domain.BKIConsentMonths))
	}
	return s.respond(c, domain.SourceBKI, req.INN)
}

func (s *sim) respond(c echo.Context, src domain.Source, inn string) error {
	switch s.trip(src, inn) {
	case modeTimeout:
		// держим ответ, пока клиент не бросит запрос по своему таймауту
		<-c.Request().Context().Done()
		return echo.NewHTTPError(http.StatusGatewayTimeout)
	case modeHTTP500, modeFlaky:
		return echo.NewHTTPError(http.StatusInternalServerError)
	case modeHTTP503:
		return echo.NewHTTPError(http.StatusServiceUnavailable)
	case modeMissing:
		return echo.NewHTTPError(http.StatusNotFound)
	}

	doc, ok := s.fixtures[inn]
	if !ok {
		// не демо: компания из генератора. С неверными контрольными цифрами её нет,
		// как и в настоящем реестре, отвечаем 404
		var err error
		if doc, err = synth.Sources(inn); err != nil {
			return echo.NewHTTPError(http.StatusNotFound)
		}
	}
	body, ok := doc[string(src)].(map[string]any)
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound)
	}
	body, err := withMoney(src, inn, body)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, body)
}

// trip отдаёт режим сбоя для запроса. Режим на компанию сильнее режима на источник,
// тот сильнее сбоя из фикстуры. Счётчик flaky тратится здесь же.
func (s *sim) trip(src domain.Source, inn string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.faults[faultKey{src, inn}]
	if f == nil {
		f = s.faults[faultKey{src, ""}]
	}
	if f == nil {
		defaults, _ := s.fixtures[inn]["faults"].(map[string]any)
		mode, _ := defaults[string(src)].(string)
		return mode
	}
	if f.mode == modeFlaky {
		if f.left == 0 {
			return modeOK
		}
		f.left--
	}
	return f.mode
}

func (s *sim) setFault(c echo.Context) error {
	var req struct {
		Source domain.Source `json:"source"`
		Mode   string        `json:"mode"`
		INN    string        `json:"inn"`
		N      int           `json:"n"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	switch {
	case !slices.Contains(domain.Sources, req.Source):
		return echo.NewHTTPError(http.StatusBadRequest, "источники: egrul, fns, bki, internal")
	case !slices.Contains(modes, req.Mode):
		return echo.NewHTTPError(http.StatusBadRequest, "режимы: "+strings.Join(modes, ", "))
	case req.INN != "" && !domain.ValidINN(req.INN):
		return echo.NewHTTPError(http.StatusBadRequest, "неверный ИНН")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	key := faultKey{req.Source, req.INN}
	// ok на весь источник просто снимает режим, а на компанию перебивает и сбой из фикстуры
	if req.Mode == modeOK && req.INN == "" {
		delete(s.faults, key)
	} else {
		s.faults[key] = &fault{mode: req.Mode, left: req.N}
	}
	return c.NoContent(http.StatusNoContent)
}

func (s *sim) listFaults(c echo.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []map[string]any{}
	for k, f := range s.faults {
		out = append(out, map[string]any{"source": k.src, "inn": k.inn, "mode": f.mode, "left": f.left})
	}
	return c.JSON(http.StatusOK, out)
}

// withMoney дописывает в копию ответа выручку и обороты из synth, в рублях, как у источников.
func withMoney(src domain.Source, inn string, body map[string]any) (map[string]any, error) {
	if src != domain.SourceFNS && src != domain.SourceInternal {
		return body, nil
	}
	c, err := synth.New(inn)
	if err != nil {
		return nil, fmt.Errorf("synth %s: %w", inn, err)
	}
	body = maps.Clone(body)
	if src == domain.SourceFNS {
		_, out := c.Turnover()
		body["income_last_year"] = c.Income() / domain.KopInRub
		body["expenses_last_year"] = out / domain.KopInRub
		return body, nil
	}

	turnover := make([]map[string]any, 0, len(c.Months))
	balance := c.Opening
	var last6 int64
	for i, m := range c.Months {
		turnover = append(turnover, map[string]any{
			"month": m.Start.Format("2006-01"), "credit": m.In / domain.KopInRub, "debit": m.Out / domain.KopInRub,
		})
		balance += m.In - m.Out
		if i >= len(c.Months)-6 {
			last6 += balance
		}
	}
	body["turnover_12m"] = turnover
	// средний остаток считаем по концам месяцев, точнее синтетике и не надо
	body["avg_balance_6m"] = last6 / 6 / domain.KopInRub
	return body, nil
}
