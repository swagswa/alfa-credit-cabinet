package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
)

// TextReader достаёт текст из файла: текстовый слой PDF или OCR скана.
type TextReader interface {
	Text(ctx context.Context, contentType string, file []byte) (string, domain.TextSource, error)
}

// Extractor раскладывает текст по полям, на стенде это модель в Ollama.
type Extractor interface {
	Extract(ctx context.Context, kind domain.DocKind, text string) ([]byte, domain.DocValues, error)
	Model() string
	Digest(ctx context.Context) (string, error)
}

// extractorVersion поднимаем при смене промпта, схемы или предобработки,
// по нему в documents видно, каким конвейером распознан документ
const extractorVersion = "1"

type DocumentUseCase interface {
	// Read распознаёт файл и проверяет поля против ИНН заявки. Ошибка с
	// domain.ErrInvalid значит, что повтор не поможет, а rec заполнен, насколько
	// дошли: его сохраняем для разбора. Остальные ошибки это недоступный OCR или модель.
	Read(
		ctx context.Context,
		kind domain.DocKind,
		contentType string,
		file []byte,
		inn string,
	) (domain.Recognition, error)
	// Recognize распознаёт загруженный документ и пишет итог в documents.
	Recognize(ctx context.Context, id uuid.UUID) error
	// Fail ставит failed, когда ретраи кончились, клиент вводит итоги руками.
	Fail(ctx context.Context, id uuid.UUID, reason string) error
}

type documentUseCase struct {
	docs    repo.DocumentRepository
	text    TextReader
	llm     Extractor
	enabled bool
}

func NewDocument(docs repo.DocumentRepository, text TextReader, llm Extractor, enabled bool) DocumentUseCase {
	return &documentUseCase{docs: docs, text: text, llm: llm, enabled: enabled}
}

func (u *documentUseCase) Read(
	ctx context.Context, kind domain.DocKind, contentType string, file []byte, inn string,
) (rec domain.Recognition, err error) {
	start := time.Now()
	defer func() { rec.Took = time.Since(start) }()

	rec = domain.Recognition{Model: u.llm.Model(), Version: extractorVersion}
	if rec.Digest, err = u.llm.Digest(ctx); err != nil {
		return rec, fmt.Errorf("digest: %w", err)
	}
	var text string
	if text, rec.Source, err = u.text.Text(ctx, contentType, file); err != nil {
		return rec, fmt.Errorf("текст: %w", err)
	}
	if rec.Raw, rec.Values, err = u.llm.Extract(ctx, kind, text); err != nil {
		return rec, fmt.Errorf("поля: %w", err)
	}
	if rec.Problems = rec.Values.Check(inn, time.Now()); len(rec.Problems) > 0 {
		return rec, fmt.Errorf("%w: цифры из документа не сходятся, проверьте их вручную", domain.ErrInvalid)
	}
	return rec, nil
}

func (u *documentUseCase) Recognize(ctx context.Context, id uuid.UUID) error {
	d, err := u.docs.ForRecognition(ctx, id)
	if err != nil {
		return err
	}
	// повтор activity, клиент уже ввёл итоги сам или документ не из распознаваемых
	if d.Status != domain.DocUploaded && d.Status != domain.DocRecognizing ||
		d.Kind != domain.DocStatement && d.Kind != domain.DocFinancials {
		return nil
	}
	if !u.enabled {
		return u.docs.SetStatus(ctx, id, domain.DocSkipped, "")
	}
	err = u.docs.SetStatus(ctx, id, domain.DocRecognizing, "")
	if err != nil {
		return err
	}

	rec, err := u.Read(ctx, d.Kind, d.ContentType, d.Content, d.INN)
	status, reason := domain.DocRecognized, ""
	switch {
	case errors.Is(err, domain.ErrInvalid):
		status, reason = domain.DocFailed, err.Error()
	case err != nil:
		return err
	}
	return u.docs.SaveRecognition(ctx, id, status, &rec, reason)
}

func (u *documentUseCase) Fail(ctx context.Context, id uuid.UUID, reason string) error {
	return u.docs.SetStatus(ctx, id, domain.DocFailed, reason)
}
