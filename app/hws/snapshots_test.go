package hws

import (
	"context"
	"testing"

	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
)

type snapshotPort struct {
	calls           int
	bundle          ReplayBundle
	revokeAfterRead bool
	revoked         bool
}

func (p *snapshotPort) CaptureSnapshot(context.Context, Scope, core.ID, int64) (SnapshotKey, error) {
	p.calls++
	return SnapshotKey{}, nil
}
func (p *snapshotPort) ReadSnapshot(context.Context, SnapshotKey) (FrozenState, error) {
	p.calls++
	return p.bundle.Snapshot, nil
}
func (p *snapshotPort) ReadReplay(context.Context, SnapshotKey, int64) (ReplayBundle, error) {
	p.calls++
	if p.revoked {
		return ReplayBundle{}, ErrViewDenied
	}
	if p.revokeAfterRead {
		p.revoked = true
	}
	return p.bundle, nil
}
func (p *snapshotPort) ForkSnapshot(context.Context, ForkSpec) (Snapshot, error) {
	p.calls++
	return Snapshot{}, nil
}
func TestSnapshotAuthorityAndCurrentReplayRevalidation(t *testing.T) {
	ctx := context.Background()
	runtime, journal, realm, grants := viewFixture(t)
	views, e := NewViewService(runtime, journal, viewClock{}, grants, viewAudit{})
	if e != nil {
		t.Fatal(e)
	}
	port := &snapshotPort{}
	service := SnapshotService{Store: port, Views: views}
	reader, e := views.Permit("researcher", realm, ResearchViewKind, "research")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = service.Capture(ctx, reader, realm.Scope, "snapshot", 1); e == nil || port.calls != 0 {
		t.Fatal("read implies retain")
	}
	if _, e = service.Fork(ctx, reader, ForkSpec{Source: SnapshotKey{Scope: realm.Scope}}); e == nil || port.calls != 0 {
		t.Fatal("read implies derive")
	}
	actor, e := views.Permit("alice", realm, ActorViewKind, "simulation")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = service.Replay(ctx, actor, SnapshotKey{Scope: realm.Scope}, 1, RecordedReplay); e == nil || port.calls != 0 {
		t.Fatal("actor got research replay")
	}
	// Explicit research retention/derivation are required independently of read.
	grants[2].Operations = []core.Operation{core.Read, core.Retain, core.Derive}
	views, e = NewViewService(runtime, journal, viewClock{}, grants, viewAudit{})
	if e != nil {
		t.Fatal(e)
	}
	reader, e = views.Permit("researcher", realm, ResearchViewKind, "research")
	if e != nil {
		t.Fatal(e)
	}
	service.Views = views
	if _, e = service.Capture(ctx, reader, realm.Scope, "snapshot", 1); e != nil || port.calls != 1 {
		t.Fatal("authorized retention denied", e)
	}
	frozen := frozenFixture(t, runtime.snapshot.State)
	frozen.Scope = realm.Scope
	for i, m := range frozen.Memories {
		scope, _ := (ViewRealm{Scope: realm.Scope, Principal: m.Scope.Owner}).MemoryScope()
		frozen.Memories[i] = FrozenMemory{Scope: scope, Entries: []graph.MemoryEntry{}}
	}
	frozen, e = SealSnapshot(frozen)
	if e != nil {
		t.Fatal(e)
	}
	port.bundle = ReplayBundle{Snapshot: frozen}
	key := SnapshotKey{Scope: realm.Scope, ID: "snapshot", Hash: frozen.Hash}
	port.revokeAfterRead = true
	if _, e = service.Replay(ctx, reader, key, 1, RecordedReplay); e == nil {
		t.Fatal("trajectory revoked during replay was returned")
	}
	port.revokeAfterRead = false
	port.revoked = false
	result, e := service.Replay(ctx, reader, key, 1, RecordedReplay)
	if e != nil || !result.Exact {
		t.Fatal("authorized recorded replay failed", e)
	}
	views.Revoke(reader)
	before := port.calls
	if _, e = service.Replay(ctx, reader, key, 1, RecordedReplay); e == nil || port.calls != before {
		t.Fatal("revoked cached authority used")
	}
}
