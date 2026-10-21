// Package mocks симулирует внешние системы: ЕГРЮЛ/ЕГРИП, ФНС, БКИ,
// внутренние данные банка. Ответы по ИНН из фикстур, режим сбоя включается
// отдельным эндпоинтом.
package mocks

import (
	"net/http"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
)

func New() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(echomw.Recover())
	e.GET("/health", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	return e
}
