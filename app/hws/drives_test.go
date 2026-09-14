package hws

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/drives"
	"github.com/tushardhara/dream/simulator/dynamics"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"github.com/tushardhara/dream/simulator/scenario"
	"reflect"
	"strings"
	"testing"
	"time"
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

func TestUpgradeLegacyIsNotBranchAuthority(t *testing.T) {
	old, e := dynamics.New("a", 0, dynamics.DefaultSubstrate())
	if e != nil {
		t.Fatal(e)
	}
	upgrade, e := drives.UpgradeLegacy(old, "claimed-child")
	if e != nil {
		t.Fatal(e)
	}
	// A well-formed provenance string grants no Derive right, even when it is
	// used as the requested branch ID. The authenticated service never reaches DB.
	runtime, journal, realm, grants := viewFixture(t)
	views, e := NewViewService(runtime, journal, viewClock{}, grants, viewAudit{})
	if e != nil {
		t.Fatal(e)
	}
	permit, e := views.Permit("researcher", realm, ResearchViewKind, "research")
	if e != nil {
		t.Fatal(e)
	}
	port := &snapshotPort{}
	svc := SnapshotService{Store: port, Views: views}
	childScope := realm.Scope
	childScope.Branch, childScope.Run = simulator.BranchID(upgrade.Upgrade.NewBranch), "claimed-run"
	spec := ForkSpec{Version: 1, Source: SnapshotKey{Scope: realm.Scope, ID: "snapshot", Hash: strings.Repeat("a", 64)}, Child: childScope, Mode: FreshSimulation, Policy: behavior.Policy, MaxDuration: time.Minute}
	if e := spec.Validate(); e != nil {
		t.Fatal("invalid negative control", e)
	}
	if _, e = svc.Fork(context.Background(), permit, spec); e == nil || port.calls != 0 {
		t.Fatal("upgrade provenance granted fork authority", e)
	}
	// The existing fork only supports the frozen cognitive policy. An actor
	// conversion cannot select a new runtime policy or replace a frozen snapshot.
	parent := syntheticRuntime(t)
	frozen := frozenFixture(t, parent)
	spec.Source = SnapshotKey{Scope: frozen.Scope, ID: "snapshot", Hash: frozen.Hash}
	spec.Child = frozen.Scope
	spec.Child.Branch, spec.Child.Run = simulator.BranchID(upgrade.Upgrade.NewBranch), "claimed-run"
	spec.Policy = drives.ModelVersion
	if _, e := ForkState(frozen, spec); e == nil {
		t.Fatal("drive upgrade silently selected fork policy")
	}
	spec.Policy = behavior.Policy
	child, e := ForkState(frozen, spec)
	if e != nil {
		t.Fatal("authorized-policy pure fork control", e)
	}
	if child.Budget != parent.Budget || child.Step != parent.Step || child.Events != parent.Events || child.At != parent.At || child.Data != parent.Data || !reflect.DeepEqual(child.Available, parent.Available) {
		t.Fatal("pure fork reset runtime accounting")
	}
	// Even a trusted host manually installing an upgraded actor payload cannot
	// replenish the enclosing runtime's step/event/horizon ceilings.
	raw, e := upgrade.Canonical()
	if e != nil {
		t.Fatal(e)
	}
	for _, ceiling := range []string{"steps", "events", "horizon"} {
		s := parent
		s.Data = string(raw)
		switch ceiling {
		case "steps":
			s.Step = s.Budget.Steps
		case "events":
			s.Events = s.Budget.Events
		case "horizon":
			if _, _, _, e := rt.Apply(s, rt.Command{Kind: "run-until", Until: s.Budget.Horizon + 1}, DriveAppraisalHandler{Source: drivePerception{}}); e == nil {
				t.Fatal("upgrade bypassed horizon")
			}
			continue
		}
		n, tr, _, e := rt.Apply(s, rt.Command{Kind: "step"}, DriveAppraisalHandler{Source: drivePerception{}})
		if e != nil || n.Status != "budget" || tr != nil || n.Step != s.Step || n.Events != s.Events {
			t.Fatal("upgrade bypassed "+ceiling, e)
		}
	}
	if _, e := DecodeDriveCheckpoint(string(raw)); e == nil {
		t.Fatal("actor conversion accepted as runtime checkpoint")
	}
}
