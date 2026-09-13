package hws

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/dynamics"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"github.com/tushardhara/dream/simulator/scenario"
)

type cognitiveFunc func(rt.Input) (CognitiveFrame, error)

func (f cognitiveFunc) Frame(i rt.Input) (CognitiveFrame, error) { return f(i) }
func cognitiveFrame(i rt.Input) CognitiveFrame {
	p, _ := (syntheticPerception{}).Perceive(i)
	return CognitiveFrame{ModelHash: strings.Repeat("a", 64), Situation: behavior.Situation{Perceived: p, Horizon: 90, Beliefs: []behavior.Belief{{Source: i.ID, Observer: i.Actor, Code: "uncertain", Value: -.4, Confidence: .3}}}}
}
func cognitiveWorld(t testing.TB) rt.State {
	t.Helper()
	s := syntheticRuntime(t)
	var sc scenario.Scenario
	_ = json.Unmarshal(s.Genesis.Payload, &sc)
	sc.Public.Humans = append(sc.Public.Humans, scenario.Human{ID: "c", Name: "Synthetic C", Age: 25}, scenario.Human{ID: "d", Name: "Synthetic D", Age: 45})
	sc.Actors = append(sc.Actors, scenario.Actor{ID: "c"}, scenario.Actor{ID: "d"})
	sc.Public.Resources = []scenario.Resource{{ID: "hours", Capacity: 2, Available: 2}}
	g, e := sc.Genesis(rt.Capabilities())
	if e != nil {
		t.Fatal(e)
	}
	s, e = rt.New(g, rt.Budgets{Steps: 16, Events: 16, Horizon: 100})
	if e != nil {
		t.Fatal(e)
	}
	s.Status = "running"
	return s
}
func TestCognitiveFourActorsReplayAndPrivatePublicBoundary(t *testing.T) {
	s := cognitiveWorld(t)
	handler := CognitiveHandler{Source: cognitiveFunc(func(i rt.Input) (CognitiveFrame, error) {
		f := cognitiveFrame(i)
		other := core.ID("b")
		if i.Actor == "b" {
			other = "a"
		}
		f.Situation.Offers = []behavior.Offer{{Kind: behavior.Ask, Recipient: other, Duration: 1}, {Kind: behavior.Help, Recipient: other, Duration: 2, Resource: "hours", Units: 1}, {Kind: behavior.SelfDisclose, Recipient: other, Duration: 1}}
		return f, nil
	})}
	initial, _ := s.Hash()
	next, tr, _, e := rt.Apply(s, rt.Command{Kind: "step"}, handler)
	if e != nil {
		t.Fatal(e)
	}
	after, _ := s.Hash()
	if initial != after {
		t.Fatal("mutated input")
	}
	c, e := DecodeCognitiveCheckpoint(next.Data)
	if e != nil {
		t.Fatal(e)
	}
	if len(c.Actors) != 4 || len(c.Outcomes) != 1 || len(tr.Draws) != 1 || c.Last.Draw != tr.Draws[0].Value || c.Last.AppraisalKey != dynamics.Key("a", "e1") {
		t.Fatal("missing cognitive stage record")
	}
	if len(next.Data) > 4096 {
		t.Fatal("state limit weakened")
	}
	for _, event := range tr.Generated {
		if strings.Contains(event.Text, "CANARY") || strings.Contains(event.Text, "belief") || strings.Contains(event.Text, "probability") || strings.Contains(event.Text, "self_disclose") {
			t.Fatal("private cognition delivered", event.Text)
		}
		var delivered deliveredBehavior
		if json.Unmarshal([]byte(event.Text), &delivered) != nil || delivered.Decision != c.Last.ID || event.Actor != delivered.Recipient || delivered.Kind != c.Last.Candidates[c.Last.Selected].Offer.Kind {
			t.Fatal("unselected behavior delivered")
		}
	}
	encoded, _ := json.Marshal(next)
	var restart rt.State
	if json.Unmarshal(encoded, &restart) != nil {
		t.Fatal("restart")
	}
	for i := 0; i < 4 && next.Status == "running"; i++ {
		a, _, _, e := rt.Apply(next, rt.Command{Kind: "step"}, handler)
		if e != nil {
			t.Fatal(e)
		}
		b, _, _, e := rt.Apply(restart, rt.Command{Kind: "step"}, handler)
		if e != nil {
			t.Fatal(e)
		}
		ha, _ := a.Hash()
		hb, _ := b.Hash()
		if ha != hb {
			t.Fatal("recorded replay diverged")
		}
		next = a
		restart = b
	}
	if strings.Contains(fmt.Sprint(c.Last), "CANARY") {
		t.Fatal("raw rationale leak")
	}
}
func TestCognitiveFailClosedAndOperationalWait(t *testing.T) {
	for _, name := range []string{"missing_record", "foreign", "unknown_action", "no_disclosure_output", "outcome_budget", "invalid_codec"} {
		t.Run(name, func(t *testing.T) {
			s := cognitiveWorld(t)
			f := cognitiveFrame(s.Queue[0])
			switch name {
			case "missing_record":
				f.ModelHash = ""
			case "foreign":
				f.Situation.Perceived.Actor = "b"
			case "unknown_action":
				f.Situation.Offers = []behavior.Offer{{Kind: "teleport", Recipient: "b", Duration: 1}}
			case "no_disclosure_output":
				f.Situation.DisclosureRecipient = "b"
			case "invalid_codec":
				s.Data = "cognitive.v2:bad"
			case "outcome_budget":
				n, _, _, e := rt.Apply(s, rt.Command{Kind: "step"}, CognitiveHandler{Source: cognitiveFunc(func(i rt.Input) (CognitiveFrame, error) { return cognitiveFrame(i), nil })})
				if e != nil {
					t.Fatal(e)
				}
				c, e := DecodeCognitiveCheckpoint(n.Data)
				if e != nil {
					t.Fatal(e)
				}
				for len(c.Outcomes) < behavior.MaxOutcomes {
					c.Outcomes = append(c.Outcomes, behavior.Outcome{Decision: core.ID(fmt.Sprintf("old-%d", len(c.Outcomes))), Observer: "a", At: 1, Horizon: 90, Status: core.Unknown})
				}
				n.Data, e = c.Encode()
				if e != nil {
					t.Fatal(e)
				}
				s = n
				f = cognitiveFrame(s.Queue[0])
			}
			if _, _, _, e := rt.Apply(s, rt.Command{Kind: "step"}, CognitiveHandler{Source: cognitiveFunc(func(rt.Input) (CognitiveFrame, error) { return f, nil })}); e == nil {
				t.Fatal("unsafe transition")
			}
		})
	}
	s := cognitiveWorld(t)
	f := cognitiveFrame(s.Queue[0])
	f.Situation.Outage = true
	f.Situation.Offers = []behavior.Offer{{Kind: behavior.Ask, Recipient: "b", Duration: 1}}
	n, tr, _, e := rt.Apply(s, rt.Command{Kind: "step"}, CognitiveHandler{Source: cognitiveFunc(func(rt.Input) (CognitiveFrame, error) { return f, nil })})
	if e != nil {
		t.Fatal(e)
	}
	c, e := DecodeCognitiveCheckpoint(n.Data)
	if e != nil || !c.Last.Operational || !c.Outcomes[0].Operational || len(tr.Generated) != 0 || c.Last.Candidates[c.Last.Selected].Offer.Kind != behavior.Wait {
		t.Fatal("operational WAIT became behavior", e)
	}
}
func TestCognitiveCodecAndDuplicateAppraisal(t *testing.T) {
	s := cognitiveWorld(t)
	h := CognitiveHandler{Source: cognitiveFunc(func(i rt.Input) (CognitiveFrame, error) { return cognitiveFrame(i), nil })}
	n, _, _, e := rt.Apply(s, rt.Command{Kind: "step"}, h)
	if e != nil {
		t.Fatal(e)
	}
	c, e := DecodeCognitiveCheckpoint(n.Data)
	if e != nil {
		t.Fatal(e)
	}
	for _, raw := range []string{n.Data + "A", strings.Repeat("a", 4097), "cognitive.v1:bad"} {
		if _, e := DecodeCognitiveCheckpoint(raw); e == nil {
			t.Fatal("noncanonical wire")
		}
	}
	// Reapplying the same event via handler is rejected even if a host bypasses
	// runtime queue deduplication; appraisal receipts are never reset by this codec.
	if _, e := h.Transition(n, s.Queue[0], rt.Clock{At: s.Queue[0].At}, rt.NewRandom(42, nil)); e == nil {
		t.Fatal("duplicate event selected twice")
	}
	c.Last.Draw = 0
	if _, e = c.Encode(); e == nil && c.Last.Selected != 0 {
		t.Fatal("forged draw")
	}
}

