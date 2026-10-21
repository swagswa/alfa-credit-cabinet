// Package logger собирает zap в общем для всех процессов виде: JSON,
// debug и info в stdout, warn и выше в stderr.
package logger

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger держит zap и закрывает его как svc.Service. Встраивать zap.Logger
// нельзя: его метод Name конфликтует с Name сервиса.
type Logger struct {
	Zap *zap.Logger
}

// New отдаёт логгер с уровнем level: debug, info, warn или error.
func New(level string) (*Logger, error) {
	lvl, err := zapcore.ParseLevel(level)
	if err != nil {
		return nil, fmt.Errorf("log level %q: %w", level, err)
	}

	enc := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.MillisDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	})

	low := zap.LevelEnablerFunc(func(l zapcore.Level) bool { return l >= lvl && l < zapcore.WarnLevel })
	high := zap.LevelEnablerFunc(func(l zapcore.Level) bool { return l >= lvl && l >= zapcore.WarnLevel })

	core := zapcore.NewTee(
		zapcore.NewCore(enc, zapcore.Lock(os.Stdout), low),
		zapcore.NewCore(enc, zapcore.Lock(os.Stderr), high),
	)

	return &Logger{Zap: zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))}, nil
}

func (l *Logger) Name() string               { return "logger" }
func (l *Logger) Init(context.Context) error { return nil }
func (l *Logger) Run(context.Context) error  { return nil }

// Stop сбрасывает буфер. На stdout в контейнере Sync отдаёт EINVAL, это не ошибка.
func (l *Logger) Stop(context.Context) error {
	if err := l.Zap.Sync(); err != nil && !errors.Is(err, syscall.EINVAL) {
		return fmt.Errorf("sync: %w", err)
	}
	return nil
}
