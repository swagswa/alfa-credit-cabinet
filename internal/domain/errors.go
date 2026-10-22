// Package domain: сущности, статусы, роли, таблица переходов, коды причин.
// Без I/O и без импортов из internal, его используют все слои.
package domain

import "errors"

var (
	ErrNotFound          = errors.New("not found")
	ErrForbidden         = errors.New("forbidden")
	ErrConflict          = errors.New("conflict")
	ErrIllegalTransition = errors.New("illegal transition")
	// ErrInvalid: запрос прошёл схему, но не прошёл проверку по смыслу (контрольные цифры ИНН и т.п.)
	ErrInvalid = errors.New("invalid")
	// ErrUnavailable: внешний источник не ответил или ответил 5xx
	ErrUnavailable = errors.New("source unavailable")
)
