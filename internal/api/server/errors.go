package server

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

// errorHandler единственное место, где ошибки превращаются в HTTP-ответ.
// Логируем только 5xx: 4xx это нормальная работа клиента.
func errorHandler(log *zap.Logger) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}

		p := problem(err)
		if p.Status >= http.StatusInternalServerError && p.Status != http.StatusNotImplemented {
			log.Error("request failed",
				zap.String("path", c.Request().URL.Path),
				zap.String("request_id", c.Response().Header().Get(echo.HeaderXRequestID)),
				zap.Error(err))
		}

		c.Response().Header().Set(echo.HeaderContentType, "application/problem+json")
		if err := c.JSON(p.Status, p); err != nil {
			log.Error("write problem", zap.Error(err))
		}
	}
}

func problem(err error) oapi.Problem {
	var he *echo.HTTPError
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return newProblem(http.StatusNotFound, oapi.ProblemCodeNotFound, "")
	case errors.Is(err, domain.ErrForbidden):
		return newProblem(http.StatusForbidden, oapi.ProblemCodeForbidden, "")
	case errors.Is(err, domain.ErrIllegalTransition):
		return newProblem(http.StatusConflict, oapi.ProblemCodeIllegalTransition, err.Error())
	case errors.Is(err, domain.ErrConflict):
		return newProblem(http.StatusConflict, oapi.ProblemCodeVersionConflict, err.Error())
	case errors.Is(err, domain.ErrInvalid):
		return newProblem(http.StatusBadRequest, oapi.ProblemCodeValidation, err.Error())
	case errors.Is(err, domain.ErrUnauthorized):
		return newProblem(http.StatusUnauthorized, oapi.ProblemCodeUnauthorized, "")
	case errors.Is(err, domain.ErrTooManyRequests):
		return newProblem(http.StatusTooManyRequests, oapi.ProblemCodeTooManyRequests, "")
	case errors.Is(err, domain.ErrUnavailable):
		// причину сбоя источника клиенту не показываем, она в логе
		return newProblem(http.StatusServiceUnavailable, oapi.ProblemCodeUnavailable, "")
	case errors.As(err, &he):
		p := newProblem(he.Code, codeOf(he.Code), "")
		// у валидатора в Message причина, у остальных обычно просто текст статуса
		if msg, ok := he.Message.(string); ok && msg != http.StatusText(he.Code) {
			p.Detail = msg
		}
		return p
	default:
		// текст внутренней ошибки наружу не отдаём
		return newProblem(http.StatusInternalServerError, "", "")
	}
}

func codeOf(status int) oapi.ProblemCode {
	switch status {
	case http.StatusBadRequest:
		return oapi.ProblemCodeValidation
	case http.StatusUnauthorized:
		return oapi.ProblemCodeUnauthorized
	case http.StatusForbidden:
		return oapi.ProblemCodeForbidden
	case http.StatusNotFound:
		return oapi.ProblemCodeNotFound
	case http.StatusRequestEntityTooLarge:
		return oapi.ProblemCodeTooLarge
	case http.StatusTooManyRequests:
		return oapi.ProblemCodeTooManyRequests
	case http.StatusServiceUnavailable:
		return oapi.ProblemCodeUnavailable
	}
	return ""
}

func newProblem(status int, code oapi.ProblemCode, detail string) oapi.Problem {
	p := oapi.Problem{Status: status, Title: http.StatusText(status), Detail: detail}
	if code != "" {
		p.Code = &code
	}
	return p
}
