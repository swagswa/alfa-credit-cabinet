package handler

import (
	"context"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
)

const statsDays = 30

func (h *Handler) BankStats(
	ctx context.Context,
	req oapi.BankStatsRequestObject,
) (oapi.BankStatsResponseObject, error) {
	days := statsDays
	set(&days, req.Params.Days)
	s, err := h.stats.Stats(ctx, days)
	if err != nil {
		return nil, err
	}
	res := oapi.BankStats200JSONResponse{
		Days:        s.Days,
		Submitted:   s.Submitted,
		Decided:     s.Decided,
		AutoDecided: s.AutoDecided,
		Approved:    s.Approved,
		IssuedKop:   s.IssuedKop,
	}
	res.DecisionTime.MedianSec = int(s.DecisionMedian.Seconds())
	res.DecisionTime.P90Sec = int(s.DecisionP90.Seconds())
	res.ByStatus = make([]struct {
		Count   int                    `json:"count"`
		Overdue int                    `json:"overdue"`
		Status  oapi.ApplicationStatus `json:"status"`
	}, len(s.ByStatus))
	for i, c := range s.ByStatus {
		r := &res.ByStatus[i]
		r.Status, r.Count, r.Overdue = oapi.ApplicationStatus(c.Status), c.Count, c.Overdue
	}
	return res, nil
}
