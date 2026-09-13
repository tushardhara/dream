package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
)

// Project commits derived rows, delivery marks and the checkpoint together.
// Repeated delivery has idempotent effects; no universal exactly-once claim.
func (s *Store) Project(ctx context.Context, name core.ID, limit int) (int, error) {
	if err := name.Validate(); err != nil {
		return 0, err
	}
	if limit < 1 || limit > 10000 {
		return 0, fmt.Errorf("invalid projection batch limit")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO dream.projection_checkpoints VALUES($1,0) ON CONFLICT DO NOTHING`, name); err != nil {
		return 0, err
	}
	var checkpoint int64
	if err = tx.QueryRow(ctx, `SELECT seq FROM dream.projection_checkpoints WHERE name=$1`, name).Scan(&checkpoint); err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, `SELECT seq,actor,namespace,id FROM dream.events WHERE seq>$1 ORDER BY seq LIMIT $2`, checkpoint, limit)
	if err != nil {
		return 0, err
	}
	type item struct {
		seq                  int64
		actor, namespace, id string
	}
	var batch []item
	for rows.Next() {
		var e item
		if err = rows.Scan(&e.seq, &e.actor, &e.namespace, &e.id); err != nil {
			rows.Close()
			return 0, err
		}
		batch = append(batch, e)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	for _, e := range batch {
		if _, err = tx.Exec(ctx, `INSERT INTO dream.projections SELECT actor,namespace,id,seq,subject,valid_from,valid_to,recorded_at FROM dream.events e WHERE actor=$1 AND namespace=$2 AND id=$3 AND NOT EXISTS(SELECT 1 FROM dream.tombstones t WHERE(t.actor,t.namespace,t.event_id)=(e.actor,e.namespace,e.id)) ON CONFLICT DO NOTHING`, e.actor, e.namespace, e.id); err != nil {
			return 0, err
		}
		if _, err = tx.Exec(ctx, `UPDATE dream.outbox SET delivered=true WHERE actor=$1 AND namespace=$2 AND event_id=$3`, e.actor, e.namespace, e.id); err != nil {
			return 0, err
		}
		checkpoint = e.seq
	}
	if _, err = tx.Exec(ctx, `UPDATE dream.projection_checkpoints SET seq=$2 WHERE name=$1`, name, checkpoint); err != nil {
		return 0, err
	}
	return len(batch), tx.Commit(ctx)
}
func (s *Store) ResetProjections(ctx context.Context) error {
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM dream.projections; UPDATE dream.projection_checkpoints SET seq=0; UPDATE dream.outbox SET delivered=false`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Query returns the latest visible event for one observer-owned subject at both
// valid time and system-known time. Revocation denies even historical reads.
func (s *Store) Query(ctx context.Context, actor, namespace core.ID, subject core.Subject, validAt core.LogicalTime, knownAsOf time.Time) (core.ID, error) {
	if err := actor.Validate(); err != nil {
		return "", err
	}
	if err := namespace.Validate(); err != nil {
		return "", err
	}
	if err := subject.ValidateFor(actor); err != nil {
		return "", err
	}
	if err := validAt.Validate(); err != nil {
		return "", err
	}
	if knownAsOf.IsZero() {
		return "", fmt.Errorf("missing known-as-of")
	}
	var id core.ID
	err := s.db.QueryRow(ctx, `SELECT event_id FROM dream.projections p WHERE actor=$1 AND namespace=$2 AND subject=$3 AND valid_from<=$4 AND (valid_to IS NULL OR valid_to>$4) AND recorded_at<=$5 AND NOT EXISTS(SELECT 1 FROM dream.tombstones t WHERE(t.actor,t.namespace,t.event_id)=(p.actor,p.namespace,p.event_id)) ORDER BY seq DESC LIMIT 1`, actor, namespace, subjectKey(subject), validAt, knownAsOf).Scan(&id)
	return id, err
}
func subjectKey(v core.Subject) string { return subject(v) }
func (s *Store) ReadPayload(ctx context.Context, actor, namespace, id core.ID) (graph.Payload, error) {
	for _, v := range []core.ID{actor, namespace, id} {
		if err := v.Validate(); err != nil {
			return graph.Payload{}, err
		}
	}
	// Single statement keeps class selection and tombstone/payload read on one DB
	// snapshot. A completed revocation removes bytes from every class table.
	var raw []byte
	err := s.db.QueryRow(ctx, `SELECT payload FROM (SELECT actor,namespace,event_id,payload FROM dream.observable_payloads UNION ALL SELECT actor,namespace,event_id,payload FROM dream.private_payloads UNION ALL SELECT actor,namespace,event_id,payload FROM dream.research_payloads) p WHERE actor=$1 AND namespace=$2 AND event_id=$3 AND NOT EXISTS(SELECT 1 FROM dream.tombstones t WHERE(t.actor,t.namespace,t.event_id)=(p.actor,p.namespace,p.event_id))`, actor, namespace, id).Scan(&raw)
	if err != nil {
		return graph.Payload{}, err
	}
	return DecodePayload(raw)
}

// DecodePayload is a fail-closed version dispatcher. Schema v1 is the first
// persisted format; no fabricated historical migration is claimed. Future
// versions require an explicit reviewed pure upcaster and compatibility tests.
func DecodePayload(raw []byte) (graph.Payload, error) {
	var p graph.Payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, err
	}
	if err := p.Validate(); err != nil {
		return p, err
	}
	canonical, err := json.Marshal(p)
	if err != nil {
		return p, err
	}
	if string(canonical) != string(raw) {
		return p, fmt.Errorf("noncanonical/unknown payload fields")
	}
	return p, nil
}
