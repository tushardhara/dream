package hws

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator"
	"github.com/tushardhara/dream/simulator/dynamics"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"github.com/tushardhara/dream/simulator/scenario"
)

type viewClock struct{}

func (viewClock) Now() time.Time { return time.Unix(1000, 0) }

type viewRuntime struct {
	scope    Scope
	snapshot Snapshot
	calls    int
	deny     bool
}

func (r *viewRuntime) LoadRun(_ context.Context, s Scope) (Snapshot, error) {
	r.calls++
	if r.deny || s != r.scope {
		return Snapshot{}, errors.New("PRIVATE_RUNTIME_ERROR")
	}
	return r.snapshot, nil
}

type viewJournal struct {
	scope   graph.MemoryScope
	entries []graph.MemoryEntry
}

func (m *viewJournal) ReadMemory(_ context.Context, s graph.MemoryScope) ([]graph.MemoryEntry, error) {
	if s != m.scope {
		return nil, errors.New("wrong scope")
	}
	return m.entries, nil
}
func (m *viewJournal) AppendMemory(context.Context, graph.AppendCommand) (graph.AppendResult, error) {
	return graph.AppendResult{}, errors.New("test read port")
}
func viewFixture(t testing.TB) (*viewRuntime, *viewJournal, ViewRealm, []ViewGrant) {
	t.Helper()
	fact := func(id, actor core.ID, text string) scenario.Fact {
		return scenario.Fact{ID: id, Observer: actor, Subject: actor, Text: text, Confidence: .7, Valid: core.Interval{}, Grants: []core.Grant{{Actor: actor, Recipient: actor, Purpose: "simulation", Operation: core.Read}}}
	}
	sc := scenario.Scenario{Version: 1, World: scenario.World{ID: "world", Seed: 42, Horizon: 100}, Public: scenario.Public{Humans: []scenario.Human{{ID: "alice", Name: "Synthetic Alice", Age: 30}, {ID: "bob", Name: "Synthetic Bob", Age: 40}}}, Actors: []scenario.Actor{{ID: "alice", Facts: []scenario.Fact{fact("self", "alice", "OWN_ALLOWED")}, Knowledge: []scenario.Knowledge{{Record: "self", LearnedAt: 0}}}, {ID: "bob", Facts: []scenario.Fact{fact("bob-secret", "bob", "PRIVATE_BOB_CANARY")}, Knowledge: []scenario.Knowledge{{Record: "bob-secret", LearnedAt: 0}}}}, Research: scenario.Research{Latent: []scenario.Latent{{Actor: "alice", Emotion: simulator.Emotion{Valence: .2, Arousal: .3}, Drives: []simulator.Drive{{Kind: "OWN_DRIVE_ALLOWED", Strength: .4}}}, {Actor: "bob", Emotion: simulator.Emotion{Valence: -.5, Arousal: .8}, Drives: []simulator.Drive{{Kind: "HIDDEN_DRIVE_CANARY", Strength: .9}}}}, Labels: []scenario.Label{{ID: "label", Text: "LABEL_SCORE_CANARY"}}}, Future: []scenario.Scheduled{{ID: "future", At: 50, Kind: "observation", Actor: "alice", Text: "FUTURE_CANARY"}}}
	genesis, err := sc.Genesis(rt.Capabilities())
	if err != nil {
		t.Fatal(err)
	}
	state, err := rt.New(genesis, rt.Budgets{Steps: 20, Events: 20, Horizon: 100})
	if err != nil {
		t.Fatal(err)
	}
	state.At = 10
	realm := ViewRealm{Scope: Scope{Actor: "operator", Namespace: "study", World: "world", Branch: "base", Run: "run"}, Principal: "alice"}
	scope, err := realm.MemoryScope()
	if err != nil {
		t.Fatal(err)
	}
	entry := graph.MemoryEntry{Sequence: 1, Event: core.Event{Version: 1, Type: graph.MemoryEventType, Stream: "observations", Subject: core.Subject{Principal: "alice"}, OccurredAt: 1, Meta: core.Metadata{ID: "permitted", Observer: "alice", Source: "alice", Sensitivity: core.Restricted, Confidence: .7, Valid: core.Interval{}, RecordedAt: time.Unix(2, 0), Rights: core.Rights{Resource: "permitted", Grants: []core.Grant{{Actor: "alice", Recipient: "alice", Purpose: "simulation", Operation: core.Read}, {Actor: "alice", Recipient: "bob", Purpose: "simulation", Operation: core.Disclose}, {Actor: "helper", Recipient: "helper", Purpose: "simulation", Operation: core.Read}}}}}, Content: &graph.MemoryContent{Version: 1, Kind: graph.EpisodicMemory, Text: "APPROVED_CONTEXT_ONLY", Salience: .5, HalfLife: 100, Learned: []graph.Learned{{Actor: "alice", At: 1}, {Actor: "helper", At: 1}}}}
	grants := []ViewGrant{{Caller: "alice", Realm: realm, Kind: ActorViewKind, Purpose: "simulation", Operations: []core.Operation{core.Read, core.Disclose}}, {Caller: "helper", Realm: realm, Kind: ExternalViewKind, Purpose: "simulation", Operations: []core.Operation{core.Read}}, {Caller: "researcher", Realm: realm, Kind: ResearchViewKind, Purpose: "research", Operations: []core.Operation{core.Read}}}
	return &viewRuntime{scope: realm.Scope, snapshot: Snapshot{Revision: 1, State: state, Deadline: time.Unix(2000, 0)}}, &viewJournal{scope: scope, entries: []graph.MemoryEntry{entry}}, realm, grants
}
func TestActorResearchExternalBoundaries(t *testing.T) {
	runtime, journal, realm, grants := viewFixture(t)
	service, err := NewViewService(runtime, journal, viewClock{}, grants, viewAudit{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := service.Permit("alice", realm, ActorViewKind, "simulation")
	if err != nil {
		t.Fatal(err)
	}
	observation, self, err := service.Actor(context.Background(), actor)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(struct {
		Observation ActorObservation
		Self        ActorSelfState
	}{observation, self})
	text := string(raw)
	for _, canary := range []string{"PRIVATE_BOB_CANARY", "HIDDEN_DRIVE_CANARY", "LABEL_SCORE_CANARY", "FUTURE_CANARY", `"scenario_hash":`, `"genesis":`, `"deadline":`, `"seed":`, `"horizon":`} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(canary)) {
			t.Fatal("actor boundary leaked", canary)
		}
	}
	if !strings.Contains(text, "OWN_ALLOWED") || !strings.Contains(text, "OWN_DRIVE_ALLOWED") {
		t.Fatal("actor own cognition denied", text)
	}
	external, err := service.Permit("helper", realm, ExternalViewKind, "simulation")
	if err != nil {
		t.Fatal(err)
	}
	before := runtime.calls
	if _, err = service.Research(context.Background(), external); err == nil || runtime.calls != before {
		t.Fatal("external GodState request reached reader")
	}
	if _, _, err = service.Actor(context.Background(), external); err == nil {
		t.Fatal("external caller obtained actor self state")
	}
	view, err := service.External(context.Background(), external, []core.ID{"permitted"})
	if err != nil || len(view.Items) != 1 || view.Items[0].Text != "APPROVED_CONTEXT_ONLY" {
		t.Fatal(view, err)
	}
	if _, err = service.External(context.Background(), external, []core.ID{"label"}); err == nil {
		t.Fatal("external label access")
	}
	researcher, err := service.Permit("researcher", realm, ResearchViewKind, "research")
	if err != nil {
		t.Fatal(err)
	}
	god, err := service.Research(context.Background(), researcher)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(god)
	if !strings.Contains(string(raw), "LABEL_SCORE_CANARY") {
		t.Fatal("explicit research privilege vacuous")
	}
	if _, err = service.Permit("helper", realm, ResearchViewKind, "research"); err == nil {
		t.Fatal("external escalated research kind")
	}
	service.Revoke(external)
	if _, err = service.External(context.Background(), external, []core.ID{"permitted"}); err == nil {
		t.Fatal("revoked external permit usable")
	}
}
func TestViewScopeBindingAndCurrentRuntimeRevocation(t *testing.T) {
	runtime, journal, realm, grants := viewFixture(t)
	service, err := NewViewService(runtime, journal, viewClock{}, grants, viewAudit{})
	if err != nil {
		t.Fatal(err)
	}
	actor, _ := service.Permit("alice", realm, ActorViewKind, "simulation")
	key, _ := realm.Key()
	variants := []ViewRealm{realm, realm, realm, realm, realm}
	variants[0].Scope.Namespace = "other"
	variants[1].Scope.World = "other"
	variants[2].Scope.Branch = "other"
	variants[3].Principal = "bob"
	variants[4].Scope.Run = "other"
	for _, r := range variants {
		other, _ := r.Key()
		if key == other {
			t.Fatal("scope-key collision")
		}
		if _, err = service.Permit("alice", r, ActorViewKind, "simulation"); err == nil {
			t.Fatal("scope rebound")
		}
	}
	cap, d, err := service.Propose(context.Background(), actor, []core.ID{"permitted"}, "bob", core.Disclose, graph.SyntheticSelfDisclosure)
	if err != nil || !d.Allowed {
		t.Fatal(d, err)
	}
	runtime.deny = true
	if _, d, err = service.Write(context.Background(), actor, cap, nil); d.Allowed || (err != nil && strings.Contains(err.Error(), "PRIVATE")) {
		t.Fatal("runtime revocation/error leaked", d, err)
	}
	if _, _, err = service.Actor(context.Background(), actor); err == nil || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("runtime error disclosure", err)
	}
}

