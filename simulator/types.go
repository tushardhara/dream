// Package simulator owns synthetic run identity, logical knowledge and latent
// state contracts. These types do not imply a working runtime or human realism.
package simulator

import (
	"fmt"
	"math"

	"github.com/tushardhara/dream/core"
)

type WorldID string
type RunID string
type BranchID string

func (v WorldID) Validate() error  { return core.ID(v).Validate() }
func (v RunID) Validate() error    { return core.ID(v).Validate() }
func (v BranchID) Validate() error { return core.ID(v).Validate() }

type SimulationStep struct {
	World  WorldID          `json:"world"`
	Run    RunID            `json:"run"`
	Branch BranchID         `json:"branch"`
	Index  uint64           `json:"index"`
	At     core.LogicalTime `json:"at"`
}

func (s SimulationStep) Validate() error {
	if err := s.World.Validate(); err != nil {
		return err
	}
	if err := s.Run.Validate(); err != nil {
		return err
	}
	if err := s.Branch.Validate(); err != nil {
		return err
	}
	return s.At.Validate()
}

// KnowledgeFact says when an actor learned a record, independently of its valid,
// occurred and system-recorded times. Knowledge does not grant disclosure rights.
type KnowledgeFact struct {
	Actor     core.ID          `json:"actor"`
	Record    core.ID          `json:"record"`
	LearnedAt core.LogicalTime `json:"learned_at"`
	Step      SimulationStep   `json:"step"`
}

func (k KnowledgeFact) Validate() error {
	if err := k.Actor.Validate(); err != nil {
		return err
	}
	if err := k.Record.Validate(); err != nil {
		return err
	}
	if err := k.LearnedAt.Validate(); err != nil {
		return err
	}
	if err := k.Step.Validate(); err != nil {
		return err
	}
	if k.LearnedAt > k.Step.At {
		return fmt.Errorf("knowledge learned after enclosing step")
	}
	return nil
}

// VirtualClock and RNG are injected at the simulator consumer. Operational
// lease clocks and provider randomness do not implement logical replay semantics.
type VirtualClock interface{ Now() core.LogicalTime }
type RNG interface{ Uint64() uint64 }

type Emotion struct {
	Valence float64 `json:"valence"`
	Arousal float64 `json:"arousal"`
}

func (e Emotion) Validate() error {
	if math.IsNaN(e.Valence) || math.IsInf(e.Valence, 0) || e.Valence < -1 || e.Valence > 1 {
		return fmt.Errorf("invalid valence")
	}
	return core.Confidence(e.Arousal).Validate()
}

type Drive struct {
	Kind     core.ID `json:"kind"`
	Strength float64 `json:"strength"`
}

func (d Drive) Validate() error {
	if err := d.Kind.Validate(); err != nil {
		return err
	}
	return core.Confidence(d.Strength).Validate()
}
