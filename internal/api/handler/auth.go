package handler

import (
	"context"
	"net/http"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/middleware"
	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

func (h *Handler) Login(ctx context.Context, req oapi.LoginRequestObject) (oapi.LoginResponseObject, error) {
	me, raw, err := h.auth.Login(ctx, req.Body.Login, req.Body.Password, middleware.ClientIP(ctx))
	if err != nil {
		return nil, err
	}
	ck := h.cookie(raw, int(h.sessionTTL.Seconds()))
	return oapi.Login200JSONResponse{Body: toMe(me), Headers: oapi.Login200ResponseHeaders{SetCookie: &ck}}, nil
}

func (h *Handler) Logout(ctx context.Context, _ oapi.LogoutRequestObject) (oapi.LogoutResponseObject, error) {
	s, ok := middleware.ActorFrom(ctx)
	if !ok {
		return nil, domain.ErrUnauthorized
	}
	if err := h.auth.Logout(ctx, s.ID); err != nil {
		return nil, err
	}
	ck := h.cookie("", -1)
	return oapi.Logout204Response{Headers: oapi.Logout204ResponseHeaders{SetCookie: &ck}}, nil
}

func (h *Handler) Me(ctx context.Context, _ oapi.MeRequestObject) (oapi.MeResponseObject, error) {
	s, ok := middleware.ActorFrom(ctx)
	if !ok {
		return nil, domain.ErrUnauthorized
	}
	me, err := h.auth.Me(ctx, s.UserID)
	if err != nil {
		return nil, err
	}
	return oapi.Me200JSONResponse(toMe(me)), nil
}

// cookie: maxAge < 0 стирает cookie в браузере
func (h *Handler) cookie(value string, maxAge int) string {
	ck := http.Cookie{ //nolint:gosec // Secure из конфига: на стенде по http его приходится выключать
		Name:     middleware.SessionCookie,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
	return ck.String()
}

func toMe(me domain.Me) oapi.Me {
	out := oapi.Me{
		User: oapi.User{
			Id:       me.User.ID,
			Login:    me.User.Login,
			FullName: me.User.FullName,
			Role:     oapi.Role(me.User.Role),
		},
		Companies: make([]oapi.Membership, 0, len(me.Memberships)),
	}
	for _, m := range me.Memberships {
		out.Companies = append(out.Companies, oapi.Membership{
			Company: oapi.CompanyRef{
				Id:        m.CompanyID,
				Inn:       m.INN,
				ShortName: m.ShortName,
				Kind:      oapi.CompanyKind(m.Kind),
				Simulated: m.Simulated,
			},
			RepresentativeRole: oapi.RepresentativeRole(m.Role),
		})
	}
	return out
}
