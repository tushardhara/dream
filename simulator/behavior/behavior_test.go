package behavior

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/dynamics"
)

func actor(t testing.TB) Actor {
	t.Helper()
	s, e := dynamics.New("a", 0, dynamics.DefaultSubstrate())
	if e != nil {
		t.Fatal(e)
	}
	return Actor{State: s, Memory: []Memory{}, Beliefs: []Belief{}}
}
func situation() Situation {
	p := dynamics.Perceived{Event: "e", Actor: "a", OccurredAt: 1, LearnedAt: 1, Confidence: .8, Signals: dynamics.Signals{OtherNeed: .7}, Rights: core.Rights{Resource: "e", Grants: []core.Grant{{Actor: "a", Recipient: "a", Purpose: "simulation", Operation: core.Read}, {Actor: "a", Recipient: "a", Purpose: "simulation", Operation: core.Derive}}}}
	return Situation{Perceived: p, Horizon: 20, Present: []core.ID{"a", "b", "c"}, Resources: map[core.ID]int64{"time": 2}, Offers: []Offer{{Kind: Ask, Recipient: "b", Duration: 1}, {Kind: Help, Recipient: "b", Duration: 2, Resource: "time", Units: 1}, {Kind: Invite, Recipient: "c", Duration: 1}, {Kind: Decline, Recipient: "b", Duration: 1}, {Kind: ThirdPartySupport, Recipient: "c", Duration: 1}, {Kind: SelfDisclose, Recipient: "b", Duration: 1}}}
}
func TestChoiceDeterminismSensitivityAndWait(t *testing.T) {
	a := actor(t)
	s := situation()
	n, d, e := Choose(a, s, 1, 0)
	if e != nil {
		t.Fatal(e)
	}
	if d.Selected != 0 || d.Candidates[0].Offer.Kind != Wait || d.Operational || d.AppraisalKey != dynamics.Key("a", "e") || len(n.State.Applied) != 1 {
		t.Fatal("WAIT/appraisal not recorded")
	}
	_, again, e := Choose(a, s, 1, 0)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(d)
	c, _ := json.Marshal(again)
	if string(b) != string(c) {
		t.Fatal("same input changed")
	}
	if _, _, e = Choose(n, s, 1, 1); e == nil {
		t.Fatal("double appraisal/action")
	}
	sum := 0.0
	for _, c := range d.Candidates {
		sum += c.Probability
		if c.Offer.Kind == SelfDisclose {
			t.Fatal("unapproved disclosure")
		}
	}
	if math.Abs(sum-1) > 1e-12 {
		t.Fatal("not normalized")
	}
	// Ablations compare distributions, not forced selections or fitted outcomes.
	sensitive := a
	sensitive.Memory = []Memory{{Other: "b", Trust: .9, Disclosure: .9, Evidence: []core.ID{"m"}}}
	_, memory, e := Choose(sensitive, s, 1, 0)
	if e != nil {
		t.Fatal(e)
	}
	b, _ = json.Marshal(memory.Candidates)
	c, _ = json.Marshal(d.Candidates)
	if string(b) == string(c) {
		t.Fatal("memory has no behavioral effect")
	}
	relations := s
	relations.Relationships = []Memory{{Other: "b", Trust: -.8, Disclosure: -.8, Evidence: []core.ID{"edge-source"}}}
	_, edge, e := Choose(a, relations, 1, 0)
	if e != nil {
		t.Fatal(e)
	}
	b, _ = json.Marshal(edge.Candidates)
	if string(b) == string(c) {
		t.Fatal("retrieved relationship has no behavioral effect")
	}
	sensitive = actor(t)
	sensitive.State, e = dynamics.Intervene(sensitive.State, dynamics.Fatigue, .99, 0, "research")
	if e != nil {
		t.Fatal(e)
	}
	_, state, e := Choose(sensitive, s, 1, 0)
	if e != nil {
		t.Fatal(e)
	}
	b, _ = json.Marshal(state.Candidates)
	if string(b) == string(c) {
		t.Fatal("state has no behavioral effect")
	}
	// Contradictory belief is preserved as a hypothesis, never replaced by a label.
	s.Beliefs = []Belief{{Source: "claim", Observer: "a", Code: "contradicted", Value: -.8, Confidence: .4}}
	n, _, e = Choose(a, s, 1, 0)
	if e != nil || n.Beliefs[0].Value != -.8 {
		t.Fatal("subjective belief lost", e)
	}
}
func TestRegistryConstraintsAndTampering(t *testing.T) {
	if len(Registry()) != 9 {
		t.Fatal("registry changed without version")
	}
	for _, k := range Registry() {
		t.Run(string(k), func(t *testing.T) {
			a := actor(t)
			s := situation()
			s.Offers = nil
			o := Offer{Kind: k, Duration: 1}
			if k != Wait && k != Observe {
				o.Recipient = "b"
			}
			if k == Help {
				o.Resource = "time"
				o.Units = 1
			}
			if k == BreakPromise {
				o.Commitment = "promise"
				s.Commitments = []Commitment{{ID: "promise", Actor: "a", Recipient: "b", Resource: "time", Units: 1, Due: 10, Status: "pending"}}
			}
			if k == SelfDisclose {
				s.DisclosureRecipient = "b"
			}
			s.Offers = []Offer{o}
			_, d, e := Choose(a, s, 1, math.MaxUint64)
			if e != nil || d.Candidates[d.Selected].Offer.Kind != k {
				t.Fatal("action unreachable", d, e)
			}
		})
	}
	for name, change := range map[string]func(*Situation){
		"unknown":            func(s *Situation) { s.Offers[0].Kind = "teleport" },
		"money_in_utterance": func(s *Situation) { s.Offers[0].Resource = "time"; s.Offers[0].Units = 1 },
		"unknown_interpretation": func(s *Situation) {
			s.Beliefs = []Belief{{Source: "x", Observer: "a", Code: "do_this", Confidence: .5}}
		},
		"foreign_belief": func(s *Situation) {
			s.Beliefs = []Belief{{Source: "x", Observer: "b", Code: "supported", Confidence: .5}}
		},
		"nan":       func(s *Situation) { s.Perceived.Signals.Effort = math.NaN() },
		"revoked":   func(s *Situation) { s.Perceived.Rights.Revoked = true },
		"duplicate": func(s *Situation) { s.Offers = append(s.Offers, s.Offers[0]) },
	} {
		t.Run(name, func(t *testing.T) {
			s := situation()
			change(&s)
			if _, _, e := Choose(actor(t), s, 1, 0); e == nil {
				t.Fatal("accepted unsafe input")
			}
		})
	}
	s := situation()
	s.Resources["time"] = 0
	s.Present = []core.ID{"a"}
	_, d, e := Choose(actor(t), s, 1, math.MaxUint64)
	if e != nil || len(d.Candidates) != 1 || d.Reason != "constraints_wait" {
		t.Fatal("recipient/resource constraint", e)
	}
	s = situation()
	s.Outage = true
	_, d, e = Choose(actor(t), s, 1, 123)
	if e != nil || !d.Operational || d.Reason != "provider_outage" || len(d.Candidates) != 1 {
		t.Fatal("operational WAIT", e)
	}
	d.Candidates[0].Probability = .9
	if d.Validate() == nil {
		t.Fatal("forged distribution")
	}
	s = situation()
	a := actor(t)
	a.AvailableAt = 10
	_, d, e = Choose(a, s, 1, 0)
	if e != nil || len(d.Candidates) != 1 {
		t.Fatal("actor time double booked", e)
	}
}
func TestDelayedLearningEvidenceBoundsAndRepair(t *testing.T) {
	a := actor(t)
	s := situation()
	s.Offers = []Offer{{Kind: Ask, Recipient: "b", Duration: 1}}
	a, d, e := Choose(a, s, 1, math.MaxUint64)
	if e != nil {
		t.Fatal(e)
	}
	o, e := Track(d, 10)
	if e != nil || o.Status != core.Unknown {
		t.Fatal(e)
	}
	response := s.Perceived
	response.Event = "reply"
	response.Rights.Resource = "reply"
	response.OccurredAt = 3
	response.LearnedAt = 4
	n, resolved, e := Resolve(a, o, response, "b", "dismissive")
	if e != nil || resolved.Status != core.Observed || resolved.LearnedAt != 4 || resolved.Evidence != "reply" || n.Memory[0].Trust >= 0 || len(a.Memory) != 0 {
		t.Fatal("delayed evidence/learning", e)
	}
	if _, _, e = Resolve(n, resolved, response, "b", "supportive"); e == nil {
		t.Fatal("outcome resolved twice")
	}
	if _, _, e = Resolve(n, o, response, "b", "supportive"); e == nil {
		t.Fatal("evidence applied twice")
	}
	for _, which := range []string{"foreign", "late", "revoked", "wrong_other", "unknown_response", "operational"} {
		t.Run(which, func(t *testing.T) {
			r := response
			copy := o
			other := core.ID("b")
			code := "supportive"
			switch which {
			case "foreign":
				r.Actor = "c"
			case "late":
				r.LearnedAt = 11
			case "revoked":
				r.Rights.Revoked = true
			case "wrong_other":
				other = "c"
			case "unknown_response":
				code = "guaranteed_success"
			case "operational":
				copy.Operational = true
			}
			if _, _, e := Resolve(a, copy, r, other, code); e == nil {
				t.Fatal("unsafe outcome")
			}
		})
	}
	// Later supportive evidence repairs bounded observer state, not model weights.
	second := o
	second.Decision = "second"
	response.Event = "repair"
	response.Rights.Resource = "repair"
	response.Confidence = 1
	repaired, _, e := Resolve(n, second, response, "b", "supportive")
	if e != nil || repaired.Memory[0].Trust <= n.Memory[0].Trust {
		t.Fatal("no repair", e)
	}
	censored, e := Censor(o, 10)
	if e != nil || censored.Status != core.Censored || censored.Evidence != "" {
		t.Fatal("censor invented outcome", e)
	}
	if _, e = Censor(o, 9); e == nil {
		t.Fatal("early censor")
	}
	if _, _, e = Resolve(a, censored, response, "b", "supportive"); e == nil {
		t.Fatal("censored outcome trained")
	}
}
func TestReceiptExhaustionDoesNotEvict(t *testing.T) {
	a := actor(t)
	for i := 0; i < dynamics.MaxReceipts; i++ {
		a.State.Applied = append(a.State.Applied, dynamics.Receipt{Event: core.ID("old" + strings.Repeat("x", i)), Digest: strings.Repeat("a", 64)})
	}
	if _, _, e := Choose(a, situation(), 1, 0); e == nil {
		t.Fatal("ledger silently evicted")
	}
}
func FuzzChoiceDraw(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(math.MaxUint64))
	f.Fuzz(func(t *testing.T, draw uint64) {
		_, d, e := Choose(actor(t), situation(), 1, draw)
		if e != nil || d.Validate() != nil {
			t.Fatal(e)
		}
	})
}
