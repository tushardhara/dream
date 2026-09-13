package graph

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tushardhara/dream/core"
)

const MemoryEventType core.ID = "memory.v1"
const MaxMemoryRecords = 512
const MaxMemoryResults = 50

type MemoryKind string

const (
	EpisodicMemory     MemoryKind = "episodic"
	SemanticMemory     MemoryKind = "semantic"
	RelationshipMemory MemoryKind = "relationship"
	NarrativeMemory    MemoryKind = "narrative"
	ClaimMemory        MemoryKind = "claim"
	HypothesisMemory   MemoryKind = "hypothesis"
	OpenLoopMemory     MemoryKind = "open_loop"
	IntentMemory       MemoryKind = "intent"
)

type Learned struct {
	Actor core.ID          `json:"actor"`
	At    core.LogicalTime `json:"at"`
}

// MemoryContent is the purgeable part of a memory.v1 event. Metadata and subject
// stay in the existing journal envelope. A hypothesis is always the envelope
// observer's belief, including when the subject is another principal.
type MemoryContent struct {
	Version  uint32            `json:"version"`
	Kind     MemoryKind        `json:"kind"`
	Text     string            `json:"text"`
	Salience float64           `json:"salience"`
	HalfLife core.LogicalTime  `json:"half_life"`
	Learned  []Learned         `json:"learned"`
	Expires  *core.LogicalTime `json:"expires,omitempty"`
}

type MemoryScope struct{ Owner, Namespace core.ID }

func (s MemoryScope) Validate() error {
	if err := s.Owner.Validate(); err != nil {
		return err
	}
	return s.Namespace.Validate()
}

type MemoryRecord struct {
	Event      core.Event    `json:"event"`
	Content    MemoryContent `json:"content"`
	Supersedes core.ID       `json:"supersedes,omitempty"`
}

func (r MemoryRecord) Validate() error {
	if err := r.Event.Validate(); err != nil {
		return err
	}
	if r.Event.Type != MemoryEventType || r.Content.Version != 1 {
		return fmt.Errorf("unsupported memory version")
	}
	c := r.Content
	switch c.Kind {
	case EpisodicMemory, SemanticMemory, RelationshipMemory, NarrativeMemory, ClaimMemory, HypothesisMemory, OpenLoopMemory, IntentMemory:
	default:
		return fmt.Errorf("unknown memory kind")
	}
	if !utf8.ValidString(c.Text) || strings.TrimSpace(c.Text) == "" || len(c.Text) > 2048 {
		return fmt.Errorf("invalid memory text")
	}
	if math.IsNaN(c.Salience) || math.IsInf(c.Salience, 0) || c.Salience < 0 || c.Salience > 1 || c.HalfLife <= 0 {
		return fmt.Errorf("invalid salience/decay")
	}
	if len(c.Learned) == 0 || len(c.Learned) > 32 || len(r.Event.Meta.Rights.Grants) > 128 {
		return fmt.Errorf("memory audience budget")
	}
	seen := map[core.ID]bool{}
	for _, l := range c.Learned {
		if l.Actor.Validate() != nil || l.At < r.Event.OccurredAt || seen[l.Actor] {
			return fmt.Errorf("invalid learned-at audience")
		}
		seen[l.Actor] = true
	}
	if !seen[r.Event.Meta.Observer] {
		return fmt.Errorf("observer learned-at missing")
	}
	if c.Expires != nil && *c.Expires <= r.Event.OccurredAt {
		return fmt.Errorf("invalid expiry")
	}
	if (c.Kind == OpenLoopMemory || c.Kind == IntentMemory) && c.Expires == nil {
		return fmt.Errorf("loop/intent requires expiry")
	}
	if c.Kind == ClaimMemory && len(r.Event.Meta.Supporting)+len(r.Event.Meta.Contradicting) == 0 {
		return fmt.Errorf("claim needs evidence")
	}
	if c.Kind == NarrativeMemory && len(r.Event.Meta.Parents) == 0 {
		return fmt.Errorf("summary needs material sources")
	}
	if r.Supersedes != "" && (r.Supersedes.Validate() != nil || r.Supersedes == r.Event.Meta.ID) {
		return fmt.Errorf("invalid supersession")
	}
	if len(memorySources(r)) > 32 {
		return fmt.Errorf("memory evidence budget")
	}
	return nil
}

