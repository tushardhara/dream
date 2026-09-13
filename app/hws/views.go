package hws

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/dynamics"
	"github.com/tushardhara/dream/simulator/scenario"
)

var ErrViewDenied = errors.New("view unavailable or not authorized")

type ViewKind string

const (
	ActorViewKind    ViewKind = "actor"
	ExternalViewKind ViewKind = "external"
	ResearchViewKind ViewKind = "research"
)

type ViewRealm struct {
	Scope     Scope
	Principal core.ID
}

func (r ViewRealm) Key() (core.ID, error) {
	if r.Scope.Validate() != nil || r.Principal.Validate() != nil {
		return "", ErrViewDenied
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return "", ErrViewDenied
	}
	sum := sha256.Sum256(raw)
	return core.ID("hws:" + hex.EncodeToString(sum[:])), nil
}
func (r ViewRealm) MemoryScope() (graph.MemoryScope, error) {
	key, err := r.Key()
	return graph.MemoryScope{Owner: r.Principal, Namespace: key}, err
}

type ViewGrant struct {
	Caller     core.ID
	Realm      ViewRealm
	Kind       ViewKind
	Purpose    core.ID
	Operations []core.Operation
}

func (g ViewGrant) key() (core.ID, error) {
	if g.Caller.Validate() != nil || g.Purpose.Validate() != nil {
		return "", ErrViewDenied
	}
	realm, err := g.Realm.Key()
	if err != nil {
		return "", err
	}
	if g.Kind != ActorViewKind && g.Kind != ExternalViewKind && g.Kind != ResearchViewKind {
		return "", ErrViewDenied
	}
	if g.Kind == ActorViewKind && g.Caller != g.Realm.Principal {
		return "", ErrViewDenied
	}
	raw, _ := json.Marshal([]string{string(realm), string(g.Caller), string(g.Kind), string(g.Purpose)})
	sum := sha256.Sum256(raw)
	return core.ID("view:" + hex.EncodeToString(sum[:])), nil
}

type ViewPermit struct {
	issuer  *ViewService
	binding core.ID
}

func (p ViewPermit) Binding() core.ID { return p.binding }

type ViewRuntimeReader interface {
	LoadRun(context.Context, Scope) (Snapshot, error)
}

// ViewService is composed with trusted grants at the host boundary. Constructors
// are not network authorization endpoints. External clients/writers never receive
// the runtime reader, grant map, constructor or raw research snapshot capability.
type ViewService struct {
	runtime  ViewRuntimeReader
	clock    OperationalClock
	mu       sync.RWMutex
	grants   map[core.ID]ViewGrant
	policy   *graph.PolicyService
	recorder graph.DecisionRecorder
}

