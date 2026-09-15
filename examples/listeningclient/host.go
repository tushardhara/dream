// Package listeningclient is a bounded offline host. It exercises the reusable
// helper with synthetic participant-authored memories, not a world or live chat.
package listeningclient

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
)

type Local struct {
	mu         sync.Mutex
	session    core.ID
	helper     core.ID
	people     [2]core.ID
	now        core.LogicalTime
	entries    map[graph.MemoryScope][]graph.MemoryEntry
	current    map[core.ID]core.ListeningAccount
	requests   map[core.ID]assistance.ListeningRequest
	boundaries []core.Boundary
	records    []assistance.ListeningRecord
	audits     []graph.PolicyAudit
}
type transactionKey struct{}
type transaction struct{ host *Local }

func copyValue[T any](v T) T {
	raw, _ := json.Marshal(v)
	var out T
	_ = json.Unmarshal(raw, &out)
	return out
}
func (l *Local) lock(ctx context.Context) func() {
	if tx, ok := ctx.Value(transactionKey{}).(*transaction); ok && tx.host == l {
		return func() {}
	}
	l.mu.Lock()
	return l.mu.Unlock
}
func New(session, helper, a, b core.ID) *Local {
	return &Local{session: session, helper: helper, people: [2]core.ID{a, b}, entries: map[graph.MemoryScope][]graph.MemoryEntry{}, current: map[core.ID]core.ListeningAccount{}, requests: map[core.ID]assistance.ListeningRequest{}}
}
func (l *Local) other(actor core.ID) core.ID {
	if actor == l.people[0] {
		return l.people[1]
	}
	if actor == l.people[1] {
		return l.people[0]
	}
	return ""
}

func (l *Local) appendMemory(actor, id core.ID, text string, sensitivity core.Sensitivity, grants []core.Grant, parents []core.ID, supersedes core.ID, at core.LogicalTime) error {
	scope := graph.MemoryScope{Owner: actor, Namespace: l.session}
	entries := l.entries[scope]
	if l.other(actor) == "" || at < l.now || len(entries) >= 32 {
		return assistance.ErrDenied
	}
	for _, old := range entries {
		if old.Event.Meta.ID == id {
			return assistance.ErrInvalid
		}
	}
	e := graph.MemoryEntry{Sequence: int64(len(entries) + 1), Event: core.Event{Version: 1, Type: graph.MemoryEventType, Stream: "listening", Subject: core.Subject{Principal: actor}, OccurredAt: at, Meta: core.Metadata{ID: id, Observer: actor, Source: actor, Sensitivity: sensitivity, Confidence: .8, Parents: parents, Valid: core.Interval{Start: at}, RecordedAt: time.Unix(int64(at), 0).UTC(), Rights: core.Rights{Resource: id, Grants: grants}}}, Content: &graph.MemoryContent{Version: 1, Kind: graph.EpisodicMemory, Text: text, Salience: .5, HalfLife: 1000, Learned: []graph.Learned{{Actor: actor, At: at}, {Actor: l.helper, At: at}}}, Supersedes: supersedes}
	next := append(copyValue(entries), e)
	if _, err := graph.RebuildMemory(scope, next); err != nil {
		return err
	}
	l.entries[scope] = copyValue(next)
	l.now = at
	return nil
}

// PutAccount represents authenticated participant submission. Only that speaker
// may replace their own current account, in the same domain/frame and pair.
func (l *Local) PutAccount(actor core.ID, a core.ListeningAccount, at core.LogicalTime) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if actor != a.Speaker || a.Other != l.other(actor) || a.Validate() != nil {
		return assistance.ErrDenied
	}
	old, exists := l.current[actor]
	if exists && (a.Corrects != old.Source || a.Focus != old.Focus || a.Confirmation != "corrected") || !exists && a.Corrects != "" {
		return assistance.ErrInvalid
	}
	raw, _ := core.EncodeListeningAccount(a)
	grants := []core.Grant{{Actor: l.helper, Recipient: l.helper, Purpose: "help", Operation: core.Read}}
	if err := l.appendMemory(actor, a.Source, string(raw), core.Restricted, grants, nil, a.Corrects, at); err != nil {
		return err
	}
	l.current[actor] = copyValue(a)
	return nil
}

// PutSummary is an explicit participant-selected text submission, not generated
// paraphrase. Public sensitivity removes the standing strict third-party ban;
// exact recipient/operation grants still apply. Parents retain any declared
// derivation, so a paraphrase of a restricted source cannot bypass lineage gates.
func (l *Local) PutSummary(actor, id core.ID, words string, recipients, parents []core.ID, at core.LogicalTime) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	grants := []core.Grant{{Actor: l.helper, Recipient: l.helper, Purpose: "help", Operation: core.Read}}
	for _, recipient := range recipients {
		if recipient != actor && recipient != l.other(actor) {
			return assistance.ErrDenied
		}
		grants = append(grants, core.Grant{Actor: l.helper, Recipient: recipient, Purpose: "help", Operation: core.ShareOnRequest})
	}
	return l.appendMemory(actor, id, words, core.Public, grants, parents, "", at)
}

func (l *Local) AppendBoundary(actor core.ID, b core.Boundary) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if actor != b.Meta.Observer || l.other(actor) == "" || b.Validate() != nil || b.LearnedAt < l.now {
		return assistance.ErrDenied
	}
	next := append(copyValue(l.boundaries), b)
	if core.ValidateBoundaryLog(next) != nil {
		return assistance.ErrInvalid
	}
	l.boundaries = copyValue(next)
	l.now = b.LearnedAt
	return nil
}