// EncodeMemoryPayload excludes all wall-clock metadata from the logical command
// bytes. The database remains the sole authority for Event.Meta.RecordedAt.
func EncodeMemoryPayload(r MemoryRecord) (Payload, error) {
	if err := r.Validate(); err != nil {
		return Payload{}, err
	}
	c := r.Content
	c.Learned = append([]Learned{}, c.Learned...)
	sort.Slice(c.Learned, func(i, j int) bool { return c.Learned[i].Actor < c.Learned[j].Actor })
	if c.Salience == 0 {
		c.Salience = 0
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return Payload{}, err
	}
	p := Payload{Version: 1, Text: string(raw)}
	return p, p.Validate()
}
func DecodeMemoryPayload(event core.Event, supersedes core.ID, p Payload) (MemoryRecord, error) {
	r := MemoryRecord{Event: event, Supersedes: supersedes}
	if err := p.Validate(); err != nil {
		return r, err
	}
	dec := json.NewDecoder(strings.NewReader(p.Text))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r.Content); err != nil {
		return r, err
	}
	canonical, err := EncodeMemoryPayload(r)
	if err != nil {
		return r, err
	}
	if canonical != p {
		return r, fmt.Errorf("noncanonical memory content")
	}
	return r, nil
}
func memorySources(r MemoryRecord) []core.ID {
	ids := append([]core.ID{}, r.Event.Meta.Parents...)
	ids = append(ids, r.Event.Meta.Supporting...)
	ids = append(ids, r.Event.Meta.Contradicting...)
	// Supersession is lineage for revocation, but the replacement need not depend
	// on the superseded proposition's temporal validity for retrieval.
	return ids
}

// MemoryEntry is a trusted storage snapshot entry. Revoked content must be nil;
// the retained envelope and supersession link cannot resurrect purged text.
type MemoryEntry struct {
	Sequence   int64          `json:"sequence"`
	Event      core.Event     `json:"event"`
	Content    *MemoryContent `json:"content,omitempty"`
	Supersedes core.ID        `json:"supersedes,omitempty"`
	Revoked    bool           `json:"revoked"`
}

func (e MemoryEntry) record() MemoryRecord { return MemoryRecord{e.Event, *e.Content, e.Supersedes} }

type MemoryJournal interface {
	// Both methods use a consistent current snapshot; append validates sources
	// under the journal write lock, atomically with the new event and payload.
	ReadMemory(context.Context, MemoryScope) ([]MemoryEntry, error)
	AppendMemory(context.Context, AppendCommand) (AppendResult, error)
}

type MemoryService struct{ Journal MemoryJournal }

func (s MemoryService) Put(ctx context.Context, scope MemoryScope, key core.ID, expected int64, r MemoryRecord, purpose core.ID) (AppendResult, error) {
	if s.Journal == nil {
		return AppendResult{}, fmt.Errorf("memory journal required")
	}
	if err := scope.Validate(); err != nil {
		return AppendResult{}, err
	}
	if r.Event.Meta.Observer != scope.Owner || purpose.Validate() != nil {
		return AppendResult{}, fmt.Errorf("memory owner/purpose mismatch")
	}
	p, err := EncodeMemoryPayload(r)
	if err != nil {
		return AppendResult{}, err
	}
	grant := core.Grant{Actor: scope.Owner, Recipient: scope.Owner, Purpose: purpose, Operation: core.Derive}
	// All memory contents use the private role boundary, including public metadata.
	c := AppendCommand{Actor: scope.Owner, Namespace: scope.Namespace, Operation: "memory.put", Key: key, ExpectedVersion: expected, Event: r.Event, Class: PrivatePayload, Payload: p, Supersedes: r.Supersedes, DerivationContext: &grant}
	if err = c.Validate(); err != nil {
		return AppendResult{}, err
	}
	return s.Journal.AppendMemory(ctx, c)
}

// MemoryIndex is a bounded, disposable projection. Callers cannot mutate its
// internal values. Every retrieval refreshes it from the trusted current journal.
type MemoryIndex struct {
	scope   MemoryScope
	entries []MemoryEntry
	byID    map[core.ID]MemoryEntry
}

func NewMemoryIndex(scope MemoryScope) (*MemoryIndex, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	return &MemoryIndex{scope: scope, byID: map[core.ID]MemoryEntry{}}, nil
}
func RebuildMemory(scope MemoryScope, entries []MemoryEntry) (*MemoryIndex, error) {
	idx, err := NewMemoryIndex(scope)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if err = idx.Apply(e); err != nil {
			return nil, err
		}
	}
	return idx, nil
}

