package mocks

import (
	"crypto/rand"
	_ "embed"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"go.yaml.in/yaml/v3"
)

// Альфа ID в форме authorization code: страница согласия выдаёт одноразовый код,
// кабинет меняет его на профиль сервер-сервер. Подписи токенов и client_secret нет,
// это симулятор.

//go:embed alfaid.yaml
var alfaIDClients []byte

const codeTTL = time.Minute

type alfaProfile struct {
	Sub      string `json:"sub"`
	Login    string `json:"login"     yaml:"login"`
	FullName string `json:"full_name" yaml:"full_name"`
	INN      string `json:"inn"       yaml:"inn"`
	Phone    string `json:"phone"     yaml:"phone"`
	Email    string `json:"email"     yaml:"email"`
}

type issued struct {
	p   alfaProfile
	exp time.Time
}

type alfaID struct {
	clients []alfaProfile

	mu    sync.Mutex
	codes map[string]issued
}

func newAlfaID() (*alfaID, error) {
	a := &alfaID{codes: map[string]issued{}}
	if err := yaml.Unmarshal(alfaIDClients, &a.clients); err != nil {
		return nil, fmt.Errorf("alfaid.yaml: %w", err)
	}
	for i := range a.clients {
		a.clients[i].Sub = "aid-" + a.clients[i].INN
	}
	return a, nil
}

func (a *alfaID) routes(e *echo.Echo) {
	e.GET("/alfa-id/authorize", a.page)
	e.POST("/alfa-id/authorize", a.authorize)
	e.POST("/alfa-id/token", a.token)
}

// localURI: возвращаем только на свой же сайт, иначе код уедет куда угодно
func localURI(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.Contains(raw, `\`) ||
		u.Host != "" {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "redirect_uri только относительный")
	}
	return u, nil
}

func (a *alfaID) page(c echo.Context) error {
	if _, err := localURI(c.QueryParam("redirect_uri")); err != nil {
		return err
	}
	var b strings.Builder
	err := consentPage.Execute(&b, map[string]any{
		"Clients":  a.clients,
		"Redirect": c.QueryParam("redirect_uri"),
		"State":    c.QueryParam("state"),
	})
	if err != nil {
		return fmt.Errorf("consent page: %w", err)
	}
	return c.HTML(http.StatusOK, b.String())
}

func (a *alfaID) authorize(c echo.Context) error {
	back, err := localURI(c.FormValue("redirect_uri"))
	if err != nil {
		return err
	}
	q := url.Values{"state": {c.FormValue("state")}}
	if c.FormValue("deny") != "" {
		q.Set("error", "access_denied")
	} else {
		p, ok := a.client(c.FormValue("login"))
		if !ok {
			return echo.NewHTTPError(http.StatusBadRequest, "нет такого клиента")
		}
		code := rand.Text()
		a.mu.Lock()
		a.codes[code] = issued{p: p, exp: time.Now().Add(codeTTL)}
		a.mu.Unlock()
		q.Set("code", code)
	}
	back.RawQuery = q.Encode()
	return c.Redirect(http.StatusFound, back.String())
}

func (a *alfaID) token(c echo.Context) error {
	code := c.FormValue("code")
	a.mu.Lock()
	it, ok := a.codes[code]
	delete(a.codes, code)
	a.mu.Unlock()
	if !ok || time.Now().After(it.exp) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
	}
	return c.JSON(http.StatusOK, it.p)
}

func (a *alfaID) client(login string) (alfaProfile, bool) {
	for _, p := range a.clients {
		if p.Login == login {
			return p, true
		}
	}
	return alfaProfile{}, false
}

var consentPage = template.Must(template.New("consent").Parse(`<!doctype html>
<html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Альфа ID</title>
<style>
body{margin:0;font:16px/1.4 system-ui,sans-serif;background:#f3f4f5;color:#0b1f35}
main{max-width:420px;margin:40px auto;padding:24px;background:#fff;border-radius:16px}
h1{margin:0 0 4px;font-size:24px}.note{margin:0 0 20px;color:#6b7785;font-size:13px}
label{display:flex;gap:12px;align-items:center;padding:12px;border:1px solid #dbdee1;border-radius:12px;margin-bottom:8px;cursor:pointer}
label small{display:block;color:#6b7785}
.scope{margin:20px 0;padding:12px;background:#f3f4f5;border-radius:12px;font-size:14px}
button{width:100%;padding:14px;border:0;border-radius:12px;font-size:16px;cursor:pointer;margin-top:8px}
.allow{background:#ef3124;color:#fff}.deny{background:#f3f4f5;color:#0b1f35}
@media(max-width:480px){main{margin:0;border-radius:0}}
</style></head><body><main>
<h1>Альфа ID</h1>
<p class="note">Симулятор входа для прототипа Alfa-Hack, настоящий Альфа ID не используется</p>
<form method="post" action="/alfa-id/authorize">
<input type="hidden" name="redirect_uri" value="{{.Redirect}}">
<input type="hidden" name="state" value="{{.State}}">
{{range $i, $c := .Clients}}<label><input type="radio" name="login" value="{{$c.Login}}"{{if eq $i 0}} checked{{end}}>
<span>{{$c.FullName}}<small>ИНН {{$c.INN}}</small></span></label>
{{end}}
<p class="scope">Кредитный кабинет запрашивает: ФИО, телефон, почта, ИНН компании</p>
<button class="allow" type="submit">Разрешить</button>
<button class="deny" type="submit" name="deny" value="1" formnovalidate>Отказаться</button>
</form></main></body></html>`))
