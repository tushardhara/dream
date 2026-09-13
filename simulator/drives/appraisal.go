package drives

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/dynamics"
	"math"
	"sort"
)

const ContextCount = 6
const (
	Setting = iota
	History
	Resources
	Relationships
	Beliefs
	Uncertainty
)

// Cue means this actor's uncertain perception of a dimension, not global truth.
// Slots: setting opportunity, history support, resource availability, relationship
// support, belief in safety, uncertainty. Each has its own evidence and rights.
type Cue struct {
	Evidence dynamics.Perceived `json:"evidence"`
	Value    float64            `json:"value"`
}
type Observation struct {
	Event   dynamics.Perceived `json:"event"`
	Context [ContextCount]Cue  `json:"context"`
}

func (o Observation) Validate(actor core.ID, at core.LogicalTime) error {
	if e := o.Event.Validate(actor, at); e != nil {
		return e
	}
	for _, c := range o.Context {
		if !unit(c.Value) {
			return fmt.Errorf("invalid perceived context")
		}
		if e := c.Evidence.Validate(actor, at); e != nil {
			return e
		}
	}
	return nil
}
func normalized(p dynamics.Perceived) dynamics.Perceived {
	p.Rights.Grants = append([]core.Grant{}, p.Rights.Grants...)
	sort.Slice(p.Rights.Grants, func(i, j int) bool {
		a, _ := json.Marshal(p.Rights.Grants[i])
		b, _ := json.Marshal(p.Rights.Grants[j])
		return string(a) < string(b)
	})
	if p.Confidence == 0 {
		p.Confidence = 0
	}
	x := &p.Signals
	for _, v := range []*float64{&x.Effort, &x.Rest, &x.Scarcity, &x.Opportunity, &x.OtherNeed, &x.Support, &x.StatusThreat, &x.Inclusion, &x.Exclusion} {
		if *v == 0 {
			*v = 0
		}
	}
	return p
}
func observationHash(o Observation) string {
	o.Event = normalized(o.Event)
	for i := range o.Context {
		o.Context[i].Evidence = normalized(o.Context[i].Evidence)
		if o.Context[i].Value == 0 {
			o.Context[i].Value = 0
		}
	}
	raw, _ := json.Marshal(o)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func Key(actor, event core.ID) string {
	raw, _ := json.Marshal([]string{ModelVersion, string(actor), string(event)})
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

// Rationale is an enum-only stage trace with bounded deltas; no private prose or
// unrestricted chain-of-thought. The receipt digest binds all permitted inputs.
type Rationale struct {
	Version   int            `json:"version"`
	Registry  string         `json:"registry"`
	Key       string         `json:"key"`
	Cause     core.ID        `json:"cause"`
	Stages    [4]string      `json:"stages"`
	Deltas    [Count]float64 `json:"deltas"`
	Duplicate bool           `json:"duplicate"`
}

func (r Rationale) Validate() error {
	if r.Version != Version || r.Registry != RegistryVersion || !validHash(r.Key) || r.Cause.Validate() != nil || r.Stages != [4]string{"event", "perception", "appraisal", "state_change"} {
		return fmt.Errorf("invalid appraisal rationale")
	}
	for _, v := range r.Deltas {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < -1 || v > 1 || r.Duplicate && v != 0 {
			return fmt.Errorf("invalid appraisal delta")
		}
	}
	return nil
}
func Appraise(s State, o Observation, at core.LogicalTime) (State, Rationale, bool, error) {
	if e := s.Validate(); e != nil {
		return State{}, Rationale{}, false, e
	}
	if at < s.At {
		return State{}, Rationale{}, false, fmt.Errorf("retroactive appraisal")
	}
	if e := o.Validate(s.Actor, at); e != nil {
		return State{}, Rationale{}, false, e
	}
	digest := observationHash(o)
	r := Rationale{Version: Version, Registry: RegistryVersion, Key: Key(s.Actor, o.Event.Event), Cause: o.Event.Event, Stages: [4]string{"event", "perception", "appraisal", "state_change"}}
	for _, old := range s.Applied {
		if old.Event == o.Event.Event {
			if old.Digest != digest {
				return State{}, Rationale{}, false, fmt.Errorf("appraisal context/idempotency conflict")
			}
			r.Duplicate = true
			return clone(s), r, true, nil
		}
	}
	if len(s.Applied) >= MaxReceipts {
		return State{}, Rationale{}, false, fmt.Errorf("appraisal receipt budget")
	}
	out, e := Advance(s, at)
	if e != nil {
		return State{}, Rationale{}, false, e
	}
	// Confidence gates each context contribution; unknown context contributes zero.
	var c [ContextCount]float64
	for i, cue := range o.Context {
		c[i] = cue.Value * float64(cue.Evidence.Confidence)
	}
	x := o.Event.Signals
	support := .4*c[History] + .6*c[Relationships]
	safe := c[Beliefs]
	available := c[Resources]
	opportunity := c[Setting]
	fatigue := out.Variables[EffortAvoidance].Values[0]
	threat := x.StatusThreat + x.Exclusion
	raw := [Count]float64{
		x.Scarcity + x.Opportunity + .3*opportunity - .4*available,
		x.StatusThreat + .3*x.Opportunity - .3*support,
		x.StatusThreat + .3*threat*(1-safe) - .3*support,
		threat*(1-.5*safe) + .25*x.Scarcity*x.Effort - .3*x.Support,
		x.Opportunity + .4*opportunity + .2*x.Inclusion - .5*fatigue - .3*threat,
		x.Effort + .25*x.Scarcity*x.Effort - x.Rest - .2*available,
		x.Opportunity + .3*opportunity - .2*x.Scarcity,
		threat + .3*x.Scarcity - .4*safe - .3*x.Support,
		x.Exclusion + .2*threat - x.Inclusion - .3*support,
		x.StatusThreat + .3*x.Effort - .2*x.Support,
		x.Effort*(1-.4*fatigue) + .3*x.Opportunity - .3*threat,
		x.OtherNeed*(1-.5*fatigue) + .3*support + .2*x.Support - .3*x.Effort*fatigue,
		x.Scarcity + x.Exclusion - .3*support,
		x.OtherNeed + .5*support - .3*x.Effort,
		x.StatusThreat + .3*x.Opportunity - .25*x.Support,
		threat + .3*c[History] - .3*safe,
		x.Opportunity + .4*opportunity - .4*threat - .2*fatigue,
		.4*x.OtherNeed + .4*support + .2*x.Effort - .3*threat,
		c[Uncertainty] + .4*threat - .4*safe,
		x.Opportunity + .5*opportunity - .3*c[History] - .3*threat,
		x.Exclusion + .5*support + .3*x.Inclusion - .2*x.StatusThreat,
		x.Scarcity + .5*threat + .3*c[Uncertainty] - .3*available,
	}
	gain := (.1 + .9*out.Substrate.Reactivity) * out.Substrate.Plasticity * float64(o.Event.Confidence) * (1 - .5*c[Uncertainty])
	for i, value := range raw {
		before := out.Variables[i]
		delta := math.Max(-1, math.Min(1, value)) * definitions[i].Gain * gain
		level := quant(before.Values[0] + delta)
		confidence := before.Values[2]
		if delta != 0 {
			confidence = quant(confidence*(1-gain) + float64(o.Event.Confidence)*gain)
		}
		out.Variables[i] = Variable{[4]float64{level, level, confidence, confidence}, at}
		r.Deltas[i] = math.Round((level-before.Values[0])*1e9) / 1e9
		if r.Deltas[i] == 0 {
			r.Deltas[i] = 0
		}
	}
	for _, cue := range o.Context {
		if e = addCause(&out, cue.Evidence.Event); e != nil {
			return State{}, Rationale{}, false, e
		}
	}
	if e = addCause(&out, o.Event.Event); e != nil {
		return State{}, Rationale{}, false, e
	}
	out.Applied = append(out.Applied, dynamics.Receipt{Event: o.Event.Event, Digest: digest})
	return out, r, false, out.Validate()
}

// Response exposes competing tendencies only. It does not select interventions,
// rank relationships, maximize attachment or optimize engagement.
func Response(s State) (dynamics.Tendencies, error) {
	if e := s.Validate(); e != nil {
		return dynamics.Tendencies{}, e
	}
	v := func(i int) float64 { return s.Variables[i].Values[0] }
	return dynamics.Tendencies{
		Rest:     quant(.8*v(EffortAvoidance) + .2*v(Safety)),
		Approach: quant(.3*v(ApproachDesire) + .15*v(Curiosity) + .15*v(Novelty) + .1*v(RewardSeeking) + .1*v(Competence) + .1*v(Autonomy) + .1*v(Meaning) - .2*v(ThreatResponse)),
		Support:  quant((.45*v(Care) + .2*v(Reciprocity) + .15*v(Fairness) + .1*v(Belonging) + .1*v(Attachment)) * (1 - .5*v(EffortAvoidance))),
		Defend:   quant(.2*v(StatusProtection) + .15*v(ThreatResponse) + .15*v(IdentityProtection) + .15*v(LossAvoidance) + .1*v(Status) + .1*v(Comparison) + .15*v(Acquisition)),
		Wait:     quant(.1 + .3*v(EffortAvoidance) + .2*v(Safety) + .2*v(Certainty) + .2*v(LossAvoidance) - .1*v(ApproachDesire))}, nil
}
