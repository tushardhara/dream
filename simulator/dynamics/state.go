package dynamics

import (
	"fmt"
	"github.com/tushardhara/dream/core"
	"math"
)

const MaxReceipts = 32

// Substrate remains stable across reference transitions. Plasticity/reactivity
// are explicit research parameters, not inferred properties of real people.
type Substrate struct {
	Baseline   [Count]float64 `json:"baseline"`
	Reactivity float64        `json:"reactivity"`
	Plasticity float64        `json:"plasticity"`
}

// Variable keeps a decay anchor so time-only subdivisions never feed quantized
// intermediate levels back into the exponential. Confidence is subjective, not a
// calibrated probability, and decays toward zero using the same time constant.
type Variable struct {
	Level            float64          `json:"level"`
	Anchor           float64          `json:"anchor"`
	Confidence       core.Confidence  `json:"confidence"`
	AnchorConfidence core.Confidence  `json:"anchor_confidence"`
	AnchorAt         core.LogicalTime `json:"anchor_at"`
}
type Receipt struct {
	Event  core.ID `json:"event"`
	Digest string  `json:"digest"`
}
type State struct {
	Version   int              `json:"version"`
	Registry  string           `json:"registry"`
	Model     string           `json:"model"`
	Actor     core.ID          `json:"actor"`
	At        core.LogicalTime `json:"at"`
	Substrate Substrate        `json:"substrate"`
	Variables [Count]Variable  `json:"variables"`
	Causes    []core.ID        `json:"causes"`
	Applied   []Receipt        `json:"applied"`
}

func DefaultSubstrate() Substrate {
	s := Substrate{Reactivity: .5, Plasticity: .5}
	for i, d := range definitions {
		s.Baseline[i] = d.Baseline
	}
	return s
}
func New(actor core.ID, at core.LogicalTime, p Substrate) (State, error) {
	s := State{Version: Version, Registry: RegistryVersion, Model: ModelVersion, Actor: actor, At: at, Substrate: p, Causes: []core.ID{}, Applied: []Receipt{}}
	for i, b := range p.Baseline {
		s.Variables[i] = Variable{Level: quant(b), Anchor: quant(b), Confidence: .5, AnchorConfidence: .5, AnchorAt: at}
	}
	return s, s.Validate()
}
func unit(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1 }
func quant(v float64) float64 {
	v = math.Max(0, math.Min(1, v))
	v = math.Round(v*1e9) / 1e9
	if v == 0 {
		return 0
	}
	return v
}
func (s State) Validate() error {
	if s.Version != Version || s.Registry != RegistryVersion || s.Model != ModelVersion || s.Actor.Validate() != nil || s.At < 0 || !unit(s.Substrate.Reactivity) || !unit(s.Substrate.Plasticity) || len(s.Causes) > 32 || len(s.Applied) > MaxReceipts {
		return fmt.Errorf("invalid synthetic state envelope or bounds")
	}
	for i, v := range s.Variables {
		if !unit(s.Substrate.Baseline[i]) || !unit(v.Level) || !unit(v.Anchor) || v.Confidence.Validate() != nil || v.AnchorConfidence.Validate() != nil || v.AnchorAt < 0 || v.AnchorAt > s.At {
			return fmt.Errorf("invalid variable %s", definitions[i].ID)
		}
		level, confidence := decayed(v, s.Substrate.Baseline[i], definitions[i].HalfLife, s.At)
		if math.Abs(level-v.Level) > 1e-9 || math.Abs(float64(confidence-v.Confidence)) > 1e-9 {
			return fmt.Errorf("variable inconsistent with decay anchor")
		}
	}
	seen := map[core.ID]bool{}
	for _, id := range s.Causes {
		if id.Validate() != nil || seen[id] {
			return fmt.Errorf("invalid causal references")
		}
		seen[id] = true
	}
	seen = map[core.ID]bool{}
	for _, r := range s.Applied {
		if r.Event.Validate() != nil || seen[r.Event] || len(r.Digest) != 64 {
			return fmt.Errorf("invalid appraisal ledger")
		}
		for _, c := range r.Digest {
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				return fmt.Errorf("invalid receipt hash")
			}
		}
		seen[r.Event] = true
	}
	return nil
}
func decayed(v Variable, baseline float64, half core.LogicalTime, at core.LogicalTime) (float64, core.Confidence) {
	factor := math.Exp2(-float64(at-v.AnchorAt) / float64(half))
	return quant(baseline + (v.Anchor-baseline)*factor), core.Confidence(quant(float64(v.AnchorConfidence) * factor))
}

// Advance changes virtual time only. No wall clocks or random draws enter it.
func Advance(s State, at core.LogicalTime) (State, error) {
	if err := s.Validate(); err != nil {
		return State{}, err
	}
	if at < s.At {
		return State{}, fmt.Errorf("retroactive decay")
	}
	out := clone(s)
	out.At = at
	for i, v := range out.Variables {
		out.Variables[i].Level, out.Variables[i].Confidence = decayed(v, out.Substrate.Baseline[i], definitions[i].HalfLife, at)
	}
	return out, out.Validate()
}
func clone(s State) State {
	s.Causes = append([]core.ID{}, s.Causes...)
	s.Applied = append([]Receipt{}, s.Applied...)
	return s
}

// Intervene is an explicitly research-only counterfactual state edit. It does
// not modify the substrate, execute an action, or mutate a running world. A host
// must event/branch such interventions; never apply this as a storage backdoor.
func Intervene(s State, index int, level float64, at core.LogicalTime, cause core.ID) (State, error) {
	if index < 0 || index >= Count || !unit(level) || cause.Validate() != nil {
		return State{}, fmt.Errorf("invalid research intervention")
	}
	out, err := Advance(s, at)
	if err != nil {
		return State{}, err
	}
	out.Variables[index] = Variable{Level: quant(level), Anchor: quant(level), Confidence: 1, AnchorConfidence: 1, AnchorAt: at}
	if err := addCause(&out, cause); err != nil {
		return State{}, err
	}
	return out, out.Validate()
}

// Tendencies are bounded reference scores, not probabilities or selected actions.
type Tendencies struct {
	Rest     float64 `json:"rest"`
	Approach float64 `json:"approach"`
	Support  float64 `json:"support"`
	Defend   float64 `json:"defend"`
	Wait     float64 `json:"wait"`
}

func Response(s State) (Tendencies, error) {
	if err := s.Validate(); err != nil {
		return Tendencies{}, err
	}
	v := s.Variables
	f, p, c, st, b, r := v[Fatigue].Level, v[ScarcityOpportunity].Level, v[Care].Level, v[Status].Level, v[Belonging].Level, v[SlowResidue].Level
	return Tendencies{quant(.75*f + .25*r), quant(b*(1-.4*f) + .15*(1-p) - .2*r), quant(c * (1 - .6*f) * (1 - .4*p)), quant(.5*st + .3*p + .2*r), quant(.1 + .7*f + .2*r - .2*c)}, nil
}

func addCause(s *State, id core.ID) error {
	for _, old := range s.Causes {
		if old == id {
			return nil
		}
	}
	if len(s.Causes) >= 32 {
		return fmt.Errorf("causal reference budget")
	}
	s.Causes = append(s.Causes, id)
	return nil
}
