// Package domain: сущности, статусы, роли, таблица переходов, коды причин.
// Без I/O и без импортов из internal, его используют все слои.
package domain

import "errors"

var (
	ErrNotFound          = errors.New("not found")
	ErrForbidden         = errors.New("forbidden")
	ErrConflict          = errors.New("conflict")
	ErrIllegalTransition = errors.New("illegal transition")
)
