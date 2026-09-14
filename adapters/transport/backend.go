package transport

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	decoder "github.com/tushardhara/dream/adapters/scenario"
	pb "github.com/tushardhara/dream/adapters/transport/gen/dream/v1"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator"
	rt "github.com/tushardhara/dream/simulator/runtime"
	scenario "github.com/tushardhara/dream/simulator/scenario"
)

type RuntimePersistence interface {
	hws.RuntimeStore
	hws.SnapshotStore
}

// Backend is trusted host composition. No model/handler/configuration can be
// selected by a wire request. Export and external action composition are separate
// capabilities rather than a backdoor to a raw runtime reader.
type Backend struct {
	commandMu sync.Mutex
	leases    map[hws.Scope]hws.Lease

	pb.UnimplementedResearchServer
	Auth    *Authenticator
	Store   RuntimePersistence
	Views   *hws.ViewService
	Handler rt.Handler
	// StepExecutor composes a trusted cognitive/model service outside SQL transactions.
	StepExecutor func(context.Context, hws.Scope, hws.Lease, core.ID, func(context.Context) error) (hws.Receipt, error)
}

func (b *Backend) identity(ctx context.Context, method string, r any) (Credential, error) {
	if b == nil || b.Auth == nil || b.Store == nil || b.Views == nil {
		return Credential{}, ErrDenied
	}
	if err := authorizeWire(ctx, b.Auth, method, r); err != nil {
		return Credential{}, err
	}
	return CurrentIdentity(ctx, b.Auth)
}
func document(schema string, value any) (*pb.Document, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(raw) > 32<<20 {
		return nil, ErrLimited
	}
	return &pb.Document{Schema: schema, CanonicalJson: raw}, nil
}
func input(r *pb.Input) *rt.Input {
	if r == nil {
		return nil
	}
	return &rt.Input{ID: core.ID(r.Id), At: core.LogicalTime(r.At), Kind: core.ID(r.Kind), Actor: core.ID(r.Actor), Text: r.Text, Resource: core.ID(r.Resource), Units: r.Units, Priority: int(r.Priority)}
}
func scope(r *pb.Scope) hws.Scope {
	if r == nil {
		return hws.Scope{}
	}
	return hws.Scope{Actor: core.ID(r.Operator), Namespace: core.ID(r.Namespace), World: simulator.WorldID(r.World), Branch: simulator.BranchID(r.Branch), Run: simulator.RunID(r.Run)}
}
func handle(r *pb.SnapshotHandle) hws.SnapshotKey {
	if r == nil {
		return hws.SnapshotKey{}
	}
	return hws.SnapshotKey{Scope: scope(r.Scope), ID: core.ID(r.Key), Hash: r.Hash}
}
func (b *Backend) permit(c Credential) (hws.ViewPermit, error) {
	return b.Views.Permit(c.Caller, hws.ViewRealm{Scope: c.Scope, Principal: c.Principal}, c.Role, c.Purpose)
}
func (b *Backend) ValidateScenario(ctx context.Context, r *pb.ScenarioRequest) (*pb.Document, error) {
	if _, err := b.identity(ctx, "ValidateScenario", r); err != nil {
		return nil, err
	}
	sc, err := decoder.Parse(strings.NewReader(r.Yaml))
	if err != nil {
		return nil, err
	}
	if sc.World.ID != simulator.WorldID(r.Scope.World) {
		return nil, ErrDenied
	}
	return document("scenario.v1", sc)
}
func (b *Backend) CreateWorld(ctx context.Context, r *pb.CreateWorldRequest) (*pb.Document, error) {
	c, err := b.identity(ctx, "CreateWorld", r)
	if err != nil {
		return nil, err
	}
	sc, err := decoder.Parse(strings.NewReader(r.Yaml))
	if err != nil {
		return nil, err
	}
	genesis, err := sc.Genesis(scenario.Engine{ScenarioVersion: 1, Capabilities: []core.ID{"genesis.v1", "resources.v1", "memory.v1", "relationships.v1", "latent.v1", "schedule.v1"}})
	if err != nil {
		return nil, err
	}
	if r.MaxDurationSeconds <= 0 || r.MaxDurationSeconds > 86400 {
		return nil, ErrDenied
	}
	if _, err = CurrentIdentity(ctx, b.Auth); err != nil {
		return nil, err
	}
	out, err := b.Store.CreateRun(ctx, hws.Manifest{Version: 1, Scope: c.Scope, Genesis: genesis, Budget: rt.Budgets{Steps: r.MaxSteps, Events: r.MaxEvents, Horizon: core.LogicalTime(r.Horizon)}, MaxDuration: time.Duration(r.MaxDurationSeconds) * time.Second})
	if err != nil {
		return nil, err
	}
	return document("runtime.snapshot.v1", out)
}
func (b *Backend) Control(ctx context.Context, r *pb.ControlRequest) (*pb.Document, error) {
	c, err := b.identity(ctx, "Control", r)
	if err != nil {
		return nil, err
	}
	permit, err := b.permit(c)
	if err != nil {
		return nil, err
	}
	if _, err = b.Views.Research(ctx, permit); err != nil {
		return nil, err
	}
	if (r.Command == "step" && b.Handler == nil && b.StepExecutor == nil) || (r.Command == "run-until" && b.Handler == nil) {
		return nil, ErrDenied
	}
	command := rt.Command{Kind: r.Command, Until: core.LogicalTime(r.Until), Input: input(r.Input)}
	if command.Validate() != nil || core.ID(r.OperationId).Validate() != nil {
		return nil, ErrDenied
	}
	runtime := hws.Runtime{Store: b.Store, Handler: b.Handler, BeforeCommit: func(ctx context.Context) error {
		if _, err := CurrentIdentity(ctx, b.Auth); err != nil {
			return err
		}
		_, err := b.Views.Research(ctx, permit)
		return err
	}}
	out, err := b.execute(ctx, runtime, c, core.ID(r.OperationId), command)
	if err != nil {
		return nil, err
	}
	return document("runtime.receipt.v1", out)
}
func (b *Backend) GetOperation(ctx context.Context, r *pb.OperationRequest) (*pb.Document, error) {
	c, err := b.identity(ctx, "GetOperation", r)
	if err != nil {
		return nil, err
	}
	permit, err := b.permit(c)
	if err != nil {
		return nil, err
	}
	if _, err = b.Views.Research(ctx, permit); err != nil {
		return nil, err
	}
	out, err := b.Store.Operation(ctx, c.Scope, core.ID(r.OperationId))
	if err != nil || out == nil {
		return nil, ErrDenied
	}
	return document("runtime.operation.v1", out)
}
func (b *Backend) CaptureSnapshot(ctx context.Context, r *pb.SnapshotRequest) (*pb.SnapshotHandle, error) {
	c, err := b.identity(ctx, "CaptureSnapshot", r)
	if err != nil {
		return nil, err
	}
	permit, err := b.permit(c)
	if err != nil {
		return nil, err
	}
	out, err := (hws.SnapshotService{Store: b.Store, Views: b.Views}).Capture(ctx, permit, c.Scope, core.ID(r.Key), r.Revision)
	if err != nil {
		return nil, err
	}
	return &pb.SnapshotHandle{Scope: r.Scope, Key: string(out.ID), Hash: out.Hash}, nil
}
func (b *Backend) Replay(ctx context.Context, r *pb.ReplayRequest) (*pb.Document, error) {
	c, err := b.identity(ctx, "Replay", r)
	if err != nil {
		return nil, err
	}
	permit, err := b.permit(c)
	if err != nil {
		return nil, err
	}
	out, err := (hws.SnapshotService{Store: b.Store, Views: b.Views}).Replay(ctx, permit, handle(r.Source), r.ThroughRevision, hws.ReplayMode(r.Mode))
	if err != nil {
		return nil, err
	}
	return document("replay.v1", out)
}
func (b *Backend) ActorView(ctx context.Context, r *pb.Query) (*pb.Document, error) {
	c, err := b.identity(ctx, "ActorView", r)
	if err != nil {
		return nil, err
	}
	permit, err := b.permit(c)
	if err != nil {
		return nil, err
	}
	observation, self, err := b.Views.Actor(ctx, permit)
	if err != nil {
		return nil, err
	}
	return document("actor.view.v1", struct {
		Observation hws.ActorObservation
		Self        hws.ActorSelfState
	}{observation, self})
}
func (b *Backend) ResearchView(ctx context.Context, r *pb.Query) (*pb.Document, error) {
	c, err := b.identity(ctx, "ResearchView", r)
	if err != nil {
		return nil, err
	}
	permit, err := b.permit(c)
	if err != nil {
		return nil, err
	}
	out, err := b.Views.Research(ctx, permit)
	if err != nil {
		return nil, err
	}
	if r.ModelUsage {
		if r.AuditSource != nil || r.AuditThroughRevision != 0 {
			return nil, ErrDenied
		}
		store, ok := b.Store.(hws.ModelUsageReader)
		if !ok {
			return nil, ErrDenied
		}
		usage, e := store.ReadModelUsage(ctx, c.Scope)
		if e != nil {
			return nil, e
		}
		if _, e = b.Views.Research(ctx, permit); e != nil {
			return nil, e
		}
		return document("model.usage.v1", usage)
	}
	if r.AuditSource != nil {
		if scope(r.AuditSource.Scope) != c.Scope {
			return nil, ErrDenied
		}
		store, ok := b.Store.(hws.AuditStore)
		if !ok {
			return nil, ErrDenied
		}
		packet, e := store.ReadAudit(ctx, handle(r.AuditSource), r.AuditThroughRevision)
		if e != nil {
			return nil, e
		}
		if _, e = b.Views.Research(ctx, permit); e != nil {
			return nil, e
		}
		return document("audit.v1", packet)
	}
	if r.AuditThroughRevision != 0 {
		return nil, ErrDenied
	}
	return document("research.view.v1", out)
}
func (b *Backend) ExternalObserve(ctx context.Context, r *pb.ExternalRequest) (*pb.Document, error) {
	c, err := b.identity(ctx, "ExternalObserve", r)
	if err != nil {
		return nil, err
	}
	permit, err := b.permit(c)
	if err != nil {
		return nil, err
	}
	if len(r.SourceIds) > 16 {
		return nil, ErrLimited
	}
	sources := make([]core.ID, len(r.SourceIds))
	for i, id := range r.SourceIds {
		sources[i] = core.ID(id)
	}
	out, err := b.Views.External(ctx, permit, sources)
	if err != nil {
		return nil, err
	}
	return document("external.view.v1", out)
}

