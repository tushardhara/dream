package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
	"testing"
)

type fakeMaintenance struct {
	ready bool
	calls int
	fail  bool
}

func (f *fakeMaintenance) RuntimeReady(context.Context) error {
	if !f.ready {
		return errors.New("private-dsn-canary")
	}
	return nil
}
func (f *fakeMaintenance) ReconcileModels(context.Context, hws.Scope) (int, error) {
	f.calls++
	if f.fail {
		return 0, errors.New("private-provider-canary")
	}
	return 1, nil
}
func (f *fakeMaintenance) Project(context.Context, core.ID, int) (int, error) { return 2, nil }
func workerConfig() configuration {
	return configuration{Version: 1, Scopes: []hws.Scope{{Actor: "operator", Namespace: "fixture", World: "w", Branch: "b", Run: "r"}}, Development: true, IntervalSeconds: 1, MaxCycles: 1}
}
func TestBoundedWorkerAndSanitizedFailures(t *testing.T) {
	c := workerConfig()
	f := &fakeMaintenance{ready: true}
	var out bytes.Buffer
	if work(context.Background(), f, c, &out) != 0 || f.calls != 1 {
		t.Fatal("unbounded or missing cycle", out.String())
	}
	f.ready = false
	out.Reset()
	if work(context.Background(), f, c, &out) != 1 || f.calls != 1 || bytes.Contains(out.Bytes(), []byte("canary")) {
		t.Fatal("readiness failure ignored or leaked", out.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f.ready = true
	if work(ctx, f, c, &out) != 0 || f.calls != 1 {
		t.Fatal("shutdown launched work")
	}
}
func TestWorkerConfigurationBounds(t *testing.T) {
	for _, mutate := range []func(*configuration){func(c *configuration) { c.MaxCycles = 0 }, func(c *configuration) { c.MaxCycles = 3601 }, func(c *configuration) { c.IntervalSeconds = 0 }, func(c *configuration) { c.Scopes = append(c.Scopes, c.Scopes[0]) }, func(c *configuration) { c.Scopes[0].Actor = "" }} {
		c := workerConfig()
		mutate(&c)
		raw, _ := json.Marshal(c)
		if _, e := decode(raw); e == nil {
			t.Fatal("unsafe config")
		}
	}
	c := workerConfig()
	raw, _ := json.Marshal(c)
	if _, e := decode(raw); e != nil {
		t.Fatal(e)
	}
	if _, e := decode(append(raw, []byte(" {}")...)); e == nil {
		t.Fatal("trailing config")
	}
	var out bytes.Buffer
	if run(context.Background(), []string{"--help"}, &out) != 0 {
		t.Fatal("help failed")
	}
}
