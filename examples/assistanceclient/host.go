// Package assistanceclient is an offline second host with no simulator imports.
package assistanceclient

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
)

// Local is a bounded synthetic host. Access changes, checks and appends share a
// lock. A private context token lets policy reads join the atomic commit without
// reacquiring that lock. Planning occurs outside this critical section.
// This example is not durable storage and performs no external delivery.
type Local struct {
	boundaries   []core.Boundary
	now          core.LogicalTime
	audits       []graph.PolicyAudit
	mu           sync.Mutex
	helper, user core.ID
	goal         assistance.Goal
	allowed      bool
	requests     map[core.ID]assistance.Request
	entries      map[graph.MemoryScope][]graph.MemoryEntry
	history      []assistance.Interaction
	deliveries   int
}
type transactionKey struct{}
type transaction struct{ owner *Local }

func (l *Local) lock(ctx context.Context) func() {
	if tx, ok := ctx.Value(transactionKey{}).(*transaction); ok && tx.owner == l {
		return func() {}
	}
	l.mu.Lock()
	return l.mu.Unlock
}
func New(helper, user core.ID, goal assistance.Goal) *Local {
	return &Local{helper: helper, user: user, goal: goal, allowed: true, requests: map[core.ID]assistance.Request{}, entries: map[graph.MemoryScope][]graph.MemoryEntry{}}
}
func copyValue[T any](v T) T {
	b, _ := json.Marshal(v)
	var out T
	_ = json.Unmarshal(b, &out)
	return out
}

// Register is trusted synthetic host composition, not a user-controlled endpoint.
func (l *Local) Register(r assistance.Request) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if r.Validate() != nil || r.Helper != l.helper || r.User != l.user || r.Goal != l.goal || len(l.requests) >= assistance.MaxHistory {
		return assistance.ErrInvalid
	}
	if old, ok := l.requests[r.ID]; ok && assistance.Digest(old) != assistance.Digest(r) {
		return assistance.ErrInvalid
	}
	if r.Version == assistance.ScopedVersion {
		if _, exists := l.requests[r.ID]; !exists && r.At < l.now {
			return assistance.ErrDenied
		}
		if r.At > l.now {
			l.now = r.At
		}
	}
	l.requests[r.ID] = copyValue(r)
	return nil
}
func (l *Local) SetMemory(scope graph.MemoryScope, entries []graph.MemoryEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries[scope] = copyValue(entries)
}
func (l *Local) SetAllowed(v bool) { l.mu.Lock(); defer l.mu.Unlock(); l.allowed = v }
func (l *Local) Check(ctx context.Context, r assistance.Request, c assistance.Candidate) error {
	defer l.lock(ctx)()
	registered, ok := l.requests[r.ID]
	if !ok || assistance.Digest(registered) != assistance.Digest(r) || r.Helper != l.helper || r.User != l.user || r.Goal != l.goal || (!l.allowed && c.Action != assistance.Wait) {
		return assistance.ErrDenied
	}
	return nil
}
func (l *Local) Read(ctx context.Context, r assistance.Request) ([]assistance.Interaction, error) {
	defer l.lock(ctx)()
	if r.Helper != l.helper || r.User != l.user {
		return nil, assistance.ErrDenied
	}
	return copyValue(l.history), nil
}
func (l *Local) Commit(ctx context.Context, r assistance.Request, out assistance.Interaction, validate func(context.Context) error) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	tx := context.WithValue(ctx, transactionKey{}, &transaction{owner: l})
	if ctx.Err() != nil || validate(tx) != nil {
		return assistance.ErrDenied
	}
	for _, old := range l.history {
		if old.ID == out.ID {
			if assistance.Digest(old) != assistance.Digest(out) {
				return assistance.ErrInvalid
			}
			return nil
		}
	}
	if len(l.history) >= assistance.MaxHistory {
		return assistance.ErrDenied
	}
	l.history = append(l.history, copyValue(out))
	if out.Delivered {
		l.deliveries++
	}
	return nil
}
func (l *Local) Deliveries() int { l.mu.Lock(); defer l.mu.Unlock(); return l.deliveries }
func (l *Local) ReadMemory(ctx context.Context, s graph.MemoryScope) ([]graph.MemoryEntry, error) {
	defer l.lock(ctx)()
	return copyValue(l.entries[s]), nil
}
func (l *Local) AppendMemory(context.Context, graph.AppendCommand) (graph.AppendResult, error) {
	return graph.AppendResult{}, assistance.ErrDenied
}
func (l *Local) ValidateContext(ctx context.Context, p graph.ContextProposal) (bool, error) {
	defer l.lock(ctx)()
	r, ok := l.requests[p.Binding]
	if !ok {
		return false, nil
	}
	for _, allowed := range r.Contexts {
		if assistance.Digest(allowed) == assistance.Digest(p) {
			return true, nil
		}
	}
	return false, nil
}
func (l *Local) IsFictional(context.Context, core.ID, core.ID) (bool, error) { return false, nil }
func (l *Local) RecordPolicyDecision(ctx context.Context, audit graph.PolicyAudit) error {
	defer l.lock(ctx)()
	if len(l.audits) >= 1024 {
		return assistance.ErrDenied
	}
	l.audits = append(l.audits, copyValue(audit))
	return nil
}
func (l *Local) Host(planner assistance.Planner) assistance.Host {
	return assistance.Host{Policy: graph.NewPolicyService(l, l, l), Eligibility: &assistance.BoundaryGate{Auth: l, Source: l, History: l}, Journal: l, Planner: planner}
}
func Run(ctx context.Context) (assistance.Interaction, error) {
	local := New("helper", "person", assistance.Listen)
	r := assistance.Request{Version: assistance.Version, ID: "second-host", Helper: "helper", User: "person", Purpose: "help", Participants: []core.ID{"person"}, Arm: assistance.Simple, Goal: assistance.Listen, At: 1}
	if e := local.Register(r); e != nil {
		return assistance.Interaction{}, e
	}
	return local.Host(assistance.FakePlanner{}).Execute(ctx, r, nil)
}
