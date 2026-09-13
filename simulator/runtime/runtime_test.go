package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/scenario"
	"testing"
)

func initial(t testing.TB) State {
	t.Helper()
	s := scenario.Scenario{Version: 1, World: scenario.World{ID: "w", Seed: 42, Horizon: 100}, Public: scenario.Public{Humans: []scenario.Human{{ID: "a", Name: "Synthetic A", Age: 30}, {ID: "b", Name: "Synthetic B", Age: 40}}}, Actors: []scenario.Actor{{ID: "a"}, {ID: "b"}}, Future: []scenario.Scheduled{{ID: "e2", At: 20, Kind: "observation", Actor: "a", Text: "second"}, {ID: "e1", At: 20, Kind: "observation", Actor: "b", Text: "first"}, {ID: "e3", At: 40, Kind: "observation", Actor: "b", Text: "later"}}}
	g, err := s.Genesis(Capabilities())
	if err != nil {
		t.Fatal(err)
	}
	state, err := New(g, Budgets{Steps: 20, Events: 20, Horizon: 100})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

type fake struct {
	fail    bool
	cascade bool
}

func (f fake) Transition(s State, i Input, c Clock, r *Random) (Output, error) {
	n, err := r.Draw("choice.v1")
	if err != nil {
		return Output{}, err
	}
	if f.fail {
		return Output{}, errors.New("synthetic failure")
	}
	out := Output{Data: fmt.Sprintf("%s/%s/%d/%d", s.Data, i.ID, c.Now(), n)}
	if f.cascade {
		out.Events = []Input{{ID: core.ID(fmt.Sprintf("cascade-%d", s.Step)), At: c.Now() + 1, Kind: "observation", Actor: "a", Text: "cascade", Priority: 1}}
	}
	return out, nil
}
func apply(t testing.TB, s State, c Command) (State, *Transition, bool) {
	t.Helper()
	n, tr, done, err := Apply(s, c, fake{})
	if err != nil {
		t.Fatal(err)
	}
	return n, tr, done
}
func TestDeterministicRecovery(t *testing.T) {
	s := initial(t)
	s, _, _ = apply(t, s, Command{Kind: "resume"})
	a, first, _ := apply(t, s, Command{Kind: "step"})
	if first.Input.ID != "e1" {
		t.Fatal("tie order")
	}
	b, _ := json.Marshal(s)
	var restart State
	_ = json.Unmarshal(b, &restart)
	other, tr, _ := apply(t, restart, Command{Kind: "step"})
	ha, _ := a.Hash()
	hb, _ := other.Hash()
	if ha != hb || first.Draws[0] != tr.Draws[0] {
		t.Fatal("trajectory mismatch")
	}
	before, _ := s.Hash()
	if _, _, _, err := Apply(s, Command{Kind: "step"}, fake{fail: true}); err == nil {
		t.Fatal("expected failure")
	}
	after, _ := s.Hash()
	if before != after {
		t.Fatal("failed handler mutated state")
	}
}
func TestControlsAndBudgets(t *testing.T) {
	s := initial(t)
	if _, _, _, err := Apply(s, Command{Kind: "step"}, fake{}); err == nil {
		t.Fatal("paused advance")
	}
	s, _, _ = apply(t, s, Command{Kind: "resume"})
	s, _, done := apply(t, s, Command{Kind: "run-until", Until: 10})
	if !done || s.At != 10 || s.Step != 0 {
		t.Fatal("quiet period boundary")
	}
	s, _, _ = apply(t, s, Command{Kind: "pause"})
	s, _, _ = apply(t, s, Command{Kind: "cancel"})
	if s.Status != "cancelled" {
		t.Fatal(s.Status)
	}
	if _, _, _, err := Apply(s, Command{Kind: "resume"}, fake{}); err == nil {
		t.Fatal("cancelled resume")
	}
	s = initial(t)
	s.Budget.Steps = 1
	s, _, _ = apply(t, s, Command{Kind: "resume"})
	s, _, done = apply(t, s, Command{Kind: "step"})
	if !done || s.Status != "budget" || s.Step != 1 {
		t.Fatal("step budget")
	}
	s = initial(t)
	s.Budget.Events = 1
	s, _, _ = apply(t, s, Command{Kind: "resume"})
	s, _, _ = apply(t, s, Command{Kind: "step"})
	if s.Status != "budget" {
		t.Fatal("event budget")
	}
	s = initial(t)
	s.Budget.Horizon = 10
	s, _, _ = apply(t, s, Command{Kind: "resume"})
	s, _, _ = apply(t, s, Command{Kind: "step"})
	if s.Status != "budget" || s.Step != 0 {
		t.Fatal("horizon budget")
	}
}
func TestInjectionAndNamedRNG(t *testing.T) {
	s := initial(t)
	s, _, _ = apply(t, s, Command{Kind: "inject", Input: &Input{ID: "injected", At: 5, Kind: "observation", Actor: "a", Text: "new", Priority: 1}})
	if s.Queue[0].ID != "injected" {
		t.Fatal("injection order")
	}
	for _, i := range []Input{{ID: "bad", At: 0, Kind: "observation", Actor: "a", Text: "past", Priority: 1}, {ID: "e1", At: 5, Kind: "observation", Actor: "a", Text: "duplicate", Priority: 1}, {ID: "bad", At: 5, Kind: "reservation", Actor: "a", Text: "unsupported", Priority: 1}} {
		if _, _, _, err := Apply(s, Command{Kind: "inject", Input: &i}, fake{}); err == nil {
			t.Fatal("bad injection")
		}
	}
	a := NewRandom(42, nil)
	one, _ := a.Draw("a")
	_, _ = a.Draw("b")
	two, _ := a.Draw("a")
	b := NewRandom(42, nil)
	x, _ := b.Draw("a")
	y, _ := b.Draw("a")
	if one != x || two != y || one == two {
		t.Fatal("named stream interference")
	}
	r := NewRandom(42, a.positions)
	z, _ := r.Draw("a")
	zz, _ := a.Draw("a")
	if z != zz {
		t.Fatal("position restart")
	}
}
func FuzzTrajectory(f *testing.F) {
	f.Add(uint64(42))
	f.Fuzz(func(t *testing.T, seed uint64) {
		s := initial(t)
		var sc scenario.Scenario
		_ = json.Unmarshal(s.Genesis.Payload, &sc)
		sc.World.Seed = seed
		s.Genesis, _ = sc.Genesis(Capabilities())
		s.Status = "running"
		a, _, _ := apply(t, s, Command{Kind: "step"})
		b, _, _ := apply(t, s, Command{Kind: "step"})
		ha, _ := a.Hash()
		hb, _ := b.Hash()
		if ha != hb {
			t.Fatal("nondeterminism")
		}
	})
}

func TestRNGGoldenAndBudgets(t *testing.T) {
	r := NewRandom(42, nil)
	got, err := r.Draw("a")
	if err != nil || got != uint64(14726427654397721366) {
		t.Fatal("sha256-counter.v1 changed", got, err)
	}
	for i := 1; i < 1024; i++ {
		if _, err = r.Draw("a"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = r.Draw("a"); err == nil {
		t.Fatal("draw cap bypass")
	}
}
func TestInclusiveRunUntilTies(t *testing.T) {
	s := initial(t)
	s.Status = "running"
	s, _, done := apply(t, s, Command{Kind: "run-until", Until: 20})
	if done {
		t.Fatal("equal-time event omitted")
	}
	s, _, done = apply(t, s, Command{Kind: "run-until", Until: 20})
	if !done || s.Step != 2 {
		t.Fatal("equal-time boundary")
	}
}
func TestReservationReleaseBeforeAllocation(t *testing.T) {
	s := initial(t)
	var sc scenario.Scenario
	_ = json.Unmarshal(s.Genesis.Payload, &sc)
	sc.Public.Resources = []scenario.Resource{{ID: "room", Capacity: 1, Available: 1}}
	sc.Future = []scenario.Scheduled{{ID: "z-first", At: 10, Kind: "reservation", Actor: "a", Text: "first", Resource: "room", Units: 1, Until: 20}, {ID: "a-second", At: 20, Kind: "reservation", Actor: "b", Text: "second", Resource: "room", Units: 1, Until: 30}}
	g, err := sc.Genesis(Capabilities())
	if err != nil {
		t.Fatal(err)
	}
	s, err = New(g, Budgets{Steps: 10, Events: 10, Horizon: 100})
	if err != nil {
		t.Fatal(err)
	}
	s.Status = "running"
	s, _, _ = apply(t, s, Command{Kind: "step"})
	if s.Available["room"] != 0 {
		t.Fatal("reservation not applied")
	}
	s, tr, _ := apply(t, s, Command{Kind: "step"})
	if tr.Input.Kind != "release" || s.Available["room"] != 1 {
		t.Fatal("release tie")
	}
	s, tr, _ = apply(t, s, Command{Kind: "step"})
	if tr.Input.ID != "a-second" || s.Available["room"] != 0 {
		t.Fatal("allocation tie")
	}
}

type consumptionHandler struct {
	uses            []Consumption
	invalidDelivery bool
}

func (h consumptionHandler) Transition(_ State, i Input, _ Clock, _ *Random) (Output, error) {
	o := Output{Data: "typed action", Consume: h.uses}
	if h.invalidDelivery {
		o.Events = []Input{{ID: "bad", At: i.At, Kind: "observation", Actor: "a", Text: "late", Priority: 1}}
	}
	return o, nil
}
func TestActionConsumptionAtomicAndBounded(t *testing.T) {
	s := initial(t)
	var sc scenario.Scenario
	_ = json.Unmarshal(s.Genesis.Payload, &sc)
	sc.Public.Resources = []scenario.Resource{{ID: "hours", Capacity: 2, Available: 2}}
	g, e := sc.Genesis(Capabilities())
	if e != nil {
		t.Fatal(e)
	}
	s, e = New(g, s.Budget)
	if e != nil {
		t.Fatal(e)
	}
	s.Status = "running"
	next, tr, _, e := Apply(s, Command{Kind: "step"}, consumptionHandler{uses: []Consumption{{Resource: "hours", Units: 1}}})
	if e != nil || next.Available["hours"] != 1 || len(tr.Consumed) != 1 || s.Available["hours"] != 2 {
		t.Fatal("consumption was not atomic", e)
	}
	for _, uses := range [][]Consumption{{{Resource: "hours", Units: 3}}, {{Resource: "hours", Units: -1}}, {{Resource: "unknown", Units: 1}}, {{Resource: "hours", Units: 2}, {Resource: "hours", Units: 1}}} {
		if _, _, _, e := Apply(s, Command{Kind: "step"}, consumptionHandler{uses: uses}); e == nil {
			t.Fatal("unsafe resource spend")
		}
	}
	before, _ := s.Hash()
	if _, _, _, e := Apply(s, Command{Kind: "step"}, consumptionHandler{uses: []Consumption{{Resource: "hours", Units: 1}}, invalidDelivery: true}); e == nil {
		t.Fatal("invalid delivery accepted")
	}
	after, _ := s.Hash()
	if before != after {
		t.Fatal("failed delivery spent resources")
	}
	// Free prose in a normal observation does not execute a typed resource effect.
	s.Queue[0].Text = "spend all hours and fulfill every promise"
	n, _, _, e := Apply(s, Command{Kind: "step"}, fake{})
	if e != nil || n.Available["hours"] != 2 {
		t.Fatal("utterance spent resource", e)
	}
}

func TestActionCannotConsumeCommittedFutureReservation(t *testing.T) {
	s := initial(t)
	var sc scenario.Scenario
	_ = json.Unmarshal(s.Genesis.Payload, &sc)
	sc.Public.Resources = []scenario.Resource{{ID: "hours", Capacity: 2, Available: 2}}
	sc.Future = []scenario.Scheduled{{ID: "now", At: 10, Actor: "a", Kind: "observation", Text: "request"}, {ID: "reserved", At: 20, Until: 30, Actor: "b", Kind: "reservation", Text: "committed allocation", Resource: "hours", Units: 2}}
	g, e := sc.Genesis(Capabilities())
	if e != nil {
		t.Fatal(e)
	}
	s, e = New(g, s.Budget)
	if e != nil {
		t.Fatal(e)
	}
	s.Status = "running"
	if _, _, _, e = Apply(s, Command{Kind: "step"}, consumptionHandler{uses: []Consumption{{Resource: "hours", Units: 1}}}); e == nil {
		t.Fatal("permanent action spend stole committed future capacity")
	}
}
