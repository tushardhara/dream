package evals

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/experiment"
)

type studyMemory struct {
	mu         sync.Mutex
	events     []StudyEvent
	failReport bool
}

func (m *studyMemory) Load(context.Context, StudyScope) ([]StudyEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	raw, _ := json.Marshal(m.events)
	var out []StudyEvent
	_ = json.Unmarshal(raw, &out)
	return out, nil
}
func (m *studyMemory) Append(_ context.Context, _ StudyScope, expected int, e StudyEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if expected != len(m.events) || m.failReport && e.Kind == "reported" {
		return fmt.Errorf("CAS/failure")
	}
	m.events = append(m.events, e)
	return nil
}

type studyFakeClock struct{ at time.Time }

func (c *studyFakeClock) Now() time.Time { return c.at }
func studyFixture(t *testing.T) (StudyPlan, *studyMemory, *studyFakeClock, StudyController, []experiment.Request) {
	t.Helper()
	d, _ := fixture(t)
	hash, _ := Digest("frozen synthetic baseline fixture")
	clock := &studyFakeClock{fixtureNow}
	ref := StudyProviderRef{ID: "primary", Kind: "fake", Version: experiment.Version, Artifact: "sha256:" + strings.Repeat("0", 64)}
	second := ref
	second.ID = "second"
	plan := StudyPlan{Seed: 11, Variant: experiment.Stateful, Version: "study-protocol.v1", Scope: StudyScope{Owner: "evaluator", Namespace: "study", ID: "fixed-plan"}, StartUTC: clock.at, Days: 30, FrozenBaselineHash: hash, CandidateVersion: experiment.Version, Providers: [2]StudyProviderRef{ref, second}, DailyPredictions: 2, TotalPredictions: 60}
	journal := &studyMemory{}
	controller := StudyController{Journal: journal, Clock: clock, Providers: [2]StudyProvider{{Ref: ref, Generator: experiment.Generator{}}, {Ref: second, Generator: experiment.Generator{}}}}
	requests := []experiment.Request{{Input: d.Cases[0].Input, Variant: experiment.Stateful, Seed: 11}}
	return plan, journal, clock, controller, requests
}
func TestStudyFakeClockProtocolSequenceNotARealStudy(t *testing.T) {
	p, j, clock, c, requests := studyFixture(t)
	s, e := c.Register(context.Background(), p)
	if e != nil {
		t.Fatal(e)
	}
	deadline := s.Deadline
	for day := 0; day < 30; day++ {
		clock.at = p.StartUTC.Add(time.Duration(day) * RealDay)
		s, e = c.RunDay(context.Background(), p.Scope, core.ID(fmt.Sprintf("worker:%d", day)), requests)
		if e != nil {
			t.Fatal(day, e)
		}
		if s.Deadline != deadline || s.DaysRecorded != day+1 || s.Reserved != 2*(day+1) {
			t.Fatal("deadline extended or quota reset")
		}
		r := s.Reports[day]
		if r.HumanValidity != NotTested || r.CrossModel != NotTested || r.Behavioral != Inconclusive || r.LiveCalls != 0 {
			t.Fatal("fake protocol became human/cross-model evidence")
		}
		if _, e = c.RunDay(context.Background(), p.Scope, "duplicate", requests); e == nil {
			t.Fatal("second daily mission launched")
		}
	}
	events, _ := j.Load(context.Background(), p.Scope)
	recovered, e := RebuildStudy(events)
	if e != nil || recovered.Hash != s.Hash || len(events) != 61 {
		t.Fatal("explicit checkpoint reconstruction failed", e)
	}
	p.StartUTC = p.StartUTC.Add(RealDay)
	if _, e = c.Register(context.Background(), p); e == nil {
		t.Fatal("restart extended frozen study")
	}
}
func TestStudyQuotaUncertainRecoveryAndRepeatedFailure(t *testing.T) {
	p, j, clock, c, requests := studyFixture(t)
	p.TotalPredictions = 2
	if _, e := c.Register(context.Background(), p); e != nil {
		t.Fatal(e)
	}
	j.failReport = true
	if _, e := c.RunDay(context.Background(), p.Scope, "interrupted", requests); e == nil {
		t.Fatal("missing durable report ignored")
	}
	j.failReport = false
	if _, e := c.RunDay(context.Background(), p.Scope, "retry", requests); e == nil {
		t.Fatal("uncertain generation repeated")
	}
	if _, e := c.AbandonPending(context.Background(), p.Scope); e == nil {
		t.Fatal("active worker silently abandoned")
	}
	clock.at = clock.at.Add(5 * time.Minute)
	s, e := c.AbandonPending(context.Background(), p.Scope)
	if e != nil || s.Reserved != 2 || s.Reports[0].Engineering != Inconclusive {
		t.Fatal("uncertainty/refund", e)
	}
	clock.at = p.StartUTC.Add(RealDay)
	if _, e = c.RunDay(context.Background(), p.Scope, "next", requests); e == nil {
		t.Fatal("quota reset after restart/uncertainty")
	}
	p, _, clock, c, requests = studyFixture(t)
	_, _ = c.Register(context.Background(), p)
	c.Providers[0].Generator = batchFunc(func(context.Context, []experiment.Request) ([]experiment.Projection, error) {
		return nil, fmt.Errorf("fake outage")
	})
	for day := 0; day < 3; day++ {
		clock.at = p.StartUTC.Add(time.Duration(day) * RealDay)
		if _, e = c.RunDay(context.Background(), p.Scope, core.ID(fmt.Sprintf("failed:%d", day)), requests); e != nil {
			t.Fatal(e)
		}
	}
	clock.at = p.StartUTC.Add(3 * RealDay)
	if _, e = c.RunDay(context.Background(), p.Scope, "fourth-failure", requests); e == nil {
		t.Fatal("identical failure loop continued")
	}
}
func TestStudyConcurrentReservationAndProviderIsolation(t *testing.T) {
	p, _, _, c, requests := studyFixture(t)
	_, _ = c.Register(context.Background(), p)
	entered := make(chan struct{})
	release := make(chan struct{})
	c.Providers[0].Generator = batchFunc(func(ctx context.Context, r []experiment.Request) ([]experiment.Projection, error) {
		close(entered)
		<-release
		out, e := (experiment.Generator{}).Generate(ctx, r)
		r[0].Input.ID = "mutated-after-generation"
		return out, e
	})
	result := make(chan error, 1)
	go func() { _, e := c.RunDay(context.Background(), p.Scope, "first-worker", requests); result <- e }()
	<-entered
	if _, e := c.RunDay(context.Background(), p.Scope, "competing-worker", requests); e == nil {
		t.Fatal("competing daily writer admitted")
	}
	close(release)
	if e := <-result; e != nil {
		t.Fatal("provider inputs aliased", e)
	}
}
func TestStudyRejectsLiveAndMissedWindows(t *testing.T) {
	p, _, clock, c, requests := studyFixture(t)
	p.Providers[1].Kind = "live"
	if _, e := c.Register(context.Background(), p); e == nil {
		t.Fatal("live run authorized by caller metadata")
	}
	p.Providers[1].Kind = "fake"
	_, _ = c.Register(context.Background(), p)
	clock.at = p.StartUTC.Add(-time.Second)
	if _, e := c.RunDay(context.Background(), p.Scope, "early", requests); e == nil {
		t.Fatal("early launch")
	}
	clock.at = p.StartUTC.Add(2 * RealDay)
	if _, e := c.RunDay(context.Background(), p.Scope, "catch-up", requests); e == nil {
		t.Fatal("missing real days fabricated by catch-up")
	}
}

func TestStudyStopsNewProviderAtFixedDeadline(t *testing.T) {
	p, _, clock, c, requests := studyFixture(t)
	_, _ = c.Register(context.Background(), p)
	secondCalls := 0
	c.Providers[0].Generator = batchFunc(func(ctx context.Context, r []experiment.Request) ([]experiment.Projection, error) {
		out, e := (experiment.Generator{}).Generate(ctx, r)
		clock.at = p.StartUTC.Add(30 * RealDay)
		return out, e
	})
	c.Providers[1].Generator = batchFunc(func(ctx context.Context, r []experiment.Request) ([]experiment.Projection, error) {
		secondCalls++
		return (experiment.Generator{}).Generate(ctx, r)
	})
	s, e := c.RunDay(context.Background(), p.Scope, "deadline-worker", requests)
	if e != nil || secondCalls != 0 || s.Reserved != 2 || s.Reports[0].Finished != 1 || s.Reports[0].Engineering != Inconclusive {
		t.Fatal("deadline launched another provider or refunded quota", e)
	}
}