// Apply accepts an ordered journal event once. Repeated delivery is idempotent;
// a current tombstone may replace a live entry but can never be undone by replay.
func (x *MemoryIndex) Apply(e MemoryEntry) error {
	if x == nil || x.byID == nil || x.scope.Validate() != nil {
		return fmt.Errorf("memory index must be initialized")
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	var owned MemoryEntry
	if err = json.Unmarshal(raw, &owned); err != nil {
		return err
	}
	e = owned
	if e.Sequence <= 0 || e.Event.Meta.Observer != x.scope.Owner || e.Event.Type != MemoryEventType || e.Event.Validate() != nil {
		return fmt.Errorf("invalid scoped memory entry")
	}
	if e.Revoked {
		if e.Content != nil {
			return fmt.Errorf("revoked content retained")
		}
	} else {
		if e.Content == nil {
			return fmt.Errorf("missing memory content")
		}
		if _, err = EncodeMemoryPayload(e.record()); err != nil {
			return err
		}
	}
	old, exists := x.byID[e.Event.Meta.ID]
	if exists {
		a, b := old, e
		a.Content = nil
		b.Content = nil
		a.Revoked = false
		b.Revoked = false
		ar, _ := json.Marshal(a)
		br, _ := json.Marshal(b)
		if !bytes.Equal(ar, br) {
			return fmt.Errorf("conflicting memory envelope")
		}
		if old.Revoked {
			return nil
		}
		if !e.Revoked {
			or, _ := json.Marshal(old)
			if !bytes.Equal(or, raw) {
				return fmt.Errorf("conflicting memory content")
			}
			return nil
		}
		for i := range x.entries {
			if x.entries[i].Event.Meta.ID == e.Event.Meta.ID {
				x.entries[i] = e
			}
		}
		x.byID[e.Event.Meta.ID] = e
		return nil
	}
	if len(x.entries) >= MaxMemoryRecords {
		return fmt.Errorf("memory scope budget exhausted")
	}
	if len(x.entries) > 0 && e.Sequence <= x.entries[len(x.entries)-1].Sequence {
		return fmt.Errorf("memory journal out of order")
	}
	sources := append([]core.ID{}, e.Event.Meta.Parents...)
	sources = append(sources, e.Event.Meta.Supporting...)
	sources = append(sources, e.Event.Meta.Contradicting...)
	if e.Supersedes != "" {
		sources = append(sources, e.Supersedes)
	}
	for _, id := range sources {
		p, ok := x.byID[id]
		if !ok {
			return fmt.Errorf("unknown/forward/cyclic memory evidence")
		}
		if id == e.Supersedes && !sameSubject(p.Event.Subject, e.Event.Subject) {
			return fmt.Errorf("supersession subject mismatch")
		}
	}
	x.entries = append(x.entries, e)
	x.byID[e.Event.Meta.ID] = e
	return nil
}
func (x *MemoryIndex) LogicalHash() (string, error) {
	if x == nil || x.byID == nil {
		return "", fmt.Errorf("memory index must be initialized")
	}
	entries := append([]MemoryEntry{}, x.entries...)
	for i := range entries {
		if entries[i].Content != nil {
			p, err := EncodeMemoryPayload(entries[i].record())
			if err != nil {
				return "", err
			}
			var c MemoryContent
			if err = json.Unmarshal([]byte(p.Text), &c); err != nil {
				return "", err
			}
			entries[i].Content = &c
		}
		entries[i].Sequence = 0
		entries[i].Event.Meta.RecordedAt = time.Unix(0, 0).UTC()
		raw, err := core.CanonicalEvent(entries[i].Event)
		if err != nil {
			return "", err
		}
		if err = json.Unmarshal(raw, &entries[i].Event); err != nil {
			return "", err
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Event.Meta.ID < entries[j].Event.Meta.ID })
	raw, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
func sameSubject(a, b core.Subject) bool {
	if a.Principal != b.Principal {
		return false
	}
	if a.Reference == nil || b.Reference == nil {
		return a.Reference == nil && b.Reference == nil
	}
	return *a.Reference == *b.Reference
}

// ValidateMemoryAppend is invoked inside the adapter's write transaction, not as
// a racy preflight in the service. The existing journal enforces grant intersection,
// sensitivity, idempotency, optimistic version, and durable revocation lineage.
func ValidateMemoryAppend(scope MemoryScope, entries []MemoryEntry, c AppendCommand) error {
	if c.Actor != scope.Owner || c.Namespace != scope.Namespace || c.Operation != "memory.put" || c.Class != PrivatePayload {
		return fmt.Errorf("invalid memory command boundary")
	}
	r, err := DecodeMemoryPayload(c.Event, c.Supersedes, c.Payload)
	if err != nil {
		return err
	}
	idx, err := RebuildMemory(scope, entries)
	if err != nil {
		return err
	}
	if old, ok := idx.byID[r.Event.Meta.ID]; ok {
		// The generic journal will compare the complete scoped command digest.
		if old.Revoked {
			return fmt.Errorf("memory was revoked")
		}
		return nil
	}
	if len(entries) >= MaxMemoryRecords {
		return fmt.Errorf("memory scope budget exhausted")
	}
	sources := memorySources(r)
	if r.Supersedes != "" {
		sources = append(sources, r.Supersedes)
	}
	for _, id := range sources {
		p, ok := idx.byID[id]
		if !ok || p.Revoked || p.Content == nil {
			return fmt.Errorf("unknown/revoked memory source")
		}
		// No derived audience learns evidence before that audience learned its source.
		for _, l := range r.Content.Learned {
			at, known := learnedAt(*p.Content, l.Actor)
			if !known || l.At < at {
				return fmt.Errorf("derivative precedes audience source knowledge")
			}
		}
	}
	return nil
}
func learnedAt(c MemoryContent, actor core.ID) (core.LogicalTime, bool) {
	for _, l := range c.Learned {
		if l.Actor == actor {
			return l.At, true
		}
	}
	return 0, false
}
