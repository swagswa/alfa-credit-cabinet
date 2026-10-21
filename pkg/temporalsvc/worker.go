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
}

// NewWorker: клиент к моменту Init воркера уже должен пройти свой Init,
// поэтому в списке сервисов он идёт раньше.
func NewWorker(c *Client, queue string, opts worker.Options, register Register) *Worker {
	return &Worker{client: c, queue: queue, opts: opts, register: register}
}

func (w *Worker) Name() string { return "temporal-worker:" + w.queue }

func (w *Worker) Init(context.Context) error {
	w.w = worker.New(w.client, w.queue, w.opts)
	w.register(w.w)
	return nil
}

func (w *Worker) Run(ctx context.Context) error {
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

// Stop не нужен: Run сам выходит по отмене контекста и дожидается activities.
func (w *Worker) Stop(context.Context) error { return nil }
