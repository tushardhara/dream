package behavior

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/drives"
	"github.com/tushardhara/dream/simulator/dynamics"
)

func actionFixture(t testing.TB) (ActionActor, ActionSituation) {
	t.Helper()
	a, e := NewActionActor("a", 0)
	if e != nil {
		t.Fatal(e)
	}
	p := situation().Perceived
	s := ActionSituation{Observation: drives.Observation{Event: p}, Sources: []core.ID{"e"}, Horizon: 20, Present: []core.ID{"a", "b", "c"}, Resources: map[core.ID]int64{"time": 2}}
	for i := range s.Observation.Context {
		q := p
		q.Confidence = 0
		q.Signals = dynamics.Signals{}
		s.Observation.Context[i] = drives.Cue{Evidence: q}
	}
	s.Contexts = []DisclosureContext{{Observer: "a", Recipient: "b", Role: "friend", Trust: ContextValue{.8, 1, "e"}, ExpectedReaction: ContextValue{.8, 1, "e"}, RoleExpectation: ContextValue{.2, .5, "e"}}}
	return a, s
}
func offerFixture(d ActionDefinition) ActionOffer {
	o := ActionOffer{Kind: d.Kind, Duration: 2, Mode: d.Disclosure}
	if d.Recipient != "none" {
		o.Recipient = "b"
	}
	if d.Evidence {
		o.Evidence = []core.ID{"e"}
	}
	if d.Kind == Help || d.Kind == Promise {
		o.Resource = "time"
		o.Units = 1
	}
	if d.Kind == Promise {
		o.Commitment = "new-promise"
		o.Due = 10
	}
	if d.Kind == BreakPromise {
		o.Commitment = "old-promise"
	}
	return o
}
func TestExact27ActionRegistry(t *testing.T) {
	want := strings.Fields("say ask answer reveal partially_reveal hide lie joke challenge apologize support complain argue withdraw coordinate invite decline promise break_promise help ignore delay change_topic seek_third_party_support reconnect leave wait")
	r := ActionRegistry()
	if ActionRegistryHash() != "41ed597c559981bd3a7369562e1f0e6d0949d3969c09691d3ff6ce665c55d185" {
		t.Fatal("v2 registry wire hash changed")
	}
	if len(r) != len(want) {
		t.Fatal("required27 source actions")
	}
	seen := map[Kind]bool{}
	for i, d := range r {
		if string(d.Kind) != want[i] || seen[d.Kind] || d.Effect == "" || d.Resolution == "" {
			t.Fatal("source ordinal/semantics", i, d)
		}
		seen[d.Kind] = true
	}
	r[0].Kind = Wait
	if ActionRegistry()[0].Kind != Say {
		t.Fatal("mutable registry")
	}
	// v2 is a separate wire contract. Legacy helpers stay valid only in v1.
	if !Observe.Valid() || !SelfDisclose.Valid() || !ThirdPartySupport.Valid() || Say.Valid() {
		t.Fatal("legacy enum silently reinterpreted")
	}
	if _, ok := Definition(Observe); ok {
		t.Fatal("legacy helper counted in27")
	}
}
func TestEveryActionValidationEligibilityAndChoiceContract(t *testing.T) {
	for _, definition := range ActionRegistry() {
		t.Run(string(definition.Kind), func(t *testing.T) {
			a, s := actionFixture(t)
			o := offerFixture(definition)
			if o.Kind == Reconnect {
				a.Contact = "left"
			}
			if o.Kind == BreakPromise {
				s.Commitments = []Commitment{{ID: "old-promise", Actor: "a", Recipient: "b", Resource: "time", Units: 1, Due: 10, Status: "pending"}}
			}
			if o.Mode != "" && o.Mode != Silence {
				s.Disclosure = &DisclosureGrant{Recipient: "b", Mode: o.Mode, Sources: []core.ID{"e"}}
			}
			s.Offers = []ActionOffer{o}
			if e := o.Validate(); e != nil {
				t.Fatal(e)
			}
			n, d, e := ChooseAction(a, s, 1, math.MaxUint64)
			if e != nil {
				t.Fatal(e)
			}
			if d.Candidates[d.Selected].Offer.Kind != o.Kind || n.Private.Intention != o.Kind {
				t.Fatal("action inaccessible", d)
			}
			if o.Kind != Wait && n.AvailableAt != 3 {
				t.Fatal("duration not applied")
			}
			switch o.Kind {
			case Leave:
				if n.Contact != "left" {
					t.Fatal("leave")
				}
			case Withdraw:
				if n.Contact != "withdrawn" {
					t.Fatal("withdraw")
				}
			case Reconnect:
				if n.Contact != "engaged" {
					t.Fatal("reconnect")
				}
			case Hide:
				if n.Private.Suppressed != Silence {
					t.Fatal("hide lost private suppression")
				}
			}
			outcome, e := TrackAction(d, 20)
			if e != nil || outcome.Action != o.Kind || outcome.Outcome.Status != core.Unknown || outcome.ObservationStage != "pending" || outcome.LearningStage != "pending" {
				t.Fatal("fabricated later outcome", outcome, e)
			}
			bad := o
			bad.Duration = 0
			if bad.Validate() == nil {
				t.Fatal("invalid duration accepted")
			}
			if definition.Evidence {
				bad = o
				bad.Evidence = nil
				if bad.Validate() == nil {
					t.Fatal("missing evidence accepted")
				}
			}
			blocked := s
			blocked.Outage = true
			_, wait, e := ChooseAction(a, blocked, 1, math.MaxUint64)
			if e != nil || len(wait.Candidates) != 1 || !wait.Operational || wait.Candidates[0].Offer.Kind != Wait {
				t.Fatal("outage executed action", e)
			}
			blocked = s
			blocked.Present = []core.ID{"a"}
			if o.Recipient != "" {
				_, wait, e = ChooseAction(a, blocked, 1, math.MaxUint64)
				if e != nil || len(wait.Candidates) != 1 {
					t.Fatal("absent recipient eligible", e)
				}
			}
		})
	}
}
func TestActionContextSensitivityUnknownAndForeignEvidence(t *testing.T) {
	a, s := actionFixture(t)
	s.Offers = []ActionOffer{{Kind: Say, Recipient: "b", Duration: 1, Evidence: []core.ID{"e"}, Mode: Softened}}
	s.Disclosure = &DisclosureGrant{Recipient: "b", Mode: Softened, Sources: []core.ID{"e"}}
	_, baseline, e := ChooseAction(a, s, 1, 0)
	if e != nil {
		t.Fatal(e)
	}
	names := []string{"Trust", "RoleExpectation", "Sensitivity", "Fear", "Pride", "Shame", "ExpectedReaction", "ProtectiveIntent", "SocialNorm", "Stress", "PriorOutcome"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			changed := s
			changed.Contexts = append([]DisclosureContext{}, s.Contexts...)
			field := reflect.ValueOf(&changed.Contexts[0]).Elem().FieldByName(name)
			old := field.Interface().(ContextValue)
			value := -.7
			if old.Value == value {
				value = .9
			}
			field.Set(reflect.ValueOf(ContextValue{value, 1, "e"}))
			_, d, e := ChooseAction(a, changed, 1, 0)
			if e != nil {
				t.Fatal(e)
			}
			if d.Candidates[1].Consequence == baseline.Candidates[1].Consequence {
				t.Fatal("disclosure input inert", name)
			}
		})
	}
	s.Offers[0].Kind = Reveal
	s.Offers[0].Mode = Full
	s.Disclosure.Mode = Full
	s.Contexts = nil
	_, d, e := ChooseAction(a, s, 1, math.MaxUint64)
	if e != nil || len(d.Candidates) != 1 {
		t.Fatal("unknown trust became full disclosure", e)
	}
	s.Contexts = []DisclosureContext{{Observer: "other", Recipient: "b"}}
	if _, _, e = ChooseAction(a, s, 1, 0); e == nil {
		t.Fatal("foreign context accepted")
	}
	s.Contexts = nil
	s.Offers[0].Evidence = []core.ID{"unapproved"}
	if _, _, e = ChooseAction(a, s, 1, 0); e == nil {
		t.Fatal("unapproved action source")
	}
}
func TestActionLifecycleDeterminismLearningAndTampering(t *testing.T) {
	a, s := actionFixture(t)
	s.Offers = []ActionOffer{{Kind: Ask, Recipient: "b", Duration: 1, Evidence: []core.ID{"e"}}}
	n, d, e := ChooseAction(a, s, 1, math.MaxUint64)
	if e != nil {
		t.Fatal(e)
	}
	_, again, e := ChooseAction(a, s, 1, math.MaxUint64)
	if e != nil || !reflect.DeepEqual(d, again) {
		t.Fatal("replay", e)
	}
	_, alternative, e := ChooseAction(a, s, 1, 0)
	if e != nil || alternative.Selected == d.Selected {
		t.Fatal("no alternate future", e)
	}
	if _, _, e = ChooseAction(n, s, 1, 0); e == nil {
		t.Fatal("duplicate appraisal")
	}
	for i := range d.Stages {
		bad := d
		bad.Stages[i] = ""
		if bad.Validate() == nil {
			t.Fatal("stage omitted", i)
		}
	}
	o, e := TrackAction(d, 20)
	if e != nil {
		t.Fatal(e)
	}
	p := s.Observation.Event
	p.Event = "response"
	p.OccurredAt = 3
	p.LearnedAt = 3
	p.Rights.Resource = p.Event
	supported, observed, e := ResolveAction(n, o, p, "b", "supportive")
	if e != nil {
		t.Fatal(e)
	}
	dismissed, _, e := ResolveAction(n, o, p, "b", "dismissive")
	if e != nil {
		t.Fatal(e)
	}
	if observed.ObservationStage != "done" || observed.LearningStage != "done" || supported.Memory[0].Trust <= dismissed.Memory[0].Trust {
		t.Fatal("later learning missing")
	}
	other, _ := NewActionActor("c", 0)
	if _, _, e = ResolveAction(other, o, p, "b", "supportive"); e == nil {
		t.Fatal("foreign actor learned")
	}
	if _, _, e = ResolveAction(supported, observed, p, "b", "dismissive"); e == nil {
		t.Fatal("response learned twice")
	}
	c, e := CensorAction(o, 20)
	if e != nil || c.Outcome.Status != core.Censored || c.LearningStage != "censored" {
		t.Fatal("censor fabricated observation", e)
	}
	b, _ := json.Marshal(n)
	if len(b) > 65536 {
		t.Fatal("actor bound")
	}
}
func TestActionResourcesCommitmentsAndContactConstraints(t *testing.T) {
	a, s := actionFixture(t)
	promise := offerFixture(actionDefinitions[17])
	s.Offers = []ActionOffer{promise}
	_, d, e := ChooseAction(a, s, 1, math.MaxUint64)
	if e != nil || d.Candidates[d.Selected].Offer.Kind != Promise {
		t.Fatal(e)
	}
	for _, name := range []string{"capacity", "deadline", "duplicate"} {
		t.Run(name, func(t *testing.T) {
			_, s := actionFixture(t)
			s.Offers = []ActionOffer{promise}
			switch name {
			case "capacity":
				s.Resources["time"] = 0
			case "deadline":
				s.Offers[0].Due = 2
			case "duplicate":
				s.Commitments = []Commitment{{ID: promise.Commitment, Actor: "a", Recipient: "b", Resource: "time", Units: 1, Due: 10, Status: "pending"}}
			}
			_, d, e := ChooseAction(a, s, 1, math.MaxUint64)
			if e != nil || len(d.Candidates) != 1 {
				t.Fatal("unsafe promise", e)
			}
		})
	}
	a.Contact = "left"
	s.Offers = []ActionOffer{{Kind: Ask, Recipient: "b", Duration: 1, Evidence: []core.ID{"e"}}}
	_, d, e = ChooseAction(a, s, 1, math.MaxUint64)
	if e != nil || len(d.Candidates) != 1 {
		t.Fatal("left actor continued conversation", e)
	}
	s.Outage = true
	s.Offers[0].Kind = "unparsed-model-command"
	if _, _, e = ChooseAction(a, s, 1, 0); e == nil {
		t.Fatal("outage hid malformed action")
	}
}

