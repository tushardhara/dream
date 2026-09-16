package hws

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/drives"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"github.com/tushardhara/dream/simulator/scenario"
)

func actionFrame(i rt.Input) CognitiveFrame {
	f := cognitiveFrame(i)
	f.Actions = &ActionFrame{Sources: []core.ID{i.ID}}
	return f
}
func selectingRandom(t testing.TB, actor core.ID) *rt.Random {
	t.Helper()
	for seed := uint64(0); seed < 10000; seed++ {
		r := rt.NewRandom(seed, nil)
		draw, e := r.Draw(core.ID("human-choice:" + string(actor)))
		if e != nil {
			t.Fatal(e)
		}
		if float64(draw>>11)/float64(uint64(1)<<53) > .99 {
			return rt.NewRandom(seed, nil)
		}
	}
	t.Fatal("no deterministic upper-tail fixture")
	return nil
}
func initialActions(t testing.TB, s rt.State) ActionCheckpoint {
	t.Helper()
	var sc scenario.Scenario
	if json.Unmarshal(s.Genesis.Payload, &sc) != nil {
		t.Fatal("genesis")
	}
	c := ActionCheckpoint{Version: 2, Policy: behavior.ActionPolicy, RegistryHash: behavior.ActionRegistryHash(), Actors: []behavior.ActionActor{}, Outcomes: []behavior.ActionOutcome{}, Commitments: []behavior.Commitment{}}
	for _, h := range sc.Public.Humans {
		a, e := behavior.NewActionActor(h.ID, 0)
		if e != nil {
			t.Fatal(e)
		}
		c.Actors = append(c.Actors, a)
	}
	return c
}
func TestAll27ActionsExecuteBoundedEffects(t *testing.T) {
	for _, definition := range behavior.ActionRegistry() {
		t.Run(string(definition.Kind), func(t *testing.T) {
			s := cognitiveWorld(t)
			i := s.Queue[0]
			f := actionFrame(i)
			o := behavior.ActionOffer{Kind: definition.Kind, Duration: 2, Mode: definition.Disclosure}
			if definition.Recipient != "none" {
				o.Recipient = "b"
			}
			if definition.Evidence {
				o.Evidence = []core.ID{i.ID}
			}
			if o.Kind == behavior.Help || o.Kind == behavior.Promise {
				o.Resource = "hours"
				o.Units = 1
			}
			if o.Kind == behavior.Promise {
				o.Commitment = "promise"
				o.Due = 30
			}
			if o.Mode != "" && o.Mode != behavior.Silence {
				f.Actions.Disclosure = &behavior.DisclosureGrant{Recipient: "b", Mode: o.Mode, Sources: []core.ID{i.ID}}
				f.Disclosure = "TYPED_OWN_FICTION"
				v := behavior.ContextValue{Value: .8, Confidence: 1, Evidence: i.ID}
				f.Actions.Contexts = []behavior.DisclosureContext{{Observer: "a", Recipient: "b", Trust: v, ExpectedReaction: v}}
			}
			if o.Kind == behavior.Reconnect || o.Kind == behavior.BreakPromise {
				c := initialActions(t, s)
				if o.Kind == behavior.Reconnect {
					c.Actors[0].Contact = "left"
				} else {
					o.Commitment = "promise"
					c.Commitments = []behavior.Commitment{{ID: "promise", Actor: "a", Recipient: "b", Resource: "hours", Units: 1, Due: 30, Status: "pending"}}
				}
				var e error
				s.Data, e = c.Encode()
				if e != nil {
					t.Fatal(e)
				}
			}
			f.Actions.Offers = []behavior.ActionOffer{o}
			h := CognitiveHandler{Policy: behavior.ActionPolicy, Source: cognitiveFunc(func(rt.Input) (CognitiveFrame, error) { return f, nil })}
			before, _ := s.Hash()
			out, e := h.Transition(s, i, rt.Clock{At: i.At}, selectingRandom(t, i.Actor))
			if e != nil {
				t.Fatal(e)
			}
			after, _ := s.Hash()
			if before != after {
				t.Fatal("executor mutated source")
			}
			c, e := DecodeActionCheckpoint(out.Data)
			if e != nil {
				t.Fatal(e)
			}
			if c.Last.Candidates[c.Last.Selected].Offer.Kind != o.Kind || c.Last.Stages[11] != "done" || c.Outcomes[0].Outcome.Status != core.Unknown {
				t.Fatal("effect not executed/unknown response fabricated")
			}
			if o.Delivered() {
				if len(out.Events) != 1 || out.Events[0].At != i.At+o.Duration || out.Events[0].Actor != o.Recipient {
					t.Fatal("delivery contract")
				}
				var delivery deliveredAction
				if json.Unmarshal([]byte(out.Events[0].Text), &delivery) != nil {
					t.Fatal("delivery codec")
				}
				want := o.Kind
				if want == behavior.Lie {
					want = behavior.Say
				}
				if delivery.Kind != want {
					t.Fatal("wrong observed speech act")
				}
				if strings.Contains(out.Events[0].Text, "subjective_consequence") || strings.Contains(out.Events[0].Text, "appraisal") || strings.Contains(out.Events[0].Text, "\"kind\":\"lie\"") {
					t.Fatal("private cognition leaked")
				}
			} else if len(out.Events) != 0 {
				t.Fatal("no-delivery contract")
			}
			switch o.Kind {
			case behavior.Wait:
				if c.Actors[0].AvailableAt != 0 || c.Actors[0].Contact != "engaged" || len(c.Commitments) != 0 {
					t.Fatal("WAIT had an availability/contact/commitment effect")
				}
			case behavior.Help:
				if len(out.Consume) != 1 || out.Consume[0].Resource != "hours" || out.Consume[0].Units != 1 {
					t.Fatal("help did not consume")
				}
			case behavior.Promise:
				if len(c.Commitments) != 1 || c.Commitments[0].Status != "pending" || c.Commitments[0].Due != 30 {
					t.Fatal("promise not created")
				}
			case behavior.BreakPromise:
				if c.Commitments[0].Status != "broken" {
					t.Fatal("breach missing")
				}
			case behavior.Leave:
				if c.Actors[0].Contact != "left" {
					t.Fatal("leave missing")
				}
			case behavior.Withdraw:
				if c.Actors[0].Contact != "withdrawn" {
					t.Fatal("withdraw missing")
				}
			case behavior.Reconnect:
				if c.Actors[0].Contact != "engaged" {
					t.Fatal("reconnect missing")
				}
			}
			if o.Kind != behavior.Help && len(out.Consume) != 0 {
				t.Fatal("speech consumed resources")
			}
		})
	}
}
func TestActionPromiseActualFulfillmentAndBreach(t *testing.T) {
	for _, tc := range []struct {
		name, status      string
		learned, occurred core.LogicalTime
		reject            bool
	}{
		{"fulfilled", "fulfilled", 15, 15, false},
		{"broken", "broken", 15, 15, false},
		{"late_fulfilled", "fulfilled", 45, 45, true},
		{"learned_after_due", "fulfilled", 45, 30, false},
		{"late_broken", "broken", 45, 45, false},
	} {
		status := tc.status
		t.Run(tc.name, func(t *testing.T) {
			s := cognitiveWorld(t)
			input := s.Queue[0]
			step := func(i rt.Input, f CognitiveFrame) ActionCheckpoint {
				t.Helper()
				h := CognitiveHandler{Policy: behavior.ActionPolicy, Source: cognitiveFunc(func(rt.Input) (CognitiveFrame, error) { return f, nil })}
				out, e := h.Transition(s, i, rt.Clock{At: i.At}, selectingRandom(t, i.Actor))
				if e != nil {
					t.Fatal(e)
				}
				s.Data = out.Data
				c, e := DecodeActionCheckpoint(out.Data)
				if e != nil {
					t.Fatal(e)
				}
				return c
			}
			f := actionFrame(input)
			f.Actions.Offers = []behavior.ActionOffer{{Kind: behavior.Promise, Recipient: "b", Duration: 1, Evidence: []core.ID{input.ID}, Resource: "hours", Units: 1, Commitment: "promise", Due: 30}}
			c := step(input, f)
			if c.Commitments[0].Status != "pending" {
				t.Fatal("promise prematurely fulfilled")
			}
			input.ID = "help-event"
			input.At = 12
			f = actionFrame(input)
			f.Actions.Offers = []behavior.ActionOffer{{Kind: behavior.Help, Recipient: "b", Duration: 1, Evidence: []core.ID{input.ID}, Resource: "hours", Units: 1, Commitment: "promise"}}
			c = step(input, f)
			decision := c.Last.ID
			if c.Commitments[0].Status != "pending" {
				t.Fatal("resource use fabricated fulfillment")
			}
			input.ID = "later-response"
			input.At = tc.learned
			f = actionFrame(input)
			f.Situation.Perceived.OccurredAt = tc.occurred
			f.Response = &CognitiveResponse{Decision: decision, Other: "b", Code: "supportive", CommitmentStatus: status}
			if tc.reject {
				h := CognitiveHandler{Policy: behavior.ActionPolicy, Source: cognitiveFunc(func(rt.Input) (CognitiveFrame, error) { return f, nil })}
				before, _ := s.Hash()
				_, err := h.Transition(s, input, rt.Clock{At: input.At}, selectingRandom(t, input.Actor))
				after, _ := s.Hash()
				if err == nil || !strings.Contains(err.Error(), "late fulfillment is not established") || before != after {
					t.Fatal("post-deadline fulfillment was not atomically rejected for its occurrence time", err)
				}
				return
			}
			c = step(input, f)
			if c.Commitments[0].Status != status || c.Outcomes[1].ObservationStage != "done" || c.Outcomes[1].LearningStage != "done" || len(c.Actors[0].Memory) != 1 {
				t.Fatal("actual later result not recorded")
			}
		})
	}
}
func TestActionRuntimeReplaySizeAndVersionDispatch(t *testing.T) {
	s := cognitiveWorld(t)
	h := CognitiveHandler{Policy: behavior.ActionPolicy, Source: cognitiveFunc(func(i rt.Input) (CognitiveFrame, error) { f := actionFrame(i); return f, nil })}
	n, _, _, e := rt.Apply(s, rt.Command{Kind: "step"}, h)
	if e != nil {
		t.Fatal(e)
	}
	c, e := DecodeActionCheckpoint(n.Data)
	if e != nil || len(c.Actors) != 4 || len(c.Actors[0].Drives.Variables) != 22 || len(n.Data) > 4096 {
		t.Fatal("new core/codec not used", e)
	}
	raw, _ := json.Marshal(n)
	var restart rt.State
	if json.Unmarshal(raw, &restart) != nil {
		t.Fatal("restart")
	}
	x, _, _, e := rt.Apply(n, rt.Command{Kind: "step"}, h)
	if e != nil {
		t.Fatal(e)
	}
	y, _, _, e := rt.Apply(restart, rt.Command{Kind: "step"}, h)
	if e != nil {
		t.Fatal(e)
	}
	hx, _ := x.Hash()
	hy, _ := y.Hash()
	if hx != hy {
		t.Fatal("v2 recorded restart diverged")
	}
	if _, _, _, e = rt.Apply(n, rt.Command{Kind: "step"}, CognitiveHandler{Source: h.Source}); e == nil {
		t.Fatal("v2 state silently ran v1")
	}
	old, _, _, e := rt.Apply(s, rt.Command{Kind: "step"}, CognitiveHandler{Source: cognitiveFunc(func(i rt.Input) (CognitiveFrame, error) { return cognitiveFrame(i), nil })})
	if e != nil {
		t.Fatal(e)
	}
	if _, _, _, e = rt.Apply(old, rt.Command{Kind: "step"}, h); e == nil {
		t.Fatal("v1 state silently upgraded")
	}
	for _, wire := range []string{strings.Replace(n.Data, "cognitive.v2:", "cognitive.v3:", 1), n.Data + "garbage"} {
		if _, e = DecodeActionCheckpoint(wire); e == nil {
			t.Fatal("unknown/malformed wire accepted")
		}
	}
	for _, change := range []func(*ActionCheckpoint){func(c *ActionCheckpoint) { c.Version = 1 }, func(c *ActionCheckpoint) { c.Policy = behavior.Policy }, func(c *ActionCheckpoint) { c.RegistryHash = "wrong" }, func(c *ActionCheckpoint) { c.Last.Stages[9] = "" }} {
		copy := c
		last := *c.Last
		copy.Last = &last
		change(&copy)
		if _, e = copy.Encode(); e == nil {
			t.Fatal("malformed checkpoint accepted")
		}
	}
}
func TestActionStateAndPermissionSensitivity(t *testing.T) {
	a, _ := behavior.NewActionActor("a", 0)
	i := rt.Input{ID: "e", Actor: "a", At: 10}
	f := actionFrame(i)
	s := behavior.ActionSituation{Observation: drives.Observation{Event: f.Situation.Perceived}, Sources: []core.ID{"e"}, Present: []core.ID{"a", "b"}, Resources: map[core.ID]int64{"hours": 2}, Horizon: 90, Offers: []behavior.ActionOffer{{Kind: behavior.Help, Recipient: "b", Resource: "hours", Units: 1, Duration: 1, Evidence: []core.ID{"e"}}}}
	for j := range s.Observation.Context {
		s.Observation.Context[j] = drives.Cue{Evidence: s.Observation.Event}
	}
	_, base, e := behavior.ChooseAction(a, s, 10, math.MaxUint64)
	if e != nil {
		t.Fatal(e)
	}
	changed := a
	changed.Drives, e = drives.Intervene(a.Drives, drives.Care, .99, 0, "research-control")
	if e != nil {
		t.Fatal(e)
	}
	_, d, e := behavior.ChooseAction(changed, s, 10, math.MaxUint64)
	if e != nil || reflect.DeepEqual(base.Candidates, d.Candidates) {
		t.Fatal("new22-drive state is inert", e)
	}
	s.Observation.Context[0].Evidence.Rights.Revoked = true
	if _, _, e = behavior.ChooseAction(a, s, 10, 0); e == nil {
		t.Fatal("revoked context accepted")
	}
}

