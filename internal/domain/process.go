package domain

import "time"

// Действие человека над идущей заявкой: ответ клиента, запрос сотрудника,
// отмена, решение андеррайтера. Version карточки, которую он видел, сверяет Transition.
type Action struct {
	Actor   Actor
	Version int
	Codes   []string
	Comment string
	// Documents только у запроса сотрудника
	Documents []DocKind
	// Outcome и Terms только у решения андеррайтера
	Outcome Status
	Terms   *Terms
	// RevenueKop уточнённая выручка из ответа клиента
	RevenueKop *int64
}

// Deadlines сроки шагов, которые ведёт процесс. На стенде минуты, на проде рабочие дни.
type Deadlines struct {
	// NeedInfo сколько ждём ответа клиента, потом заявка истекает
	NeedInfo time.Duration
	// Review SLA андеррайтера: по нему только эскалация, статус не меняется
	Review time.Duration
	// Offer сколько ждём, пока клиент примет условия
	Offer time.Duration
	// Signing сколько ждём подписи, потом заявка истекает
	Signing time.Duration
	// Close через сколько после выдачи заявка закрывается
	Close time.Duration
}

// Сам код не храним, сверяем по хэшу.
type SigningCode struct {
	Hash         []byte
	AttemptsLeft int
	ExpiresAt    time.Time
}
