package hws

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tushardhara/dream/simulator/demo"
	rt "github.com/tushardhara/dream/simulator/runtime"
)

func demoArtifact(t *testing.T) DemoArtifact {
	t.Helper()
	return demoArtifactFor(t, 4, false)
}
func demoArtifactFor(t *testing.T, people int, relational bool) DemoArtifact {
	t.Helper()
	sc, e := demo.Scenario(4, 1, 11)
	if relational {
		sc, e = demo.RelationalScenario(people, 1, 11)
	}
	if e != nil {
		t.Fatal(e)
	}
	g, e := sc.Genesis(rt.Capabilities())
	if e != nil {
		t.Fatal(e)
	}
	state, e := rt.New(g, rt.Budgets{Steps: 20, Events: 20, Horizon: sc.World.Horizon})
	if e != nil {
		t.Fatal(e)
	}
	b := ReplayBundle{Snapshot: frozenFixture(t, state)}
	b = appendReplay(t, b, rt.Command{Kind: "resume"}, demo.Handler{})
	for range 2 {
		b = appendReplay(t, b, rt.Command{Kind: "run-until", Until: sc.World.Horizon}, demo.Handler{})
	}
	final := b.Frames[len(b.Frames)-1].State
	w, e := demo.Projection(final)
	if e != nil {
		t.Fatal(e)
	}
	hash, _ := w.Hash()
	a, e := SealDemo(DemoArtifact{Version: w.Version, Replay: b, WorldHash: hash, Completed: final.Status == "completed", SimulatedHorizon: sc.World.Horizon, RealStudy: "not-run", HumanValidity: "not-tested", CrossModel: "not-tested", ProviderMode: "deterministic_fake"})
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func TestDemoArtifactReplayAndPrivateExport(t *testing.T) {
	a := demoArtifact(t)
	w, e := VerifyDemo(a, a.Hash)
	if e != nil || !a.Completed || w.Period != 2 {
		t.Fatal("demo replay incomplete", e)
	}
	out, e := ExportDemoActor(a, a.Hash, "person:01")
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(out)
	for _, hidden := range []string{"OWN_FICTIONAL_NOTE:person:02", "DEMO_RESEARCH_LABEL_CANARY", "research", "future"} {
		if strings.Contains(string(raw), hidden) {
			t.Fatal("foreign view leak", hidden)
		}
	}
	if !strings.Contains(string(raw), "OWN_FICTIONAL_NOTE:person:01") {
		t.Fatal("own fictional evidence removed")
	}
	if _, e = ExportDemoActor(a, a.Hash, "outsider"); e == nil {
		t.Fatal("unknown actor exported")
	}
}
func TestDemoCannotClaimRealStudyOrAcceptCorruption(t *testing.T) {
	for _, which := range []string{"study", "human", "cross_model", "complete", "world", "draw"} {
		t.Run(which, func(t *testing.T) {
			a := demoArtifact(t)
			switch which {
			case "study":
				a.RealStudy = "completed"
			case "human":
				a.HumanValidity = "pass"
			case "cross_model":
				a.CrossModel = "pass"
			case "complete":
				a.Completed = false
			case "world":
				a.WorldHash = strings.Repeat("0", 64)
			case "draw":
				a.Replay.Frames[1].Transition.Draws[0].Value++
			}
			a, _ = SealDemo(a)
			if _, e := VerifyDemo(a, a.Hash); e == nil {
				t.Fatal("re-sealed false claim/corruption accepted")
			}
		})
	}
}

func TestDemoArtifactCanonicalWireRoundTrip(t *testing.T) {
	a := demoArtifact(t)
	raw, e := json.Marshal(a)
	if e != nil {
		t.Fatal(e)
	}
	var recovered DemoArtifact
	if json.Unmarshal(raw, &recovered) != nil {
		t.Fatal("wire decode")
	}
	if _, e = VerifyDemo(recovered, a.Hash); e != nil {
		t.Fatal("persisted replay changed", e)
	}
}

func TestRelationalDemoActorExportAndVersionBinding(t *testing.T) {
	for _, n := range []int{5, 24} {
		a := demoArtifactFor(t, n, true)
		out, e := ExportDemoActor(a, a.Hash, "person:02")
		if e != nil {
			t.Fatal(e)
		}
		if out.Version != demo.RelationalVersion || out.ActionState == nil || out.ActionState.Drives.Actor != "person:02" || out.State != nil || len(out.ActionDecisions) != 2 {
			t.Fatal("wrong actor/version projection")
		}
		for _, d := range out.ActionDecisions {
			if d.Actor != "person:02" {
				t.Fatal("foreign private decision")
			}
		}
		for _, o := range out.ActionOutcomes {
			if o.Outcome.Observer != "person:02" {
				t.Fatal("foreign private outcome")
			}
		}
		for _, c := range out.View.Contexts {
			if c.Observer != "person:02" || n == 5 && c.Other == "person:05" {
				t.Fatal("foreign relationship/unknown W-B")
			}
		}
		raw, _ := json.Marshal(out)
		for _, secret := range []string{"DEMO_RESEARCH_LABEL_CANARY", "OWN_FICTIONAL_NOTE:person:01", "OWN_FICTIONAL_NOTE:person:03"} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("foreign private state export", secret)
			}
		}
		a.Version = demo.Version
		a, _ = SealDemo(a)
		if _, e = VerifyDemo(a, a.Hash); e == nil {
			t.Fatal("v2 replay relabelled v1")
		}
	}
}

