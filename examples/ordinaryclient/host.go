// Package ordinaryclient is a bounded in-memory second host. Authenticated actor
// arguments represent trusted composition, never an unauthenticated endpoint.
package ordinaryclient

import (
	"context"
	"encoding/json"
	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
	"sync"
	"time"
)

type key struct {
	Person, Activity core.ID
	Kind             core.OrdinaryKind
}
type Local struct {
	mu              sync.Mutex
	session, helper core.ID
	people          map[core.ID]bool
	now             core.LogicalTime
	entries         map[graph.MemoryScope][]graph.MemoryEntry
	preferences     map[key]core.OrdinaryPreference
	stories         map[key]core.OrdinaryStory
	requests        map[core.ID]assistance.OrdinaryRequest
	responses       []assistance.OrdinaryResponse
	experiences     []core.OrdinaryExperience
	boundaries      []core.Boundary
	groups          core.GroupHistory
	reservations    []core.GroupReservation
	budget          core.GroupBudget
	audits          []graph.PolicyAudit
}
type transactionKey struct{}
type transaction struct{ owner *Local }

func copyValue[T any](v T) T {
	raw, _ := json.Marshal(v)
	var out T
	_ = json.Unmarshal(raw, &out)
	return out
}
func (l *Local) lock(ctx context.Context) func() {
	if tx, ok := ctx.Value(transactionKey{}).(*transaction); ok && tx.owner == l {
		return func() {}
	}
	l.mu.Lock()
	return l.mu.Unlock
}
func New(session, helper core.ID, people []core.ID, h core.GroupHistory, b core.GroupBudget, rs []core.GroupReservation, boundaries []core.Boundary) (*Local, error) {
	if session.Validate() != nil || helper.Validate() != nil || len(people) < 2 || len(people) > 8 || core.ValidateGroupPortfolio(h, rs, b) != nil || core.ValidateBoundaryLog(boundaries) != nil {
		return nil, assistance.ErrInvalid
	}
	known := map[core.ID]bool{}
	for _, p := range people {
		if p.Validate() != nil || p == helper || known[p] {
			return nil, assistance.ErrInvalid
		}
		known[p] = true
	}
	return &Local{session: session, helper: helper, people: known, entries: map[graph.MemoryScope][]graph.MemoryEntry{}, preferences: map[key]core.OrdinaryPreference{}, stories: map[key]core.OrdinaryStory{}, requests: map[core.ID]assistance.OrdinaryRequest{}, responses: []assistance.OrdinaryResponse{}, experiences: []core.OrdinaryExperience{}, boundaries: copyValue(boundaries), groups: copyValue(h), reservations: copyValue(rs), budget: copyValue(b)}, nil
}
func (l *Local) appendMemory(actor, id core.ID, raw []byte, sensitivity core.Sensitivity, share, parents []core.ID, corrects core.ID, at core.LogicalTime) error {
	if !l.people[actor] || at < l.now {
		return assistance.ErrDenied
	}
	scope := graph.MemoryScope{Owner: actor, Namespace: l.session}
	entries := l.entries[scope]
	if len(entries) >= 32 {
		return assistance.ErrDenied
	}
	for _, old := range entries {
		if old.Event.Meta.ID == id {
			return assistance.ErrInvalid
		}
	}
	grants := []core.Grant{{Actor: l.helper, Recipient: l.helper, Purpose: "help", Operation: core.Read}, {Actor: l.helper, Recipient: l.helper, Purpose: "help", Operation: core.Derive}}
	seen := map[core.ID]bool{}
	for _, p := range share {
		if !l.people[p] || seen[p] {
			return assistance.ErrDenied
		}
		seen[p] = true
		grants = append(grants, core.Grant{Actor: l.helper, Recipient: p, Purpose: "help", Operation: core.ShareOnRequest})
	}
	e := graph.MemoryEntry{Sequence: int64(len(entries) + 1), Event: core.Event{Version: 1, Type: graph.MemoryEventType, Stream: "ordinary", Subject: core.Subject{Principal: actor}, OccurredAt: at, Meta: core.Metadata{ID: id, Observer: actor, Source: actor, Sensitivity: sensitivity, Confidence: .8, Parents: parents, Valid: core.Interval{Start: at}, RecordedAt: time.Unix(int64(at), 0).UTC(), Rights: core.Rights{Resource: id, Grants: grants}}}, Content: &graph.MemoryContent{Version: 1, Kind: graph.EpisodicMemory, Text: string(raw), Salience: .5, HalfLife: 1000, Learned: []graph.Learned{{Actor: actor, At: at}, {Actor: l.helper, At: at}}}, Supersedes: corrects}
	next := append(copyValue(entries), e)
	if _, e := graph.RebuildMemory(scope, next); e != nil {
		return e
	}
	l.entries[scope] = copyValue(next)
	l.now = at
	return nil
}
func (l *Local) PutPreference(actor core.ID, p core.OrdinaryPreference, share []core.ID, at core.LogicalTime) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if actor != p.Person || p.Validate() != nil {
		return assistance.ErrDenied
	}
	k := key{p.Person, p.Activity, p.Kind}
	old, exists := l.preferences[k]
	if exists && p.Corrects != old.Source || !exists && p.Corrects != "" {
		return assistance.ErrInvalid
	}
	raw, _ := core.EncodeOrdinaryPreference(p)
	if e := l.appendMemory(actor, p.Source, raw, core.Public, share, nil, p.Corrects, at); e != nil {
		return e
	}
	l.preferences[k] = copyValue(p)
	return nil
}
func (l *Local) PutStory(actor core.ID, s core.OrdinaryStory, sensitivity core.Sensitivity, share, parents []core.ID, at core.LogicalTime) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if actor != s.Author || s.Validate() != nil {
		return assistance.ErrDenied
	}
	k := key{s.Author, s.Activity, s.Kind}
	old, exists := l.stories[k]
	if exists && s.Corrects != old.Source || !exists && s.Corrects != "" {
		return assistance.ErrInvalid
	}
	raw, _ := core.EncodeOrdinaryStory(s)
	if e := l.appendMemory(actor, s.Source, raw, sensitivity, share, parents, s.Corrects, at); e != nil {
		return e
	}
	l.stories[k] = copyValue(s)
	return nil
}
func (l *Local) Revoke(actor, id core.ID) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	scope := graph.MemoryScope{Owner: actor, Namespace: l.session}
	for i, e := range l.entries[scope] {
		if e.Event.Meta.ID == id {
			l.entries[scope][i].Revoked = true
			l.entries[scope][i].Content = nil
			return nil
		}
	}
	return assistance.ErrDenied
}
func (l *Local) AppendBoundary(actor core.ID, b core.Boundary) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.people[actor] || b.Principal != actor || b.Meta.Observer != actor || b.Meta.Source != actor || b.LearnedAt < l.now {
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

// Request constructs the only approved context proposals from current actor-owned
// records. The caller selects an opportunity, not source IDs, source text or grants.
func (l *Local) Request(actor core.ID, r assistance.OrdinaryRequest, storyAuthor core.ID) (assistance.OrdinaryRequest, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.people[actor] || r.At < l.now {
		return r, assistance.ErrDenied
	}
	r = copyValue(r)
	r.Version, r.Session, r.Helper, r.User = core.OrdinaryVersion, l.session, l.helper, actor
	r.Preferences = nil
	r.Story = nil
	proposal := func(owner, source core.ID) graph.ContextProposal {
		return graph.ContextProposal{Version: 2, Binding: r.ID, Recipient: l.helper, Operation: core.Read, Mode: graph.ExternalContext, Sources: []core.ID{source}, Query: graph.MemoryQuery{Scope: graph.MemoryScope{Owner: owner, Namespace: l.session}, Actor: l.helper, Purpose: "help", Subject: core.Subject{Principal: owner}, ValidAt: r.At, KnownAt: r.At, RecordedAsOf: time.Unix(int64(r.At)+1, 0).UTC(), Limit: 1}}
	}
	for _, p := range r.Participants {
		if !l.people[p] {
			return r, assistance.ErrDenied
		}
		if pref, ok := l.preferences[key{p, r.Activity, r.Kind}]; ok {
			r.Preferences = append(r.Preferences, proposal(p, pref.Source))
		}
	}
	if s, ok := l.stories[key{storyAuthor, r.Activity, r.Kind}]; ok {
		p := proposal(storyAuthor, s.Source)
		r.Story = &p
	}
	if r.Validate() != nil {
		return r, assistance.ErrInvalid
	}
	if old, ok := l.requests[r.ID]; ok {
		if assistance.Digest(old) != assistance.Digest(r) {
			return r, assistance.ErrDenied
		}
		return r, nil
	}
	if len(l.requests) >= assistance.MaxHistory {
		return r, assistance.ErrDenied
	}
	l.requests[r.ID] = copyValue(r)
	l.now = r.At
	return copyValue(r), nil
}
func (l *Local) snapshot(r assistance.OrdinaryRequest) assistance.OrdinarySnapshot {
	current := map[core.ID]core.ID{}
	for _, p := range r.Participants {
		if v, ok := l.preferences[key{p, r.Activity, r.Kind}]; ok {
			current[p] = v.Source
		}
	}
	story := core.ID("")
	if r.Story != nil {
		story = l.stories[key{r.Story.Query.Scope.Owner, r.Activity, r.Kind}].Source
	}
	history := []assistance.OrdinaryResponse{}
	for _, v := range l.responses {
		if v.User == r.User {
			history = append(history, copyValue(v))
		}
	}
	participation := []assistance.OrdinaryParticipation{}
	for _, e := range l.experiences {
		prior, ok := l.requests[e.Opportunity]
		if ok && prior.User == r.User && prior.Activity == r.Activity && e.LearnedAt <= l.now {
			participation = append(participation, assistance.OrdinaryParticipation{Opportunity: e.Opportunity, Participant: e.Participant, At: e.OccurredAt, Choice: e.Participation})
		}
	}
	return assistance.OrdinarySnapshot{Participation: participation, Now: l.now, CurrentPreferences: current, CurrentStory: story, Boundaries: copyValue(l.boundaries), History: history, Groups: copyValue(l.groups), Reservations: copyValue(l.reservations), Budget: copyValue(l.budget)}
}
func (l *Local) VisitOrdinary(ctx context.Context, r assistance.OrdinaryRequest, fn func(context.Context, assistance.OrdinarySnapshot) (assistance.OrdinaryResponse, error)) (assistance.OrdinaryResponse, error) {
	defer l.lock(ctx)()
	old, ok := l.requests[r.ID]
	if !ok || assistance.Digest(old) != assistance.Digest(r) || ctx.Err() != nil {
		return assistance.OrdinaryResponse{}, assistance.ErrDenied
	}
	tx := context.WithValue(ctx, transactionKey{}, &transaction{owner: l})
	out, e := fn(tx, l.snapshot(r))
	if e != nil {
		return assistance.OrdinaryResponse{}, e
	}
	if ctx.Err() != nil {
		return assistance.OrdinaryResponse{}, assistance.ErrDenied
	}
	for _, old := range l.responses {
		if old.ID == r.ID {
			if assistance.Digest(old) != assistance.Digest(out) {
				return assistance.OrdinaryResponse{}, assistance.ErrDenied
			}
			return copyValue(out), nil
		}
	}
	if len(l.responses) >= assistance.MaxHistory {
		return assistance.OrdinaryResponse{}, assistance.ErrDenied
	}
	l.responses = append(l.responses, copyValue(out))
	return copyValue(out), nil
}
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
	proposals := append([]graph.ContextProposal{}, r.Preferences...)
	if r.Story != nil {
		proposals = append(proposals, *r.Story)
	}
	for _, base := range proposals {
		for _, variant := range []string{"read", "derive", "share"} {
			q := copyValue(base)
			if variant == "derive" {
				q.Mode, q.Operation = graph.InternalContext, core.Derive
			}
			if variant == "share" {
				q.Mode, q.Operation, q.Recipient = graph.AssistantDisclosure, core.ShareOnRequest, r.User
			}
			if assistance.Digest(p) == assistance.Digest(q) {
				return true, nil
			}
		}
	}
	return false, nil
}
func (l *Local) IsFictional(context.Context, core.ID, core.ID) (bool, error) { return false, nil }
func (l *Local) RecordPolicyDecision(ctx context.Context, a graph.PolicyAudit) error {
	defer l.lock(ctx)()
	if len(l.audits) >= 4096 {
		return assistance.ErrDenied
	}
	l.audits = append(l.audits, copyValue(a))
	return nil
}
func (l *Local) Host() assistance.OrdinaryHost {
	return assistance.OrdinaryHost{Policy: graph.NewPolicyService(l, l, l), Journal: l}
}

