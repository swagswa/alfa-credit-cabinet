package logger_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/swagswa/alfa-credit-cabinet/pkg/logger"
)

func TestMask(t *testing.T) {
	tests := []struct {
		name  string
		field zap.Field
		want  any
	}{
		{"фио в строке", zap.String("comment", "звонила Соколова Анна Викторовна"), "звонила ***"},
		{"двойная фамилия", zap.String("comment", "Петрова-Водкина Мария Ильинична, директор"), "***, директор"},
		{"ип с инициалами", zap.String("company", "ИП Лебедев С. Н."), "ИП ***"},
		{"фио в ошибке", zap.Error(errors.New("egrul: director Орлов Дмитрий Андреевич: decode")),
			"egrul: director ***: decode"},
		{"ключ fio", zap.String("fio", "Морозов"), "***"},
		{"ключ login любого типа", zap.Any("Login", []string{"coffee"}), "***"},
		{"инн не трогаем", zap.String("inn", "000020001010"), "000020001010"},
		{"id и сумма", zap.String("app", "01930000-0000-7000-8000-000000000001 на 100000000"),
			"01930000-0000-7000-8000-000000000001 на 100000000"},
		{"название ооо", zap.String("company", "ООО «Тёплое зерно»"), "ООО «Тёплое зерно»"},
		{"фраза с заглавной", zap.String("reason", "Нет Согласия Клиента"), "Нет Согласия Клиента"},
		{"не строка", zap.Int("amount", 42), int64(42)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, logs := observer.New(zapcore.DebugLevel)
			zap.New(logger.Mask(core)).Info("msg", tt.field)
			require.Equal(t, tt.want, logs.All()[0].ContextMap()[tt.field.Key])
		})
	}
}

func TestMaskWithAndMessage(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	l := zap.New(logger.Mask(core)).With(zap.String("full_name", "Захарова Елена Павловна"))
	l.Debug("ниже уровня")
	l.Info("вошла Захарова Елена Павловна")

	require.Equal(t, 1, logs.Len())
	e := logs.All()[0]
	require.Equal(t, "вошла ***", e.Message)
	require.Equal(t, map[string]any{"full_name": "***"}, e.ContextMap())
}
