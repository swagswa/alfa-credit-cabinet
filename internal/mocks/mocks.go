// Package mocks симулирует внешние системы: ЕГРЮЛ/ЕГРИП, ФНС, БКИ,
// внутренние данные банка. Демо-ИНН отвечают фикстурами, остальные синтетикой
// из synth, режим сбоя включается отдельным эндпоинтом.
package mocks

import (
	"cmp"
	"mime"
	"net/http"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"

	"github.com/swagswa/alfa-credit-cabinet/internal/docgen"
)

func New() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(echomw.Recover())
	e.GET("/health", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	// «Взять пример выписки»: nginx отдаёт этот путь сюда, мимо api
	e.GET("/api/samples/:inn", sample)

	s, err := newSim()
	if err != nil {
		// фикстуры встроены в бинарник: сломаться они могут только при сборке, тест это ловит
		panic(err)
	}
	s.routes(e)
	return e
}

func sample(c echo.Context) error {
	d := docgen.Doc{INN: c.Param("inn"), Kind: c.QueryParam("kind"), Format: cmp.Or(c.QueryParam("format"), "a")}
	file, _, err := docgen.Generate(d)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	ct := "application/pdf"
	if d.Ext() == ".png" {
		ct = "image/png"
	}
	name := mime.FormatMediaType("attachment", map[string]string{"filename": d.Name() + d.Ext()})
	c.Response().Header().Set(echo.HeaderContentDisposition, name)
	return c.Blob(http.StatusOK, ct, file)
}
