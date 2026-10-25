package handler

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/middleware"
	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

func (h *Handler) Login(ctx context.Context, req oapi.LoginRequestObject) (oapi.LoginResponseObject, error) {
	me, raw, err := h.auth.Login(ctx, req.Body.Login, req.Body.Password, middleware.ClientIP(ctx))
	if err != nil {
		return nil, err
	}
	ck := h.cookie(middleware.SessionCookie, "/", raw, int(h.sessionTTL.Seconds()))
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
	ck := h.cookie(middleware.SessionCookie, "/", "", -1)
	return oapi.Logout204Response{Headers: oapi.Logout204ResponseHeaders{SetCookie: &ck}}, nil
}

func (h *Handler) Me(ctx context.Context, _ oapi.MeRequestObject) (oapi.MeResponseObject, error) {
	s, ok := middleware.ActorFrom(ctx)
	if !ok {
		return oapi.Me204Response{}, nil
	}
	me, err := h.auth.Me(ctx, s.UserID)
	if err != nil {
		return nil, err
	}
	return oapi.Me200JSONResponse(toMe(me)), nil
}

const (
	alfaPath        = "/api/auth/alfa-id"
	alfaStateCookie = "alfa_id_state"
	alfaStateAge    = 5 * 60
)

// AlfaIDStart: state ещё и в cookie, иначе чужую ссылку callback с кодом злоумышленника
// можно подсунуть жертве и залогинить её под ним
func (h *Handler) AlfaIDStart(
	ctx context.Context,
	_ oapi.AlfaIDStartRequestObject,
) (oapi.AlfaIDStartResponseObject, error) {
	state, err := h.auth.AlfaIDStart(ctx)
	if err != nil {
		return nil, err
	}
	q := url.Values{"client_id": {"credit-cabinet"}, "redirect_uri": {alfaPath + "/callback"}, "state": {state}}
	// страница согласия на стенде за тем же nginx, на проде тут адрес Альфа ID
	loc := "/alfa-id/authorize?" + q.Encode()
	ck := h.cookie(alfaStateCookie, alfaPath, state, alfaStateAge)
	return oapi.AlfaIDStart302Response{Headers: oapi.AlfaIDStart302ResponseHeaders{Location: &loc, SetCookie: &ck}}, nil
}

func (h *Handler) AlfaIDCallback(
	ctx context.Context,
	req oapi.AlfaIDCallbackRequestObject,
) (oapi.AlfaIDCallbackResponseObject, error) {
	back := func(reason string) oapi.AlfaIDCallbackResponseObject {
		loc := "/login?error=" + reason
		return oapi.AlfaIDCallback302Response{Headers: oapi.AlfaIDCallback302ResponseHeaders{Location: &loc}}
	}
	p := req.Params
	if p.Error != nil {
		return back("alfa_id_denied"), nil
	}
	if p.State == nil || p.Code == nil || p.AlfaIdState == nil || *p.State != *p.AlfaIdState {
		return back("alfa_id_expired"), nil
	}
	raw, err := h.auth.AlfaIDLogin(ctx, *p.State, *p.Code, middleware.ClientIP(ctx))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return back("alfa_id_unknown"), nil
	case errors.Is(err, domain.ErrUnauthorized):
		return back("alfa_id_expired"), nil
	case errors.Is(err, domain.ErrTooManyRequests):
		return back("too_many_attempts"), nil
	case err != nil:
		return nil, err
	}
	loc := "/client"
	ck := h.cookie(middleware.SessionCookie, "/", raw, int(h.sessionTTL.Seconds()))
	return oapi.AlfaIDCallback302Response{
		Headers: oapi.AlfaIDCallback302ResponseHeaders{Location: &loc, SetCookie: &ck},
	}, nil
}

// cookie: maxAge < 0 стирает cookie в браузере
func (h *Handler) cookie(name, path, value string, maxAge int) string {
	ck := http.Cookie{ //nolint:gosec // Secure из конфига: на стенде по http его приходится выключать
		Name:     name,
		Value:    value,
		Path:     path,
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
