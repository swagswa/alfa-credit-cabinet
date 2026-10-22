// Package httpserver держит http.Server как svc.Service.
package httpserver

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

const probeTimeout = 3 * time.Second

type Config struct {
	Addr              string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	// WriteTimeout оставляем нулевым у api: иначе сервер рвёт SSE-поток
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

type Server struct {
	name string
	srv  *http.Server
	ln   net.Listener
}

func New(name string, cfg Config, h http.Handler) *Server {
	return &Server{
		name: name,
		srv: &http.Server{
			Addr:              cfg.Addr,
			Handler:           h,
			ReadHeaderTimeout: cfg.ReadHeaderTimeout,
			ReadTimeout:       cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
		},
	}
}

func (s *Server) Name() string { return s.name }

// Init занимает порт сразу, чтобы занятый порт ронял старт, а не Run.
func (s *Server) Init(ctx context.Context) error {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.srv.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.srv.Addr, err)
	}
	s.ln = ln
	return nil
}

func (s *Server) Run(context.Context) error {
	if err := s.srv.Serve(s.ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	if err := s.srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}

// Probe нужен для healthcheck в compose: в distroless нет curl, поэтому
// бинарник сам стучится в свой health и выходит с кодом.
func Probe(ctx context.Context, url string) error {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("probe request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("probe: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("probe: status %d", resp.StatusCode)
	}
	return nil
}

// ProbeMode обрабатывает запуск бинарника с -healthcheck <url>: проверяет адрес
// и завершает процесс с кодом 0 или 1. Без флага ничего не делает.
func ProbeMode(ctx context.Context) {
	url := flag.String("healthcheck", "", "проверить health по адресу и выйти, для healthcheck в compose")
	flag.Parse()
	if *url == "" {
		return
	}
	if err := Probe(ctx, *url); err != nil {
		log.Print(err)
		os.Exit(1)
	}
	os.Exit(0)
}
