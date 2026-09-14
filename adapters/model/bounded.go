package model

import (
	"context"
	"github.com/tushardhara/dream/app/hws"
	"time"
)

// BoundedProvider is shared by every gateway using a single host provider route.
// Excess calls fail without queueing. A provider ignoring context retains its
// slot until it really returns; timeouts cannot create hidden overlapping calls.
type BoundedProvider struct {
	provider hws.ModelProvider
	slots    chan struct{}
	observer hws.OperationalObserver
}

func NewBoundedProvider(p hws.ModelProvider, limit int, o hws.OperationalObserver) (*BoundedProvider, error) {
	if p == nil || limit < 1 || limit > 8 {
		return nil, hws.ErrModelBudget
	}
	return &BoundedProvider{provider: p, slots: make(chan struct{}, limit), observer: o}, nil
}
func (p *BoundedProvider) Generate(ctx context.Context, input hws.ProviderInput) (response hws.ProviderResponse, err error) {
	if p == nil {
		return response, hws.ErrModel
	}
	if err = ctx.Err(); err != nil {
		return response, err
	}
	select {
	case p.slots <- struct{}{}:
		defer func() { <-p.slots }()
	default:
		return response, hws.ErrModelBusy
	}
	started := time.Now()
	defer func() {
		if p.observer != nil {
			outcome := hws.OperationalSuccess
			switch {
			case err != nil:
				outcome = hws.ProviderOutage
			case response.Status == hws.ProviderUnavailable || response.Status == hws.ProviderRateLimited:
				outcome = hws.ProviderOutage
			case response.Status == hws.ProviderRefused:
				outcome = hws.SafetyDenied
			case response.Status != hws.ProviderOK:
				outcome = hws.OperationalFailure
			}
			p.observer.Observe(hws.ProviderCall, outcome, time.Since(started))
		}
	}()
	return p.provider.Generate(ctx, input)
}
