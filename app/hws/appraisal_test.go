package hws

import (
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator"
	"github.com/tushardhara/dream/simulator/dynamics"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"github.com/tushardhara/dream/simulator/scenario"
	"strings"
	"testing"
)

type syntheticPerception struct {
	deny    bool
	foreign bool
}

func (s syntheticPerception) Perceive(i rt.Input) (dynamics.Perceived, error) {
	p := dynamics.Perceived{Event: i.ID, Actor: i.Actor, OccurredAt: i.At, LearnedAt: i.At, Confidence: .8, Signals: dynamics.Signals{Effort: .5, OtherNeed: .7}, Rights: core.Rights{Resource: i.ID, Grants: []core.Grant{{Actor: i.Actor, Recipient: i.Actor, Purpose: "simulation", Operation: core.Read}, {Actor: i.Actor, Recipient: i.Actor, Purpose: "simulation", Operation: core.Derive}}}}
	if s.deny {
		p.Rights.Revoked = true
	}
	if s.foreign {
		p.Actor = "other"
	}
	return p, nil
}
func syntheticRuntime(t testing.TB) rt.State {
	t.Helper()
	sc := scenario.Scenario{Version: 1, World: scenario.World{ID: "appraisal-world", Seed: 42, Horizon: 100}, Public: scenario.Public{Humans: []scenario.Human{{ID: "a", Name: "Synthetic A", Age: 30}, {ID: "b", Name: "Synthetic B", Age: 40}}}, Actors: []scenario.Actor{{ID: "a"}, {ID: "b"}}, Future: []scenario.Scheduled{{ID: "e1", At: 10, Kind: "observation", Actor: "a", Text: "RAW_PRIVATE_TEXT_CANARY"}, {ID: "e2", At: 20, Kind: "observation", Actor: "b", Text: "second"}}}
	g, err := sc.Genesis(rt.Capabilities())
	if err != nil {
		t.Fatal(err)
	}
	s, err := rt.New(g, rt.Budgets{Steps: 10, Events: 10, Horizon: 100})
	if err != nil {
		t.Fatal(err)
	}
	s.Status = "running"
	return s
}
func TestAppraisalRuntimeRestartAndBoundary(t *testing.T) {
	s := syntheticRuntime(t)
	h := AppraisalHandler{Source: syntheticPerception{}}
	next, tr, _, err := rt.Apply(s, rt.Command{Kind: "step"}, h)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(next.Data, "CANARY") || len(tr.Draws) != 0 {
		t.Fatal("raw rationale leak or unrequested RNG")
	}
	checkpoint, err := DecodeAppraisalCheckpoint(next.Data)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.Last == nil || checkpoint.Last.Cause != "e1" || checkpoint.Actors[0].Variables[dynamics.Fatigue].Level <= .2 {
		t.Fatal("appraisal not applied")
	}
	persisted, err := json.Marshal(next)
	if err != nil {
		t.Fatal(err)
	}
	var restart rt.State
	_ = json.Unmarshal(persisted, &restart)
	a, _, _, err := rt.Apply(next, rt.Command{Kind: "step"}, h)
	if err != nil {
		t.Fatal(err)
	}
	b, _, _, err := rt.Apply(restart, rt.Command{Kind: "step"}, AppraisalHandler{Source: syntheticPerception{}})
	if err != nil {
		t.Fatal(err)
	}
	ha, _ := a.Hash()
	hb, _ := b.Hash()
	if ha != hb {
		t.Fatal("appraisal restart changed trajectory")
	}
	for _, src := range []syntheticPerception{{deny: true}, {foreign: true}} {
		if _, _, _, err := rt.Apply(s, rt.Command{Kind: "step"}, AppraisalHandler{Source: src}); err == nil {
			t.Fatal("perception boundary bypass")
		}
	}
	checkpoint.Last.Codes = []string{"unrestricted private rationale"}
	raw, _ := json.Marshal(checkpoint)
	if _, err = DecodeAppraisalCheckpoint(string(raw)); err == nil {
		t.Fatal("unrestricted rationale accepted")
	}
}
func TestAppraisalParameterCoverageFailsClosed(t *testing.T) {
	for _, kind := range []string{"unknown-drive", "emotion"} {
		s := syntheticRuntime(t)
		var sc scenario.Scenario
		_ = json.Unmarshal(s.Genesis.Payload, &sc)
		sc.Research.Latent = []scenario.Latent{{Actor: "a"}}
		if kind == "emotion" {
			sc.Research.Latent[0].Emotion.Arousal = .5
		} else {
			sc.Research.Latent[0].Drives = append(sc.Research.Latent[0].Drives, simulator.Drive{Kind: core.ID(kind), Strength: .5})
		}
		g, err := sc.Genesis(rt.Capabilities())
		if err != nil {
			t.Fatal(err)
		}
		s.Genesis = g
		if _, _, _, err = rt.Apply(s, rt.Command{Kind: "step"}, AppraisalHandler{Source: syntheticPerception{}}); err == nil {
			t.Fatalf("silently ignored %s", kind)
		}
	}
}
func TestCheckpointBudgetAndStageValidation(t *testing.T) {
	s := syntheticRuntime(t)
	n, _, _, err := rt.Apply(s, rt.Command{Kind: "step"}, AppraisalHandler{Source: syntheticPerception{}})
	if err != nil {
		t.Fatal(err)
	}
	c, err := DecodeAppraisalCheckpoint(n.Data)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 32; i++ {
		id := core.ID(fmt.Sprintf("long-event-%03d-abcdefghijklmnopqrstuvwxyz", i))
		c.Actors[1].Applied = append(c.Actors[1].Applied, dynamics.Receipt{Event: id, Digest: strings.Repeat("a", 64)})
	}
	if _, err = c.canonical(); err == nil {
		t.Fatal("runtime byte cap weakened")
	}
	if _, err = DecodeAppraisalCheckpoint(strings.Repeat(" ", 4097)); err == nil {
		t.Fatal("unbounded checkpoint")
	}
}