func (b *Backend) Fork(ctx context.Context, r *pb.ForkRequest) (*pb.Document, error) {
	c, err := b.identity(ctx, "Fork", r)
	if err != nil {
		return nil, err
	}
	child := scope(r.Child)
	if !b.Auth.configuredChild(c, child) || r.MaxDurationSeconds <= 0 || r.MaxDurationSeconds > 86400 {
		return nil, ErrDenied
	}
	if _, err = b.Views.Permit(c.Caller, hws.ViewRealm{Scope: child, Principal: c.Principal}, c.Role, c.Purpose); err != nil {
		return nil, err
	}
	permit, err := b.permit(c)
	if err != nil {
		return nil, err
	}
	spec := hws.ForkSpec{Version: 1, Source: handle(r.Source), Child: child, Mode: hws.FreshSimulation, PairedExogenous: r.PairedExogenous, Policy: r.Policy, Alternative: input(r.Alternative), MaxDuration: time.Duration(r.MaxDurationSeconds) * time.Second}
	out, err := (hws.SnapshotService{Store: b.Store, Views: b.Views}).Fork(ctx, permit, spec)
	if err != nil {
		return nil, err
	}
	return document("runtime.snapshot.v1", out)
}

// ExternalAct records a bounded requested intent as an observation for the
// trusted handler. It does not claim completion, change latent state directly,
// or let an external client author arbitrary observation text or resource effects.
func (b *Backend) ExternalAct(ctx context.Context, r *pb.ControlRequest) (*pb.Document, error) {
	c, err := b.identity(ctx, "ExternalAct", r)
	if err != nil {
		return nil, err
	}
	if r.Command != "inject" || r.Until != 0 || r.Input == nil || r.Input.Actor != string(c.Principal) || r.Input.Kind != "observation" || r.Input.Resource != "" || r.Input.Units != 0 || r.Input.Priority != 0 {
		return nil, ErrDenied
	}
	switch r.Input.Text {
	case "wait", "observe", "ask":
	default:
		return nil, ErrDenied
	}
	permit, err := b.permit(c)
	if err != nil {
		return nil, err
	}
	if err = b.Views.CheckOperation(ctx, permit, core.Derive); err != nil {
		return nil, err
	}
	current, err := b.Store.LoadRun(ctx, c.Scope)
	if err != nil || int64(current.State.At) != r.Input.At {
		return nil, ErrDenied
	}
	event := input(r.Input)
	event.Text = "external requested intent: " + event.Text
	event.Priority = 1 // canonical observation priority; the client cannot choose it.
	if current.State.At >= current.State.Budget.Horizon {
		return nil, ErrLimited
	}
	event.At = current.State.At + 1 // a requested intent takes effect at the next boundary.
	runtime := hws.Runtime{Store: b.Store, Handler: b.Handler, BeforeCommit: func(ctx context.Context) error {
		if _, err := CurrentIdentity(ctx, b.Auth); err != nil {
			return err
		}
		return b.Views.CheckOperation(ctx, permit, core.Derive)
	}}
	out, err := b.execute(ctx, runtime, c, core.ID(r.OperationId), rt.Command{Kind: "inject", Input: event})
	if err != nil {
		return nil, err
	}
	return document("external.intent.receipt.v1", out)
}