func TestCognitiveDelayedResponseUpdatesOwnState(t *testing.T) {
	s := cognitiveWorld(t)
	var first core.ID
	source := cognitiveFunc(func(i rt.Input) (CognitiveFrame, error) {
		f := cognitiveFrame(i)
		if i.ID == "e1" {
			f.Situation.Offers = []behavior.Offer{{Kind: behavior.Ask, Recipient: "b", Duration: 1}}
		}
		if i.ID == "late-response" {
			f.Response = &CognitiveResponse{Decision: first, Other: "b", Code: "dismissive"}
		}
		return f, nil
	})
	h := CognitiveHandler{Source: source}
	next, _, _, e := rt.Apply(s, rt.Command{Kind: "step"}, h)
	if e != nil {
		t.Fatal(e)
	}
	c, e := DecodeCognitiveCheckpoint(next.Data)
	if e != nil {
		t.Fatal(e)
	}
	first = c.Last.ID
	if c.Last.Candidates[c.Last.Selected].Offer.Kind != behavior.Ask {
		t.Fatal("fixture seed no longer samples asking; review policy/fixture version")
	}
	next, _, _, e = rt.Apply(next, rt.Command{Kind: "inject", Input: &rt.Input{ID: "late-response", At: 30, Kind: "observation", Actor: "a", Text: "Synthetic later response", Priority: 1}}, h)
	if e != nil {
		t.Fatal(e)
	}
	for next.At < 30 {
		next, _, _, e = rt.Apply(next, rt.Command{Kind: "step"}, h)
		if e != nil {
			t.Fatal(e)
		}
	}
	c, e = DecodeCognitiveCheckpoint(next.Data)
	if e != nil {
		t.Fatal(e)
	}
	if c.Outcomes[0].Status != core.Observed || c.Outcomes[0].Evidence != "late-response" || c.Outcomes[0].LearnedAt != 30 || c.Actors[0].Memory[0].Trust >= 0 || len(c.Actors[1].Memory) != 0 {
		t.Fatal("delayed update leaked or vanished")
	}
	// The response was appraised once and learned once by its observer; neither
	// stage repeats the original event's appraisal or modifies another actor.
	count := 0
	for _, r := range c.Actors[0].State.Applied {
		if r.Event == "e1" {
			count++
		}
	}
	if count != 1 || len(c.Actors[0].State.Applied) != 2 {
		t.Fatal("double appraisal")
	}
}

