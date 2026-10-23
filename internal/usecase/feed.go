package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
)

type FeedUseCase interface {
	// List отдаёт ленту компании клиента: сначала подсказки по профилю, за ними
	// события заявок, новые сверху, всего не больше limit.
	List(ctx context.Context, actor domain.Actor, companyID uuid.UUID, limit int) ([]domain.FeedItem, error)
}

type feedUseCase struct {
	feed  repo.FeedRepository
	rules domain.FeedRules
}

func NewFeed(feed repo.FeedRepository, rules domain.FeedRules) FeedUseCase {
	return &feedUseCase{feed: feed, rules: rules}
}

func (u *feedUseCase) List(
	ctx context.Context, actor domain.Actor, companyID uuid.UUID, limit int,
) ([]domain.FeedItem, error) {
	p, err := u.feed.Profile(ctx, actor.ID, companyID)
	if err != nil {
		return nil, err
	}
	events, err := u.feed.Events(ctx, actor.ID, p.CompanyID, limit)
	if err != nil {
		return nil, err
	}

	// у подсказки нет своей даты, она актуальна на момент запроса
	now := time.Now().UTC()
	items := make([]domain.FeedItem, 0, limit)
	for _, t := range u.rules.Pick(&p) {
		items = append(items, domain.FeedItem{
			ID: "tip-" + t.Code, Kind: domain.FeedTip, Title: t.Title, Text: t.Text, At: now,
		})
	}
	for i := range events {
		if it, ok := u.rules.Event(&events[i]); ok {
			items = append(items, it)
		}
	}
	return items[:min(limit, len(items))], nil
}
