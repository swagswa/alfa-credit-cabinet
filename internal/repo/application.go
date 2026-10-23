package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

type ApplicationRepository interface {
	WithTx(tx pgx.Tx) ApplicationRepository
	// Lock берёт строку заявки FOR UPDATE до конца транзакции.
	Lock(ctx context.Context, id uuid.UUID) (domain.Application, error)
	// Move пишет новый статус, событие аудита и уведомление для SSE.
	Move(ctx context.Context, app domain.Application, m domain.Move) error
	// Escalate пишет в журнал, что SLA андеррайтера прошёл. Статус не меняется.
	Escalate(ctx context.Context, id uuid.UUID) error

	// Company находит компанию по ИНН и говорит, представляет ли её userID.
	Company(ctx context.Context, userID uuid.UUID, inn string) (id uuid.UUID, member bool, err error)
	AddCompany(ctx context.Context, c domain.Company) (uuid.UUID, error)
	SetMember(ctx context.Context, userID, companyID uuid.UUID, role domain.RepresentativeRole) error
	// Create заводит черновик. Если ключ уже был, отдаёт ту заявку и её отпечаток.
	Create(ctx context.Context, by uuid.UUID, key string, hash []byte, app domain.Application) (
		uuid.UUID, []byte, error)
	// Card отдаёт заявку целиком. С owner чужая заявка это ErrNotFound.
	Card(ctx context.Context, id uuid.UUID, owner *uuid.UUID) (domain.Application, error)
	List(ctx context.Context, f domain.ApplicationFilter) ([]domain.Application, int, error)
	// Checks отдаёт последний результат по каждому источнику, по алфавиту источников.
	Checks(ctx context.Context, id uuid.UUID) ([]domain.CheckResult, error)
	// History это журнал заявки, старые сверху.
	History(ctx context.Context, id uuid.UUID) ([]domain.AuditEntry, error)
	// SaveDraft пишет анкету, если заявка всё ещё draft той же версии, иначе ErrConflict.
	SaveDraft(ctx context.Context, id uuid.UUID, version int, d domain.Draft) error
}

type applicationRepo struct {
	q *db.Queries
}

func NewApplication(q *db.Queries) ApplicationRepository {
	return &applicationRepo{q: q}
}

func (r *applicationRepo) WithTx(tx pgx.Tx) ApplicationRepository {
	return &applicationRepo{q: r.q.WithTx(tx)}
}

func (r *applicationRepo) Lock(ctx context.Context, id uuid.UUID) (domain.Application, error) {
	row, err := r.q.LockApplication(ctx, id)
	if err != nil {
		return domain.Application{}, notFound(err, "lock application "+id.String())
	}
	return domain.Application{
		ID:        row.ID,
		CompanyID: row.CompanyID,
		Status:    domain.Status(row.Status),
		Version:   int(row.Version),
	}, nil
}

func (r *applicationRepo) Move(ctx context.Context, app domain.Application, m domain.Move) error {
	due := pgtype.Timestamptz{}
	if m.DueAt != nil {
		due = pgtype.Timestamptz{Time: *m.DueAt, Valid: true}
	}
	version, err := r.q.SetApplicationStatus(ctx, db.SetApplicationStatusParams{
		ID: app.ID, Status: string(m.To), DueAt: due,
	})
	if err != nil {
		return fmt.Errorf("set status %s: %w", app.ID, err)
	}
	err = r.q.AddTransitionEvent(ctx, db.AddTransitionEventParams{
		ApplicationID: app.ID,
		FromStatus:    pgtype.Text{String: string(app.Status), Valid: true},
		ToStatus:      pgtype.Text{String: string(m.To), Valid: true},
		ActorID:       pgtype.UUID{Bytes: m.Actor.ID, Valid: m.Actor.ID != uuid.Nil},
		ActorRole:     string(m.Actor.Role),
		ReasonCodes:   m.Codes,
		Comment:       pgtype.Text{String: m.Comment, Valid: m.Comment != ""},
	})
	if err != nil {
		return fmt.Errorf("transition event %s: %w", app.ID, err)
	}
	err = r.q.NotifyApplication(ctx, db.NotifyApplicationParams{
		ID: app.ID, CompanyID: app.CompanyID, Status: string(m.To), Version: version,
	})
	if err != nil {
		return fmt.Errorf("notify %s: %w", app.ID, err)
	}
	return nil
}

