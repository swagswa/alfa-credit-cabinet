package workflow

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/swagswa/alfa-credit-cabinet/internal/worker/activity"
)

const (
	// на CPU выписка идёт 1-2 минуты (docs/ai-eval.md), первый запрос ещё и грузит модель.
	// Худший случай по умолчаниям docs-worker: скан 5 страниц по OCR_TIMEOUT 30 с и OLLAMA_TIMEOUT 240 с,
	// итого 390 с. Поднимаешь таймауты в конфиге, подними и этот
	recognizeTimeout  = 7 * time.Minute
	recognizeRetry    = 5 * time.Second
	recognizeAttempts = 2
)

// RecognizeWorkflow распознаёт документ на очереди docs. Заявка его не ждёт:
// итог пишется в documents, клиент видит его в форме и может ввести всё сам.
func RecognizeWorkflow(ctx workflow.Context, doc uuid.UUID) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: recognizeTimeout,
		RetryPolicy:         &temporal.RetryPolicy{InitialInterval: recognizeRetry, MaximumAttempts: recognizeAttempts},
	})
	var a *activity.DocumentActivities
	err := workflow.ExecuteActivity(ctx, a.Recognize, doc).Get(ctx, nil)
	if err == nil {
		return nil
	}
	// ретраи кончились, Ollama или Tesseract так и не ответили
	workflow.GetLogger(ctx).Warn("recognize failed", "doc", doc, "error", err)
	reason := "распознавание сейчас недоступно, введите итоги вручную"
	if err := workflow.ExecuteActivity(ctx, a.FailDocument, doc, reason).Get(ctx, nil); err != nil {
		return fmt.Errorf("fail document %s: %w", doc, err)
	}
	return nil
}