func TestActionForkPreservesPolicyAndBudgets(t *testing.T) {
	s := cognitiveWorld(t)
	h := CognitiveHandler{Policy: behavior.ActionPolicy, Source: cognitiveFunc(func(i rt.Input) (CognitiveFrame, error) { return actionFrame(i), nil })}
	n, _, _, e := rt.Apply(s, rt.Command{Kind: "step"}, h)
	if e != nil {
		t.Fatal(e)
	}
	frozen := frozenFixture(t, n)
	spec := ForkSpec{Version: 1, Source: SnapshotKey{Scope: frozen.Scope, ID: "snapshot", Hash: frozen.Hash}, Child: frozen.Scope, Mode: FreshSimulation, Policy: behavior.ActionPolicy, MaxDuration: time.Minute}
	spec.Child.Branch = "action-child"
	spec.Child.Run = "action-child"
	child, e := ForkState(frozen, spec)
	if e != nil {
		t.Fatal(e)
	}
	if child.Step != n.Step || child.Budget != n.Budget || child.At != n.At || child.Data != n.Data {
		t.Fatal("fork reset cognition/time/budget")
	}
	spec.Policy = behavior.Policy
	if _, e = ForkState(frozen, spec); e == nil {
		t.Fatal("v2 checkpoint forked as legacy")
	}
	bundle := appendReplay(t, ReplayBundle{Snapshot: frozen}, rt.Command{Kind: "step"}, h)
	for _, mode := range []ReplayMode{EventReplay, RecordedReplay} {
		r, e := Replay(bundle, mode)
		if e != nil || !r.Exact {
			t.Fatal("v2 replay", e)
		}
	}
}

