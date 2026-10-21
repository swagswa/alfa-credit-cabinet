// Package zapadapter отдаёт zap в Temporal SDK, чтобы логи воркеров и
// клиента шли в том же формате, что и наши.
package zapadapter

import (
	"fmt"

	"go.temporal.io/sdk/log"
	"go.uber.org/zap"
)

type Adapter struct {
	zl *zap.Logger
}

var _ log.Logger = (*Adapter)(nil)

func New(zl *zap.Logger) *Adapter {
	// пропускаем кадр самого адаптера, иначе caller всегда указывает сюда
	return &Adapter{zl: zl.WithOptions(zap.AddCallerSkip(1))}
}

func (a *Adapter) Debug(msg string, keyvals ...any) { a.zl.Debug(msg, fields(keyvals)...) }
func (a *Adapter) Info(msg string, keyvals ...any)  { a.zl.Info(msg, fields(keyvals)...) }
func (a *Adapter) Warn(msg string, keyvals ...any)  { a.zl.Warn(msg, fields(keyvals)...) }
func (a *Adapter) Error(msg string, keyvals ...any) { a.zl.Error(msg, fields(keyvals)...) }

func (a *Adapter) With(keyvals ...any) log.Logger {
	return &Adapter{zl: a.zl.With(fields(keyvals)...)}
}

func (a *Adapter) WithCallerSkip(skip int) log.Logger {
	return &Adapter{zl: a.zl.WithOptions(zap.AddCallerSkip(skip))}
}

// fields: SDK передаёт пары ключ-значение подряд, как в slog
func fields(keyvals []any) []zap.Field {
	if len(keyvals)%2 != 0 {
		return []zap.Field{zap.Any("keyvals", keyvals)}
	}
	out := make([]zap.Field, 0, len(keyvals)/2)
	for i := 0; i < len(keyvals); i += 2 {
		key, ok := keyvals[i].(string)
		if !ok {
			key = fmt.Sprint(keyvals[i])
		}
		out = append(out, zap.Any(key, keyvals[i+1]))
	}
	return out
}
