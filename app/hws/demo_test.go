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
	sc, e := demo.Scenario(4, 1, 11)
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
	a, e := SealDemo(DemoArtifact{Version: demo.Version, Replay: b, WorldHash: hash, Completed: final.Status == "completed", SimulatedHorizon: sc.World.Horizon, RealStudy: "not-run", HumanValidity: "not-tested", CrossModel: "not-tested", ProviderMode: "deterministic_fake"})
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
