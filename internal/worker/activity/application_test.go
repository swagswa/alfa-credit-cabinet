package activity_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
	"github.com/swagswa/alfa-credit-cabinet/internal/worker/activity"
)

type fakeApps struct {
	usecase.ApplicationUseCase
	err error
}

func (f fakeApps) Transition(context.Context, uuid.UUID, domain.Move) error { return f.err }

func TestTransition(t *testing.T) {
	boom := errors.New("connection reset")
	tests := []struct {
		name    string
		err     error
		errType string // пусто: ошибка повторяемая
	}{
		{"нет заявки", domain.ErrNotFound, usecase.NotFoundErrorType},
		{
			"переход не по таблице",
			fmt.Errorf("%w: из a в b", domain.ErrIllegalTransition),
			usecase.IllegalTransitionErrorType,
		},
		{"устаревшая версия", domain.ErrConflict, usecase.VersionConflictErrorType},
		{"бки без согласия", fmt.Errorf("bki: %w", domain.ErrForbidden), usecase.ForbiddenErrorType},
		{"источник не понял запрос", fmt.Errorf("fns: 400: %w", domain.ErrInvalid), usecase.InvalidErrorType},
		{"сбой бд", boom, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := activity.NewApplicationActivities(fakeApps{err: tt.err}, nil, nil, domain.Deadlines{})
			err := a.Transition(t.Context(), uuid.New(), domain.Move{To: domain.StatusChecking})

			var appErr *temporal.ApplicationError
			if tt.errType == "" {
				require.ErrorIs(t, err, boom)
				require.False(t, errors.As(err, &appErr))
				return
			}
			require.ErrorAs(t, err, &appErr)
			require.Equal(t, tt.errType, appErr.Type())
			require.True(t, appErr.NonRetryable())
		})
	}

	require.NoError(
		t,
		activity.NewApplicationActivities(fakeApps{}, nil, nil, domain.Deadlines{}).
			Transition(t.Context(), uuid.New(), domain.Move{}),
	)
}