func (r *applicationRepo) Escalate(ctx context.Context, id uuid.UUID) error {
	if err := r.q.AddEscalation(ctx, id); err != nil {
		return fmt.Errorf("escalate %s: %w", id, err)
	}
	return nil
}

func (r *applicationRepo) Company(ctx context.Context, userID uuid.UUID, inn string) (uuid.UUID, bool, error) {
	row, err := r.q.CompanyByINN(ctx, db.CompanyByINNParams{UserID: userID, Inn: inn})
	if err != nil {
		return uuid.Nil, false, notFound(err, "company "+inn)
	}
	return row.ID, row.Member, nil
}

func (r *applicationRepo) AddCompany(ctx context.Context, c domain.Company) (uuid.UUID, error) {
	id, err := r.q.AddCompany(ctx, db.AddCompanyParams{
		Inn:          c.INN,
		Kind:         string(c.Kind),
		Ogrn:         c.OGRN,
		Kpp:          text(c.KPP),
		ShortName:    c.ShortName,
		FullName:     c.FullName,
		Okved:        c.Okved.Code,
		Address:      c.Address,
		RegisteredAt: pgtype.Date{Time: c.RegisteredAt, Valid: true},
		Simulated:    c.Simulated,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("add company %s: %w", c.INN, err)
	}
	return id, nil
}

func (r *applicationRepo) SetMember(
	ctx context.Context, userID, companyID uuid.UUID, role domain.RepresentativeRole,
) error {
	err := r.q.SetMember(
		ctx,
		db.SetMemberParams{UserID: userID, CompanyID: companyID, RepresentativeRole: string(role)},
	)
	if err != nil {
		return fmt.Errorf("member %s: %w", companyID, err)
	}
	return nil
}

func (r *applicationRepo) Create(
	ctx context.Context, by uuid.UUID, key string, hash []byte, app domain.Application,
) (uuid.UUID, []byte, error) {
	d := app.Draft
	id, err := r.q.CreateApplication(ctx, db.CreateApplicationParams{
		CompanyID:         app.CompanyID,
		CreatedBy:         by,
		IdempotencyKey:    key,
		RequestHash:       hash,
		Product:           string(d.Product),
		Purpose:           string(d.Purpose),
		AmountKop:         d.AmountKop,
		TermMonths:        int32(d.TermMonths), //nolint:gosec // 3-60 по схеме и CHECK
		MonthlyRevenueKop: kop(d.MonthlyRevenueKop),
		EquipmentCostKop:  kop(d.EquipmentCostKop),
		BkiConsent:        d.BKIConsent,
		PdConsent:         d.PDConsent,
		Comment:           text(d.Comment),
	})
	if err == nil {
		return id, hash, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil, fmt.Errorf("create application: %w", err)
	}
	row, err := r.q.ApplicationByKey(ctx, db.ApplicationByKeyParams{CreatedBy: by, IdempotencyKey: key})
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("application by key: %w", err)
	}
	return row.ID, row.RequestHash, nil
}

func (r *applicationRepo) Card(ctx context.Context, id uuid.UUID, owner *uuid.UUID) (domain.Application, error) {
	row, err := r.q.ApplicationCard(ctx, db.ApplicationCardParams{ID: id, Owner: optUUID(owner)})
	if err != nil {
		return domain.Application{}, notFound(err, "application "+id.String())
	}
	app := card(&row)
	docs, err := r.q.ApplicationDocuments(ctx, id)
	if err != nil {
		return app, fmt.Errorf("application %s documents: %w", id, err)
	}
	for _, d := range docs {
		app.Documents = append(app.Documents, domain.Document{
			ID: d.ID, Kind: domain.DocKind(d.Kind), Status: domain.DocStatus(d.Status), FileName: d.FileName,
		})
	}
	return app, nil
}

