package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

func (u *guidanceUseCase) Preliminary(ctx context.Context, actor domain.Actor, inn string) (domain.Preliminary, error) {
	p := domain.Preliminary{Version: u.rules.Version, Sources: []domain.Source{}, Items: []domain.PreliminaryAmount{}}
	if actor.Role != domain.RoleClient {
		return p, domain.ErrForbidden
	}
	if !domain.ValidINN(inn) {
		return p, domain.ErrInvalid
	}
	companies, err := u.users.Companies(ctx, actor.ID)
	if err != nil {
		return p, err
	}
	if !slices.ContainsFunc(companies, func(c domain.ProfileCompany) bool { return c.INN == inn }) {
		return p, domain.ErrNotFound
	}
	tax, bank, err := u.estimateReplies(ctx, inn)
	if err != nil {
		return p, err
	}
	p.Sources = []domain.Source{domain.SourceFNS, domain.SourceInternal}
	p.MonthlyRevenueKop, err = preliminaryRevenue(tax, bank)
	if err != nil {
		return p, err
	}
	if tax.Arrears.Tax > 0 || tax.Arrears.Penalty > 0 || tax.Arrears.Fine > 0 || len(tax.AccountBlocks) > 0 ||
		bank.AMLHighRisk {
		return p, nil
	}
	for i := range u.catalog.Products {
		product := &u.catalog.Products[i]
		amount := int64(
			math.Min(
				float64(product.AmountMaxKop),
				float64(p.MonthlyRevenueKop)*u.rules.RevenueMultipliers[product.Code],
			),
		)
		amount = amount / u.catalog.AmountStepKop * u.catalog.AmountStepKop
		if amount >= product.AmountMinKop {
			p.Items = append(p.Items, domain.PreliminaryAmount{Product: product.Code, AmountKop: amount})
		}
	}
	return p, nil
}

func (u *guidanceUseCase) estimateReplies(ctx context.Context, inn string) (fnsReply, bankReply, error) {
	// Здесь намеренно только два источника. Ни согласия, ни запроса БКИ на этом шаге нет.
	var tax fnsReply
	var bank bankReply
	for _, source := range []struct {
		code  domain.Source
		reply any
	}{
		{domain.SourceFNS, &tax}, {domain.SourceInternal, &bank},
	} {
		raw, err := u.sources.Fetch(ctx, source.code, inn, domain.BKIConsent{})
		if err != nil {
			return tax, bank, fmt.Errorf("ориентир %s: %w", source.code, err)
		}
		if err := json.Unmarshal(raw, source.reply); err != nil {
			return tax, bank, domain.ErrUnavailable
		}
		if source.code == domain.SourceInternal {
			var marker struct {
				IsClient *bool `json:"is_client"`
			}
			if json.Unmarshal(raw, &marker) != nil || marker.IsClient == nil {
				return tax, bank, domain.ErrUnavailable
			}
		}
	}
	return tax, bank, nil
}

func preliminaryRevenue(tax fnsReply, bank bankReply) (int64, error) {
	// Пустой/битый ответ не должен выглядеть как низкая выручка или отсутствие долга.
	if tax.Income <= 0 || tax.Income > math.MaxInt64/domain.KopInRub {
		return 0, domain.ErrUnavailable
	}
	monthly := tax.Income * domain.KopInRub / monthsInYear
	if bank.IsClient {
		var series []int64
		for _, month := range bank.Turnover {
			if month.Credit < 0 || month.Credit > math.MaxInt64/domain.KopInRub/int64(len(bank.Turnover)) {
				return 0, domain.ErrUnavailable
			}
			series = append(series, month.Credit*domain.KopInRub)
		}
		if len(series) == 0 {
			return 0, domain.ErrUnavailable
		}
		monthly = min(monthly, mean(series))
	}
	if tax.Arrears.Tax < 0 || tax.Arrears.Penalty < 0 || tax.Arrears.Fine < 0 {
		return 0, domain.ErrUnavailable
	}
	return monthly, nil
}
