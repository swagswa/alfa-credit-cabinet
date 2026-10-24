// Package telemetry держит трейсы и метрики процесса как svc.Service. Трейсы уходят
// по OTLP, только если задан OTEL_EXPORTER_OTLP_ENDPOINT (его читает сам SDK), без него
// стенд работает как раньше. Метрики Prometheus забирает с /metrics на отдельном порту.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/swagswa/alfa-credit-cabinet/pkg/httpserver"
)

const readTimeout = 5 * time.Second

type Telemetry struct {
	name    string
	metrics *httpserver.Server
	mp      *sdkmetric.MeterProvider
	tp      *sdktrace.TracerProvider
}

// New готовит телеметрию процесса name. В списке сервисов она первая: клиенты
// базы и Temporal берут провайдеры у otel при старте.
func New(name, metricsAddr string) *Telemetry {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	cfg := httpserver.Config{Addr: metricsAddr, ReadHeaderTimeout: readTimeout}
	return &Telemetry{name: name, metrics: httpserver.New("metrics", cfg, mux)}
}

func (t *Telemetry) Name() string { return "telemetry" }

func (t *Telemetry) Init(ctx context.Context) error {
	res := resource.NewSchemaless(attribute.String("service.name", t.name))
	// экспортёр регистрируется в общем реестре client_golang, там же метрики рантайма Go
	exp, err := prometheus.New()
	if err != nil {
		return fmt.Errorf("prometheus exporter: %w", err)
	}
	t.mp = sdkmetric.NewMeterProvider(sdkmetric.WithReader(exp), sdkmetric.WithResource(res))
	otel.SetMeterProvider(t.mp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" {
		// соединение ленивое: коллектор может подняться позже, старт это не держит
		te, err := otlptracegrpc.New(ctx)
		if err != nil {
			return fmt.Errorf("otlp exporter: %w", err)
		}
		t.tp = sdktrace.NewTracerProvider(sdktrace.WithBatcher(te), sdktrace.WithResource(res))
		otel.SetTracerProvider(t.tp)
	}
	return t.metrics.Init(ctx)
}

func (t *Telemetry) Run(ctx context.Context) error { return t.metrics.Run(ctx) }

// Stop дописывает накопленные спаны, иначе последние запросы до Jaeger не дойдут.
func (t *Telemetry) Stop(ctx context.Context) error {
	err := errors.Join(t.metrics.Stop(ctx), t.mp.Shutdown(ctx))
	if t.tp != nil {
		err = errors.Join(err, t.tp.Shutdown(ctx))
	}
	return err
}
