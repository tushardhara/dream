package hws

import (
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator"
	"github.com/tushardhara/dream/simulator/drives"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"github.com/tushardhara/dream/simulator/scenario"
	"strings"
	"testing"
)

type drivePerception struct{ deny bool }

func (d drivePerception) PerceiveDrives(i rt.Input) (drives.Observation, error) {
	p, e := (syntheticPerception{deny: d.deny}).Perceive(i)
	o := drives.Observation{Event: p}
	for j := range o.Context {
		o.Context[j] = drives.Cue{Evidence: p, Value: .4}
	}
	return o, e
}
func TestDriveRuntimeReplayAndVersionBoundary(t *testing.T) {
	s := syntheticRuntime(t)
	h := DriveAppraisalHandler{Source: drivePerception{}}
	next, tr, _, e := rt.Apply(s, rt.Command{Kind: "step"}, h)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(next.Data, "CANARY") || len(tr.Draws) != 0 {
		t.Fatal("private prose/RNG leak")
	}
	c, e := DecodeDriveCheckpoint(next.Data)
	if e != nil || len(c.Actors[0].Variables) != 22 {
		t.Fatal("registry checkpoint", e)
	}
	raw, _ := json.Marshal(next)
	var restarted rt.State
	if json.Unmarshal(raw, &restarted) != nil {
		t.Fatal("restart decode")
	}
	a, _, _, e := rt.Apply(next, rt.Command{Kind: "step"}, h)
	if e != nil {
		t.Fatal(e)
	}
	b, _, _, e := rt.Apply(restarted, rt.Command{Kind: "step"}, h)
	if e != nil {
		t.Fatal(e)
	}
	ha, _ := a.Hash()
	hb, _ := b.Hash()
	if ha != hb {
		t.Fatal("drive restart replay changed")
	}
	if _, _, _, e := rt.Apply(s, rt.Command{Kind: "step"}, DriveAppraisalHandler{Source: drivePerception{deny: true}}); e == nil {
		t.Fatal("revoked source")
	}
	old, _, _, e := rt.Apply(s, rt.Command{Kind: "step"}, AppraisalHandler{Source: syntheticPerception{}})
	if e != nil {
		t.Fatal(e)
	}
	if _, _, _, e := rt.Apply(old, rt.Command{Kind: "step"}, h); e == nil {
		t.Fatal("old checkpoint silently reinterpreted")
	}
	if _, _, _, e := rt.Apply(next, rt.Command{Kind: "step"}, AppraisalHandler{Source: syntheticPerception{}}); e == nil {
		t.Fatal("new checkpoint decoded as old")
	}
}

func TestDriveInitialRegistryAndCheckpointBudget(t *testing.T) {
	for _, definition := range drives.Registry() {
		s := syntheticRuntime(t)
		var sc scenario.Scenario
		if json.Unmarshal(s.Genesis.Payload, &sc) != nil {
			t.Fatal("scenario")
		}
		sc.Research.Latent = []scenario.Latent{{Actor: "a", Drives: []simulator.Drive{{Kind: definition.ID, Strength: .9}}}}
		genesis, e := sc.Genesis(rt.Capabilities())
		if e != nil {
			t.Fatal(e)
		}
		s, e = rt.New(genesis, s.Budget)
		if e != nil {
			t.Fatal(e)
		}
		s.Status = "running"
		if _, _, _, e = rt.Apply(s, rt.Command{Kind: "step"}, DriveAppraisalHandler{Source: drivePerception{}}); e != nil {
			t.Fatal(definition.ID, e)
		}
	}
	s := syntheticRuntime(t)
	next, _, _, e := rt.Apply(s, rt.Command{Kind: "step"}, DriveAppraisalHandler{Source: drivePerception{}})
	if e != nil {
		t.Fatal(e)
	}
	c, e := DecodeDriveCheckpoint(next.Data)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 6; i++ {
		actor := c.Actors[0]
		actor.Actor = core.ID(fmt.Sprintf("extra:%d", i))
		c.Actors = append(c.Actors, actor)
	}
	if _, e := c.Canonical(); e == nil {
		t.Fatal("runtime checkpoint byte cap removed")
	}
}
