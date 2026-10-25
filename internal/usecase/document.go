package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
)

// TextReader достаёт текст из файла: текстовый слой PDF или OCR скана.
type TextReader interface {
	Text(ctx context.Context, contentType string, file []byte) (string, domain.TextSource, error)
	// Reread даёт второе прочтение скана другим режимом, у текстового PDF пустую строку.
	Reread(ctx context.Context, contentType string, file []byte) (string, error)
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

// RecognizeWorkflowName регистрирует docs-worker, id процесса doc-<id документа>
const RecognizeWorkflowName = "RecognizeWorkflow"

// docTypes: что принимаем, тип определяем по содержимому, а не по заголовку от браузера
var docTypes = map[string]bool{"application/pdf": true, "image/png": true, "image/jpeg": true}

type DocumentUseCase interface {
	// Upload принимает документ к черновику или по запросу банка. Выписку и
	// отчётность сразу отдаёт на распознавание, без ИИ они skipped.
	Upload(ctx context.Context, actor domain.Actor, appID uuid.UUID, kind domain.DocKind, name string, file []byte) (
		domain.Document, error)
	Get(ctx context.Context, actor domain.Actor, id uuid.UUID) (domain.Document, error)
	File(ctx context.Context, actor domain.Actor, id uuid.UUID) (domain.DocFile, error)
	// Confirm сохраняет итоги, которые клиент проверил или ввёл сам. В скоринг идут только они.
	Confirm(ctx context.Context, actor domain.Actor, id uuid.UUID, v domain.DocValues) (domain.Document, error)

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
	docs      repo.DocumentRepository
	apps      repo.ApplicationRepository
	workflows Workflows
	text      TextReader
	llm       Extractor
	enabled   bool
}

// NewDocument собирает usecase документов. api нужны apps и workflows, docs-worker нужны text и llm, остальное можно nil.
func NewDocument(
	docs repo.DocumentRepository, apps repo.ApplicationRepository, workflows Workflows,
	text TextReader, llm Extractor, enabled bool,
) DocumentUseCase {
	return &documentUseCase{docs: docs, apps: apps, workflows: workflows, text: text, llm: llm, enabled: enabled}
}

func (u *documentUseCase) Upload(
	ctx context.Context, actor domain.Actor, appID uuid.UUID, kind domain.DocKind, name string, file []byte,
) (domain.Document, error) {
	ct := http.DetectContentType(file)
	switch {
	case len(file) > domain.MaxDocSize:
		return domain.Document{}, domain.ErrTooLarge
	case len(file) == 0:
		return domain.Document{}, fmt.Errorf("%w: файл пустой", domain.ErrInvalid)
	case !docTypes[ct]:
		return domain.Document{}, fmt.Errorf("%w: принимаем PDF, PNG или JPEG", domain.ErrInvalid)
	case kind == domain.DocLoanAgreement:
		return domain.Document{}, fmt.Errorf("%w: договор формирует банк", domain.ErrInvalid)
	}
	if err := u.open(ctx, actor, appID); err != nil {
		return domain.Document{}, err
	}

	f := domain.DocFile{Kind: kind, Status: domain.DocUploaded, FileName: name, ContentType: ct, Content: file}
	if kind.Recognizable() {
		f.Status = domain.DocSkipped
		if u.enabled {
			f.Status = domain.DocRecognizing
		}
	}
	id, err := u.docs.Add(ctx, appID, actor.ID, &f)
	if err != nil {
		return domain.Document{}, err
	}
	if f.Status == domain.DocRecognizing {
		// заявка от распознавания не зависит: Temporal недоступен, клиент вводит итоги руками,
		// а что Temporal лежит, видно в health. Повторный старт с тем же id Temporal отклонит,
		// так что дважды один файл модель не читает
		err = u.workflows.Start(ctx, QueueDocs, "doc-"+id.String(), RecognizeWorkflowName, id)
		if err != nil {
			err = u.docs.SetStatus(ctx, id, domain.DocFailed, "распознавание сейчас недоступно, введите итоги вручную")
		}
		if err != nil {
			return domain.Document{}, err
		}
	}
	return u.Get(ctx, actor, id)
}

// open: документы заявки меняются только в черновике или по запросу банка
func (u *documentUseCase) open(ctx context.Context, actor domain.Actor, appID uuid.UUID) error {
	app, err := u.apps.Card(ctx, appID, owner(actor))
	if err != nil {
		return err
	}
	if app.Status != domain.StatusDraft && app.Status != domain.StatusNeedInfo {
		return fmt.Errorf("%w: документы принимаем в черновике или по запросу банка", domain.ErrIllegalTransition)
	}
	return nil
}

func (u *documentUseCase) Get(ctx context.Context, actor domain.Actor, id uuid.UUID) (domain.Document, error) {
	return u.docs.Card(ctx, id, owner(actor))
}

func (u *documentUseCase) File(ctx context.Context, actor domain.Actor, id uuid.UUID) (domain.DocFile, error) {
	return u.docs.File(ctx, id, owner(actor))
}

func (u *documentUseCase) Confirm(
	ctx context.Context, actor domain.Actor, id uuid.UUID, v domain.DocValues,
) (domain.Document, error) {
	d, err := u.Get(ctx, actor, id)
	if err == nil {
		err = u.open(ctx, actor, d.ApplicationID)
	}
	switch {
	case err != nil:
		return d, err
	case d.Status == domain.DocConfirmed:
		return d, fmt.Errorf("%w: документ уже подтверждён", domain.ErrIllegalTransition)
	case !v.Fits(d.Kind):
		return d, fmt.Errorf("%w: заполните итоги под вид документа %s", domain.ErrInvalid, d.Kind)
	}
	if p := v.Check(d.INN, time.Now()); len(p) > 0 {
		return d, domain.FieldErrors(p)
	}
	if err := u.docs.Confirm(ctx, id, v, v.Edited(d.Extracted), actor.ID); err != nil {
		return d, err
	}
	return u.Get(ctx, actor, id)
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
	if rec.Problems = rec.Values.Check(inn, time.Now()); len(rec.Problems) > 0 && rec.Source == domain.TextOCR {
		u.reread(ctx, kind, contentType, file, inn, &rec)
	}
	if len(rec.Problems) > 0 {
		return rec, fmt.Errorf("%w: цифры из документа не сходятся, проверьте подсвеченные поля", domain.ErrInvalid)
	}
	return rec, nil
}

// reread: скан не сошёлся, читаем его ещё раз и сводим два прочтения. Это ещё
// один проход модели, поэтому только для сканов с ошибкой. Второе прочтение не
// удалось, значит, остаётся первое: клиент увидит его в форме с подсветкой.
func (u *documentUseCase) reread(
	ctx context.Context, kind domain.DocKind, contentType string, file []byte, inn string, rec *domain.Recognition,
) {
	text, err := u.text.Reread(ctx, contentType, file)
	if err != nil || text == "" {
		return
	}
	raw, alt, err := u.llm.Extract(ctx, kind, text)
	if err != nil {
		return
	}
	// в аудите оба ответа модели, по порядку прочтений
	if both, err := json.Marshal([]json.RawMessage{rec.Raw, raw}); err == nil {
		rec.Raw = both
	}
	if v, ok := rec.Values.Reconcile(alt, inn, time.Now()); ok {
		rec.Values, rec.Problems = v, nil
	}
}

func (u *documentUseCase) Recognize(ctx context.Context, id uuid.UUID) error {
	d, err := u.docs.File(ctx, id, nil)
	if err != nil {
		return err
	}
	// повтор activity, клиент уже ввёл итоги сам или документ не из распознаваемых
	if d.Status != domain.DocUploaded && d.Status != domain.DocRecognizing || !d.Kind.Recognizable() {
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
		status, reason = domain.DocFailed, domain.Human(err)
	case err != nil:
		return err
	}
	return u.docs.SaveRecognition(ctx, id, status, &rec, reason)
}

func (u *documentUseCase) Fail(ctx context.Context, id uuid.UUID, reason string) error {
	return u.docs.SetStatus(ctx, id, domain.DocFailed, reason)
}
