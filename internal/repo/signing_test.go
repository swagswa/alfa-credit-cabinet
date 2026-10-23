package repo_test

import (
	"crypto/sha256"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/migrations"
	"github.com/swagswa/alfa-credit-cabinet/pkg/testenv"
)

func TestSigning(t *testing.T) {
	pool := testenv.Postgres(t, migrations.FS)
	ctx := t.Context()
	doc := upload(t, pool)
	var id uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, "SELECT application_id FROM documents WHERE id = $1", doc).Scan(&id))
	signing := repo.NewSigning(pool)

	_, err := signing.LockCode(ctx, id)
	require.ErrorIs(t, err, domain.ErrNotFound)

	// новый код перезаписывает старый и возвращает попытки
	old := sha256.Sum256([]byte("111111"))
	require.NoError(t, signing.SaveCode(ctx, id,
		domain.SigningCode{Hash: old[:], AttemptsLeft: 3, ExpiresAt: time.Now()}))
	require.NoError(t, signing.SetAttempts(ctx, id, 1))
	fresh := sha256.Sum256([]byte("222222"))
	due := time.Now().Add(5 * time.Minute).UTC().Truncate(time.Microsecond)
	require.NoError(t, signing.SaveCode(ctx, id, domain.SigningCode{Hash: fresh[:], AttemptsLeft: 3, ExpiresAt: due}))
	c, err := signing.LockCode(ctx, id)
	require.NoError(t, err)
	c.ExpiresAt = c.ExpiresAt.UTC()
	require.Equal(t, domain.SigningCode{Hash: fresh[:], AttemptsLeft: 3, ExpiresAt: due}, c)

	require.NoError(t, signing.DeleteCode(ctx, id))
	_, err = signing.LockCode(ctx, id)
	require.ErrorIs(t, err, domain.ErrNotFound)

	require.NoError(t, signing.Payout(ctx, id, 250000000))
	require.NoError(t, signing.Payout(ctx, id, 250000000))
	var n int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM payouts WHERE application_id = $1", id).Scan(&n))
	require.Equal(t, 1, n)

	// договор тоже один, сколько бы раз шаг ни повторился
	docs := repo.NewDocument(pool)
	f := &domain.DocFile{FileName: "dogovor-1.pdf", Content: []byte("%PDF")}
	require.NoError(t, docs.AddContract(ctx, id, f))
	require.NoError(t, docs.AddContract(ctx, id, f))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM documents
		WHERE application_id = $1 AND kind = 'loan_agreement' AND uploaded_by IS NULL`, id).Scan(&n))
	require.Equal(t, 1, n)

	// клиент договор от своего имени не положит
	_, err = docs.Add(ctx, id, uuid.MustParse("01930000-0000-7000-8000-000000000001"),
		&domain.DocFile{Kind: domain.DocLoanAgreement, Status: domain.DocUploaded, FileName: "x.pdf",
			ContentType: "application/pdf", Content: []byte("%PDF")})
	require.Error(t, err)

	client := domain.Actor{ID: uuid.MustParse("01930000-0000-7000-8000-000000000001"), Role: domain.RoleClient}
	require.NoError(t, repo.NewApplication(db.New(pool)).Note(ctx, id, client, "неверный код, осталось 2"))
	var kind, role, comment string
	require.NoError(t, pool.QueryRow(ctx, `SELECT kind, actor_role, comment FROM application_events
		WHERE application_id = $1`, id).Scan(&kind, &role, &comment))
	require.Equal(t, []string{"signing", "client", "неверный код, осталось 2"}, []string{kind, role, comment})
}