func NewViewService(runtime ViewRuntimeReader, journal graph.MemoryJournal, clock OperationalClock, grants []ViewGrant, recorder graph.DecisionRecorder) (*ViewService, error) {
	if runtime == nil || journal == nil || clock == nil || recorder == nil || len(grants) > 128 {
		return nil, ErrViewDenied
	}
	v := &ViewService{runtime: runtime, clock: clock, grants: map[core.ID]ViewGrant{}, recorder: recorder}
	for _, g := range grants {
		key, err := g.key()
		if err != nil {
			return nil, err
		}
		if _, ok := v.grants[key]; ok {
			return nil, ErrViewDenied
		}
		if len(g.Operations) == 0 || len(g.Operations) > 9 {
			return nil, ErrViewDenied
		}
		seen := map[core.Operation]bool{}
		for _, op := range g.Operations {
			if seen[op] || (core.Grant{Actor: g.Caller, Recipient: g.Caller, Purpose: g.Purpose, Operation: op}).Validate() != nil {
				return nil, ErrViewDenied
			}
			seen[op] = true
		}
		g.Operations = append([]core.Operation{}, g.Operations...)
		v.grants[key] = g
	}
	v.policy = graph.NewPolicyService(journal, v, recorder)
	return v, nil
}
func (v *ViewService) Permit(caller core.ID, realm ViewRealm, kind ViewKind, purpose core.ID) (ViewPermit, error) {
	if v == nil {
		return ViewPermit{}, ErrViewDenied
	}
	key, err := (ViewGrant{Caller: caller, Realm: realm, Kind: kind, Purpose: purpose}).key()
	if err != nil {
		return ViewPermit{}, err
	}
	v.mu.RLock()
	_, ok := v.grants[key]
	v.mu.RUnlock()
	if !ok {
		return ViewPermit{}, ErrViewDenied
	}
	return ViewPermit{v, key}, nil
}
func (v *ViewService) Revoke(permit ViewPermit) {
	if v == nil || permit.issuer != v {
		return
	}
	v.mu.Lock()
	delete(v.grants, permit.binding)
	v.mu.Unlock()
}
func (v *ViewService) grant(permit ViewPermit) (ViewGrant, error) {
	if v == nil || permit.issuer != v {
		return ViewGrant{}, ErrViewDenied
	}
	v.mu.RLock()
	g, ok := v.grants[permit.binding]
	v.mu.RUnlock()
	if !ok {
		return ViewGrant{}, ErrViewDenied
	}
	return g, nil
}
func (v *ViewService) snapshot(ctx context.Context, permit ViewPermit) (Snapshot, ViewGrant, error) {
	g, err := v.grant(permit)
	if err != nil {
		return Snapshot{}, ViewGrant{}, err
	}
	snapshot, err := v.runtime.LoadRun(ctx, g.Realm.Scope)
	if err != nil || snapshot.State.Validate() != nil || snapshot.State.Genesis.World != g.Realm.Scope.World {
		return Snapshot{}, ViewGrant{}, ErrViewDenied
	}
	if _, err = v.grant(permit); err != nil {
		return Snapshot{}, ViewGrant{}, err
	}
	return snapshot, g, nil
}
func permitsOperation(g ViewGrant, op core.Operation) bool {
	for _, allowed := range g.Operations {
		if allowed == op {
			return true
		}
	}
	return false
}

// ValidateContext binds generic policy to current namespace/world/branch/run/
// principal and trusted virtual/system clocks before any memory/provider access.
func (v *ViewService) ValidateContext(ctx context.Context, p graph.ContextProposal) (bool, error) {
	permit := ViewPermit{v, p.Binding}
	snap, g, err := v.snapshot(ctx, permit)
	if err != nil {
		return false, ErrViewDenied
	}
	scope, err := g.Realm.MemoryScope()
	if err != nil {
		return false, err
	}
	if p.Query.Scope != scope || p.Query.Actor != g.Caller || p.Query.Purpose != g.Purpose || !permitsOperation(g, p.Operation) || p.AuthorityRevision != snap.Revision || p.Query.KnownAt != snap.State.At || p.Query.ValidAt != snap.State.At || p.Query.RecordedAsOf.After(v.clock.Now()) {
		return false, nil
	}
	switch g.Kind {
	case ActorViewKind:
		return p.Mode == graph.InternalContext || p.Mode == graph.AssistantDisclosure || p.Mode == graph.SyntheticSelfDisclosure, nil
	case ExternalViewKind:
		return p.Mode == graph.ExternalContext && p.Operation == core.Read, nil
	}
	return false, nil
}
func (v *ViewService) IsFictional(ctx context.Context, binding, actor core.ID) (bool, error) {
	snap, g, err := v.snapshot(ctx, ViewPermit{v, binding})
	if err != nil {
		return false, err
	}
	if g.Kind != ActorViewKind || g.Caller != actor || g.Realm.Principal != actor {
		return false, nil
	}
	var sc scenario.Scenario
	if json.Unmarshal(snap.State.Genesis.Payload, &sc) != nil {
		return false, ErrViewDenied
	}
	for _, human := range sc.Public.Humans {
		if human.ID == actor {
			return true, nil
		}
	}
	return false, nil
}
func (v *ViewService) Propose(ctx context.Context, permit ViewPermit, sources []core.ID, recipient core.ID, op core.Operation, mode graph.PolicyMode) (graph.ApprovedContext, graph.PolicyDecision, error) {
	snap, g, err := v.snapshot(ctx, permit)
	if err != nil {
		return graph.ApprovedContext{}, graph.PolicyDecision{Action: "WAIT"}, err
	}
	scope, err := g.Realm.MemoryScope()
	if err != nil {
		return graph.ApprovedContext{}, graph.PolicyDecision{Action: "WAIT"}, err
	}
	q := graph.MemoryQuery{Scope: scope, Actor: g.Caller, Purpose: g.Purpose, Subject: core.Subject{Principal: g.Realm.Principal}, ValidAt: snap.State.At, KnownAt: snap.State.At, RecordedAsOf: v.clock.Now(), Limit: 16}
	return v.policy.Approve(ctx, graph.ContextProposal{Version: 1, AuthorityRevision: snap.Revision, Query: q, Binding: permit.binding, Recipient: recipient, Operation: op, Mode: mode, Sources: sources})
}
func (v *ViewService) Write(ctx context.Context, permit ViewPermit, approved graph.ApprovedContext, writer graph.ApprovedWriter) (graph.ValidatedOutput, graph.PolicyDecision, error) {
	if _, err := v.grant(permit); err != nil {
		return graph.ValidatedOutput{}, graph.PolicyDecision{Action: "WAIT"}, err
	}
	return v.policy.Write(ctx, approved, permit.binding, writer)
}

