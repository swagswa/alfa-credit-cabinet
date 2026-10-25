package rules

import (
	"bytes"
	"fmt"
	"os"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

type noticesFile struct {
	Version     string                   `yaml:"version"`
	Statuses    map[string]domain.Phrase `yaml:"statuses"`
	SigningCode domain.Phrase            `yaml:"signing_code"`
}

// Notices читает rules/notifications.yaml, кривой файл роняет старт.
func Notices(path string) (domain.NoticeRules, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return domain.NoticeRules{}, fmt.Errorf("rules: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var f noticesFile
	if err = dec.Decode(&f); err != nil {
		return domain.NoticeRules{}, fmt.Errorf("rules %s: %w", path, err)
	}
	r := domain.NoticeRules{Version: f.Version, Statuses: map[domain.Status]domain.Phrase{}, SigningCode: f.SigningCode}
	for s, ph := range f.Statuses {
		st := domain.Status(s)
		if !slices.Contains(domain.Statuses, st) || ph.Title == "" || ph.Text == "" {
			return r, fmt.Errorf("rules %s: кривая фраза для %q", path, s)
		}
		r.Statuses[st] = ph
	}
	// без кода в тексте клиенту нечем подписать
	if !strings.Contains(f.SigningCode.Text, "{code}") || f.SigningCode.Title == "" {
		return r, fmt.Errorf("rules %s: в signing_code нужен {code}", path)
	}
	return r, nil
}
