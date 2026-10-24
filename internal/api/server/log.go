package server

import (
	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// requestLogger: обычные запросы в debug, чтобы не шуметь, ошибки пишет errorHandler
func requestLogger(log *zap.Logger) echo.MiddlewareFunc {
	return echomw.RequestLoggerWithConfig(echomw.RequestLoggerConfig{
		LogMethod:    true,
		LogURIPath:   true,
		LogStatus:    true,
		LogLatency:   true,
		LogRequestID: true,
		LogValuesFunc: func(c echo.Context, v echomw.RequestLoggerValues) error {
			log.Debug("request",
				zap.String("method", v.Method),
				zap.String("path", v.URIPath),
				zap.Int("status", v.Status),
				zap.Duration("latency", v.Latency),
				zap.String("request_id", v.RequestID),
				zap.Stringer("trace_id", trace.SpanContextFromContext(c.Request().Context()).TraceID()))
			return nil
		},
	})
}
