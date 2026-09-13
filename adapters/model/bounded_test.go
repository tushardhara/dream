package model

import (
	"context"
	"errors"
	"github.com/tushardhara/dream/app/hws"
	"sync/atomic"
	"testing"
	"time"
)

type blockingProvider struct {
	entered chan struct{}
	release chan struct{}
	active  atomic.Int64
}

func (p *blockingProvider) Generate(context.Context, hws.ProviderInput) (hws.ProviderResponse, error) {
	p.active.Add(1)
	defer p.active.Add(-1)
	p.entered <- struct{}{}
	<-p.release
	return hws.ProviderResponse{Status: hws.ProviderUnavailable}, nil
}
func TestProviderSlotsSurviveIgnoredCancellation(t *testing.T) {
	inner := &blockingProvider{entered: make(chan struct{}, 2), release: make(chan struct{})}
	bounded, err := NewBoundedProvider(inner, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{}, 2)
	for range 2 {
		go func() { _, _ = bounded.Generate(ctx, hws.ProviderInput{}); done <- struct{}{} }()
	}
	<-inner.entered
	<-inner.entered
	cancel()
	for range 20 {
		if _, err = bounded.Generate(context.Background(), hws.ProviderInput{}); !errors.Is(err, hws.ErrModelBusy) {
			t.Fatal("cancelled but active call freed its slot", err)
		}
	}
	if inner.active.Load() != 2 {
		t.Fatal("concurrency ceiling changed")
	}
	close(inner.release)
	for range 2 {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("provider did not finish")
		}
	}
	if _, err = bounded.Generate(ctx, hws.ProviderInput{}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled call started")
	}
}
