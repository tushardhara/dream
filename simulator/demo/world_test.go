package demo

import (
	"encoding/json"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"strings"
	"testing"
)

func runWorld(t *testing.T, people, months int, seed uint64) rt.State {
	t.Helper()
	sc, e := Scenario(people, months, seed)
	if e != nil {
		t.Fatal(e)
	}
	g, e := sc.Genesis(rt.Capabilities())
	if e != nil {
		t.Fatal(e)
	}
	state, e := rt.New(g, rt.Budgets{Steps: 100, Events: 100, Horizon: sc.World.Horizon})
	if e != nil {
		t.Fatal(e)
	}
	state, _, _, e = rt.Apply(state, rt.Command{Kind: "resume"}, Handler{})
	if e != nil {
		t.Fatal(e)
	}
	for range 2 * months {
		before, _ := state.Hash()
		next, tr, _, e := rt.Apply(state, rt.Command{Kind: "step"}, Handler{})
		if e != nil {
			t.Fatal(e)
		}
		if len(tr.Draws) != people || len(next.Data) > 4096 {
			t.Fatal("runtime bounds or recorded draws")
		}
		raw, _ := json.Marshal(state)
		var restarted rt.State
		_ = json.Unmarshal(raw, &restarted)
		again, _, _, e := rt.Apply(restarted, rt.Command{Kind: "step"}, Handler{})
		ah, _ := again.Hash()
		nh, _ := next.Hash()
		if e != nil || ah != nh {
			t.Fatal("restart changed transition", e)
		}
		after, _ := state.Hash()
		if before != after {
			t.Fatal("transition mutated input")
		}
		state = next
	}
	return state
}
func TestTwentyFourPersonYearReconstructionAndRecovery(t *testing.T) {
	state := runWorld(t, 24, 12, 11)
	world, e := Projection(state)
	if e != nil {
		t.Fatal(e)
	}
	if world.Period != 24 || len(world.Actors) != 24 || len(world.Decisions) != 576 {
		t.Fatal("incomplete simulated year")
	}
	for _, a := range world.Actors {
		if len(a.State.Applied) != 24 || a.Validate() != nil {
			t.Fatal("receipt bound/evolution")
		}
	}
	if world.Resources["shared-time"] < 0 {
		t.Fatal("overspent shared resources")
	}
	state.Data = strings.Replace(state.Data, "world_hash", "wrong_hash", 1)
	if _, e := Projection(state); e == nil {
		t.Fatal("tampered derived checkpoint")
	}
}
func TestSmokeSeedsAndHiddenResearch(t *testing.T) {
	a := runWorld(t, 4, 2, 11)
	b := runWorld(t, 4, 2, 23)
	wa, _ := Projection(a)
	wb, _ := Projection(b)
	ha, _ := wa.Hash()
	hb, _ := wb.Hash()
	if ha == hb {
		t.Fatal("choice seeds ignored")
	}
	sc, e := Scenario(4, 2, 11)
	if e != nil {
		t.Fatal(e)
	}
	original, e := reconstruct(sc, 4, "", "", false, nil)
	if e != nil {
		t.Fatal(e)
	}
	sc.Research.Labels[0].Text = "CHANGED_LABEL_MUST_NOT_CHANGE_ACTORS"
	changed, e := reconstruct(sc, 4, "", "", false, nil)
	if e != nil {
		t.Fatal(e)
	}
	x, _ := original.Hash()
	y, _ := changed.Hash()
	if x != y {
		t.Fatal("research label affected generation")
	}
	raw, _ := json.Marshal(original.Decisions)
	if strings.Contains(string(raw), "OWN_FICTIONAL_NOTE") || strings.Contains(string(raw), "CANARY") {
		t.Fatal("private prose leaked into decisions")
	}
}

func TestDemoInitialDerivationAndUnseenGroup(t *testing.T) {
	sc, e := Scenario(24, 1, 11)
	if e != nil {
		t.Fatal(e)
	}
	before, e := reconstruct(sc, 1, "", "", false, nil)
	if e != nil {
		t.Fatal(e)
	}
	var p Period
	_ = json.Unmarshal([]byte(sc.Future[0].Text), &p)
	p.Theme = "compound"
	raw, _ := json.Marshal(p)
	sc.Future[0].Text = string(raw)
	after, e := reconstruct(sc, 1, "", "", false, nil)
	if e != nil {
		t.Fatal(e)
	}
	x, _ := digest(before.Actors[23])
	y, _ := digest(after.Actors[23])
	if x != y {
		t.Fatal("unseen group event changed outsider state")
	}
	sc.Actors[0].Facts[0].Grants = sc.Actors[0].Facts[0].Grants[:1]
	if sc.Validate() != nil {
		t.Fatal("read-only fixture must remain schema-valid")
	}
	if _, e = reconstruct(sc, 1, "", "", false, nil); e == nil {
		t.Fatal("read permission silently broadened to derive")
	}
}

// demoStateWithVersion builds a schema-valid five-person relational state whose
// every future period declares the given demo version. The version is set before
// Genesis is computed, because the genesis envelope is hash-bound: a payload
// edited after the fact fails Genesis.Validate long before Projection's guard.
func demoStateWithVersion(t *testing.T, version string) rt.State {
	t.Helper()
	sc, e := RelationalScenario(5, 1, 11)
	if e != nil {
		t.Fatal(e)
	}
	for i := range sc.Future {
		var p Period
		if json.Unmarshal([]byte(sc.Future[i].Text), &p) != nil {
			t.Fatal("unreadable period fixture")
		}
		p.Version = version
		raw, e := json.Marshal(p)
		if e != nil {
			t.Fatal(e)
		}
		sc.Future[i].Text = string(raw)
	}
	if e = sc.Validate(); e != nil {
		t.Fatal("fixture stopped being schema-valid", e)
	}
	g, e := sc.Genesis(rt.Capabilities())
	if e != nil {
		t.Fatal(e)
	}
	state, e := rt.New(g, rt.Budgets{Steps: 100, Events: 100, Horizon: sc.World.Horizon})
	if e != nil {
		t.Fatal(e)
	}
	return state
}

// A scenario carrying a demo version this build does not know must be refused by
// Projection rather than reconstructed on a guessed pipeline. The guard was
// removable with the whole suite green (#74). Ablating it shows why: Projection
// then returns a nil error and reconstructs the unknown version on the legacy
// pipeline, so this guard is the only thing refusing it. The assertion pins the
// exact message rather than merely "an error" so that the guard cannot later rot
// behind an unrelated failure that happens to reject the same fixture.
func TestUnsupportedDemoVersionIsRejected(t *testing.T) {
	const want = "unsupported demo version"
	supported := demoStateWithVersion(t, RelationalVersion)
	if _, e := Projection(supported); e != nil {
		t.Fatal("supported demo version rejected", e)
	}
	for _, version := range []string{"backend-demo.v99", "backend-demo.v0", GroupVersion, ""} {
		t.Run(version, func(t *testing.T) {
			_, e := Projection(demoStateWithVersion(t, version))
			if e == nil {
				t.Fatal("unsupported demo version accepted")
			}
			if e.Error() != want {
				t.Fatal("wrong guard reached: want "+want+", got", e)
			}
		})
	}
}
