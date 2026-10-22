package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/migrations"
	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
	"github.com/swagswa/alfa-credit-cabinet/pkg/testenv"
)

// черновик кофейни из демо-сида и выписка к нему, пока API загрузки нет
func upload(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, postgres.Exec(migrations.SeedDemo)(ctx, pool))
	var id uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `
		WITH app AS (
			INSERT INTO applications (company_id, created_by, idempotency_key, product, purpose, amount_kop, term_months)
			VALUES ('01930000-0000-7000-8000-000000000101', '01930000-0000-7000-8000-000000000001', 'k1',
			        'working_capital', 'cash_gap', 100000000, 12)
			RETURNING id
		)
		INSERT INTO documents (application_id, kind, file_name, content_type, size_bytes, sha256, content, uploaded_by)
		SELECT id, 'statement', 'vypiska.pdf', 'application/pdf', 3, sha256('pdf'), 'pdf',
		       '01930000-0000-7000-8000-000000000001'
		FROM app
		RETURNING id`).Scan(&id))
	return id
}

func TestDocumentRecognition(t *testing.T) {
	pool := testenv.Postgres(t, migrations.FS)
	docs := repo.NewDocument(pool)
	ctx := t.Context()
	id := upload(t, pool)

	d, err := docs.ForRecognition(ctx, id)
	require.NoError(t, err)
	require.Equal(t, domain.DocFile{
		Kind: domain.DocStatement, Status: domain.DocUploaded, ContentType: "application/pdf",
		Content: []byte("pdf"), INN: "0000100012",
	}, d)

	_, err = docs.ForRecognition(ctx, uuid.New())
	require.ErrorIs(t, err, domain.ErrNotFound)

	rec := &domain.Recognition{
		Source: domain.TextOCR,
		Raw:    []byte(`{"inn":"0000100012","turnover_in":1`), // модель оборвалась на полуслове
		Values: domain.DocValues{Statement: &domain.Statement{INN: "0000100012"}},
		Problems: []domain.FieldError{
			{Field: "turnover_in_kop", Message: "нет поступлений за период"},
		},
		Model: "qwen3:4b-instruct-2507-q4_K_M", Digest: "0edcdef34593", Version: "1", Took: 42 * time.Second,
	}
	// не в recognizing итог не пишется
	require.NoError(t, docs.SaveRecognition(ctx, id, domain.DocFailed, rec, "не сошлось"))
	require.Equal(t, "uploaded", status(ctx, t, pool, id))

	require.NoError(t, docs.SetStatus(ctx, id, domain.DocRecognizing, ""))
	require.NoError(t, docs.SaveRecognition(ctx, id, domain.DocFailed, rec, "не сошлось"))

	var (
		raw, problems   string
		extracted, errs *string
		ms              int
	)
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT status, extracted_raw::text, extracted::text, validation_errors::text, error, duration_ms
		FROM documents WHERE id = $1`, id).Scan(new(string), &raw, &extracted, &problems, &errs, &ms))
	require.Equal(t, `"{\"inn\":\"0000100012\",\"turnover_in\":1"`, raw)
	require.Nil(t, extracted, "не прошло проверку, в форму не идёт")
	require.JSONEq(t, `[{"field":"turnover_in_kop","message":"нет поступлений за период"}]`, problems)
	require.Equal(t, "не сошлось", *errs)
	require.Equal(t, 42000, ms)
	require.Equal(t, "failed", status(ctx, t, pool, id))

	// failed больше не меняется: повтор activity или Fail после ретраев
	require.NoError(t, docs.SetStatus(ctx, id, domain.DocSkipped, ""))
	require.Equal(t, "failed", status(ctx, t, pool, id))
}

func TestDocumentRecognized(t *testing.T) {
	pool := testenv.Postgres(t, migrations.FS)
	docs := repo.NewDocument(pool)
	ctx := t.Context()
	id := upload(t, pool)

	require.NoError(t, docs.SetStatus(ctx, id, domain.DocRecognizing, ""))
	rec := &domain.Recognition{
		Source: domain.TextPDF,
		Raw:    []byte(`{"inn":"0000100012","year":2025}`),
		Values: domain.DocValues{Financials: &domain.Financials{INN: "0000100012", Year: 2025, Revenue: 100}},
		Model:  "qwen3:4b-instruct-2507-q4_K_M", Version: "1",
	}
	require.NoError(t, docs.SaveRecognition(ctx, id, domain.DocRecognized, rec, ""))

	var extracted string
	var problems *string
	require.NoError(
		t,
		pool.QueryRow(ctx, `SELECT extracted::text, validation_errors::text FROM documents WHERE id = $1`,
			id).Scan(&extracted, &problems),
	)
	require.JSONEq(t, `{"financials":{"inn":"0000100012","year":2025,"revenue_kop":100,"net_profit_kop":0,
		"liabilities_long_kop":0,"liabilities_short_kop":0,"balance_total_kop":0}}`, extracted)
	require.Nil(t, problems)
	require.Equal(t, "recognized", status(ctx, t, pool, id))
}

func status(ctx context.Context, t *testing.T, pool *pgxpool.Pool, id uuid.UUID) string {
	t.Helper()
	var s string
	require.NoError(t, pool.QueryRow(ctx, "SELECT status FROM documents WHERE id = $1", id).Scan(&s))
	return s
}
