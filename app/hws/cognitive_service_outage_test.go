package hws

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/dynamics"
	rt "github.com/tushardhara/dream/simulator/runtime"
)

// Fakes below exist so the forged-outage guard can be exercised without a
// database. app/hws may not import adapters (the architecture guard parses test
// files too), so the provider and store are defined here rather than borrowed.

type outageModelStore struct{}

func (outageModelStore) ConfigureModels(context.Context, Scope, ModelLimits) error { return nil }
func (outageModelStore) BeginModel(context.Context, ModelIntent, ModelLimits) (ModelAttempt, error) {
	return ModelAttempt{}, errors.New("unreachable: Apply consumes an already durable artifact")
}
func (outageModelStore) FinishModel(context.Context, ModelAttempt, *ModelArtifact, ProviderStatus) error {
	return errors.New("unreachable: Apply consumes an already durable artifact")
}

type outageProvider struct{}

func (outageProvider) Generate(context.Context, ProviderInput) (ProviderResponse, error) {
	return ProviderResponse{}, errors.New("unreachable: no provider call inside Apply")
}

type outageRuntimeStore struct{ snapshot Snapshot }

func (s outageRuntimeStore) LoadRun(context.Context, Scope) (Snapshot, error) { return s.snapshot, nil }
func (outageRuntimeStore) CreateRun(context.Context, Manifest) (Snapshot, error) {
	return Snapshot{}, errors.New("unreachable in this test")
}
func (outageRuntimeStore) Acquire(context.Context, Scope, core.ID, time.Duration) (Lease, error) {
	return Lease{}, errors.New("unreachable in this test")
}
func (outageRuntimeStore) Renew(context.Context, Scope, Lease, time.Duration) (Lease, error) {
	return Lease{}, errors.New("unreachable in this test")
}
func (outageRuntimeStore) Operation(context.Context, Scope, core.ID) (*Operation, error) {
	return nil, errors.New("unreachable in this test")
}
func (outageRuntimeStore) CommitRun(context.Context, Commit) (Receipt, error) {
	return Receipt{}, errors.New("unreachable in this test")
}

type outagePlanner struct{ frame CognitiveFrame }

func (p outagePlanner) Plan(context.Context, rt.Input, graph.SafeContext) (CognitiveFrame, error) {
	return p.frame, nil
}

func outageRoute() ModelRoute {
	return ModelRoute{
		Versions:            ModelVersions{Schema: "model.v1", Capability: "cognition.v1", Model: "configured-fake", Prompt: "cognition.v1", Policy: "policy.v1"},
		Capabilities:        []ModelCapability{ModelAppraisal, ModelInterpretation, ModelReconciliation, ModelCandidates},
		InputMicrosPerToken: 1, OutputMicrosPerToken: 1, MaxOutputTokens: 256,
		Limits: ModelLimits{Version: 1, Tokens: 60000, SpendMicros: 60000, MaxInFlight: 1, MaxAttempts: 3, AttemptTokens: 6000, AttemptSpendMicros: 6000, Timeout: time.Second},
	}
}

