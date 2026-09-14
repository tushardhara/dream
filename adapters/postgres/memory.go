package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/tushardhara/dream/app/graph"
)

var _ graph.MemoryJournal = (*Store)(nil)

type memoryQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func readMemory(ctx context.Context, db memoryQuerier, scope graph.MemoryScope) ([]graph.MemoryEntry, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	// One SQL statement observes metadata, payload and current tombstones on the
	// same snapshot. Full scope, not a prefiltered subject, resolves evidence safely.
	rows, err := db.Query(ctx, `SELECT e.seq,e.envelope,coalesce(e.supersedes,''),
 EXISTS(SELECT 1 FROM dream.tombstones t WHERE (t.actor,t.namespace,t.event_id)=(e.actor,e.namespace,e.id)),
 p.payload FROM dream.events e LEFT JOIN dream.private_payloads p ON (p.actor,p.namespace,p.event_id)=(e.actor,e.namespace,e.id)
 WHERE e.actor=$1 AND e.namespace=$2 AND e.envelope->>'type'=$3 ORDER BY e.seq LIMIT $4`, scope.Owner, scope.Namespace, graph.MemoryEventType, graph.MaxMemoryRecords+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []graph.MemoryEntry{}
	for rows.Next() {
		var e graph.MemoryEntry
		var envelope, payload []byte
		if err = rows.Scan(&e.Sequence, &envelope, &e.Supersedes, &e.Revoked, &payload); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(envelope, &e.Event); err != nil {
			return nil, err
		}
		if !e.Revoked {
			p, err := DecodePayload(payload)
			if err != nil {
				return nil, err
			}
			r, err := graph.DecodeMemoryPayload(e.Event, e.Supersedes, p)
			if err != nil {
				return nil, err
			}
			e.Content = &r.Content
		}
		entries = append(entries, e)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(entries) > graph.MaxMemoryRecords {
		return nil, fmt.Errorf("memory scope budget exhausted")
	}
	return entries, nil
}
func (s *Store) ReadMemory(ctx context.Context, scope graph.MemoryScope) ([]graph.MemoryEntry, error) {
	return readMemory(ctx, s.db, scope)
}
func (s *Store) AppendMemory(ctx context.Context, c graph.AppendCommand) (graph.AppendResult, error) {
	if err := c.Validate(); err != nil {
		return graph.AppendResult{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return graph.AppendResult{}, err
	}
	defer tx.Rollback(ctx)
	scope := graph.MemoryScope{Owner: c.Actor, Namespace: c.Namespace}
	entries, err := readMemory(ctx, tx, scope)
	if err != nil {
		return graph.AppendResult{}, err
	}
	if err = graph.ValidateMemoryAppend(scope, entries, c); err != nil {
		return graph.AppendResult{}, err
	}
	result, err := s.append(ctx, tx, c)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
