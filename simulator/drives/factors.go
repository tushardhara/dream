package drives

import (
	"fmt"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/dynamics"
	"math"
)

const FactorVersion = "legacy-factors.v1"
const (
	Fatigue = iota
	ScarcityOpportunity
	SlowResidue
	FactorCount
)

var factorIndices = [FactorCount]int{dynamics.Fatigue, dynamics.ScarcityOpportunity, dynamics.SlowResidue}

// Legacy factors are not substitutes or aliases for any of the22 required drives.
type Factors struct {
	Version   string                `json:"version"`
	Baseline  [FactorCount]float64  `json:"baseline"`
	Variables [FactorCount]Variable `json:"variables"`
}

func newFactors(at core.LogicalTime) Factors {
	f := Factors{Version: FactorVersion}
	r := dynamics.Registry()
	for i, index := range factorIndices {
		b := r[index].Baseline
		f.Baseline[i] = b
		f.Variables[i] = Variable{[4]float64{b, b, .5, .5}, at}
	}
	return f
}
func (f Factors) validate(at core.LogicalTime) error {
	if f.Version != FactorVersion {
		return fmt.Errorf("unsupported retained factor version")
	}
	r := dynamics.Registry()
	for i, v := range f.Variables {
		if !unit(f.Baseline[i]) || v.AnchorAt < 0 || v.AnchorAt > at {
			return fmt.Errorf("invalid retained factor anchor")
		}
		for _, x := range v.Values {
			if !unit(x) {
				return fmt.Errorf("invalid retained factor/confidence")
			}
		}
		a, c := decayed(v, f.Baseline[i], r[factorIndices[i]].HalfLife, at)
		if a != v.Values[0] || c != v.Values[2] {
			return fmt.Errorf("retained factor decay mismatch")
		}
	}
	return nil
}
func (f Factors) advance(at core.LogicalTime) Factors {
	r := dynamics.Registry()
	for i, v := range f.Variables {
		f.Variables[i].Values[0], f.Variables[i].Values[2] = decayed(v, f.Baseline[i], r[factorIndices[i]].HalfLife, at)
	}
	return f
}
func (f Factors) appraise(p dynamics.Perceived, substrate Substrate, at core.LogicalTime) Factors {
	x := p.Signals
	fatigue := f.Variables[Fatigue].Values[0]
	raw := [FactorCount]float64{x.Effort - x.Rest + .25*x.Scarcity*x.Effort, x.Scarcity - x.Opportunity + .25*fatigue*x.Effort, .4*x.StatusThreat + .4*x.Exclusion + .2*x.Scarcity + .3*x.StatusThreat*x.Exclusion - .2*x.Support - .1*x.Rest}
	gain := (.1 + .9*substrate.Reactivity) * substrate.Plasticity * float64(p.Confidence)
	defs := dynamics.Registry()
	for i, value := range raw {
		old := f.Variables[i]
		delta := math.Max(-1, math.Min(1, value)) * defs[factorIndices[i]].Gain * gain
		level := quant(old.Values[0] + delta)
		confidence := old.Values[2]
		if delta != 0 {
			confidence = quant(confidence*(1-gain) + float64(p.Confidence)*gain)
		}
		f.Variables[i] = Variable{[4]float64{level, level, confidence, confidence}, at}
	}
	return f
}

type UpgradeRecord struct {
	Version        int     `json:"version"`
	SourceRegistry string  `json:"source_registry"`
	SourceHash     string  `json:"source_hash"`
	NewBranch      core.ID `json:"new_branch"`
}

// UpgradeLegacy creates a new versioned research state with explicit provenance.
// It never writes history or resets receipts. The host must use the existing
// authorized fork/new-run path to establish NewBranch and preserve run budgets.
func UpgradeLegacy(old dynamics.State, newBranch core.ID) (State, error) {
	if e := old.Validate(); e != nil {
		return State{}, e
	}
	if newBranch.Validate() != nil {
		return State{}, fmt.Errorf("explicit new branch identity required")
	}
	hash, e := old.Hash()
	if e != nil {
		return State{}, e
	}
	p := DefaultSubstrate()
	p.Reactivity = old.Substrate.Reactivity
	p.Plasticity = old.Substrate.Plasticity
	same := [3][2]int{{Care, dynamics.Care}, {Status, dynamics.Status}, {Belonging, dynamics.Belonging}}
	for _, pair := range same {
		p.Baseline[pair[0]] = old.Substrate.Baseline[pair[1]]
	}
	out, e := New(old.Actor, old.At, p)
	if e != nil {
		return State{}, e
	}
	// Unobserved new drives start at explicit baselines with zero confidence.
	for i := range out.Variables {
		out.Variables[i].Values[2] = 0
		out.Variables[i].Values[3] = 0
	}
	copyVar := func(v dynamics.Variable) Variable {
		return Variable{[4]float64{v.Level, v.Anchor, float64(v.Confidence), float64(v.AnchorConfidence)}, v.AnchorAt}
	}
	for _, pair := range same {
		out.Variables[pair[0]] = copyVar(old.Variables[pair[1]])
	}
	for i, index := range factorIndices {
		out.Factors.Baseline[i] = old.Substrate.Baseline[index]
		out.Factors.Variables[i] = copyVar(old.Variables[index])
	}
	out.Causes = append([]core.ID{}, old.Causes...)
	out.Applied = append([]dynamics.Receipt{}, old.Applied...)
	out.Upgrade = &UpgradeRecord{Version: 1, SourceRegistry: dynamics.RegistryVersion, SourceHash: hash, NewBranch: newBranch}
	return out, out.Validate()
}
