package usecase

import (
	"slices"
	"strings"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

func (u *decisionUseCase) Check(f *domain.Facts) domain.Decision {
	// по возрастанию силы: отказ сильнее запроса, запрос сильнее ручного
	strength := []domain.Status{"", domain.StatusManualReview, domain.StatusNeedInfo, domain.StatusRejected}
	d := domain.Decision{Stage: domain.StageChecks, PolicyVersion: u.policy.Version}
	docs := make([]string, 0, len(f.MissingDocuments))
	for _, k := range f.MissingDocuments {
		docs = append(docs, u.policy.DocumentNames[k])
		d.Documents = append(d.Documents, domain.DocKind(k))
	}
	args := map[string]string{"doc": strings.Join(docs, ", ")}

	for _, r := range u.policy.StopFactors {
		v, _ := f.Input(r.Input)
		if !hit(r.Op, v, r.Value) {
			continue
		}
		d.Hits = append(d.Hits, domain.StopHit{Code: r.Code, Outcome: r.Outcome})
		d.Reasons = append(d.Reasons, u.reason(r.Code, args))
		if slices.Index(strength, r.Outcome) > slices.Index(strength, d.Outcome) {
			d.Outcome = r.Outcome
		}
	}
	// мягкие хиты не останавливают: скоринг идёт дальше, ручное ставит Policy
	if d.Outcome == domain.StatusManualReview {
		d.Outcome = ""
	}
	return d
}

// загрузчик правил уже проверил, что gt и lt стоят только на числах
func hit(op domain.Op, fact, v any) bool {
	a, _ := fact.(float64)
	b, _ := v.(float64)
	switch op {
	case domain.OpEq:
		return fact == v
	case domain.OpNe:
		return fact != v
	case domain.OpGt:
		return a > b
	case domain.OpLt:
		return a < b
	}
	return false
}

// reason собирает объяснение по шаблону из policy.yaml, подстановки вида {doc}.
// Нет шаблона, значит клиенту код не показываем и текст пустой.
func (u *decisionUseCase) reason(code string, args map[string]string) domain.Reason {
	var pairs []string
	for k, v := range args {
		pairs = append(pairs, "{"+k+"}", v)
	}
	return domain.Reason{Code: code, Text: strings.NewReplacer(pairs...).Replace(u.policy.Texts[code])}
}
