package postgres

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
	"time"
)

type RevokedReference struct {
	Actor     core.ID `json:"actor"`
	Namespace core.ID `json:"namespace"`
	Event     core.ID `json:"event"`
}
type RevocationJournal struct {
	Version    int                `json:"version"`
	References []RevokedReference `json:"references"`
}

func (j RevocationJournal) Digest() (string, error) {
	if j.Version != 1 || len(j.References) > 100000 {
		return "", hws.ErrCommand
	}
	seen := map[RevokedReference]bool{}
	for _, r := range j.References {
		if r.Actor.Validate() != nil || r.Namespace.Validate() != nil || r.Event.Validate() != nil || seen[r] {
			return "", hws.ErrCommand
		}
		seen[r] = true
	}
	return hws.ModelDigest(j)
}

// ExportRevocations is an operator port, not a public unauthenticated endpoint.
// The journal contains retained identifiers only and needs separate secure
// retention/freshness attestation outside the backup it will be applied to.
func (s *Store) ExportRevocations(ctx context.Context) (RevocationJournal, error) {
	j := RevocationJournal{Version: 1, References: []RevokedReference{}}
	tx, err := s.begin(ctx)
	if err != nil {
		return j, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT actor,namespace,event_id FROM dream.tombstones UNION SELECT actor,namespace,event_id FROM dream.restored_revocations ORDER BY actor,namespace,event_id LIMIT 100001`)
	if err != nil {
		return j, err
	}
	defer rows.Close()
	for rows.Next() {
		var r RevokedReference
		if err = rows.Scan(&r.Actor, &r.Namespace, &r.Event); err != nil {
			return j, err
		}
		j.References = append(j.References, r)
	}
	if err = rows.Err(); err != nil {
		return j, err
	}
	_, err = j.Digest()
	return j, err
}

// QuarantineRestore is called while the restored database is isolated, before
// any service is started. expected must be independently retained, never inferred
// from the old backup. An empty journal is valid only if the operator attests it.
func (s *Store) QuarantineRestore(ctx context.Context, expected string) error {
	raw, e := hex.DecodeString(expected)
	if e != nil || len(raw) != 32 {
		return hws.ErrCommand
	}
	tx, e := s.beginRecovery(ctx, true)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	// Versioned event precedes the admission change. Repeating the same journal is
	// idempotent; it deliberately closes admission again on an explicit restore.
	hash, _ := hws.ModelDigest(struct{ Phase, Hash string }{"quarantine", expected})
	if _, e = s.append(ctx, tx, restoreEvent(core.ID("restore:"+hash), "quarantine")); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE dream.recovery_gate SET ready=false,expected_hash=$1 WHERE singleton`, expected); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func restoreEvent(id core.ID, phase string) graph.AppendCommand {
	return graph.AppendCommand{Actor: "recovery-service", Namespace: "recovery", Operation: "restore.audit", Key: id, Class: graph.PrivatePayload, Payload: graph.Payload{Version: 1, Text: phase}, Event: core.Event{Version: 1, Stream: id, Type: "restore.audit.v1", Subject: core.Subject{Principal: "recovery-service"}, Meta: core.Metadata{ID: id, Observer: "recovery-service", Source: "operations.v1", Sensitivity: core.Restricted, Confidence: 1, Valid: core.Interval{}, RecordedAt: time.Unix(0, 0), Rights: core.Rights{Resource: id}}}}
}
func (s *Store) ApplyRevocationJournal(ctx context.Context, j RevocationJournal) error {
	hash, e := j.Digest()
	if e != nil {
		return e
	}
	tx, e := s.beginRecovery(ctx, true)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var expected string
	var ready bool
	if e = tx.QueryRow(ctx, `SELECT ready,expected_hash FROM dream.recovery_gate WHERE singleton`).Scan(&ready, &expected); e != nil || ready || expected != hash {
		return hws.ErrViewDenied
	}
	idHash, _ := hws.ModelDigest(struct{ Phase, Hash string }{"apply", hash})
	if _, e = s.append(ctx, tx, restoreEvent(core.ID("restore:"+idHash), "revocations applied")); e != nil {
		return e
	}
	// Retain even references absent in the old backup; a later append cannot
	// recreate their IDs. Existing sources trigger the same cross-scope closure.
	encoded, _ := json.Marshal(j.References)
	if _, e = tx.Exec(ctx, `INSERT INTO dream.restored_revocations SELECT actor,namespace,event FROM jsonb_to_recordset($1::jsonb) AS x(actor text,namespace text,event text) ON CONFLICT DO NOTHING`, string(encoded)); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO dream.tombstones SELECT r.actor,r.namespace,r.event_id,clock_timestamp() FROM dream.restored_revocations r JOIN dream.events e ON(e.actor,e.namespace,e.id)=(r.actor,r.namespace,r.event_id) ON CONFLICT DO NOTHING`); e != nil {
		return e
	}
	if e = purgeRevoked(ctx, tx); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE dream.recovery_gate SET ready=true WHERE singleton`); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
