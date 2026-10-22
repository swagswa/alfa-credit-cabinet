package repo

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

type DocumentRepository interface {
	Add(ctx context.Context, appID, by uuid.UUID, f *domain.DocFile) (uuid.UUID, error)
	// Card и File с owner ищут только среди заявок его компаний, без owner любой документ
	Card(ctx context.Context, id uuid.UUID, owner *uuid.UUID) (domain.Document, error)
	File(ctx context.Context, id uuid.UUID, owner *uuid.UUID) (domain.DocFile, error)
	// Confirm пишет итоги клиента один раз, уже подтверждённый даёт ErrIllegalTransition
	Confirm(ctx context.Context, id uuid.UUID, v domain.DocValues, edited []string, by uuid.UUID) error
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

func (r *documentRepo) Add(ctx context.Context, appID, by uuid.UUID, f *domain.DocFile) (uuid.UUID, error) {
	sum := sha256.Sum256(f.Content)
	id, err := r.q.AddDocument(ctx, db.AddDocumentParams{
		ApplicationID: appID,
		Kind:          string(f.Kind),
		Status:        string(f.Status),
		FileName:      f.FileName,
		ContentType:   f.ContentType,
		SizeBytes:     int32(len(f.Content)), //nolint:gosec // до 10 МБ, проверено выше и CHECK в схеме
		Sha256:        sum[:],
		Content:       f.Content,
		UploadedBy:    by,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("add document: %w", err)
	}
	return id, nil
}

func (r *documentRepo) Card(ctx context.Context, id uuid.UUID, owner *uuid.UUID) (domain.Document, error) {
	row, err := r.q.DocumentCard(ctx, db.DocumentCardParams{ID: id, Owner: optUUID(owner)})
	if err != nil {
		return domain.Document{}, notFound(err, "document "+id.String())
	}
	d := domain.Document{
		ID:            row.ID,
		ApplicationID: row.ApplicationID,
		Kind:          domain.DocKind(row.Kind),
		Status:        domain.DocStatus(row.Status),
		FileName:      row.FileName,
		ContentType:   row.ContentType,
		Size:          int(row.SizeBytes),
		SHA256:        row.Sha256,
		INN:           row.Inn,
		TextSource:    domain.TextSource(row.TextSource.String),
		EditedFields:  row.EditedFields,
		Error:         row.Error.String,
		Model:         row.Model.String,
		CreatedAt:     row.CreatedAt,
	}
	if row.ConfirmedAt.Valid {
		d.ConfirmedAt = &row.ConfirmedAt.Time
	}
	// jsonb пишем только мы сами, своими же типами
	if err := unmarshal(row.Extracted, &d.Extracted); err != nil {
		return d, err
	}
	if err := unmarshal(row.Confirmed, &d.Confirmed); err != nil {
		return d, err
	}
	return d, unmarshal(row.ValidationErrors, &d.Problems)
}

func (r *documentRepo) File(ctx context.Context, id uuid.UUID, owner *uuid.UUID) (domain.DocFile, error) {
	row, err := r.q.DocumentFile(ctx, db.DocumentFileParams{ID: id, Owner: optUUID(owner)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DocFile{}, fmt.Errorf("document %s: %w", id, domain.ErrNotFound)
	}
	if err != nil {
		return domain.DocFile{}, fmt.Errorf("document %s: %w", id, err)
	}
	return domain.DocFile{
		Kind:        domain.DocKind(row.Kind),
		Status:      domain.DocStatus(row.Status),
		FileName:    row.FileName,
		ContentType: row.ContentType,
		Content:     row.Content,
		INN:         row.Inn,
	}, nil
}

func (r *documentRepo) Confirm(
	ctx context.Context, id uuid.UUID, v domain.DocValues, edited []string, by uuid.UUID,
) error {
	values, _ := json.Marshal(v)
	n, err := r.q.ConfirmDocument(ctx, db.ConfirmDocumentParams{
		ID: id, Confirmed: values, EditedFields: edited, ConfirmedBy: optUUID(&by),
	})
	if err != nil {
		return fmt.Errorf("confirm document %s: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("%w: документ уже подтверждён", domain.ErrIllegalTransition)
	}
	return nil
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

func unmarshal[T any](b []byte, v *T) error {
	if b == nil {
		return nil
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("document json: %w", err)
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