// The transport owns a bounded lease cache, serialized across local commands.
// Persisted completed operations are returned before touching leases, including
// terminal-run retries. A restart reconstructs operations and waits for fencing.
func (b *Backend) execute(ctx context.Context, runtime hws.Runtime, c Credential, key core.ID, command rt.Command) (hws.Receipt, error) {
	b.commandMu.Lock()
	defer b.commandMu.Unlock()
	if err := ctx.Err(); err != nil {
		return hws.Receipt{}, err
	}
	if _, err := CurrentIdentity(ctx, b.Auth); err != nil {
		return hws.Receipt{}, err
	}
	op, err := b.Store.Operation(ctx, c.Scope, key)
	if err != nil {
		return hws.Receipt{}, err
	}
	if op != nil && op.Receipt.Done {
		original, err := hws.CommandDigest(op.Command)
		if err != nil {
			return hws.Receipt{}, err
		}
		requested, err := hws.CommandDigest(command)
		if err != nil || original != requested {
			return hws.Receipt{}, hws.ErrCommand
		}
		return op.Receipt, nil
	}
	if b.leases == nil {
		b.leases = map[hws.Scope]hws.Lease{}
	}
	lease, exists := b.leases[c.Scope]
	if exists {
		lease, err = b.Store.Renew(ctx, c.Scope, lease, time.Minute)
	}
	if !exists || err == hws.ErrLease {
		if !exists && len(b.leases) >= 128 {
			return hws.Receipt{}, ErrLimited
		}
		lease, err = b.Store.Acquire(ctx, c.Scope, c.ID, time.Minute)
	}
	if err != nil {
		return hws.Receipt{}, err
	}
	b.leases[c.Scope] = lease
	if command.Kind == "step" && b.StepExecutor != nil {
		return b.StepExecutor(ctx, c.Scope, lease, key, runtime.BeforeCommit)
	}
	return runtime.Execute(ctx, c.Scope, lease, key, command)
}
