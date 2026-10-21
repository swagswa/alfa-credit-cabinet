// Package workflow: процессы Temporal. Код детерминированный: время через
// workflow.Now, ожидание через workflow.NewTimer, всё внешнее через activity.
package workflow

import "go.temporal.io/sdk/workflow"

const (
	QueueCredit = "credit"
	QueueDocs   = "docs"
)

// ApplicationWorkflow ведёт заявку от подачи до выдачи, id workflow равен id заявки.
// Пока пустой: шаги появятся вместе с проверками и скорингом.
func ApplicationWorkflow(_ workflow.Context, _ string) error {
	return nil
}

// RecognizeWorkflow распознаёт один документ, id workflow doc-<id документа>.
// Пока пустой: тело появится вместе с распознаванием.
func RecognizeWorkflow(_ workflow.Context, _ string) error {
	return nil
}