func (l *Local) Revoke(actor, source core.ID) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	scope := graph.MemoryScope{Owner: actor, Namespace: l.session}
	for i, entry := range l.entries[scope] {
		if entry.Event.Meta.ID == source {
			l.entries[scope][i].Revoked = true
			l.entries[scope][i].Content = nil
			return nil
		}
	}
	return assistance.ErrDenied
}

// Request and Register model authenticated host composition, not an endpoint
// that accepts a caller's claimed actor. Tests can submit invalid proposals to
// prove the service independently enforces policy and source matching.
func (l *Local) Request(actor, id core.ID, mode string, focus core.RelationshipFocus, at core.LogicalTime, share, invite bool) assistance.ListeningRequest {
	l.mu.Lock()
	defer l.mu.Unlock()
	r := assistance.ListeningRequest{Version: assistance.ListeningFlowVersion, ID: id, Session: l.session, Helper: l.helper, User: actor, Other: l.other(actor), Purpose: "help", At: at, Focus: focus, Mode: mode, Invite: invite}
	for _, owner := range []core.ID{actor, l.other(actor)} {
		a, ok := l.current[owner]
		if !ok || mode == "private" && owner != actor {
			continue
		}
		p := graph.ContextProposal{Version: 2, Binding: id, Recipient: l.helper, Operation: core.Read, Mode: graph.ExternalContext, Sources: []core.ID{a.Source}, Query: graph.MemoryQuery{Scope: graph.MemoryScope{Owner: owner, Namespace: l.session}, Actor: l.helper, Purpose: "help", Subject: core.Subject{Principal: owner}, ValidAt: at, KnownAt: at, RecordedAsOf: time.Unix(int64(at)+1, 0).UTC(), Limit: 1}}
		r.Accounts = append(r.Accounts, p)
		if share && a.Summary != "" {
			p.Sources = []core.ID{a.Summary}
			p.Mode, p.Operation, p.Recipient = graph.AssistantDisclosure, core.ShareOnRequest, actor
			r.Summaries = append(r.Summaries, p)
		}
	}
	return r
}
func (l *Local) Register(actor core.ID, r assistance.ListeningRequest) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if actor != r.User || r.Other != l.other(actor) || r.Helper != l.helper || r.Session != l.session || r.Validate() != nil || r.At < l.now {
		return assistance.ErrDenied
	}
	if old, exists := l.requests[r.ID]; exists {
		if assistance.Digest(old) != assistance.Digest(r) {
			return assistance.ErrInvalid
		}
		return nil
	}
	if len(l.requests) >= assistance.MaxHistory {
		return assistance.ErrDenied
	}
	l.requests[r.ID], l.now = copyValue(r), r.At
	return nil
}
func (l *Local) Snapshot(ctx context.Context, r assistance.ListeningRequest) (assistance.ListeningSnapshot, error) {
	defer l.lock(ctx)()
	old, ok := l.requests[r.ID]
	if !ok || assistance.Digest(old) != assistance.Digest(r) {
		return assistance.ListeningSnapshot{}, assistance.ErrDenied
	}
	current := map[core.ID]core.ID{}
	for owner, a := range l.current {
		current[owner] = a.Source
	}
	return assistance.ListeningSnapshot{Now: l.now, Current: current, Boundaries: copyValue(l.boundaries)}, nil
}
func (l *Local) History(ctx context.Context, r assistance.ListeningRequest) ([]assistance.ListeningRecord, error) {
	defer l.lock(ctx)()
	if r.Helper != l.helper || r.Session != l.session || l.other(r.User) == "" {
		return nil, assistance.ErrDenied
	}
	return copyValue(l.records), nil
}
func (l *Local) Commit(ctx context.Context, r assistance.ListeningRequest, record assistance.ListeningRecord, validate func(context.Context) error) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	tx := context.WithValue(ctx, transactionKey{}, &transaction{host: l})
	if validate(tx) != nil {
		return assistance.ErrDenied
	}
	for _, old := range l.records {
		if old.ID == record.ID {
			if assistance.Digest(old) != assistance.Digest(record) {
				return assistance.ErrInvalid
			}
			return nil
		}
	}
	if len(l.records) >= assistance.MaxHistory {
		return assistance.ErrDenied
	}
	l.records = append(l.records, copyValue(record))
	return nil
}
func (l *Local) ReadMemory(ctx context.Context, scope graph.MemoryScope) ([]graph.MemoryEntry, error) {
	defer l.lock(ctx)()
	return copyValue(l.entries[scope]), nil
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
	for _, allowed := range append(copyValue(r.Accounts), r.Summaries...) {
		if assistance.Digest(allowed) == assistance.Digest(p) {
			return true, nil
		}
	}
	return false, nil
}
func (*Local) IsFictional(context.Context, core.ID, core.ID) (bool, error) { return false, nil }
func (l *Local) RecordPolicyDecision(ctx context.Context, audit graph.PolicyAudit) error {
	defer l.lock(ctx)()
	if len(l.audits) >= 4096 {
		return assistance.ErrDenied
	}
	l.audits = append(l.audits, copyValue(audit))
	return nil
}
func (l *Local) Host(model assistance.ListeningInterpreter) assistance.ListeningHost {
	return assistance.ListeningHost{Policy: graph.NewPolicyService(l, l, l), Journal: l, Interpreter: model}
}
