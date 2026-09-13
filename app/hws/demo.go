package hws

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/demo"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"github.com/tushardhara/dream/simulator/scenario"
)

type DemoStore interface {
	RuntimeStore
	SnapshotStore
}
type DemoOptions struct {
	Namespace     core.ID
	Holder        core.ID
	People        int
	Months        int
	Seed          uint64
	MaxBoundaries int
}
type DemoArtifact struct {
	Version          string           `json:"version"`
	Replay           ReplayBundle     `json:"replay"`
	WorldHash        string           `json:"world_hash"`
	Completed        bool             `json:"simulated_demo_completed"`
	SimulatedHorizon core.LogicalTime `json:"simulated_horizon_ns"`
	RealStudy        string           `json:"30_real_day_study"`
	HumanValidity    string           `json:"real_human_validity"`
	CrossModel       string           `json:"cross_model_validity"`
	ProviderMode     string           `json:"provider_mode"`
	ProviderCalls    int              `json:"live_provider_calls"`
	APICostMicros    int64            `json:"incurred_api_cost_micros"`
	Elapsed          time.Duration    `json:"measured_wall_runtime_ns"`
	Hash             string           `json:"hash"`
}

func SealDemo(a DemoArtifact) (DemoArtifact, error) {
	a.Hash = ""
	h, e := ModelDigest(a)
	a.Hash = h
	return a, e
}
func VerifyDemo(a DemoArtifact, expected string) (demo.World, error) {
	sealed, e := SealDemo(a)
	if e != nil || a.Hash != expected || a.Hash != sealed.Hash || a.Version != demo.Version || a.RealStudy != "not-run" || a.HumanValidity != "not-tested" || a.CrossModel != "not-tested" || a.ProviderMode != "deterministic_fake" || a.ProviderCalls != 0 || a.APICostMicros != 0 || a.Elapsed < 0 {
		return demo.World{}, fmt.Errorf("invalid demo artifact or scientific claim")
	}
	replay, e := Replay(a.Replay, RecordedReplay)
	if e != nil {
		return demo.World{}, e
	}
	// Recorded replay checks the runtime journal. Reconstruction additionally checks
	// the demo's derived state against its immutable inputs and seeded randomness.
	for _, frame := range a.Replay.Frames {
		if _, e = demo.Projection(frame.State); e != nil {
			return demo.World{}, e
		}
	}
	world, e := demo.Projection(replay.Final)
	if e != nil {
		return demo.World{}, e
	}
	hash, e := world.Hash()
	if e != nil || hash != a.WorldHash || a.SimulatedHorizon != replay.Final.Budget.Horizon || a.Completed != (replay.Final.Status == "completed") {
		return demo.World{}, fmt.Errorf("demo completion/projection mismatch")
	}
	return world, nil
}

