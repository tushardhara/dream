package graph

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/tushardhara/dream/core"
)

type MemoryQuery struct {
	Scope            MemoryScope
	Actor, Purpose   core.ID
	Subject          core.Subject
	ValidAt, KnownAt core.LogicalTime
	RecordedAsOf     time.Time
	Limit            int
}

func (q MemoryQuery) Validate() error {
	if q.Scope.Validate() != nil || q.Actor.Validate() != nil || q.Purpose.Validate() != nil || q.Subject.ValidateFor(q.Scope.Owner) != nil {
		return fmt.Errorf("invalid retrieval scope")
	}
	if q.ValidAt < 0 || q.KnownAt < 0 || q.RecordedAsOf.IsZero() || q.RecordedAsOf.Year() < 1 || q.RecordedAsOf.Year() > 9999 || q.Limit < 1 || q.Limit > MaxMemoryResults {
		return fmt.Errorf("invalid retrieval time/budget")
	}
	return nil
}

type SelectionRationale struct {
	Codes           []string `json:"codes"`
	DecayedSalience float64  `json:"decayed_salience"`
	Recency         float64  `json:"recency"`
	Score           float64  `json:"score"`
}
type MemorySelection struct {
	Record    MemoryRecord
	Rationale SelectionRationale
}

// MemoryCache is one bounded selection of IDs and numeric rationale, never text,
// evidence, or rights. Unexported fields prevent a caller from forging a hit. It
// is a value: separate requests need no shared mutable cache or cache mutex.
type MemoryCache struct {
	key   [32]byte
	picks []memoryPick
}
type memoryPick struct {
	ID        core.ID
	Rationale SelectionRationale
}

func (s MemoryService) Retrieve(ctx context.Context, q MemoryQuery) ([]MemorySelection, error) {
	out, _, _, err := s.RetrieveCached(ctx, q, MemoryCache{})
	return out, err
}
func (s MemoryService) RetrieveCached(ctx context.Context, q MemoryQuery, cache MemoryCache) ([]MemorySelection, MemoryCache, bool, error) {
	if err := q.Validate(); err != nil {
		return nil, MemoryCache{}, false, err
	}
	if s.Journal == nil {
		return nil, MemoryCache{}, false, fmt.Errorf("memory journal required")
	}
	entries, err := s.Journal.ReadMemory(ctx, q.Scope)
	if err != nil {
		return nil, MemoryCache{}, false, err
	}
	idx, err := RebuildMemory(q.Scope, entries)
	if err != nil {
		return nil, MemoryCache{}, false, err
	}
	// Includes recorded-at and current tombstones, unlike a replay logical hash.
	raw, err := json.Marshal(struct {
		Query   MemoryQuery
		Entries []MemoryEntry
	}{q, entries})
	if err != nil {
		return nil, MemoryCache{}, false, err
	}
	key := sha256.Sum256(raw)
	hit := cache.key == key
	if !hit {
		cache = MemoryCache{key: key, picks: idx.selectMemory(q)}
	}
	out := make([]MemorySelection, 0, len(cache.picks))
	for _, p := range cache.picks {
		e, ok := idx.byID[p.ID]
		if !ok || e.Revoked || e.Content == nil {
			return nil, MemoryCache{}, false, fmt.Errorf("invalid cached reference")
		}
		out = append(out, MemorySelection{e.record(), p.Rationale})
	}
	// Do not hand callers aliases into the retained cache rationale.
	cloned, err := json.Marshal(out)
	if err != nil {
		return nil, MemoryCache{}, false, err
	}
	var detached []MemorySelection
	if err = json.Unmarshal(cloned, &detached); err != nil {
		return nil, MemoryCache{}, false, err
	}
	return detached, cache, hit, nil
}

func (x *MemoryIndex) selectMemory(q MemoryQuery) []memoryPick {
	return x.selectMatchingMemory(q, nil)
}