func TestCognitiveActorViewDoesNotExposePrivatePipeline(t *testing.T) {
	runtime, journal, realm, grants := viewFixture(t)
	c := CognitiveCheckpoint{Version: 1, Policy: behavior.Policy, Actors: []behavior.Actor{}, Outcomes: []behavior.Outcome{}, Commitments: []behavior.Commitment{}}
	for _, id := range []core.ID{"alice", "bob"} {
		state, e := dynamics.New(id, 0, dynamics.DefaultSubstrate())
		if e != nil {
			t.Fatal(e)
		}
		c.Actors = append(c.Actors, behavior.Actor{State: state, Memory: []behavior.Memory{}, Beliefs: []behavior.Belief{{Source: core.ID("PRIVATE_" + string(id)), Observer: id, Code: "uncertain", Value: .3, Confidence: .5}}})
	}
	var e error
	runtime.snapshot.State.Data, e = c.Encode()
	if e != nil {
		t.Fatal(e)
	}
	service, e := NewViewService(runtime, journal, viewClock{}, grants, viewAudit{})
	if e != nil {
		t.Fatal(e)
	}
	permit, e := service.Permit("alice", realm, ActorViewKind, "simulation")
	if e != nil {
		t.Fatal(e)
	}
	observation, self, e := service.Actor(context.Background(), permit)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal([]any{observation, self})
	if strings.Contains(string(raw), "PRIVATE_bob") || strings.Contains(string(raw), "beliefs") || len(self.CurrentDrives) != dynamics.Count {
		t.Fatal("private cognition exposed")
	}
}

func TestCognitiveExpansionBombAndCanonicalEnvelope(t *testing.T) {
	wire := func(raw string) string {
		var b bytes.Buffer
		z := zlib.NewWriter(&b)
		_, _ = z.Write([]byte(raw))
		_ = z.Close()
		return cognitivePrefix + base64.RawStdEncoding.EncodeToString(b.Bytes())
	}
	for _, raw := range []string{strings.Repeat("x", 65537), `{"version":1,"unknown":true}`, `{} {}`} {
		if _, e := DecodeCognitiveCheckpoint(wire(raw)); e == nil {
			t.Fatal("bad expanded checkpoint accepted")
		}
	}
}
