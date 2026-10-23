package domain

import "time"

// Action это действие человека над идущей заявкой: ответ клиента, запрос сотрудника,
// отмена. Version карточки, которую он видел, сверяет Transition.
type Action struct {
	Actor   Actor
	Version int
	Codes   []string
	Comment string
}

// Deadlines сроки шагов, которые ведёт процесс. На стенде минуты, на проде рабочие дни.
type Deadlines struct {
	// NeedInfo сколько ждём ответа клиента, потом заявка истекает
	NeedInfo time.Duration
	// Review SLA андеррайтера: по нему только эскалация, статус не меняется
	Review time.Duration
}
