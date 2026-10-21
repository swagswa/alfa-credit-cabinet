package svc_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/swagswa/alfa-credit-cabinet/pkg/svc"
)

type fake struct {
	name    string
	log     *journal
	initErr error
	runErr  error
}

type journal struct {
	mu    sync.Mutex
	steps []string
}

func (j *journal) add(s string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.steps = append(j.steps, s)
}

func (f *fake) Name() string { return f.name }

func (f *fake) Init(context.Context) error {
	f.log.add("init " + f.name)
	return f.initErr
}

func (f *fake) Run(ctx context.Context) error {
	if f.runErr != nil {
		return f.runErr
	}
	<-ctx.Done()
	return nil
}

func (f *fake) Stop(context.Context) error {
	f.log.add("stop " + f.name)
	return nil
}

func TestRun(t *testing.T) {
	boom := errors.New("boom")

	tests := []struct {
		name    string
		initErr error
		runErr  error
		want    []string
	}{
		{
			name:   "ошибка run гасит всех в обратном порядке",
			runErr: boom,
			want:   []string{"init a", "init b", "stop b", "stop a"},
		},
		{
			name:    "ошибка init закрывает только поднятых",
			initErr: boom,
			want:    []string{"init a", "init b", "stop a"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := &journal{}
			a := &fake{name: "a", log: j}
			b := &fake{name: "b", log: j, initErr: tt.initErr, runErr: tt.runErr}

			err := svc.Run(context.Background(), zap.NewNop(), a, b)

			require.ErrorIs(t, err, boom)
			require.Equal(t, tt.want, j.steps)
		})
	}
}
