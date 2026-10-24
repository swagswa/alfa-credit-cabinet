package usecase

import (
	"context"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

// Faults переключатель сбоев, на стенде это ручка самого мока.
type Faults interface {
	List(ctx context.Context) ([]domain.Fault, error)
	Set(ctx context.Context, f domain.Fault) error
}

// flakyFails меньше попыток процесса: моргнувший источник отвечает с четвёртой
const flakyFails = 3

type FaultUseCase interface {
	// List режим каждого источника в порядке опроса, без сбоя FaultOff.
	// Сбои на отдельную компанию пульт не показывает и не трогает.
	List(ctx context.Context) ([]domain.Fault, error)
	Set(ctx context.Context, src domain.Source, mode string) error
}

type faultUseCase struct {
	faults Faults
}

func NewFault(faults Faults) FaultUseCase {
	return &faultUseCase{faults: faults}
}

func (u *faultUseCase) List(ctx context.Context) ([]domain.Fault, error) {
	all, err := u.faults.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Fault, len(domain.Sources))
	for i, src := range domain.Sources {
		out[i] = domain.Fault{Source: src, Mode: domain.FaultOff}
	}
	for _, f := range all {
		for i := range out {
			if f.INN == "" && f.Source == out[i].Source {
				out[i] = f
			}
		}
	}
	return out, nil
}

func (u *faultUseCase) Set(ctx context.Context, src domain.Source, mode string) error {
	f := domain.Fault{Source: src, Mode: mode}
	if mode == "flaky" {
		f.Left = flakyFails
	}
	return u.faults.Set(ctx, f)
}
