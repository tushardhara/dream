// Package groupclient is an in-memory second host for reusable group assistance.
// It receives core records, not a mandatory simulator world or model provider.
package groupclient

import (
	"context"
	"encoding/json"
	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
	"sync"
)

type Local struct {
	mu           sync.Mutex
	session      core.ID
	now          core.LogicalTime
	history      core.GroupHistory
	reservations []core.GroupReservation
	budget       core.GroupBudget
	boundaries   []core.Boundary
	requests     map[core.ID]assistance.GroupRequest
	responses    map[core.ID]assistance.GroupResponse
}

func copyValue[T any](v T) T {
	raw, _ := json.Marshal(v)
	var out T
	_ = json.Unmarshal(raw, &out)
	return out
}
func New(session core.ID, h core.GroupHistory, b core.GroupBudget, rs []core.GroupReservation, boundaries []core.Boundary, now core.LogicalTime) (*Local, error) {
	if session.Validate() != nil || h.Validate() != nil || b.Validate() != nil || core.ValidateGroupPortfolio(h, rs, b) != nil || core.ValidateBoundaryLog(boundaries) != nil || now < 0 {
		return nil, assistance.ErrInvalid
	}
	return &Local{session: session, history: copyValue(h), budget: copyValue(b), reservations: copyValue(rs), boundaries: copyValue(boundaries), now: now, requests: map[core.ID]assistance.GroupRequest{}, responses: map[core.ID]assistance.GroupResponse{}}, nil
}
func (l *Local) Request(actor, id, decision core.ID, at core.LogicalTime) (assistance.GroupRequest, error) {
	return l.request(actor, id, decision, at, assistance.GroupFlowVersion, "")
}

// ArmedRequest builds the opt-in v2 request carrying a matched-arm label. It is
// a separate entry point so the v1 path stays byte-identical for every existing
// caller; the admission checks below are shared, not duplicated.
func (l *Local) ArmedRequest(actor, id, decision core.ID, at core.LogicalTime, arm assistance.Arm) (assistance.GroupRequest, error) {
	return l.request(actor, id, decision, at, assistance.GroupArmVersion, arm)
}

func (l *Local) request(actor, id, decision core.ID, at core.LogicalTime, version string, arm assistance.Arm) (assistance.GroupRequest, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	r := assistance.GroupRequest{Version: version, ID: id, Session: l.session, User: actor, Helper: "helper", Decision: decision, Purpose: "help", At: at, Arm: arm}
	if r.Validate() != nil || at < l.now {
		return r, assistance.ErrDenied
	}
	member := false
	for _, d := range l.history.Decisions {
		if d.ID == decision {
			for _, p := range d.Affected {
				member = member || p == actor
			}
		}
	}
	if !member {
		return r, assistance.ErrDenied
	}
	if old, ok := l.requests[id]; ok {
		if assistance.Digest(old) != assistance.Digest(r) {
			return r, assistance.ErrDenied
		}
		return r, nil
	}
	if len(l.requests) >= 32 {
		return r, assistance.ErrDenied
	}
	l.requests[id] = r
	l.now = at
	return r, nil
}
func (l *Local) snapshot() assistance.GroupSnapshot {
	return assistance.GroupSnapshot{Now: l.now, History: copyValue(l.history), Reservations: copyValue(l.reservations), Budget: copyValue(l.budget), Boundaries: copyValue(l.boundaries)}
}
func (l *Local) VisitGroup(ctx context.Context, r assistance.GroupRequest, fn func(assistance.GroupSnapshot) (assistance.GroupResponse, error)) (assistance.GroupResponse, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	old, ok := l.requests[r.ID]
	if !ok || assistance.Digest(old) != assistance.Digest(r) || ctx.Err() != nil {
		return assistance.GroupResponse{}, assistance.ErrDenied
	}
	result, e := fn(l.snapshot())
	if ctx.Err() != nil {
		return assistance.GroupResponse{}, assistance.ErrDenied
	}
	if e != nil {
		return assistance.GroupResponse{}, e
	}
	if old, ok := l.responses[r.ID]; ok && assistance.Digest(old) != assistance.Digest(result) {
		return assistance.GroupResponse{}, assistance.ErrDenied
	}
	l.responses[r.ID] = copyValue(result)
	return copyValue(result), nil
}
func (l *Local) Host() assistance.GroupHost { return assistance.GroupHost{Journal: l} }

// Reserve is a separate explicit authenticated human operation. Helper Execute
// never calls it. It checks all current rights, boundaries and resource conflicts
// under the same mutex as correction/revocation/reservations, including replay.
func (l *Local) Reserve(actor, id, decision, option core.ID, at core.LogicalTime) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if at < l.now {
		return assistance.ErrDenied
	}
	var d core.GroupDecision
	for _, v := range l.history.Decisions {
		if v.ID == decision {
			d = v
		}
	}
	if actor != d.Meta.Observer {
		return assistance.ErrDenied
	}
	r := assistance.GroupRequest{Version: assistance.GroupFlowVersion, ID: id, Session: l.session, User: actor, Helper: "helper", Decision: decision, Purpose: "help", At: at}
	s := l.snapshot()
	s.Now = at
	if !assistance.GroupBoundariesAllow(r, s, d) {
		return assistance.ErrDenied
	}
	grants := []core.Grant{{Actor: actor, Recipient: actor, Purpose: "help", Operation: core.Read}, {Actor: actor, Recipient: actor, Purpose: "help", Operation: core.Derive}}
	next, e := core.ReserveGroup(l.history, l.reservations, l.budget, decision, option, id, actor, at, grants)
	if e != nil {
		return e
	}
	l.reservations = next
	l.now = at
	return nil
}
func (l *Local) Correct(actor core.ID, a core.GroupAccount) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if actor != a.Member || a.Supersedes == "" || a.LearnedAt < l.now {
		return assistance.ErrDenied
	}
	next := copyValue(l.history)
	next.Accounts = append(next.Accounts, copyValue(a))
	if next.Validate() != nil {
		return assistance.ErrInvalid
	}
	l.history = next
	l.now = a.LearnedAt
	return nil
}
func (l *Local) Revoke(actor, id core.ID) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, a := range l.history.Accounts {
		if a.Meta.ID == id && a.Member == actor {
			l.history.Accounts[i].Meta.Rights.Revoked = true
			return nil
		}
	}
	for i, d := range l.history.Decisions {
		if d.Meta.ID == id && d.Meta.Observer == actor {
			l.history.Decisions[i].Meta.Rights.Revoked = true
			return nil
		}
	}
	return assistance.ErrDenied
}
func (l *Local) AppendBoundary(actor core.ID, b core.Boundary) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if b.Principal != actor || b.Meta.Observer != actor || b.LearnedAt < l.now {
		return assistance.ErrDenied
	}
	next := append(copyValue(l.boundaries), b)
	if core.ValidateBoundaryLog(next) != nil {
		return assistance.ErrInvalid
	}
	l.boundaries = next
	l.now = b.LearnedAt
	return nil
}
func (l *Local) Reservations() []core.GroupReservation {
	l.mu.Lock()
	defer l.mu.Unlock()
	return copyValue(l.reservations)
}
