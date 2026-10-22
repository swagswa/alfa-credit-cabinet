// Package domain: сущности, статусы, роли, таблица переходов, коды причин.
// Без I/O и без импортов из internal, его используют все слои.
package domain

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrForbidden         = errors.New("forbidden")
	ErrConflict          = errors.New("conflict")
	ErrIllegalTransition = errors.New("illegal transition")
	// ErrInvalid: запрос прошёл схему, но не прошёл проверку по смыслу (контрольные цифры ИНН и т.п.)
	ErrInvalid = errors.New("неверные данные")
	// ErrUnavailable: внешний источник не ответил или ответил 5xx
	ErrUnavailable = errors.New("source unavailable")
	// ErrUnauthorized: нет действующей сессии или неверный логин и пароль, без уточнения, что именно
	ErrUnauthorized    = errors.New("unauthorized")
	ErrTooManyRequests = errors.New("too many requests")
	// ErrKeyReused: тот же Idempotency-Key пришёл с другим телом
	ErrKeyReused = errors.New("idempotency key reused")
	// ErrIncomplete: анкету нельзя подать, что именно не так, в FieldErrors
	ErrIncomplete = errors.New("анкета заполнена не полностью")
)

// FieldErrors копит ошибки по полям. Как error это неполная анкета при подаче.
type FieldErrors []FieldError

func (e *FieldErrors) add(field, format string, args ...any) {
	*e = append(*e, FieldError{field, fmt.Sprintf(format, args...)})
}

func (e FieldErrors) Error() string { return fmt.Sprintf("%s: %v", ErrIncomplete, []FieldError(e)) }

func (FieldErrors) Unwrap() error { return ErrIncomplete }
