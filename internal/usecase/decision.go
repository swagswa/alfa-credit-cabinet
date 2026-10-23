package usecase

import (
	"time"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

// DecisionUseCase это ядро решения: чистые функции над фактами, без БД и моков.
// W03 зовёт по шагам: Facts, Check (на checking), при пустом исходе Score и Policy.
type DecisionUseCase interface {
	// Facts собирает факты из последних ответов источников, анкеты и документов заявки.
	Facts(app *domain.Application, checks []domain.CheckResult, now time.Time) (domain.Facts, error)
	// Check считает стоп-факторы. Outcome сильнейшего: отказ > запрос > ручное,
	// пустой, если запретов нет или сработали только мягкие (они уйдут в Policy).
	Check(f *domain.Facts) domain.Decision
	Score(f *domain.Facts) domain.Score
	// Policy даёт итог по классу, продукту и лимитам; checks это результат Check.
	Policy(app *domain.Application, f *domain.Facts, checks *domain.Decision, s *domain.Score) domain.Decision
}

type decisionUseCase struct {
	catalog domain.Catalog
	scoring domain.ScoringRules
	policy  domain.PolicyRules
}

func NewDecision(c domain.Catalog, s domain.ScoringRules, p domain.PolicyRules) DecisionUseCase {
	return &decisionUseCase{catalog: c, scoring: s, policy: p}
}