func TestActorSelfStateStripsOtherDynamicsAndCausalHashes(t *testing.T) {
	runtime, journal, realm, grants := viewFixture(t)
	own, err := dynamics.New("alice", 0, dynamics.DefaultSubstrate())
	if err != nil {
		t.Fatal(err)
	}
	own.Causes = []core.ID{"GENESIS_HASH_CANARY"}
	other, err := dynamics.New("bob", 0, dynamics.DefaultSubstrate())
	if err != nil {
		t.Fatal(err)
	}
	other, err = dynamics.Intervene(other, dynamics.Fatigue, .99, 0, "OTHER_CAUSE_CANARY")
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := AppraisalCheckpoint{Version: 1, Model: dynamics.ModelVersion, Actors: []dynamics.State{own, other}}
	raw, err := checkpoint.canonical()
	if err != nil {
		t.Fatal(err)
	}
	runtime.snapshot.State.Data = string(raw)
	service, err := NewViewService(runtime, journal, viewClock{}, grants, viewAudit{})
	if err != nil {
		t.Fatal(err)
	}
	permit, _ := service.Permit("alice", realm, ActorViewKind, "simulation")
	_, self, err := service.Actor(context.Background(), permit)
	if err != nil {
		t.Fatal(err)
	}
	if len(self.CurrentDrives) != dynamics.Count || self.CurrentDrives[0].Level != .2 {
		t.Fatal("wrong actor dynamics", self)
	}
	raw, _ = json.Marshal(self)
	if strings.Contains(string(raw), "CANARY") {
		t.Fatal("causal hash or other actor leaked")
	}
	runtime.snapshot.State.At = 0
	own, err = dynamics.Advance(own, 10)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint.Actors[0] = own
	raw, err = checkpoint.canonical()
	if err != nil {
		t.Fatal(err)
	}
	runtime.snapshot.State.Data = string(raw)
	if _, _, err = service.Actor(context.Background(), permit); err == nil {
		t.Fatal("future self checkpoint exposed")
	}
}