func (r *applicationRepo) List(ctx context.Context, f domain.ApplicationFilter) ([]domain.Application, int, error) {
	rows, err := r.q.ApplicationCards(ctx, db.ApplicationCardsParams{
		Owner:    optUUID(f.Owner),
		Statuses: strs(f.Statuses),
		Grades:   f.Grades,
		Product:  text(string(f.Product)),
		Sort:     f.Sort,
		Lim:      int32(f.Limit),  //nolint:gosec // до 100 по схеме
		Off:      int32(f.Offset), //nolint:gosec // смещение страницы, не миллиарды
	})
	if err != nil {
		return nil, 0, fmt.Errorf("applications: %w", err)
	}
	out := make([]domain.Application, 0, len(rows))
	total := 0
	for i := range rows {
		out = append(out, card(&rows[i].ApplicationCard))
		total = int(rows[i].Total)
	}
	return out, total, nil
}

func (r *applicationRepo) Checks(ctx context.Context, id uuid.UUID) ([]domain.CheckResult, error) {
	rows, err := r.q.LatestChecks(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("checks %s: %w", id, err)
	}
	out := make([]domain.CheckResult, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.CheckResult{
			Source: domain.Source(row.Source), Status: domain.CheckStatus(row.Status),
			Payload: row.Payload, Error: row.Error.String, At: row.CreatedAt,
		})
	}
	return out, nil
}

func (r *applicationRepo) History(ctx context.Context, id uuid.UUID) ([]domain.AuditEntry, error) {
	rows, err := r.q.ApplicationEvents(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("history %s: %w", id, err)
	}
	out := make([]domain.AuditEntry, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		out = append(out, domain.AuditEntry{
			ID:        row.ID,
			Kind:      row.Kind,
			From:      domain.Status(row.FromStatus.String),
			To:        domain.Status(row.ToStatus.String),
			ActorRole: domain.Role(row.ActorRole),
			ActorName: row.FullName.String,
			Codes:     row.ReasonCodes,
			Comment:   row.Comment.String,
			At:        row.CreatedAt,
		})
	}
	return out, nil
}

func (r *applicationRepo) SaveDraft(ctx context.Context, id uuid.UUID, version int, d domain.Draft) error {
	n, err := r.q.UpdateDraft(ctx, db.UpdateDraftParams{
		ID:                id,
		Version:           int32(version), //nolint:gosec // версия растёт по единице
		Product:           string(d.Product),
		Purpose:           string(d.Purpose),
		AmountKop:         d.AmountKop,
		TermMonths:        int32(d.TermMonths), //nolint:gosec // 3-60 по схеме и CHECK
		MonthlyRevenueKop: kop(d.MonthlyRevenueKop),
		EquipmentCostKop:  kop(d.EquipmentCostKop),
		BkiConsent:        d.BKIConsent,
		PdConsent:         d.PDConsent,
		Comment:           text(d.Comment),
	})
	if err != nil {
		return fmt.Errorf("save draft %s: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("%w: заявка уже изменилась, обновите карточку", domain.ErrConflict)
	}
	return nil
}

func card(row *db.ApplicationCard) domain.Application {
	app := domain.Application{
		ID:        row.ID,
		CompanyID: row.CompanyID,
		Status:    domain.Status(row.Status),
		Version:   int(row.Version),
		Number:    row.Number,
		Company: domain.Company{
			INN:       row.Inn,
			Kind:      domain.CompanyKind(row.Kind),
			ShortName: row.ShortName,
			Simulated: row.Simulated,
		},
		Role:              domain.RepresentativeRole(row.RepresentativeRole),
		Product:           domain.ProductCode(row.Product),
		Purpose:           domain.Goal(row.Purpose),
		AmountKop:         row.AmountKop,
		TermMonths:        int(row.TermMonths),
		MonthlyRevenueKop: row.MonthlyRevenueKop.Int64,
		EquipmentCostKop:  row.EquipmentCostKop.Int64,
		BKIConsent:        row.BkiConsent,
		PDConsent:         row.PdConsent,
		Comment:           row.Comment.String,
		Grade:             row.Grade.String,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
	if row.Score.Valid {
		score := int(row.Score.Int16)
		app.Score = &score
	}
	if row.DueAt.Valid {
		app.DueAt = &row.DueAt.Time
	}
	if row.SubmittedAt.Valid {
		app.SubmittedAt = &row.SubmittedAt.Time
	}
	return app
}

func kop(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: v != 0} }

func optUUID(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

func strs[T ~string](vs []T) []string {
	if vs == nil {
		return nil
	}
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = string(v)
	}
	return out
}