// RunDemo is a trusted local synthetic operator use case, not a public role-header
// endpoint. Store connections must use the existing non-owner runtime role.
func RunDemo(ctx context.Context, store DemoStore, o DemoOptions, clock OperationalClock) (DemoArtifact, error) {
	start := time.Now()
	if store == nil || clock == nil || o.Namespace.Validate() != nil || o.Holder.Validate() != nil || o.MaxBoundaries < 1 || o.MaxBoundaries > 64 {
		return DemoArtifact{}, fmt.Errorf("invalid bounded demo configuration")
	}
	sc, e := demo.Scenario(o.People, o.Months, o.Seed)
	if e != nil {
		return DemoArtifact{}, e
	}
	genesis, e := sc.Genesis(rt.Capabilities())
	if e != nil {
		return DemoArtifact{}, e
	}
	scope := Scope{Actor: "demo-operator", Namespace: o.Namespace, World: sc.World.ID, Branch: "demo", Run: simulator.RunID(fmt.Sprintf("demo:%d:%d:%d", o.People, o.Months, o.Seed))}
	snapshot, e := store.CreateRun(ctx, Manifest{Version: 1, Scope: scope, Genesis: genesis, Budget: rt.Budgets{Steps: 100, Events: 100, Horizon: sc.World.Horizon}, MaxDuration: 5 * time.Minute})
	if e != nil {
		return DemoArtifact{}, e
	}
	root, e := store.CaptureSnapshot(ctx, scope, "demo-initial", 1)
	if e != nil {
		return DemoArtifact{}, e
	}
	if snapshot.State.Status != "completed" {
		lease, e := store.Acquire(ctx, scope, o.Holder, 30*time.Second)
		if e != nil {
			return DemoArtifact{}, e
		}
		// Shorten only our own live lease on a clean partial checkpoint, using the
		// existing fenced/audited renewal path. A crash retains the normal expiry.
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, _ = store.Renew(cleanup, scope, lease, time.Millisecond)
		}()
		service := Runtime{Store: store, Clock: clock, Handler: demo.Handler{}}
		if snapshot.State.Status == "paused" {
			if _, e = service.Execute(ctx, scope, lease, "demo-resume", rt.Command{Kind: "resume"}); e != nil {
				return DemoArtifact{}, e
			}
		}
		for i := 0; i < o.MaxBoundaries; i++ {
			lease, e = store.Renew(ctx, scope, lease, 30*time.Second)
			if e != nil {
				return DemoArtifact{}, e
			}
			receipt, e := service.Execute(ctx, scope, lease, "demo-run", rt.Command{Kind: "run-until", Until: sc.World.Horizon})
			if e != nil {
				return DemoArtifact{}, e
			}
			if receipt.Done {
				break
			}
		}
	}
	final, e := store.LoadRun(ctx, scope)
	if e != nil {
		return DemoArtifact{}, e
	}
	bundle, e := store.ReadReplay(ctx, root, final.Revision)
	if e != nil {
		return DemoArtifact{}, e
	}
	world, e := demo.Projection(final.State)
	if e != nil {
		return DemoArtifact{}, e
	}
	hash, e := world.Hash()
	if e != nil {
		return DemoArtifact{}, e
	}
	artifact, e := SealDemo(DemoArtifact{Version: demo.Version, Replay: bundle, WorldHash: hash, Completed: final.State.Status == "completed", SimulatedHorizon: sc.World.Horizon, RealStudy: "not-run", HumanValidity: "not-tested", CrossModel: "not-tested", ProviderMode: "deterministic_fake", Elapsed: time.Since(start)})
	if e != nil {
		return DemoArtifact{}, e
	}
	_, e = VerifyDemo(artifact, artifact.Hash)
	return artifact, e
}

type DemoActorExport struct {
	Version       string              `json:"version"`
	View          scenario.ActorView  `json:"initial_own_view"`
	State         behavior.Actor      `json:"derived_own_state"`
	Decisions     []behavior.Decision `json:"own_decisions"`
	Deliveries    []demo.Delivery     `json:"pending_own_deliveries"`
	HumanValidity string              `json:"real_human_validity"`
}

// ExportDemoActor is an offline projection over an already authorized synthetic
// artifact. A network host must use the existing authenticated view permits.
func ExportDemoActor(a DemoArtifact, expected string, actor core.ID) (DemoActorExport, error) {
	world, e := VerifyDemo(a, expected)
	if e != nil {
		return DemoActorExport{}, e
	}
	var sc scenario.Scenario
	if json.Unmarshal(a.Replay.Snapshot.State.Genesis.Payload, &sc) != nil {
		return DemoActorExport{}, fmt.Errorf("invalid genesis")
	}
	view, e := sc.View(actor)
	if e != nil {
		return DemoActorExport{}, e
	}
	out := DemoActorExport{Version: demo.Version, View: view, HumanValidity: "not-tested"}
	found := false
	for _, state := range world.Actors {
		if state.State.Actor == actor {
			out.State = state
			found = true
		}
	}
	if !found {
		return DemoActorExport{}, fmt.Errorf("unknown actor")
	}
	for _, d := range world.Decisions {
		if d.Actor == actor {
			out.Decisions = append(out.Decisions, d)
		}
	}
	for _, d := range world.Pending {
		if d.Recipient == actor {
			out.Deliveries = append(out.Deliveries, d)
		}
	}
	return out, nil
}