type ActorObservation struct {
	Principal    core.ID
	At           core.LogicalTime
	GenesisKnown scenario.ActorView
}
type ActorSelfState struct {
	Principal      core.ID
	At             core.LogicalTime
	InitialProfile *scenario.Latent
	CurrentDrives  []SelfDrive
}
type SelfDrive struct {
	ID         core.ID
	Level      float64
	Confidence core.Confidence
}
type ResearchGodState struct{ Snapshot Snapshot }
type ExternalAgentView struct{ Items []graph.SafeContextItem }

func (v *ViewService) recordAccess(ctx context.Context, permit ViewPermit, kind core.ID, allowed bool) error {
	if v == nil || v.recorder == nil {
		return ErrViewDenied
	}
	binding := permit.binding
	if binding.Validate() != nil {
		binding = "unverified"
	}
	action := "WAIT"
	clause := core.ID(string(kind) + "_view_denied")
	if allowed {
		action = "ALLOW"
		clause = core.ID(string(kind) + "_view_permitted")
	}
	if v.recorder.RecordPolicyDecision(ctx, graph.PolicyAudit{Version: 1, Binding: binding, Stage: "view_access", Decision: graph.PolicyDecision{Allowed: allowed, Action: action, Evidence: []graph.ClauseEvidence{{SourceOrdinal: -1, Clause: clause}}}}) != nil {
		return ErrViewDenied
	}
	return nil
}