func TestActionCannotLearnBeforeDelivery(t *testing.T) {
	a, s := actionFixture(t)
	s.Offers = []ActionOffer{{Kind: Ask, Recipient: "b", Evidence: []core.ID{"e"}, Duration: 5}}
	n, d, e := ChooseAction(a, s, 1, math.MaxUint64)
	if e != nil {
		t.Fatal(e)
	}
	o, e := TrackAction(d, 20)
	if e != nil {
		t.Fatal(e)
	}
	p := s.Observation.Event
	p.Event = "later"
	p.Rights.Resource = "later"
	p.OccurredAt = 2
	p.LearnedAt = 8
	if _, _, e = ResolveAction(n, o, p, "b", "supportive"); e == nil {
		t.Fatal("learned a response before action delivery")
	}
	p.OccurredAt = 6
	if _, _, e = ResolveAction(n, o, p, "b", "supportive"); e != nil {
		t.Fatal("later observation denied", e)
	}
}

func TestWaitHasNoAvailabilityOrContactEffect(t *testing.T) {
	for _, outage := range []bool{false, true} {
		for _, contact := range []string{"engaged", "withdrawn", "left"} {
			a, s := actionFixture(t)
			a.AvailableAt = 10
			a.Contact = contact
			s.Outage = outage
			next, d, e := ChooseAction(a, s, 1, math.MaxUint64)
			if e != nil {
				t.Fatal(e)
			}
			if d.Candidates[d.Selected].Offer.Kind != Wait || next.AvailableAt != a.AvailableAt || next.Contact != a.Contact {
				t.Fatal("WAIT changed availability/contact", outage, contact)
			}
			if d.Operational != outage {
				t.Fatal("behavioral WAIT/outage conflated")
			}
		}
	}
}
