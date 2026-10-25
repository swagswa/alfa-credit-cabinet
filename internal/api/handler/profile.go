package handler

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/middleware"
	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

func (h *Handler) GetProfile(
	ctx context.Context, _ oapi.GetProfileRequestObject,
) (oapi.GetProfileResponseObject, error) {
	s, ok := middleware.ActorFrom(ctx)
	if !ok {
		return nil, domain.ErrUnauthorized
	}
	p, err := h.profile.Profile(ctx, s.UserID)
	if err != nil {
		return nil, err
	}
	u := p.User
	res := oapi.GetProfile200JSONResponse{
		User: oapi.ProfileUser{
			Id: u.ID, Login: u.Login, FullName: u.FullName, Role: oapi.Role(u.Role), Phone: u.Phone, Email: u.Email,
		},
		Companies: make([]oapi.ProfileCompany, 0, len(p.Companies)),
	}
	for i := range p.Companies {
		res.Companies = append(res.Companies, toProfileCompany(&p.Companies[i]))
	}
	return res, nil
}

func toProfileCompany(c *domain.ProfileCompany) oapi.ProfileCompany {
	out := oapi.ProfileCompany{
		Company: oapi.CompanyRef{
			Id: c.ID, Inn: c.INN, ShortName: c.ShortName, Kind: oapi.CompanyKind(c.Kind), Simulated: c.Simulated,
		},
		RepresentativeRole: oapi.RepresentativeRole(c.Role),
		Ogrn:               c.OGRN,
		FullName:           c.FullName,
		OkvedMain:          oapi.Okved{Code: c.Okved.Code, Name: c.Okved.Name},
		Address:            c.Address,
		RegisteredAt:       openapi_types.Date{Time: c.RegisteredAt},
		BankUnavailable:    c.BankDown,
		Loans:              make([]oapi.Loan, 0, len(c.Loans)),
	}
	if c.KPP != "" {
		out.Kpp = &c.KPP
	}
	if b := c.Bank; b != nil {
		out.Bank = &oapi.BankAccount{
			AccountMask:    b.Mask(),
			ClientSince:    openapi_types.Date{Time: b.Since},
			Turnover12mKop: b.TurnoverKop,
			Products:       make([]oapi.BankAccountProducts, 0, len(b.Products)),
		}
		for _, p := range b.Products {
			out.Bank.Products = append(out.Bank.Products, oapi.BankAccountProducts(p))
		}
	}
	for i := range c.Loans {
		l := &c.Loans[i]
		out.Loans = append(out.Loans, oapi.Loan{
			ApplicationId:     l.ApplicationID,
			Number:            l.Number,
			Status:            oapi.ApplicationStatus(l.Status),
			IssuedAt:          l.IssuedAt,
			AmountKop:         l.Terms.AmountKop,
			TermMonths:        l.Terms.TermMonths,
			RateBp:            l.Terms.RateBP,
			MonthlyPaymentKop: l.Terms.PaymentKop,
			ContractId:        l.ContractID,
		})
	}
	return out
}