// A planner must not be able to relabel a policy failure as an operational
// outage, forge model beliefs, or switch on self disclosure by setting a field.
// The guard enforcing that runs only under Postgres today (#80), because it sits
// deep inside CognitiveService.Apply behind a view lookup, a validated artifact
// and a run snapshot.
//
// The trap this test is built to avoid: ErrModel is returned from fourteen
// places in cognitive_service.go, four of them BEFORE the guard. A test that
// merely asserts ErrModel on a forged frame passes just as happily when the
// artifact is malformed and the guard is never reached. The control below is
// therefore load-bearing: with the identical request, artifact, views and store
// and a clean frame, Apply must NOT fail with ErrModel. That is what proves the
// forged cases were rejected by this guard and not by something earlier.
func TestPlannerCannotForgeOutageOrModelState(t *testing.T) {
	ctx := context.Background()
	runtime, journal, realm, grants := viewFixture(t)
	// Cognition needs Derive, as TestCognitionCannotReuseDisclosureCapability shows.
	grants[0].Operations = append(grants[0].Operations, core.Derive)
	journal.entries[0].Event.Meta.Rights.Grants = append(journal.entries[0].Event.Meta.Rights.Grants, core.Grant{Actor: "alice", Recipient: "alice", Purpose: "simulation", Operation: core.Derive})
	views, e := NewViewService(runtime, journal, viewClock{}, grants, viewAudit{})
	if e != nil {
		t.Fatal(e)
	}
	permit, e := views.Permit("alice", realm, ActorViewKind, "simulation")
	if e != nil {
		t.Fatal(e)
	}
	approved, d, e := views.Propose(ctx, permit, []core.ID{"permitted"}, "alice", core.Derive, graph.InternalContext)
	if e != nil || !d.Allowed {
		t.Fatal(d, e)
	}
	safe, e := views.ModelContext(ctx, permit, approved, realm.Scope, "alice")
	if e != nil || len(safe.Items()) != 1 {
		t.Fatal("cognition context unavailable", e)
	}

	// The queue event is the approved source, so the approved-event check after
	// the guard can succeed and the control reaches past it.
	event := rt.Input{ID: "permitted", At: runtime.snapshot.State.At, Kind: "observation", Actor: "alice", Text: "APPROVED_CONTEXT_ONLY", Priority: 1}
	snapshot := runtime.snapshot
	snapshot.State.Queue = []rt.Input{event}
	store := outageRuntimeStore{snapshot: snapshot}

	route := outageRoute()
	gateway, e := NewModelGateway(outageModelStore{}, views, outageProvider{}, route)
	if e != nil {
		t.Fatal(e)
	}
	request := ModelRequest{Scope: realm.Scope, Principal: "alice", Key: "interpret", Capability: ModelInterpretation, Permit: permit, Approved: approved}

	// Build the artifact exactly as Apply will recompute the input, so that
	// artifact.Validate passes and cannot be the reason for any ErrModel below.
	input := ProviderInput{Version: 1, Key: request.Key, Capability: request.Capability, Versions: route.Versions, Actor: request.Principal, Context: safe.Items(), MaxOutputTokens: route.MaxOutputTokens}
	digest, e := input.Digest()
	if e != nil {
		t.Fatal(e)
	}
	output := ModelOutput{Version: 1, Capability: ModelInterpretation, Observer: "alice", Findings: []ModelFinding{{Code: "supported", Value: .5, Confidence: .7, Evidence: []core.ID{"permitted"}}}}
	raw, e := json.Marshal(output)
	if e != nil {
		t.Fatal(e)
	}
	artifact := ModelArtifact{Version: 1, RequestDigest: digest, Mode: "recorded", Output: output,
		Response: ProviderResponse{Generation: "deterministic_fake", Status: ProviderOK, Model: route.Versions.Model, Output: raw, InputTokens: 1, OutputTokens: 1, UsageKnown: true}}
	hashable := artifact
	hashable.Hash = ""
	artifact.Hash, e = ModelDigest(hashable)
	if e != nil {
		t.Fatal(e)
	}
	if e = artifact.Validate(input); e != nil {
		t.Fatal("test artifact is invalid, every case below would pass vacuously:", e)
	}

	clean := func() CognitiveFrame {
		return CognitiveFrame{Situation: behavior.Situation{Perceived: dynamics.Perceived{Event: event.ID, Actor: "alice", OccurredAt: event.At, LearnedAt: event.At, Confidence: .7}}}
	}
	apply := func(f CognitiveFrame) error {
		service := CognitiveService{Gateway: gateway, Views: views, Runtime: Runtime{Store: store}, Planner: outagePlanner{frame: f}}
		_, err := service.Apply(ctx, request, artifact, Lease{}, "key", nil)
		return err
	}

	// The control. Same request, artifact, views and store; an honest frame.
	// It need not succeed outright, but it must not be rejected as a model
	// failure, or the forged cases below prove nothing.
	if err := apply(clean()); errors.Is(err, ErrModel) {
		t.Fatal("an honest frame was rejected as ErrModel; every forged case below is vacuous:", err)
	}

	for _, c := range []struct {
		name  string
		forge func(*CognitiveFrame)
	}{
		{"outage", func(f *CognitiveFrame) { f.Situation.Outage = true }},
		{"disclosure_recipient", func(f *CognitiveFrame) { f.Situation.DisclosureRecipient = "bob" }},
		{"self_disclosure_text", func(f *CognitiveFrame) { f.Disclosure = "FORGED_FICTION" }},
		{"model_hash", func(f *CognitiveFrame) {
			f.ModelHash = "0000000000000000000000000000000000000000000000000000000000000000"
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := clean()
			c.forge(&f)
			if err := apply(f); !errors.Is(err, ErrModel) {
				t.Fatal("planner forged "+c.name+", want ErrModel, got", err)
			}
		})
	}
}
