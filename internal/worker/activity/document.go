package activity

import (
	"context"

	"github.com/google/uuid"

	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

// DocumentActivities шаги RecognizeWorkflow.
type DocumentActivities struct {
	docs usecase.DocumentUseCase
}

func NewDocumentActivities(docs usecase.DocumentUseCase) *DocumentActivities {
	return &DocumentActivities{docs: docs}
}

// Recognize распознаёт документ. Документ, который не распознать, usecase сам
// помечает failed, сюда доходят только ошибки, которые стоит повторить.
func (a *DocumentActivities) Recognize(ctx context.Context, doc uuid.UUID) error {
	return a.docs.Recognize(ctx, doc)
}

func (a *DocumentActivities) FailDocument(ctx context.Context, doc uuid.UUID, reason string) error {
	return a.docs.Fail(ctx, doc, reason)
}
