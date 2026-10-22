package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

// DocumentRepository пока только для распознавания, загрузку и подтверждение добавит API документов.
type DocumentRepository interface {
	ForRecognition(ctx context.Context, id uuid.UUID) (domain.DocFile, error)
	// SetStatus не трогает документ, у которого распознавание уже закончилось
	SetStatus(ctx context.Context, id uuid.UUID, s domain.DocStatus, reason string) error
	// SaveRecognition пишет итог, только пока документ в recognizing
	SaveRecognition(ctx context.Context, id uuid.UUID, s domain.DocStatus, rec *domain.Recognition, reason string) error
}

type documentRepo struct {
	q *db.Queries
}

func NewDocument(conn db.DBTX) DocumentRepository {
	return &documentRepo{q: db.New(conn)}
}

func (r *documentRepo) ForRecognition(ctx context.Context, id uuid.UUID) (domain.DocFile, error) {
	row, err := r.q.DocumentForRecognition(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DocFile{}, fmt.Errorf("document %s: %w", id, domain.ErrNotFound)
	}
	if err != nil {
		return domain.DocFile{}, fmt.Errorf("document %s: %w", id, err)
	}
	return domain.DocFile{
		Kind:        domain.DocKind(row.Kind),
		Status:      domain.DocStatus(row.Status),
		ContentType: row.ContentType,
		Content:     row.Content,
		INN:         row.Inn,
	}, nil
}

func (r *documentRepo) SetStatus(ctx context.Context, id uuid.UUID, s domain.DocStatus, reason string) error {
	err := r.q.SetDocumentStatus(ctx, db.SetDocumentStatusParams{ID: id, Status: string(s), Error: text(reason)})
	if err != nil {
		return fmt.Errorf("document %s status %s: %w", id, s, err)
	}
	return nil
}

func (r *documentRepo) SaveRecognition(
	ctx context.Context, id uuid.UUID, s domain.DocStatus, rec *domain.Recognition, reason string,
) error {
	ms := int32(rec.Took.Milliseconds()) //nolint:gosec // минуты, не годы
	p := db.SaveRecognitionParams{
		ID:               id,
		Status:           string(s),
		TextSource:       text(string(rec.Source)),
		ExtractedRaw:     rawJSON(rec.Raw),
		Error:            text(reason),
		Model:            text(rec.Model),
		ModelDigest:      text(rec.Digest),
		ExtractorVersion: text(rec.Version),
		DurationMs:       pgtype.Int4{Int32: ms, Valid: true},
	}
	// в extracted только то, что прошло проверку, его клиент и увидит в форме;
	// Marshal своих типов без каналов и функций не падает
	if s == domain.DocRecognized {
		p.Extracted, _ = json.Marshal(rec.Values)
	}
	if len(rec.Problems) > 0 {
		p.ValidationErrors, _ = json.Marshal(rec.Problems)
	}
	if err := r.q.SaveRecognition(ctx, p); err != nil {
		return fmt.Errorf("document %s recognition: %w", id, err)
	}
	return nil
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

// rawJSON: обрезанный ответ модели в jsonb не ляжет, а сохранить его для разбора надо
func rawJSON(b []byte) []byte {
	if b == nil || json.Valid(b) {
		return b
	}
	s, _ := json.Marshal(string(b))
	return s
}