func TestOptionalActionsWithoutRecipient(t *testing.T) {
	for _, kind := range []behavior.Kind{behavior.Withdraw, behavior.Delay, behavior.Leave, behavior.Ignore} {
		t.Run(string(kind), func(t *testing.T) {
			s := cognitiveWorld(t)
			i := s.Queue[0]
			f := actionFrame(i)
			f.Actions.Offers = []behavior.ActionOffer{{Kind: kind, Duration: 2, Evidence: []core.ID{i.ID}}}
			h := CognitiveHandler{Policy: behavior.ActionPolicy, Source: cognitiveFunc(func(rt.Input) (CognitiveFrame, error) { return f, nil })}
			out, e := h.Transition(s, i, rt.Clock{At: i.At}, selectingRandom(t, i.Actor))
			if e != nil {
				t.Fatal(e)
			}
			c, e := DecodeActionCheckpoint(out.Data)
			if e != nil || len(out.Events) != 0 || c.Outcomes[0].Outcome.Other != "" || c.Outcomes[0].Outcome.Status != core.Unknown {
				t.Fatal("no-recipient action invented delivery/response", e)
			}
			if c.Last.Candidates[c.Last.Selected].Offer.Kind != kind || c.Actors[0].AvailableAt != i.At+2 {
				t.Fatal("optional action was not executed")
			}
			for j, v := range c.Actors[0].Drives.Variables {
				if c.Actors[0].Private.Active[j] != (v.Values[0] > .5) {
					t.Fatal("drive activation not represented")
				}
			}
		})
	}
}

