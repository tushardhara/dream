// Package repairclient is an offline, authenticated-in-process reference host.
// Commands execute bounded resource effects; observations are separate voluntary
// participant submissions. It is not a human behavior model or deployed store.
package repairclient

import (
	"context"
	"encoding/json"
	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
	"sync"
	"time"
)

type Local struct {
	mu         sync.Mutex
	session    core.ID
	now        core.LogicalTime
	log        []core.RepairRecord
	boundaries []core.Boundary
	resources  map[core.ID]int64
	requests   map[core.ID]assistance.RepairRequest
	responses  map[core.ID]assistance.RepairResponse
}

func copyValue[T any](v T) T {
	raw, _ := json.Marshal(v)
	var out T
	_ = json.Unmarshal(raw, &out)
	return out
}
func New(session core.ID) *Local {
	return &Local{session: session, resources: map[core.ID]int64{"alice": 8, "bob": 8}, requests: map[core.ID]assistance.RepairRequest{}, responses: map[core.ID]assistance.RepairResponse{}}
}
func Focus() core.RelationshipFocus {
	return core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: core.Finances, RoleContext: "household"}
}
func participant(id core.ID) bool { return id == "alice" || id == "bob" }

// Permissions is an explicit fixture sharing choice, never an automatic right
// from being a partner. Every submitter chooses the permitted derived recipients.
func Permissions(owner core.ID, recipients ...core.ID) []core.Grant {
	out := []core.Grant{}
	people := map[core.ID]bool{}
	for _, p := range append([]core.ID{owner, "helper"}, recipients...) {
		if !people[p] {
			people[p] = true
			for _, op := range []core.Operation{core.Read, core.Derive} {
				out = append(out, core.Grant{Actor: p, Recipient: p, Purpose: "help", Operation: op})
			}
		}
	}
	for _, p := range recipients {
		out = append(out, core.Grant{Actor: "helper", Recipient: p, Purpose: "help", Operation: core.ShareOnRequest})
	}
	return out
}
func (l *Local) record(owner, id core.ID, at core.LogicalTime, grants []core.Grant) core.RepairRecord {
	return core.RepairRecord{Version: core.RepairVersion, Actor: "alice", Recipient: "bob", Focus: Focus(), Episode: "shared-expenses", OccurredAt: at, EffectAt: at, LearnedAt: at, Meta: core.Metadata{ID: id, Observer: owner, Source: owner, Sensitivity: core.Restricted, Confidence: .8, Valid: core.Interval{Start: at}, RecordedAt: time.Unix(int64(at), 0).UTC(), Rights: core.Rights{Resource: id, Grants: copyValue(grants)}}}
}
func (l *Local) append(actor core.ID, r core.RepairRecord) error {
	if !participant(actor) || r.Meta.Observer != actor || r.LearnedAt < l.now {
		return assistance.ErrDenied
	}
	next := append(copyValue(l.log), copyValue(r))
	if core.ValidateRepairLog(next) != nil {
		return assistance.ErrInvalid
	}
	l.log = next
	l.now = r.LearnedAt
	return nil
}

type ActionCommand struct {
	Focus      core.RelationshipFocus
	Episode    core.ID
	ID         core.ID
	Kind       string
	At         core.LogicalTime
	Commitment core.ID
	Due        *core.LogicalTime
	Units      int64
	Resource   core.ID
	Grants     []core.Grant
}

