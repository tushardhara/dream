package drives

import (
	"fmt"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/dynamics"
	"math"
)

const MaxReceipts = dynamics.MaxReceipts
const MaxBytes = dynamics.MaxBytes

// Compact ordinal tuple: level, anchor, confidence, anchor confidence. Anchor
// time is an integer outside the tuple so nanosecond precision is never lost.
type Variable struct {
	Values   [4]float64       `json:"v"`
	AnchorAt core.LogicalTime `json:"at"`
}
type Substrate struct {
	Baseline   [Count]float64 `json:"baseline"`
	Reactivity float64        `json:"reactivity"`
	Plasticity float64        `json:"plasticity"`
}
type State struct {
	Version      int                `json:"version"`
	Registry     string             `json:"registry"`
	RegistryHash string             `json:"registry_hash"`
	Model        string             `json:"model"`
	Actor        core.ID            `json:"actor"`
	At           core.LogicalTime   `json:"at"`
	Substrate    Substrate          `json:"substrate"`
	Variables    [Count]Variable    `json:"variables"`
	Causes       []core.ID          `json:"causes"`
	Applied      []dynamics.Receipt `json:"applied"`
}

func DefaultSubstrate() Substrate {
	s := Substrate{Reactivity: .5, Plasticity: .5}
	for i, d := range definitions {
		s.Baseline[i] = d.Baseline
	}
	return s
}
func New(actor core.ID, at core.LogicalTime, substrate Substrate) (State, error) {
	s := State{Version: Version, Registry: RegistryVersion, RegistryHash: RegistryHash, Model: ModelVersion, Actor: actor, At: at, Substrate: substrate, Causes: []core.ID{}, Applied: []dynamics.Receipt{}}
	for i, b := range substrate.Baseline {
		s.Variables[i] = Variable{[4]float64{quant(b), quant(b), .5, .5}, at}
	}
	return s, s.Validate()
}
func unit(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1 }
func quant(v float64) float64 {
	v = math.Round(math.Max(0, math.Min(1, v))*1e9) / 1e9
	if v == 0 {
		return 0
	}
	return v
}
func clone(s State) State {
	s.Causes = append([]core.ID{}, s.Causes...)
	s.Applied = append([]dynamics.Receipt{}, s.Applied...)
	return s
}
func decayed(v Variable, baseline float64, half, at core.LogicalTime) (float64, float64) {
	f := math.Exp2(-float64(at-v.AnchorAt) / float64(half))
	return quant(baseline + (v.Values[1]-baseline)*f), quant(v.Values[3] * f)
}
func (s State) Validate() error {
	if ValidateRegistry() != nil || s.Version != Version || s.Registry != RegistryVersion || s.RegistryHash != RegistryHash || s.Model != ModelVersion {
		return fmt.Errorf("unsupported drive state version/registry; legacy ticket7-subset.v1 requires DecodeRecorded")
	}
	if s.Actor.Validate() != nil || s.At < 0 || !unit(s.Substrate.Reactivity) || !unit(s.Substrate.Plasticity) || len(s.Causes) > 32 || len(s.Applied) > MaxReceipts {
		return fmt.Errorf("invalid drive envelope or resource bounds")
	}
	for i, v := range s.Variables {
		if !unit(s.Substrate.Baseline[i]) || v.AnchorAt < 0 || v.AnchorAt > s.At {
			return fmt.Errorf("invalid drive anchor/baseline")
		}
		for _, x := range v.Values {
			if !unit(x) {
				return fmt.Errorf("nonfinite/out-of-range drive or confidence")
			}
		}
		level, confidence := decayed(v, s.Substrate.Baseline[i], definitions[i].HalfLife, s.At)
		if level != v.Values[0] || confidence != v.Values[2] {
			return fmt.Errorf("drive differs from decay anchor")
		}
	}
	causes := map[core.ID]bool{}
	for _, id := range s.Causes {
		if id.Validate() != nil || causes[id] {
			return fmt.Errorf("invalid causal ledger")
		}
		causes[id] = true
	}
	seen := map[core.ID]bool{}
	for _, r := range s.Applied {
		if !causes[r.Event] || seen[r.Event] || !validHash(r.Digest) {
			return fmt.Errorf("invalid appraisal receipt")
		}
		seen[r.Event] = true
	}
	return nil
}
func Advance(s State, at core.LogicalTime) (State, error) {
	if e := s.Validate(); e != nil {
		return State{}, e
	}
	if at < s.At {
		return State{}, fmt.Errorf("retroactive decay")
	}
	out := clone(s)
	out.At = at
	for i, v := range out.Variables {
		out.Variables[i].Values[0], out.Variables[i].Values[2] = decayed(v, out.Substrate.Baseline[i], definitions[i].HalfLife, at)
	}
	return out, out.Validate()
}
func Intervene(s State, index int, level float64, at core.LogicalTime, cause core.ID) (State, error) {
	if index < 0 || index >= Count || !unit(level) || cause.Validate() != nil {
		return State{}, fmt.Errorf("invalid research intervention")
	}
	out, e := Advance(s, at)
	if e != nil {
		return State{}, e
	}
	out.Variables[index] = Variable{[4]float64{quant(level), quant(level), 1, 1}, at}
	if e = addCause(&out, cause); e != nil {
		return State{}, e
	}
	return out, out.Validate()
}
func addCause(s *State, id core.ID) error {
	for _, old := range s.Causes {
		if old == id {
			return nil
		}
	}
	if len(s.Causes) >= 32 {
		return fmt.Errorf("causal ledger budget")
	}
	s.Causes = append(s.Causes, id)
	return nil
}