// Reserve is a separate explicit human operation through the existing core group
// reservation contract. A helper proposal alone never spends resources.
func (l *Local) Reserve(ctx context.Context, actor, request, id core.ID) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	r, ok := l.requests[request]
	if !ok || r.User != actor || !r.Requested || ctx.Err() != nil {
		return assistance.ErrDenied
	}
	tx := context.WithValue(ctx, transactionKey{}, &transaction{owner: l})
	out, e := l.Host().Execute(tx, r, nil)
	if e != nil || out.Action != string(core.OrdinaryCoordination) {
		return assistance.ErrDenied
	}
	gs := []core.Grant{{Actor: actor, Recipient: actor, Purpose: "help", Operation: core.Read}, {Actor: actor, Recipient: actor, Purpose: "help", Operation: core.Derive}}
	next, e := core.ReserveGroup(l.groups, l.reservations, l.budget, out.PlanDecision, out.PlanOption, id, actor, l.now, gs)
	if e != nil {
		return e
	}
	l.reservations = next
	return nil
}

// Observe authenticates a distinct later participant report, not an invitation or
// reply counter. It does not update relationship memory, rank, trust or contact.
func (l *Local) Observe(actor core.ID, e core.OrdinaryExperience) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.people[actor] || e.Participant != actor || e.Validate() != nil || e.LearnedAt < l.now {
		return assistance.ErrDenied
	}
	r, ok := l.requests[e.Opportunity]
	if !ok || e.OccurredAt <= r.At {
		return assistance.ErrDenied
	}
	participant := false
	for _, p := range r.Participants {
		participant = participant || p == actor
	}
	if !participant {
		return assistance.ErrDenied
	}
	found := false
	for _, o := range l.responses {
		found = found || o.ID == e.Opportunity
	}
	if !found {
		return assistance.ErrDenied
	}
	for _, old := range l.experiences {
		if old.ID == e.ID || old.Opportunity == e.Opportunity && old.Participant == e.Participant {
			return assistance.ErrDenied
		}
	}
	if len(l.experiences) >= 128 {
		return assistance.ErrDenied
	}
	l.experiences = append(l.experiences, copyValue(e))
	l.now = e.LearnedAt
	return nil
}
func (l *Local) Experiences(actor core.ID) ([]core.OrdinaryExperience, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.people[actor] {
		return nil, assistance.ErrDenied
	}
	out := []core.OrdinaryExperience{}
	for _, e := range l.experiences {
		if e.Participant == actor {
			out = append(out, copyValue(e))
		}
	}
	return out, nil
}
func (l *Local) Reservations() []core.GroupReservation {
	l.mu.Lock()
	defer l.mu.Unlock()
	return copyValue(l.reservations)
}
