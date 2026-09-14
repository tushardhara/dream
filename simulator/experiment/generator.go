// Package experiment contains label-blind, offline synthetic reference variants.
// It cannot import evaluator data, infrastructure, files or a provider SDK.
package experiment

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/dynamics"
)

const Version = "reference-experiment.v1"
const MaxHorizon = 7 * 24 * dynamics.Hour

// Current.Horizon is an explicit affordance-validity deadline. Each conditional
// choice still uses the existing one-second action window; evidence cannot
// extend its validity merely because the evaluator asks for a later forecast.
func actionWindow(s behavior.Situation, at core.LogicalTime) behavior.Situation {
	if s.Horizon > at+1000000000 {
		s.Horizon = at + 1000000000
	}
	return s
}

type Variant string

const (
	Baseline          Variant = "deterministic_baseline"
	PersonaOnly       Variant = "persona_only"
	Stateful          Variant = "stateful"
	NoMemory          Variant = "no_memory"
	SinglePerspective Variant = "single_perspective"
)

func Variants() []Variant {
	return []Variant{Baseline, PersonaOnly, Stateful, NoMemory, SinglePerspective}
}
func (v Variant) Valid() bool {
	for _, x := range Variants() {
		if v == x {
			return true
		}
	}
	return false
}

type Observation struct {
	At        core.LogicalTime   `json:"at"`
	Situation behavior.Situation `json:"situation"`
	Draw      uint64             `json:"draw"`
}

// Input contains only the approved initial self state and observations available
// at AsOf. Neither outcome labels nor evaluator/judge results have a field here.
type Input struct {
	Version string             `json:"version"`
	ID      core.ID            `json:"id"`
	Initial behavior.Actor     `json:"initial"`
	History []Observation      `json:"history"`
	Current behavior.Situation `json:"current"`
	AsOf    core.LogicalTime   `json:"as_of"`
	Horizon core.LogicalTime   `json:"horizon"`
}

func (i Input) Validate() error {
	if i.Version != Version || i.ID.Validate() != nil || i.Initial.Validate() != nil || i.AsOf < i.Initial.State.At || i.Horizon < 1 || i.Horizon > MaxHorizon || i.AsOf > (1<<60)-i.Horizon || len(i.History) > 8 || i.Current.Horizon-i.AsOf > MaxHorizon {
		return fmt.Errorf("invalid experiment input")
	}
	previous := i.Initial.State.At
	actor := i.Initial
	seen := map[core.ID]bool{}
	for _, o := range i.History {
		if o.At < previous || o.At > i.AsOf || o.Situation.Perceived.Validate(i.Initial.State.Actor, o.At) != nil || seen[o.Situation.Perceived.Event] {
			return fmt.Errorf("future/invalid historical observation")
		}
		var err error
		actor, _, err = behavior.Choose(actor, actionWindow(o.Situation, o.At), o.At, o.Draw)
		if err != nil {
			return fmt.Errorf("invalid historical situation")
		}
		previous = o.At
		seen[o.Situation.Perceived.Event] = true
	}
	if i.Current.Perceived.Validate(i.Initial.State.Actor, i.AsOf) != nil || seen[i.Current.Perceived.Event] {
		return fmt.Errorf("future/repeated current observation")
	}
	if _, _, e := behavior.Choose(actor, actionWindow(i.Current, i.AsOf), i.AsOf, 0); e != nil {
		return fmt.Errorf("invalid current situation")
	}
	raw, e := json.Marshal(i)
	if e != nil || len(raw) > 65536 {
		return fmt.Errorf("experiment input byte budget")
	}
	return nil
}

type Projection struct {
	RequestHash string               `json:"request_hash"`
	Version     string               `json:"version"`
	Variant     Variant              `json:"variant"`
	Horizon     core.LogicalTime     `json:"horizon"`
	Assumption  string               `json:"assumption"`
	Candidates  []behavior.Candidate `json:"candidates"`
	Selected    int                  `json:"selected"`
	Abstained   bool                 `json:"abstained"`
}

