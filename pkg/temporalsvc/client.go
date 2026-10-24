// Package temporalsvc держит клиент и воркер Temporal как svc.Service.
package temporalsvc

import (
	"context"
	"errors"
	"fmt"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
	temporalotel "go.temporal.io/sdk/contrib/opentelemetry"
	"go.temporal.io/sdk/interceptor"
	"go.uber.org/zap"

	"github.com/swagswa/alfa-credit-cabinet/pkg/zapadapter"
)

// ErrAlreadyStarted значит, что workflow с таким id уже запускали, даже если он завершился.
var ErrAlreadyStarted = errors.New("workflow already started")

// ErrNoProcess значит, что workflow с таким id не идёт: не запускали или уже завершился.
var ErrNoProcess = errors.New("workflow is not running")

type Config struct {
	HostPort  string
	Namespace string
}

type Client struct {
	client.Client

	log *zap.Logger
	cfg Config
}

func NewClient(log *zap.Logger, cfg Config) *Client {
	return &Client{log: log, cfg: cfg}
}

func (c *Client) Name() string { return "temporal-client" }

func (c *Client) Init(ctx context.Context) error {
	// трейс идёт от запроса в api через update и workflow до activity, воркер берёт
	// перехватчик у клиента
	tracing, err := temporalotel.NewTracingInterceptor(temporalotel.TracerOptions{})
	if err != nil {
		return fmt.Errorf("temporal tracing: %w", err)
	}
	cl, err := client.DialContext(ctx, client.Options{
		HostPort:       c.cfg.HostPort,
		Namespace:      c.cfg.Namespace,
		Logger:         zapadapter.New(c.log),
		Interceptors:   []interceptor.ClientInterceptor{tracing},
		MetricsHandler: temporalotel.NewMetricsHandler(temporalotel.MetricsHandlerOptions{}),
	})
	if err != nil {
		return fmt.Errorf("temporal dial %s: %w", c.cfg.HostPort, err)
	}
	c.Client = cl
	return nil
}

func (c *Client) Run(context.Context) error { return nil }

func (c *Client) Stop(context.Context) error {
	if c.Client != nil {
		c.Close()
	}
	return nil
}

func (c *Client) Ping(ctx context.Context) error {
	if _, err := c.CheckHealth(ctx, &client.CheckHealthRequest{}); err != nil {
		return fmt.Errorf("temporal health: %w", err)
	}
	return nil
}

// Start запускает workflow по имени типа. Id одноразовый: второй старт с ним
// даёт ErrAlreadyStarted, так повторная подача не заведёт второй процесс.
func (c *Client) Start(ctx context.Context, queue, id, workflow string, args ...any) error {
	_, err := c.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:                    id,
		TaskQueue:             queue,
		WorkflowIDReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		// без этого SDK молча отдаёт уже идущий запуск вместо ошибки
		WorkflowExecutionErrorWhenAlreadyStarted: true,
	}, workflow, args...)
	if _, ok := errors.AsType[*serviceerror.WorkflowExecutionAlreadyStarted](err); ok {
		return fmt.Errorf("%w: %s %s", ErrAlreadyStarted, workflow, id)
	}
	if err != nil {
		return fmt.Errorf("start %s %s: %w", workflow, id, err)
	}
	return nil
}

// Update передаёт workflow действие и ждёт, пока обработчик его исполнит.
// Отказ валидатора и ошибка обработчика приходят как есть, в виде ApplicationError.
func (c *Client) Update(ctx context.Context, id, update string, arg any) error {
	h, err := c.UpdateWorkflow(ctx, client.UpdateWorkflowOptions{
		WorkflowID:   id,
		UpdateName:   update,
		Args:         []any{arg},
		WaitForStage: client.WorkflowUpdateStageCompleted,
	})
	if err == nil {
		err = h.Get(ctx, nil)
	}
	if _, ok := errors.AsType[*serviceerror.NotFound](err); ok {
		return fmt.Errorf("%w: %s", ErrNoProcess, id)
	}
	if err != nil {
		return fmt.Errorf("update %s %s: %w", update, id, err)
	}
	return nil
}
