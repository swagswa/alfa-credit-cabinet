package logger

import (
	"regexp"
	"slices"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ПДн, которые у нас бывают: ФИО пользователей и руководителей из ЕГРЮЛ, ФИО в
// названии ИП, логины. Поля с такими ключами скрываем целиком, в строках и текстах
// ошибок вырезаем то, что похоже на ФИО. ИНН компании, id и суммы оставляем,
// без них инцидент не разобрать.
const hidden = "***"

var piiKeys = map[string]bool{"fio": true, "full_name": true, "login": true}

// фамилия с именем и отчеством или с инициалами («ИП Лебедев С. Н.»). Без отчества
// не ловим: так выглядят и обычные слова с заглавной в начале фразы
var fio = regexp.MustCompile(
	`[А-ЯЁ][а-яё]+(?:-[А-ЯЁ][а-яё]+)? (?:[А-ЯЁ][а-яё]+ [А-ЯЁ][а-яё]+(?:вич|вна|чна)|[А-ЯЁ]\. ?[А-ЯЁ]\.)`)

type maskCore struct {
	zapcore.Core
}

// Mask оборачивает core так, чтобы ПДн не попадали в вывод. Оборачивать надо
// каждый core отдельно, а не Tee: Tee пишет во все свои core без проверки уровня.
func Mask(c zapcore.Core) zapcore.Core { return maskCore{c} }

func (c maskCore) With(fs []zapcore.Field) zapcore.Core { return maskCore{c.Core.With(mask(fs))} }

func (c maskCore) Check(e zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(e.Level) {
		return ce.AddCore(e, c)
	}
	return ce
}

func (c maskCore) Write(e zapcore.Entry, fs []zapcore.Field) error {
	e.Message = fio.ReplaceAllString(e.Message, hidden)
	return c.Core.Write(e, mask(fs)) //nolint:wrapcheck // прозрачная обёртка, своего контекста нет
}

func mask(fs []zapcore.Field) []zapcore.Field {
	out := slices.Clone(fs)
	for i, f := range out {
		switch {
		case piiKeys[strings.ToLower(f.Key)]:
			out[i] = zap.String(f.Key, hidden)
		case f.Type == zapcore.StringType:
			out[i].String = fio.ReplaceAllString(f.String, hidden)
		case f.Type == zapcore.ErrorType:
			// текст ошибки собирают моки, модель и валидатор запросов, в нём бывает что угодно
			if err, ok := f.Interface.(error); ok {
				out[i] = zap.String(f.Key, fio.ReplaceAllString(err.Error(), hidden))
			}
		}
	}
	return out
}