type viewAudit struct{}

func (viewAudit) RecordPolicyDecision(context.Context, graph.PolicyAudit) error { return nil }

type immutableViewRuntime struct {
	scope    Scope
	snapshot Snapshot
}

func (r immutableViewRuntime) LoadRun(_ context.Context, s Scope) (Snapshot, error) {
	if s != r.scope {
		return Snapshot{}, ErrViewDenied
	}
	return r.snapshot, nil
}
func TestConcurrentViewsAndGrantRevocation(t *testing.T) {
	runtime, journal, realm, grants := viewFixture(t)
	service, err := NewViewService(immutableViewRuntime{runtime.scope, runtime.snapshot}, journal, viewClock{}, grants, viewAudit{})
	if err != nil {
		t.Fatal(err)
	}
	actor, _ := service.Permit("alice", realm, ActorViewKind, "simulation")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_, _, err := service.Actor(context.Background(), actor)
				if err != nil && err != ErrViewDenied {
					t.Error(err)
				}
			}
		}()
	}
	service.Revoke(actor)
	wg.Wait()
	if _, _, err = service.Actor(context.Background(), actor); err != ErrViewDenied {
		t.Fatal("revoked grant survived", err)
	}
}

func TestApprovalExpiresOnRuntimeRevisionOrTime(t *testing.T) {
	for _, change := range []func(*viewRuntime){
		func(r *viewRuntime) { r.snapshot.Revision++ },
		func(r *viewRuntime) { r.snapshot.State.At++ },
	} {
		runtime, journal, realm, grants := viewFixture(t)
		service, err := NewViewService(runtime, journal, viewClock{}, grants, viewAudit{})
		if err != nil {
			t.Fatal(err)
		}
		permit, _ := service.Permit("alice", realm, ActorViewKind, "simulation")
		approved, d, err := service.Propose(context.Background(), permit, []core.ID{"permitted"}, "bob", core.Disclose, graph.SyntheticSelfDisclosure)
		if err != nil || !d.Allowed {
			t.Fatal(d, err)
		}
		change(runtime)
		_, d, _ = service.policy.Revalidate(context.Background(), approved, permit.Binding())
		if d.Allowed {
			t.Fatal("old runtime approval survived revision/time change")
		}
		_, d, err = service.Propose(context.Background(), permit, []core.ID{"permitted"}, "bob", core.Disclose, graph.SyntheticSelfDisclosure)
		if err != nil || !d.Allowed {
			t.Fatal("fresh decision incorrectly denied", d, err)
		}
	}
}
