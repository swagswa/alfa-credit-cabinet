package server_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/handler"
	"github.com/swagswa/alfa-credit-cabinet/internal/api/server"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/rbac"
	"github.com/swagswa/alfa-credit-cabinet/internal/rules"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

type guidanceApps struct {
	usecase.ApplicationUseCase
	actor   domain.Actor
	visible uuid.UUID
}

func (a *guidanceApps) Get(_ context.Context, actor domain.Actor, id uuid.UUID) (domain.Application, error) {
	a.actor = actor
	if id != a.visible {
		return domain.Application{}, domain.ErrNotFound
	}
	return domain.Application{ID: id, Status: domain.StatusRejected}, nil
}

func TestGuidanceHTTP(t *testing.T) {
	auth := newAuth(t)
	me, session, err := auth.Login(t.Context(), "coffee", "demo12345", "10.0.0.1")
	require.NoError(t, err)
	c, err := rules.Products("../../../rules/products.yaml")
	require.NoError(t, err)
	s, err := rules.Scoring("../../../rules/scoring.yaml")
	require.NoError(t, err)
	r, err := rules.Guidance("../../../rules/guidance.yaml")
	require.NoError(t, err)
	enf, err := rbac.New()
	require.NoError(t, err)
	apps := &guidanceApps{visible: uuid.New()}
	e, err := server.New(server.Deps{
		Log:  zap.NewNop(),
		Auth: auth,
		RBAC: enf,
		Handler: handler.New(
			handler.Deps{Auth: auth, Applications: apps, Guidance: usecase.NewGuidance(&fakeUsers{}, nil, c, s, r)},
		),
	})
	require.NoError(t, err)
	path := "/api/applications/" + apps.visible.String() + "/improvements"
	res := call(t, e, http.MethodGet, path, "", session)
	require.Equal(t, 200, res.Code, res.Body.String())
	require.JSONEq(t, `{"available":false,"score":0,"grade":"","version":"2026-10-26.1","items":[]}`, res.Body.String())
	require.Equal(t, domain.Actor{ID: me.User.ID, Role: domain.RoleClient}, apps.actor)
	for _, tc := range []struct {
		path, session string
		code          int
	}{
		{path, "", 401},
		{"/api/applications/" + uuid.NewString() + "/improvements", session, 404},
		{"/api/applications/broken/improvements", session, 400},
		{"/api/companies/0000100012/preliminary", session, 404},
		{"/api/companies/short/preliminary", session, 400},
		{"/api/companies/0000100012/preliminary", "", 401},
	} {
		res := call(t, e, http.MethodGet, tc.path, "", tc.session)
		require.Equal(t, tc.code, res.Code, res.Body.String())
	}
}
