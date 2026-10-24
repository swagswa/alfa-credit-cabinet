package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/swagswa/alfa-credit-cabinet/internal/docgen"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
)

const (
	signingAttempts = 3
	// код из 6 цифр
	codeSpace = 1_000_000
)

// wrongCode это ответ на неверный или сгоревший код: ошибка на поле формы, API отдаст 422
func wrongCode(msg string) error {
	return domain.FieldErrors{{Field: "code", Message: msg}}
}

// IssueUseCase всё после решения: принятие условий, договор, код подписи, выдача.
type IssueUseCase interface {
	Accept(ctx context.Context, actor domain.Actor, id uuid.UUID, version int) error
	// PrepareContract кладёт договор с графиком в документы заявки. date приходит
	// из процесса, поэтому повтор шага даёт тот же файл, а второй договор не появится.
	PrepareContract(ctx context.Context, id uuid.UUID, date time.Time) error
	// RequestCode выдаёт новый код подписи вместо старого. Только в signing.
	RequestCode(ctx context.Context, actor domain.Actor, id uuid.UUID) (string, domain.SigningCode, error)
	// Sign сверяет код и только верный передаёт процессу, без самого кода: аргументы
	// Update остаются в истории Temporal. Неверный код: FieldErrors по полю code и минус попытка.
	Sign(ctx context.Context, actor domain.Actor, id uuid.UUID, code string) error
	// Payout моделирует выдачу суммы из решения, повтор ничего не меняет.
	Payout(ctx context.Context, id uuid.UUID) error
}

type issueUseCase struct {
	db      *postgres.Client
	apps    ApplicationUseCase
	repo    repo.ApplicationRepository
	docs    repo.DocumentRepository
	signing repo.SigningRepository
	catalog domain.Catalog
	codeTTL time.Duration
}

func NewIssue(
	db *postgres.Client,
	apps ApplicationUseCase,
	appRepo repo.ApplicationRepository,
	docs repo.DocumentRepository,
	signing repo.SigningRepository,
	catalog domain.Catalog,
	codeTTL time.Duration,
) IssueUseCase {
	return &issueUseCase{
		db: db, apps: apps, repo: appRepo, docs: docs, signing: signing, catalog: catalog, codeTTL: codeTTL,
	}
}

func (u *issueUseCase) Accept(ctx context.Context, actor domain.Actor, id uuid.UUID, version int) error {
	return u.apps.Act(ctx, id, UpdateAccept, domain.Action{Actor: actor, Version: version})
}

func (u *issueUseCase) PrepareContract(ctx context.Context, id uuid.UUID, date time.Time) error {
	app, err := u.apps.Get(ctx, domain.Actor{Role: domain.RoleSystem}, id)
	if err != nil {
		return err
	}
	if app.Decision == nil || app.Decision.Terms == nil {
		return fmt.Errorf("%w: у заявки %s нет условий для договора", domain.ErrIllegalTransition, id)
	}
	t := *app.Decision.Terms
	p, _ := u.catalog.Product(t.Product)
	pdf, err := docgen.ContractPDF(docgen.Contract{
		Number: app.Number, Date: date, Product: p.Name, Company: app.Company.ShortName, INN: app.Company.INN,
		Terms: t, Repayment: p.Repayment,
	})
	if err != nil {
		return err
	}
	f := &domain.DocFile{FileName: fmt.Sprintf("dogovor-%d.pdf", app.Number), Content: pdf}
	return u.docs.AddContract(ctx, id, f)
}

func (u *issueUseCase) RequestCode(
	ctx context.Context, actor domain.Actor, id uuid.UUID,
) (string, domain.SigningCode, error) {
	app, err := u.repo.Card(ctx, id, owner(actor))
	if err != nil {
		return "", domain.SigningCode{}, err
	}
	if app.Status != domain.StatusSigning {
		return "", domain.SigningCode{}, fmt.Errorf("%w: код нужен только на подписании", domain.ErrIllegalTransition)
	}
	n, err := rand.Int(rand.Reader, big.NewInt(codeSpace))
	if err != nil {
		return "", domain.SigningCode{}, fmt.Errorf("signing code: %w", err)
	}
	code := fmt.Sprintf("%06d", n.Int64())
	sum := sha256.Sum256([]byte(code))
	c := domain.SigningCode{Hash: sum[:], AttemptsLeft: signingAttempts, ExpiresAt: time.Now().Add(u.codeTTL)}
	err = u.db.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if saveErr := u.signing.WithTx(tx).SaveCode(ctx, id, c); saveErr != nil {
			return saveErr
		}
		return u.repo.WithTx(tx).Note(ctx, id, actor, "запрошен код подписи")
	})
	if err != nil {
		return "", domain.SigningCode{}, err
	}
	return code, c, nil
}

func (u *issueUseCase) Sign(ctx context.Context, actor domain.Actor, id uuid.UUID, code string) error {
	app, err := u.repo.Card(ctx, id, owner(actor))
	if err != nil {
		return err
	}
	if app.Status != domain.StatusSigning {
		return fmt.Errorf("%w: подписать можно только на подписании", domain.ErrIllegalTransition)
	}
	// попытку списываем и при неверном коде, поэтому результат отдельно от ошибки транзакции
	var ok bool
	var left int
	err = u.db.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) (err error) {
		ok, left, err = u.attempt(ctx, tx, actor, id, code)
		return err
	})
	switch {
	case err != nil:
		return err
	case !ok && left > 0:
		return wrongCode(fmt.Sprintf("неверный код, осталось попыток: %d", left))
	case !ok:
		return wrongCode("код больше не действует, запросите новый")
	}
	return u.apps.Act(ctx, id, UpdateSign, domain.Action{Actor: actor})
}

// attempt сверяет код под блокировкой. Верный код удаляется, неверный стоит попытки.
// left сколько попыток осталось, 0 вместе с !ok: кода больше нет.
func (u *issueUseCase) attempt(
	ctx context.Context, tx pgx.Tx, actor domain.Actor, id uuid.UUID, code string,
) (ok bool, left int, err error) {
	codes := u.signing.WithTx(tx)
	c, err := codes.LockCode(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return false, 0, nil
	}
	if err != nil || c.AttemptsLeft == 0 || time.Now().After(c.ExpiresAt) {
		return false, 0, err
	}
	sum := sha256.Sum256([]byte(code))
	if subtle.ConstantTimeCompare(sum[:], c.Hash) == 1 {
		return true, c.AttemptsLeft, codes.DeleteCode(ctx, id)
	}
	left = c.AttemptsLeft - 1
	if err := codes.SetAttempts(ctx, id, left); err != nil {
		return false, left, err
	}
	note := fmt.Sprintf("неверный код, осталось %d", left)
	if left == 0 {
		note = "неверный код трижды, код сгорел"
	}
	return false, left, u.repo.WithTx(tx).Note(ctx, id, actor, note)
}

func (u *issueUseCase) Payout(ctx context.Context, id uuid.UUID) error {
	d, err := u.repo.Decision(ctx, id)
	if err != nil {
		return err
	}
	if d == nil || d.Terms == nil {
		return fmt.Errorf("%w: у заявки %s нет условий для выдачи", domain.ErrIllegalTransition, id)
	}
	return u.signing.Payout(ctx, id, d.Terms.AmountKop)
}
