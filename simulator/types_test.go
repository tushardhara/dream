package simulator_test

import (
	"math"
	"testing"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator"
)

type clock struct{}

func (clock) Now() core.LogicalTime { return 12 }

type rng struct{}

func (rng) Uint64() uint64 { return 42 }

var _ simulator.VirtualClock = clock{}
var _ simulator.RNG = rng{}

func TestSimulatorLocalContracts(t *testing.T) {
	step := simulator.SimulationStep{World: "world", Run: "run", Branch: "branch", Index: 0, At: 12}
	k := simulator.KnowledgeFact{Actor: "alice", Record: "event", LearnedAt: 11, Step: step}
	if err := k.Validate(); err != nil {
		t.Fatal(err)
	}
	k.LearnedAt = 13
	if k.Validate() == nil {
		t.Fatal("future knowledge accepted")
	}
	k.LearnedAt = -1
	if k.Validate() == nil {
		t.Fatal("negative knowledge time accepted")
	}
	step.World = ""
	if step.Validate() == nil {
		t.Fatal("missing world accepted")
	}
	if (simulator.RunID("invalid/id")).Validate() == nil {
		t.Fatal("invalid run accepted")
	}
	if (simulator.BranchID("")).Validate() == nil {
		t.Fatal("invalid branch accepted")
	}
	for _, v := range []float64{math.NaN(), math.Inf(1), 2, -2} {
		if (simulator.Emotion{Valence: v}).Validate() == nil {
			t.Fatal("invalid valence accepted")
		}
	}
	for _, v := range []float64{math.NaN(), math.Inf(-1), 1.1, -0.1} {
		if (simulator.Drive{Kind: "rest", Strength: v}).Validate() == nil {
			t.Fatal("invalid drive accepted")
		}
	}
	if (simulator.Emotion{Valence: -0.5, Arousal: 0.5}).Validate() != nil || (simulator.Drive{Kind: "rest", Strength: 0.5}).Validate() != nil {
		t.Fatal("valid bounded state rejected")
	}
}
