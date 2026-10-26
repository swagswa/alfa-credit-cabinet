package usecase

import (
	"context"
	"slices"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
)

type GuidanceUseCase interface {
	Improvements(app *domain.Application) domain.ImprovementPlan
	Preliminary(ctx context.Context, actor domain.Actor, inn string) (domain.Preliminary, error)
}

type guidanceUseCase struct {
	users   repo.UserRepository
	sources Sources
	catalog domain.Catalog
	scoring domain.ScoringRules
	rules   domain.GuidanceRules
}

func NewGuidance(
	users repo.UserRepository,
	sources Sources,
	c domain.Catalog,
	s domain.ScoringRules,
	r domain.GuidanceRules,
) GuidanceUseCase {
	return &guidanceUseCase{users: users, sources: sources, catalog: c, scoring: s, rules: r}
}

func (u *guidanceUseCase) Improvements(app *domain.Application) domain.ImprovementPlan {
	p := domain.ImprovementPlan{Version: u.rules.Version, Items: []domain.Improvement{}}
	if app.Status != domain.StatusRejected || app.Decision == nil || app.Decision.Score == nil {
		return p
	}
	s := app.Decision.Score
	// Старый снимок нельзя пересчитывать новой версией правил и выдавать за прежний расчёт.
	if s.Version != u.scoring.Version {
		return p
	}
	p.Available, p.Score, p.Grade = true, s.Points, s.Grade
	for _, rule := range u.rules.Improvements {
		for _, fc := range u.scoring.Factors {
			if fc.Input != rule.Input {
				continue
			}
			i := slices.IndexFunc(s.Factors, func(f domain.FactorScore) bool { return f.Code == fc.Code })
			if i < 0 {
				continue
			}
			points, _ := bin(fc.Bins, rule.Target)
			delta := points - s.Factors[i].Points
			if delta <= 0 {
				continue
			}
			total := s.Points + delta
			g, _ := scoreGrade(u.scoring.Grades, total)
			p.Items = append(p.Items, domain.Improvement{Input: rule.Input, Title: rule.Title, Detail: rule.Detail,
				Target: rule.Target, Delta: delta, Score: total, Grade: g})
		}
	}
	return p
}