// A moded disclosure offer must never be delivered without policy-validated
// fictional output. Two different mechanisms enforce that, and #80 conflated
// them: it expected `Actions.Disclosure = nil` beside a moded offer to produce
// "missing policy-validated fictional output". It does not. Without a matching
// grant the offer is dropped before candidates are formed, so the only
// candidate is WAIT and the transition succeeds having delivered nothing.
//
// The named guard in action_cognitive.go is reached only when the grant is
// present and matches but the typed output is empty. Both paths fail closed;
// this pins each to the mechanism that actually enforces it, so neither can be
// removed while the other keeps the suite green.
func TestModedDisclosureIsNeverDeliveredWithoutValidatedOutput(t *testing.T) {
	grant := func(f *CognitiveFrame, source core.ID) {
		f.Actions.Disclosure = &behavior.DisclosureGrant{Recipient: "b", Mode: behavior.Full, Sources: []core.ID{source}}
		f.Disclosure = "TYPED_OWN_FICTION"
		v := behavior.ContextValue{Value: .8, Confidence: 1, Evidence: source}
		f.Actions.Contexts = []behavior.DisclosureContext{{Observer: "a", Recipient: "b", Trust: v, ExpectedReaction: v}}
	}
	run := func(t *testing.T, mutate func(*CognitiveFrame)) (rt.Output, error) {
		t.Helper()
		s := cognitiveWorld(t)
		i := s.Queue[0]
		f := actionFrame(i)
		grant(&f, i.ID)
		mutate(&f)
		f.Actions.Offers = []behavior.ActionOffer{{Kind: behavior.Reveal, Recipient: "b", Duration: 2, Mode: behavior.Full, Evidence: []core.ID{i.ID}}}
		h := CognitiveHandler{Policy: behavior.ActionPolicy, Source: cognitiveFunc(func(rt.Input) (CognitiveFrame, error) { return f, nil })}
		return h.Transition(s, i, rt.Clock{At: i.At}, selectingRandom(t, i.Actor))
	}

	// Mechanism one: no matching grant, so the moded offer never becomes a
	// candidate. The person waits rather than disclosing.
	for _, c := range []struct {
		name   string
		mutate func(*CognitiveFrame)
	}{
		{"absent_grant", func(f *CognitiveFrame) { f.Actions.Disclosure = nil }},
		{"grant_names_another_mode", func(f *CognitiveFrame) { f.Actions.Disclosure.Mode = behavior.Partial }},
		{"grant_names_another_recipient", func(f *CognitiveFrame) { f.Actions.Disclosure.Recipient = "c" }},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, e := run(t, c.mutate)
			if e != nil {
				t.Fatal("expected a WAIT, not a rejection", e)
			}
			cp, e := DecodeActionCheckpoint(out.Data)
			if e != nil {
				t.Fatal(e)
			}
			if got := cp.Last.Candidates[cp.Last.Selected].Offer; got.Kind != behavior.Wait {
				t.Fatal("moded offer survived without a matching grant:", got.Kind, got.Mode)
			}
			for _, cand := range cp.Last.Candidates {
				if cand.Offer.Mode != "" {
					t.Fatal("a moded candidate was offered without a matching grant:", cand.Offer.Kind, cand.Offer.Mode)
				}
			}
		})
	}

	// Mechanism two: the grant matches, so the offer is eligible, but the policy
	// produced no fictional output. This is the guard the ticket names, and the
	// only configuration that reaches it.
	t.Run("validated_grant_without_fictional_output", func(t *testing.T) {
		const want = "missing policy-validated fictional output"
		_, e := run(t, func(f *CognitiveFrame) { f.Disclosure = "" })
		if e == nil {
			t.Fatal("moded disclosure delivered with no fictional output")
		}
		if e.Error() != want {
			t.Fatal("wrong guard reached: want "+want+", got", e)
		}
	})

	// Positive control on the same construction: fully validated, it is delivered.
	t.Run("validated_output_is_delivered", func(t *testing.T) {
		out, e := run(t, func(*CognitiveFrame) {})
		if e != nil {
			t.Fatal("validated moded disclosure refused", e)
		}
		cp, e := DecodeActionCheckpoint(out.Data)
		if e != nil {
			t.Fatal(e)
		}
		if got := cp.Last.Candidates[cp.Last.Selected].Offer; got.Kind != behavior.Reveal || got.Mode != behavior.Full {
			t.Fatal("validated moded disclosure was not the selected action:", got.Kind, got.Mode)
		}
	})
}