func (x *MemoryIndex) selectMatchingMemory(q MemoryQuery, match func(MemoryRecord) bool) []memoryPick {
	superseded := map[core.ID]bool{}
	materialChanged := map[core.ID]bool{}
	for _, e := range x.entries {
		if e.Supersedes == "" {
			continue
		}
		// Material summaries never return an obsolete source, even via an old as-of
		// query. Historical source records themselves remain queryable by time.
		materialChanged[e.Supersedes] = true
		if e.Event.Meta.RecordedAt.After(q.RecordedAsOf) || e.Event.OccurredAt > q.KnownAt || e.Event.Meta.Valid.Start > q.ValidAt || (e.Event.Meta.Valid.End != nil && *e.Event.Meta.Valid.End <= q.ValidAt) {
			continue
		}
		if e.Content != nil {
			at, ok := learnedAt(*e.Content, q.Actor)
			if !ok || at > q.KnownAt {
				continue
			}
		}
		// A purged replacement fails closed: never resurrect the old proposition.
		superseded[e.Supersedes] = true
	}
	memo := map[core.ID]bool{}
	done := map[core.ID]bool{}
	var eligible func(core.ID) bool
	eligible = func(id core.ID) bool {
		if done[id] {
			return memo[id]
		}
		done[id] = true
		e, ok := x.byID[id]
		if !ok || e.Revoked || e.Content == nil || superseded[id] {
			return false
		}
		m := e.Event.Meta
		c := *e.Content
		at, known := learnedAt(c, q.Actor)
		if !known || at > q.KnownAt || e.Event.OccurredAt > q.KnownAt || m.RecordedAt.After(q.RecordedAsOf) || m.Valid.Start > q.ValidAt || (m.Valid.End != nil && *m.Valid.End <= q.ValidAt) || (c.Expires != nil && *c.Expires <= q.KnownAt) {
			return false
		}
		if !m.Rights.Allows(core.PermissionRequest{Resource: id, Context: core.Grant{Actor: q.Actor, Recipient: q.Actor, Purpose: q.Purpose, Operation: core.Read}}) {
			return false
		}
		for _, source := range memorySources(e.record()) {
			if !eligible(source) {
				return false
			}
		}
		if c.Kind == NarrativeMemory {
			seen := map[core.ID]bool{}
			var stale func(core.ID) bool
			stale = func(id core.ID) bool {
				if seen[id] {
					return false
				}
				seen[id] = true
				if materialChanged[id] {
					return true
				}
				p, ok := x.byID[id]
				if !ok || p.Content == nil {
					return true
				}
				for _, s := range memorySources(p.record()) {
					if stale(s) {
						return true
					}
				}
				return false
			}
			for _, id := range memorySources(e.record()) {
				if stale(id) {
					return false
				}
			}
		}
		memo[id] = true
		return true
	}
	picks := []memoryPick{}
	for _, e := range x.entries {
		if !sameSubject(e.Event.Subject, q.Subject) || !eligible(e.Event.Meta.ID) {
			continue
		}
		if match != nil && !match(e.record()) {
			continue
		}
		c := *e.Content
		at, _ := learnedAt(c, q.Actor)
		age := float64(q.KnownAt-at) / float64(c.HalfLife)
		salience := c.Salience * math.Exp2(-age)
		recency := 1 / (1 + age)
		// This deterministic ranking is a heuristic, not a truth/probability score.
		score := math.Round((.75*salience+.25*recency)*1e12) / 1e12
		picks = append(picks, memoryPick{e.Event.Meta.ID, SelectionRationale{[]string{"scope", "known", "valid", "authorized", "evidence", "salience_recency"}, salience, recency, score}})
	}
	sort.Slice(picks, func(i, j int) bool {
		if picks[i].Rationale.Score != picks[j].Rationale.Score {
			return picks[i].Rationale.Score > picks[j].Rationale.Score
		}
		return picks[i].ID < picks[j].ID
	})
	if len(picks) > q.Limit {
		picks = picks[:q.Limit]
	}
	return picks
}
