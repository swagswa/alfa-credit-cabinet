// Package svc управляет жизненным циклом частей процесса: пул БД, клиент
// Temporal, HTTP-сервер, воркер. main собирает их списком и отдаёт в Run.
package svc

import (
	"context"
	"fmt"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// stopTimeout: сколько даём сервису на корректную остановку, дальше выходим как есть
const stopTimeout = 15 * time.Second

// Service это часть процесса со своим жизненным циклом.
//
// Init готовит ресурсы и вызывается по порядку списка: поздние сервисы
// могут полагаться на то, что ранние уже готовы. Run блокирует до отмены
// ctx или ошибки. Stop освобождает ресурсы и должен уложиться в контекст.
type Service interface {
	Name() string
	Init(ctx context.Context) error
	Run(ctx context.Context) error
	Stop(ctx context.Context) error
}

// Run поднимает сервисы и держит их до SIGINT, SIGTERM или первой ошибки Run,
// после чего останавливает в обратном порядке.
func Run(ctx context.Context, log *zap.Logger, services ...Service) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	for i, s := range services {
		log.Info("init", zap.String("service", s.Name()))
		if err := s.Init(ctx); err != nil {
			// уже поднятые надо закрыть, иначе пул и клиенты повиснут до выхода
			stopAll(ctx, log, services[:i])
			return fmt.Errorf("init %s: %w", s.Name(), err)
		}
	}

	g, gctx := errgroup.WithContext(ctx)
	for _, s := range services {
		g.Go(func() error {
			if err := s.Run(gctx); err != nil {
				return fmt.Errorf("run %s: %w", s.Name(), err)
			}
			return nil
		})
	}

	done := make(chan error, 1)
	go func() { done <- g.Wait() }()

	// gctx отменяется и по сигналу (через ctx), и по первой ошибке Run
	<-gctx.Done()
	stopAll(gctx, log, services)

	return <-done
}

func stopAll(ctx context.Context, log *zap.Logger, services []Service) {
	// ctx к этому моменту обычно отменён, а Shutdown с отменённым контекстом
	// не дождётся ни одного запроса
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), stopTimeout)
	defer cancel()

	for _, s := range slices.Backward(services) {
		log.Info("stop", zap.String("service", s.Name()))
		if err := s.Stop(ctx); err != nil {
			log.Error("stop", zap.String("service", s.Name()), zap.Error(err))
		}
	}
}
