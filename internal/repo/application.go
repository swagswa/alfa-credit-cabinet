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

type ApplicationRepository interface {
	WithTx(tx pgx.Tx) ApplicationRepository
	// Lock берёт строку заявки FOR UPDATE до конца транзакции.
	Lock(ctx context.Context, id uuid.UUID) (domain.Application, error)
	// Move пишет новый статус, событие аудита и уведомление для SSE.
	Move(ctx context.Context, app domain.Application, m domain.Move) error
	// Escalate пишет в журнал, что SLA андеррайтера прошёл. Статус не меняется.
	Escalate(ctx context.Context, id uuid.UUID) error
	// Note пишет в журнал шаг подписания, статус не меняется.
	Note(ctx context.Context, id uuid.UUID, actor domain.Actor, comment string) error
	// SaveDecision пишет m.Decision, его условия становятся текущими. Звать до Move:
	// approved без условий база не пропустит.
	SaveDecision(ctx context.Context, id uuid.UUID, m *domain.Move) error
	// Decision отдаёт последнее решение по заявке, nil, если решений ещё не было.
	Decision(ctx context.Context, id uuid.UUID) (*domain.Decision, error)
	// AddInfoRequest открывает запрос клиенту, by нулевой у системы.
	AddInfoRequest(ctx context.Context, id, by uuid.UUID, r domain.InfoRequest) error
	CloseInfoRequest(ctx context.Context, id uuid.UUID) error
	// InfoRequest отдаёт открытый запрос, nil, если его нет.
	InfoRequest(ctx context.Context, id uuid.UUID) (*domain.InfoRequest, error)

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
	// History отдаёт журнал, старые сверху.
	History(ctx context.Context, id uuid.UUID) ([]domain.AuditEntry, error)
	// SaveDraft пишет анкету, если заявка всё ещё draft той же версии, иначе ErrConflict.
	SaveDraft(ctx context.Context, id uuid.UUID, version int, d domain.Draft) error
	// SetRevenue пишет уточнённую выручку, пока заявка в need_info, иначе ErrIllegalTransition.
	// Версию не поднимает: зовётся только вместе с переходом.
	SetRevenue(ctx context.Context, id uuid.UUID, kop int64) error
	// AddBKIConsent делает согласие из анкеты записью, повтор ничего не меняет.
	AddBKIConsent(ctx context.Context, id uuid.UUID) error
	BKIConsent(ctx context.Context, id uuid.UUID) (domain.BKIConsent, error)
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
	err := r.q.SetApplicationStatus(ctx, db.SetApplicationStatusParams{
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
	if err := r.q.NotifyApplication(ctx, app.ID); err != nil {
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

func (r *applicationRepo) Note(ctx context.Context, id uuid.UUID, actor domain.Actor, comment string) error {
	err := r.q.AddNote(ctx, db.AddNoteParams{
		ID:        id,
		ActorID:   pgtype.UUID{Bytes: actor.ID, Valid: actor.ID != uuid.Nil},
		ActorRole: string(actor.Role),
		Comment:   text(comment),
	})
	if err != nil {
		return fmt.Errorf("note %s: %w", id, err)
	}
	return nil
}

func (r *applicationRepo) SaveDecision(ctx context.Context, id uuid.UUID, m *domain.Move) error {
	d := m.Decision
	reasons := d.Reasons
	if reasons == nil {
		reasons = []domain.Reason{}
	}
	p := db.AddDecisionParams{
		ApplicationID:  id,
		Stage:          d.Stage,
		Outcome:        string(d.Outcome),
		ReasonCodes:    d.Codes(),
		Reasons:        jsonb(&reasons),
		ScoreBreakdown: jsonb(d.Score),
		Terms:          jsonb(d.Terms),
		Recommendation: jsonb(d.Recommendation),
		ScoringVersion: text(d.ScoringVersion),
		PolicyVersion:  text(d.PolicyVersion),
		IsOverride:     d.Override,
	}
	if d.Stage == domain.StageUnderwriter {
		p.DecidedBy = optUUID(&m.Actor.ID)
		p.Comment = text(m.Comment)
	}
	o := db.SetOfferParams{ID: id, ScoringVersion: p.ScoringVersion, PolicyVersion: p.PolicyVersion}
	if s := d.Score; s != nil {
		p.Score = pgtype.Int2{Int16: int16(s.Points), Valid: true}
		p.Grade = text(s.Grade)
		o.Score, o.Grade = p.Score, p.Grade
	}
	if t := d.Terms; t != nil {
		o.Product = text(string(t.Product))
		o.AmountKop, o.PaymentKop = kop(t.AmountKop), kop(t.PaymentKop)
		o.TermMonths = pgtype.Int4{Int32: int32(t.TermMonths), Valid: true}
		o.RateBp = pgtype.Int4{Int32: int32(t.RateBP), Valid: true}
		o.Guarantor, o.Pledge = t.Guarantor, t.Pledge
	}
	if err := r.q.AddDecision(ctx, p); err != nil {
		return fmt.Errorf("decision %s: %w", id, err)
	}
	if err := r.q.SetOffer(ctx, o); err != nil {
		return fmt.Errorf("offer %s: %w", id, err)
	}
	return nil
}

func (r *applicationRepo) Decision(ctx context.Context, id uuid.UUID) (*domain.Decision, error) {
	row, err := r.q.LatestDecision(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // решений ещё не было, это не ошибка
	}
	if err != nil {
		return nil, fmt.Errorf("decision %s: %w", id, err)
	}
	d := &domain.Decision{
		Stage:          row.Stage,
		Outcome:        domain.Status(row.Outcome),
		ScoringVersion: row.ScoringVersion.String,
		PolicyVersion:  row.PolicyVersion.String,
		Override:       row.IsOverride,
	}
	return d, errors.Join(
		unmarshal(row.Reasons, &d.Reasons),
		unmarshal(row.ScoreBreakdown, &d.Score),
		unmarshal(row.Terms, &d.Terms),
		unmarshal(row.Recommendation, &d.Recommendation),
	)
}

func (r *applicationRepo) AddInfoRequest(ctx context.Context, id, by uuid.UUID, req domain.InfoRequest) error {
	reasons := req.Reasons
	if reasons == nil {
		reasons = []domain.Reason{}
	}
	err := r.q.AddInfoRequest(ctx, db.AddInfoRequestParams{
		ApplicationID: id,
		Documents:     strs(req.Documents),
		Reasons:       jsonb(&reasons),
		Comment:       text(req.Comment),
		RequestedBy:   pgtype.UUID{Bytes: by, Valid: by != uuid.Nil},
	})
	if err != nil {
		return fmt.Errorf("info request %s: %w", id, err)
	}
	return nil
}

func (r *applicationRepo) CloseInfoRequest(ctx context.Context, id uuid.UUID) error {
	if err := r.q.CloseInfoRequest(ctx, id); err != nil {
		return fmt.Errorf("close info request %s: %w", id, err)
	}
	return nil
}

func (r *applicationRepo) InfoRequest(ctx context.Context, id uuid.UUID) (*domain.InfoRequest, error) {
	row, err := r.q.OpenInfoRequest(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // запроса нет, это не ошибка
	}
	if err != nil {
		return nil, fmt.Errorf("info request %s: %w", id, err)
	}
	req := &domain.InfoRequest{Comment: row.Comment.String}
	for _, k := range row.Documents {
		req.Documents = append(req.Documents, domain.DocKind(k))
	}
	return req, unmarshal(row.Reasons, &req.Reasons)
}

// jsonb: nil остаётся NULL
func jsonb[T any](v *T) []byte {
	if v == nil {
		return nil
	}
	b, _ := json.Marshal(v)
	return b
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
		OkvedName:    c.Okved.Name,
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
		TermMonths:        int32(d.TermMonths),
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
		doc := domain.Document{
			ID: d.ID, Kind: domain.DocKind(d.Kind), Status: domain.DocStatus(d.Status), FileName: d.FileName,
		}
		if err := unmarshal(d.Confirmed, &doc.Confirmed); err != nil {
			return app, err
		}
		app.Documents = append(app.Documents, doc)
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
		Lim:      int32(f.Limit),
		Off:      int32(f.Offset),
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
		Version:           int32(version),
		Product:           string(d.Product),
		Purpose:           string(d.Purpose),
		AmountKop:         d.AmountKop,
		TermMonths:        int32(d.TermMonths),
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

func (r *applicationRepo) SetRevenue(ctx context.Context, id uuid.UUID, kop int64) error {
	n, err := r.q.SetRevenue(ctx, db.SetRevenueParams{ID: id, MonthlyRevenueKop: kop})
	if err != nil {
		return fmt.Errorf("set revenue %s: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("%w: уточнить выручку можно, только когда банк её запросил", domain.ErrIllegalTransition)
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

func (r *applicationRepo) AddBKIConsent(ctx context.Context, id uuid.UUID) error {
	if err := r.q.AddBKIConsent(ctx, id); err != nil {
		return fmt.Errorf("bki consent %s: %w", id, err)
	}
	return nil
}

func (r *applicationRepo) BKIConsent(ctx context.Context, id uuid.UUID) (domain.BKIConsent, error) {
	row, err := r.q.BKIConsent(ctx, id)
	if err != nil {
		return domain.BKIConsent{}, notFound(err, "bki consent "+id.String())
	}
	return domain.BKIConsent{ID: row.ID, GivenAt: row.GivenAt}, nil
}
