package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/trace"
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
				// по нему из лога в Grafana переходим в трейс Jaeger
				zap.Stringer("trace_id", trace.SpanContextFromContext(c.Request().Context()).TraceID()),
				zap.Error(err))
		}

		c.Response().Header().Set(echo.HeaderContentType, "application/problem+json")
		if err := c.JSON(p.Status, p); err != nil {
			log.Error("write problem", zap.Error(err))
		}
	}
}

func problem(err error) oapi.Problem {
	var (
		he     *echo.HTTPError
		fields domain.FieldErrors
	)
	switch {
	case errors.As(err, &fields):
		p := newProblem(http.StatusUnprocessableEntity, oapi.ProblemCodeIncomplete, "")
		errs := make([]oapi.FieldError, len(fields))
		for i, f := range fields {
			errs[i] = oapi.FieldError(f)
		}
		p.Errors = &errs
		return p
	case errors.Is(err, domain.ErrNotFound):
		return newProblem(http.StatusNotFound, oapi.ProblemCodeNotFound, "")
	case errors.Is(err, domain.ErrForbidden):
		return newProblem(http.StatusForbidden, oapi.ProblemCodeForbidden, "")
	case errors.Is(err, domain.ErrIllegalTransition):
		return newProblem(http.StatusConflict, oapi.ProblemCodeIllegalTransition, err.Error())
	case errors.Is(err, domain.ErrKeyReused):
		return newProblem(http.StatusConflict, oapi.ProblemCodeIdempotencyConflict, err.Error())
	case errors.Is(err, domain.ErrConflict):
		return newProblem(http.StatusConflict, oapi.ProblemCodeVersionConflict, err.Error())
	case errors.Is(err, domain.ErrTooLarge):
		return newProblem(http.StatusRequestEntityTooLarge, oapi.ProblemCodeTooLarge, err.Error())
	case errors.Is(err, domain.ErrInvalid):
		return newProblem(http.StatusBadRequest, oapi.ProblemCodeValidation, domain.Human(err))
	case errors.Is(err, domain.ErrUnauthorized):
		return newProblem(http.StatusUnauthorized, oapi.ProblemCodeUnauthorized, "")
	case errors.Is(err, domain.ErrTooManyRequests):
		return newProblem(http.StatusTooManyRequests, oapi.ProblemCodeTooManyRequests, "")
	case errors.Is(err, domain.ErrUnavailable):
		// причину сбоя источника клиенту не показываем, она в логе
		return newProblem(http.StatusServiceUnavailable, oapi.ProblemCodeUnavailable, "")
	case errors.As(err, &he):
		p := newProblem(he.Code, codeOf(he.Code), "")
		if f, ok := schemaError(he.Internal); ok {
			p.Detail = f.Message
			p.Errors = &[]oapi.FieldError{f}
			return p
		}
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
	case http.StatusUnprocessableEntity:
		return oapi.ProblemCodeIncomplete
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

// подписи полей, которые клиент вводит руками. Остальные ошибки схемы отдаём как есть:
// это уже ошибка вызывающего кода, а не человека
var fieldNames = map[string]string{
	"amount_kop":          "сумма",
	"term_months":         "срок",
	"monthly_revenue_kop": "выручка",
	"equipment_cost_kop":  "стоимость оборудования",
	"rate_bp":             "ставка",
	"comment":             "комментарий",
	"inn":                 "ИНН",
	"code":                "код подписи",
}

// schemaError переводит отказ валидатора по схеме в текст у поля, предел берёт из самой спеки
func schemaError(err error) (oapi.FieldError, bool) {
	var (
		re *openapi3filter.RequestError
		se *openapi3.SchemaError
		pe *openapi3filter.ParseError
	)
	if !errors.As(err, &re) {
		return oapi.FieldError{}, false
	}
	field, schema := "", errors.As(re.Err, &se)
	switch {
	case re.Parameter != nil:
		field = re.Parameter.Name
	case schema:
		if path := se.JSONPointer(); len(path) > 0 {
			field = path[len(path)-1]
		}
	case errors.As(re.Err, &pe):
		// multipart: у файла схема проверяет только тип
		if path := pe.Path(); len(path) > 0 && path[len(path)-1] == "file" {
			return oapi.FieldError{Field: "file", Message: "принимаем PDF, PNG или JPEG"}, true
		}
	}
	name, ok := fieldNames[field]
	if !ok {
		return oapi.FieldError{}, false
	}
	var msg string
	switch {
	case !schema:
		return oapi.FieldError{Field: field, Message: "проверьте поле «" + name + "»"}, true
	case se.SchemaField == "maximum" && se.Schema.Max != nil:
		msg = "не больше " + limit(field, *se.Schema.Max)
	case se.SchemaField == "minimum" && se.Schema.Min != nil:
		msg = "не меньше " + limit(field, *se.Schema.Min)
	case se.SchemaField == "multipleOf" && se.Schema.MultipleOf != nil:
		msg = "должна быть кратна " + limit(field, *se.Schema.MultipleOf)
	case se.SchemaField == "maxLength" && se.Schema.MaxLength != nil:
		msg = fmt.Sprintf("не длиннее %d символов", *se.Schema.MaxLength)
	default:
		return oapi.FieldError{Field: field, Message: "проверьте поле «" + name + "»"}, true
	}
	return oapi.FieldError{Field: field, Message: name + " " + msg}, true
}

// limit пишет предел в единицах человека: копейки рублями, базисные пункты процентами
func limit(field string, v float64) string {
	switch {
	case strings.HasSuffix(field, "_kop"):
		return thousands(int64(v)/domain.KopInRub) + " ₽"
	case strings.HasSuffix(field, "_bp"):
		return strconv.FormatFloat(v/100, 'f', -1, 64) + "%"
	case strings.HasSuffix(field, "_months"):
		return strconv.FormatFloat(v, 'f', -1, 64) + " мес."
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// thousands: 15000000 -> «15 000 000», как в кабинете
func thousands(n int64) string {
	const group = 3
	s := strconv.FormatInt(n, 10)
	for i := len(s) - group; i > 0; i -= group {
		s = s[:i] + " " + s[i:]
	}
	return s
}