func (v *ViewService) Actor(ctx context.Context, permit ViewPermit) (observation ActorObservation, selfOut ActorSelfState, resultErr error) {
	defer func() {
		if err := v.recordAccess(ctx, permit, "actor", resultErr == nil); err != nil {
			observation = ActorObservation{}
			selfOut = ActorSelfState{}
			resultErr = err
		}
	}()

	g, err := v.grant(permit)
	if err != nil || g.Kind != ActorViewKind || g.Purpose != "simulation" || !permitsOperation(g, core.Read) {
		return ActorObservation{}, ActorSelfState{}, ErrViewDenied
	}
	snap, g, err := v.snapshot(ctx, permit)
	if err != nil {
		return ActorObservation{}, ActorSelfState{}, err
	}
	var sc scenario.Scenario
	if json.Unmarshal(snap.State.Genesis.Payload, &sc) != nil {
		return ActorObservation{}, ActorSelfState{}, ErrViewDenied
	}
	known, err := sc.View(g.Realm.Principal)
	if err != nil {
		return ActorObservation{}, ActorSelfState{}, ErrViewDenied
	}
	self := ActorSelfState{Principal: g.Realm.Principal, At: snap.State.At, CurrentDrives: []SelfDrive{}}
	for _, profile := range sc.Research.Latent {
		if profile.Actor == g.Realm.Principal {
			owned := profile
			owned.Drives = append(owned.Drives[:0:0], profile.Drives...)
			self.InitialProfile = &owned
		}
	}
	if snap.State.Data != "" {
		checkpoint, err := DecodeAppraisalCheckpoint(snap.State.Data)
		if strings.HasPrefix(snap.State.Data, cognitivePrefix) {
			var cognitive CognitiveCheckpoint
			cognitive, err = DecodeCognitiveCheckpoint(snap.State.Data)
			checkpoint.Actors = nil
			for _, actor := range cognitive.Actors {
				checkpoint.Actors = append(checkpoint.Actors, actor.State)
			}
		}
		if err != nil {
			return ActorObservation{}, ActorSelfState{}, ErrViewDenied
		}
		found := false
		for _, state := range checkpoint.Actors {
			if state.Actor != g.Realm.Principal {
				continue
			}
			found = true
			state, err = dynamics.Advance(state, snap.State.At)
			if err != nil {
				return ActorObservation{}, ActorSelfState{}, ErrViewDenied
			}
			registry := dynamics.Registry()
			for i, drive := range state.Variables {
				self.CurrentDrives = append(self.CurrentDrives, SelfDrive{registry[i].ID, drive.Level, drive.Confidence})
			}
		}
		if !found {
			return ActorObservation{}, ActorSelfState{}, ErrViewDenied
		}
	}
	return ActorObservation{g.Realm.Principal, snap.State.At, known}, self, nil
}
func (v *ViewService) Research(ctx context.Context, permit ViewPermit) (output ResearchGodState, resultErr error) {
	defer func() {
		if err := v.recordAccess(ctx, permit, "research", resultErr == nil); err != nil {
			output = ResearchGodState{}
			resultErr = err
		}
	}()

	g, err := v.grant(permit)
	if err != nil || g.Kind != ResearchViewKind || !permitsOperation(g, core.Read) {
		return ResearchGodState{}, ErrViewDenied
	}
	snap, _, err := v.snapshot(ctx, permit)
	if err != nil {
		return ResearchGodState{}, err
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return ResearchGodState{}, ErrViewDenied
	}
	var owned Snapshot
	if json.Unmarshal(raw, &owned) != nil {
		return ResearchGodState{}, ErrViewDenied
	}
	return ResearchGodState{owned}, nil
}
func (v *ViewService) External(ctx context.Context, permit ViewPermit, sources []core.ID) (output ExternalAgentView, resultErr error) {
	defer func() {
		if err := v.recordAccess(ctx, permit, "external", resultErr == nil); err != nil {
			output = ExternalAgentView{}
			resultErr = err
		}
	}()

	g, err := v.grant(permit)
	if err != nil || g.Kind != ExternalViewKind {
		return ExternalAgentView{}, ErrViewDenied
	}
	approved, d, err := v.Propose(ctx, permit, sources, g.Caller, core.Read, graph.ExternalContext)
	if err != nil || !d.Allowed {
		return ExternalAgentView{}, ErrViewDenied
	}
	safe, d, err := v.policy.Revalidate(ctx, approved, permit.binding)
	if err != nil || !d.Allowed {
		return ExternalAgentView{}, ErrViewDenied
	}
	return ExternalAgentView{safe.Items()}, nil
}

// ModelContext is available only to trusted orchestration with an actor permit.
// Models never receive the permit, raw runtime reader or unfiltered snapshot.
func (v *ViewService) ModelContext(ctx context.Context, permit ViewPermit, approved graph.ApprovedContext, scope Scope, principal core.ID) (graph.SafeContext, error) {
	g, err := v.grant(permit)
	if err != nil || g.Kind != ActorViewKind || g.Purpose != "simulation" || g.Realm.Scope != scope || g.Realm.Principal != principal || g.Caller != principal {
		return graph.SafeContext{}, ErrViewDenied
	}
	safe, d, err := v.policy.RevalidateInternal(ctx, approved, permit.binding, principal)
	if err != nil || !d.Allowed {
		return graph.SafeContext{}, ErrViewDenied
	}
	return safe, nil
}

// CheckOperation revalidates a host permit and its explicit operation without
// returning a raw research snapshot to transport callers.
func (v *ViewService) CheckOperation(ctx context.Context, permit ViewPermit, operation core.Operation) error {
	_, g, err := v.snapshot(ctx, permit)
	if err != nil || !permitsOperation(g, operation) {
		return ErrViewDenied
	}
	return nil
}