func TestResponsiveDemoExportsOnlyOwnResponseHistory(t *testing.T) {
	sc, e := demo.ResponsiveScenario(5, 2, 11)
	if e != nil {
		t.Fatal(e)
	}
	g, e := sc.Genesis(rt.Capabilities())
	if e != nil {
		t.Fatal(e)
	}
	state, e := rt.New(g, rt.Budgets{Steps: 20, Events: 20, Horizon: sc.World.Horizon})
	if e != nil {
		t.Fatal(e)
	}
	bundle := ReplayBundle{Snapshot: frozenFixture(t, state)}
	bundle = appendReplay(t, bundle, rt.Command{Kind: "resume"}, demo.Handler{})
	for range sc.Future {
		bundle = appendReplay(t, bundle, rt.Command{Kind: "step"}, demo.Handler{})
	}
	final := bundle.Frames[len(bundle.Frames)-1].State
	world, e := demo.Projection(final)
	if e != nil {
		t.Fatal(e)
	}
	hash, _ := world.Hash()
	artifact, e := SealDemo(DemoArtifact{Version: world.Version, Replay: bundle, WorldHash: hash, Completed: final.Status == "completed", SimulatedHorizon: sc.World.Horizon, RealStudy: "not-run", HumanValidity: "not-tested", CrossModel: "not-tested", ProviderMode: "deterministic_fake"})
	if e != nil {
		t.Fatal(e)
	}
	count := 0
	for _, person := range sc.Public.Humans {
		export, e := ExportDemoActor(artifact, artifact.Hash, person.ID)
		if e != nil {
			t.Fatal(e)
		}
		if export.Version != demo.ResponsiveVersion || export.ScopedState == nil || export.ScopedState.Human.Drives.Actor != person.ID {
			t.Fatal("wrong current version/owner")
		}
		for _, o := range export.OutcomeObservations {
			count++
			if o.Meta.Observer != person.ID {
				t.Fatal("foreign private outcome exported")
			}
		}
		for _, d := range export.RecipientDecisions {
			if d.Observation.Meta.Observer != person.ID {
				t.Fatal("foreign private recipient appraisal exported")
			}
		}
		raw, _ := json.Marshal(export)
		if strings.Contains(string(raw), "DEMO_RESEARCH_LABEL_CANARY") {
			t.Fatal("research label exported")
		}
		for _, o := range world.OutcomeObservations {
			if o.Meta.Observer != person.ID && strings.Contains(string(raw), string(o.Meta.ID)) {
				t.Fatal("foreign private receipt identity exported")
			}
		}
	}
	if count != len(world.OutcomeObservations) || count == 0 {
		t.Fatal("response history missing from own exports")
	}
}
