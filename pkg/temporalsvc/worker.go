package temporalsvc

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/worker"
)

// Register вешает на воркер workflow и activities конкретного процесса.
type Register func(w worker.Registry)

type Worker struct {
	client   *Client
	queue    string
	opts     worker.Options
	register Register
	w        worker.Worker
	done     chan struct{}
}

// NewWorker собирает воркер очереди queue. Клиент к моменту Init воркера
// уже должен пройти свой Init, поэтому в списке сервисов он идёт раньше.
func NewWorker(c *Client, queue string, opts worker.Options, register Register) *Worker {
	return &Worker{client: c, queue: queue, opts: opts, register: register}
}

func (w *Worker) Name() string { return "temporal-worker:" + w.queue }

func (w *Worker) Init(context.Context) error {
	// SDK проверяет тип клиента, обёртку он не примет
	w.w = worker.New(w.client.Client, w.queue, w.opts)
	w.register(w.w)
	w.done = make(chan struct{})
	return nil
}

func (w *Worker) Run(ctx context.Context) error {
	defer close(w.done)
	stop := make(chan any)
	go func() {
		<-ctx.Done()
		close(stop)
	}()
	if err := w.w.Run(stop); err != nil {
		return fmt.Errorf("worker %s: %w", w.queue, err)
	}
	return nil
}

// Stop ждёт, пока Run доведёт остановку: иначе клиент закроется раньше
// и воркер не успеет сообщить серверу, что уходит.
func (w *Worker) Stop(ctx context.Context) error {
	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("worker %s stop: %w", w.queue, ctx.Err())
	}
}