func (p Projection) Validate() error {
	if len(p.RequestHash) != 64 {
		return fmt.Errorf("missing request binding")
	}
	if _, e := hex.DecodeString(p.RequestHash); e != nil {
		return fmt.Errorf("invalid request binding")
	}
	if p.Version != Version || !p.Variant.Valid() || p.Horizon < 1 || p.Horizon > MaxHorizon || p.Assumption != "no_unseen_events_fixed_affordances" {
		return fmt.Errorf("invalid projection envelope")
	}
	if p.Abstained {
		if len(p.Candidates) != 0 || p.Selected != -1 {
			return fmt.Errorf("abstention with a selection")
		}
		return nil
	}
	if len(p.Candidates) < 1 || len(p.Candidates) > 17 || p.Selected < 0 || p.Selected >= len(p.Candidates) {
		return fmt.Errorf("invalid projection candidates")
	}
	sum := 0.0
	for _, c := range p.Candidates {
		if !c.Offer.Kind.Valid() || !(c.Probability >= 0 && c.Probability <= 1) {
			return fmt.Errorf("invalid prediction probability")
		}
		sum += c.Probability
	}
	if sum < 1-1e-12 || sum > 1+1e-12 {
		return fmt.Errorf("prediction probabilities not normalized")
	}
	return nil
}

type Generator struct{}

func (Generator) Predict(ctx context.Context, input Input, variant Variant, seed uint64) (Projection, error) {
	if ctx.Err() != nil {
		return Projection{}, ctx.Err()
	}
	if input.Validate() != nil || !variant.Valid() {
		return Projection{}, fmt.Errorf("invalid generation input")
	}
	raw, _ := json.Marshal(input)
	var owned Input
	_ = json.Unmarshal(raw, &owned)
	requestHash, e := RequestDigest(Request{Input: input, Variant: variant, Seed: seed})
	if e != nil {
		return Projection{}, e
	}
	out := Projection{RequestHash: requestHash, Version: Version, Variant: variant, Horizon: owned.Horizon, Assumption: "no_unseen_events_fixed_affordances"}
	if owned.Current.Outage || owned.Current.Horizon < owned.AsOf+owned.Horizon {
		out.Abstained = true
		out.Selected = -1
		return out, out.Validate()
	}
	if variant == Baseline {
		out.Candidates = []behavior.Candidate{{Offer: behavior.Offer{Kind: behavior.Wait, Duration: 1}, Probability: 1}}
		return out, nil
	}
	actor := owned.Initial
	if variant != PersonaOnly {
		for _, observation := range owned.History {
			var e error
			actor, _, e = behavior.Choose(actor, actionWindow(observation.Situation, observation.At), observation.At, observation.Draw)
			if e != nil {
				return Projection{}, e
			}
		}
	} else {
		actor.Memory = nil
		actor.Beliefs = nil
		owned.Current.Relationships = nil
		owned.Current.Beliefs = nil
	}
	if variant == NoMemory {
		actor.Memory = nil
		actor.Beliefs = nil
		owned.Current.Relationships = nil
		owned.Current.Beliefs = nil
	}
	if variant == SinglePerspective {
		if len(owned.Current.Beliefs) > 1 {
			owned.Current.Beliefs = owned.Current.Beliefs[:1]
		}
		if len(owned.Current.Relationships) > 1 {
			owned.Current.Relationships = owned.Current.Relationships[:1]
		}
	}
	// The conditional roll-forward advances time using only already observed
	// evidence and fixed affordances. No actual future event is supplied or read.
	at := owned.AsOf + owned.Horizon
	if owned.Current.Horizon < at {
		out.Abstained = true
		out.Selected = -1
		return out, out.Validate()
	}
	hash := sha256.Sum256([]byte(fmt.Sprintf("choice.v1/%s/%d", owned.ID, seed)))
	_, decision, e := behavior.Choose(actor, actionWindow(owned.Current, at), at, binary.BigEndian.Uint64(hash[:8]))
	if e != nil {
		return Projection{}, e
	}
	out.Candidates = decision.Candidates
	out.Selected = decision.Selected
	return out, out.Validate()
}

func RequestDigest(r Request) (string, error) {
	raw, e := json.Marshal(r)
	if e != nil {
		return "", e
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:]), nil
}