func (l *Local) Act(actor core.ID, c ActionCommand) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	r := l.record(actor, c.ID, c.At, c.Grants)
	if c.Focus.Version != "" {
		r.Focus = c.Focus
	}
	if c.Episode != "" {
		r.Episode = c.Episode
	}
	r.Kind = "action"
	r.Action = c.Kind
	r.Commitment = c.Commitment
	r.Due = c.Due
	r.Resource = c.Resource
	r.Units = c.Units
	if c.Kind != "wait" {
		r.EffectAt++
		r.LearnedAt = r.EffectAt
	}
	if c.Kind == "help" && (c.Resource != "hours" || l.resources[actor] < c.Units) || c.Kind == "promise" && (c.Resource != "hours" || l.resources[actor] < c.Units) {
		return assistance.ErrDenied
	}
	// Contact choices in this exact topic prevent practical actions from silently
	// overriding a participant's refusal/pause/ending. Ordinary WAIT stays possible.
	for _, v := range l.log {
		if !v.Meta.Rights.Revoked && v.Focus == r.Focus && v.Kind == "action" && (v.Action == "decline" || v.Action == "withdraw" || v.Action == "leave") && c.Kind != "wait" && c.Kind != "withdraw" && c.Kind != "leave" && c.Kind != "decline" {
			return assistance.ErrDenied
		}
	}
	if e := l.append(actor, r); e != nil {
		return e
	}
	if c.Kind == "help" {
		l.resources[actor] -= c.Units
	}
	return nil
}
func (l *Local) Submit(actor, id, reference core.ID, kind, finding, phase, assessment string, at core.LogicalTime, grants []core.Grant) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	r := l.record(actor, id, at, grants)
	r.Kind = kind
	r.Reference = reference
	r.Finding = finding
	r.Phase = phase
	r.Assessment = assessment
	r.Meta.Supporting = []core.ID{reference}
	for _, v := range l.log {
		if v.Meta.ID == reference {
			r.Focus = v.Focus
			r.Episode = v.Episode
			if kind == "observation" {
				r.Commitment = v.Commitment
			}
		}
	}
	return l.append(actor, r)
}
func (l *Local) Correct(actor, oldID, newID core.ID, finding, assessment string, learned core.LogicalTime) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, old := range l.log {
		if old.Meta.ID == oldID {
			if old.Kind == "action" || old.Meta.Rights.Revoked {
				return assistance.ErrDenied
			}
			r := copyValue(old)
			r.Meta.ID = newID
			r.Meta.Rights.Resource = newID
			r.Supersedes = oldID
			r.Finding = finding
			r.Assessment = assessment
			r.LearnedAt = learned
			r.Meta.RecordedAt = time.Unix(int64(learned), 0).UTC()
			return l.append(actor, r)
		}
	}
	return assistance.ErrDenied
}
func (l *Local) Revoke(actor, id core.ID) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, v := range l.log {
		if v.Meta.ID == id && v.Meta.Observer == actor {
			l.log[i].Meta.Rights.Revoked = true
			return nil
		}
	}
	return assistance.ErrDenied
}
func (l *Local) Boundary(actor core.ID, decision core.Willingness, at core.LogicalTime) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !participant(actor) || at < l.now {
		return assistance.ErrDenied
	}
	other := core.ID("alice")
	if actor == "alice" {
		other = "bob"
	}
	id := core.ID(string(actor) + "-" + string(decision))
	b := core.Boundary{Version: core.BoundaryVersion, Meta: core.Metadata{ID: id, Observer: actor, Source: actor, Sensitivity: core.Restricted, Confidence: 1, Supporting: []core.ID{"participant-choice"}, Valid: core.Interval{Start: at}, RecordedAt: time.Unix(int64(at), 0).UTC(), Rights: core.Rights{Resource: id}}, Principal: actor, With: other, Topic: core.ID(core.Finances), Class: core.PrivatePreparation, Decision: decision, Basis: "self_report", OccurredAt: at, LearnedAt: at}
	next := append(copyValue(l.boundaries), b)
	if core.ValidateBoundaryLog(next) != nil {
		return assistance.ErrInvalid
	}
	l.boundaries = next
	l.now = at
	return nil
}
func (l *Local) Request(actor, id core.ID, at core.LogicalTime) (assistance.RepairRequest, error) {
	return l.RequestForFocus(actor, id, at, Focus())
}
func (l *Local) RequestForFocus(actor, id core.ID, at core.LogicalTime, focus core.RelationshipFocus) (assistance.RepairRequest, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	r := assistance.RepairRequest{Version: assistance.RepairFlowVersion, ID: id, Session: l.session, Helper: "helper", User: actor, Actor: "alice", Recipient: "bob", Purpose: "help", Focus: focus, At: at}
	if r.Validate() != nil {
		return r, assistance.ErrDenied
	}
	if old, ok := l.requests[id]; ok {
		if assistance.Digest(old) != assistance.Digest(r) {
			return r, assistance.ErrDenied
		}
		return r, nil
	}
	if at < l.now || len(l.requests) >= 32 {
		return r, assistance.ErrDenied
	}
	l.requests[id] = r
	l.now = at
	return r, nil
}
func (l *Local) Visit(ctx context.Context, r assistance.RepairRequest, f func(assistance.RepairSnapshot) (assistance.RepairResponse, error)) (assistance.RepairResponse, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	registered, ok := l.requests[r.ID]
	if !ok || assistance.Digest(registered) != assistance.Digest(r) || ctx.Err() != nil {
		return assistance.RepairResponse{}, assistance.ErrDenied
	}
	result, e := f(assistance.RepairSnapshot{Now: l.now, Log: copyValue(l.log), Boundaries: copyValue(l.boundaries)})
	if e != nil {
		return assistance.RepairResponse{}, e
	}
	if old, ok := l.responses[r.ID]; ok && assistance.Digest(old) != assistance.Digest(result) {
		return assistance.RepairResponse{}, assistance.ErrDenied
	}
	l.responses[r.ID] = copyValue(result)
	return copyValue(result), nil
}
func (l *Local) Host() assistance.RepairHost { return assistance.RepairHost{Journal: l} }
func (l *Local) Evidence() []core.RepairRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	return copyValue(l.log)
}
func (l *Local) Remaining(actor core.ID) int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.resources[actor]
}
