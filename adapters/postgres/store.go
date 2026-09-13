package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/migrations"
)

var (
	ErrVersion     = errors.New("stream version conflict")
	ErrIdempotency = errors.New("idempotency payload conflict")
	ErrRevoked     = errors.New("revoked provenance")
)

type Store struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Store { return &Store{db: db} }

var _ graph.EventWriter = (*Store)(nil)

// Migrate requires dedicated migration authority. Runtime writers cannot create
// schemas/roles. Supports empty, v1 (forward upgrade), or the current v3 ledger.
func Migrate(ctx context.Context, db *pgxpool.Pool) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(41000)"); err != nil {
		return err
	}
	var exists bool
	if err = tx.QueryRow(ctx, "SELECT to_regclass('dream.schema_versions') IS NOT NULL").Scan(&exists); err != nil {
		return err
	}
	if exists {
		var count, version int
		if err = tx.QueryRow(ctx, "SELECT count(*),coalesce(max(version),0) FROM dream.schema_versions").Scan(&count, &version); err != nil {
			return err
		}
		if !(count == version && version >= 1 && version <= 4) {
			return fmt.Errorf("unsupported database schema")
		}
		if version == 1 {
			if _, err = tx.Exec(ctx, migrations.Runtime); err != nil {
				return err
			}
		}
		if version < 3 {
			if _, err = tx.Exec(ctx, migrations.ReaderScopes); err != nil {
				return err
			}
		}
		if version < 4 {
			if _, err = tx.Exec(ctx, migrations.Models); err != nil {
				return err
			}
		}
	} else {
		if _, err = tx.Exec(ctx, migrations.Initial); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, migrations.Runtime); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, migrations.ReaderScopes); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, migrations.Models); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// All journal mutations/projectors share one transactional lock in this initial
// single-host implementation. This prevents sequence/commit-order gaps (a later
// commit cannot cause a projector to skip an earlier uncommitted sequence).
// No callback or provider call is accepted inside these transactions.
func (s *Store) begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(41001)"); err != nil {
		tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}
func (s *Store) Append(ctx context.Context, c graph.AppendCommand) (graph.AppendResult, error) {
	if c.Event.Type == "revoke" {
		return graph.AppendResult{}, fmt.Errorf("use Revoke for revocation events")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return graph.AppendResult{}, err
	}
	defer tx.Rollback(ctx)
	result, err := s.append(ctx, tx, c)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
func (s *Store) append(ctx context.Context, tx pgx.Tx, c graph.AppendCommand) (graph.AppendResult, error) {
	digest, err := c.Digest()
	if err != nil {
		return graph.AppendResult{}, err
	}
	var old []byte
	var result graph.AppendResult
	err = tx.QueryRow(ctx, `SELECT digest,event_id,seq,stream_version FROM dream.command_results WHERE actor=$1 AND namespace=$2 AND operation=$3 AND key=$4`, c.Actor, c.Namespace, c.Operation, c.Key).Scan(&old, &result.EventID, &result.Sequence, &result.StreamVersion)
	if err == nil {
		if !bytes.Equal(old, digest[:]) {
			return graph.AppendResult{}, ErrIdempotency
		}
		return result, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO dream.streams VALUES($1,$2,$3,0) ON CONFLICT DO NOTHING`, c.Actor, c.Namespace, c.Event.Stream)
	if err != nil {
		return result, err
	}
	var version int64
	if err = tx.QueryRow(ctx, `SELECT version FROM dream.streams WHERE actor=$1 AND namespace=$2 AND stream=$3`, c.Actor, c.Namespace, c.Event.Stream).Scan(&version); err != nil {
		return result, err
	}
	if version != c.ExpectedVersion {
		return result, ErrVersion
	}
	parents := append(append(append([]core.ID{}, c.Event.Meta.Parents...), c.Event.Meta.Supporting...), c.Event.Meta.Contradicting...)
	if c.Supersedes != "" {
		parents = append(parents, c.Supersedes)
	}
	for _, id := range parents {
		var envelope []byte
		var revoked bool
		var parentClass graph.PayloadClass
		err = tx.QueryRow(ctx, `SELECT e.envelope,e.payload_class,EXISTS(SELECT 1 FROM dream.tombstones t WHERE(t.actor,t.namespace,t.event_id)=(e.actor,e.namespace,e.id)) FROM dream.events e WHERE actor=$1 AND namespace=$2 AND id=$3`, c.Actor, c.Namespace, id).Scan(&envelope, &parentClass, &revoked)
		if err != nil {
			return result, fmt.Errorf("missing provenance: %w", err)
		}
		if revoked {
			return result, ErrRevoked
		}
		if parentClass != graph.ObservablePayload && parentClass != c.Class {
			return result, fmt.Errorf("source payload role boundary broadened")
		}
		var parent core.Event
		if err = json.Unmarshal(envelope, &parent); err != nil {
			return result, err
		}
		if parent.Meta.Sensitivity == core.Restricted && c.Event.Meta.Sensitivity != core.Restricted {
			return result, fmt.Errorf("source sensitivity broadened")
		}
		if c.Event.Type != "revoke" {
			if c.DerivationContext == nil || c.DerivationContext.Operation != core.Derive || c.DerivationContext.Actor != c.Actor || !parent.Meta.Rights.Allows(core.PermissionRequest{Resource: parent.Meta.ID, Context: *c.DerivationContext}) {
				return result, fmt.Errorf("source derivation not authorized")
			}
		}
		for _, g := range c.Event.Meta.Rights.Grants {
			if !parent.Meta.Rights.Allows(core.PermissionRequest{Resource: parent.Meta.ID, Context: g}) {
				return result, fmt.Errorf("source rights broadened")
			}
		}
		if id == c.Supersedes && subject(parent.Subject) != subject(c.Event.Subject) {
			return result, fmt.Errorf("correction subject mismatch")
		}
	}
	var recorded time.Time
	if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&recorded); err != nil {
		return result, err
	}
	c.Event.Meta.RecordedAt = recorded.UTC()
	envelope, err := core.CanonicalEvent(c.Event)
	if err != nil {
		return result, err
	}
	payload, err := json.Marshal(c.Payload)
	if err != nil {
		return result, err
	}
	var supersedes any
	if c.Supersedes != "" {
		supersedes = string(c.Supersedes)
	}
	err = tx.QueryRow(ctx, `INSERT INTO dream.events(actor,namespace,id,stream,stream_version,schema_version,subject,occurred_at,valid_from,valid_to,recorded_at,envelope,payload_class,supersedes) VALUES($1,$2,$3,$4,$5,1,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING seq`, c.Actor, c.Namespace, c.Event.Meta.ID, c.Event.Stream, version+1, subject(c.Event.Subject), c.Event.OccurredAt, c.Event.Meta.Valid.Start, c.Event.Meta.Valid.End, recorded, envelope, c.Class, supersedes).Scan(&result.Sequence)
	if err != nil {
		return result, err
	}
	table, err := payloadTable(c.Class)
	if err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO dream.`+table+` VALUES($1,$2,$3,$4)`, c.Actor, c.Namespace, c.Event.Meta.ID, payload); err != nil {
		return result, err
	}
	for _, id := range parents {
		if _, err = tx.Exec(ctx, `INSERT INTO dream.lineage VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, c.Actor, c.Namespace, c.Event.Meta.ID, id); err != nil {
			return result, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE dream.streams SET version=$4 WHERE actor=$1 AND namespace=$2 AND stream=$3`, c.Actor, c.Namespace, c.Event.Stream, version+1); err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO dream.outbox(actor,namespace,event_id) VALUES($1,$2,$3)`, c.Actor, c.Namespace, c.Event.Meta.ID); err != nil {
		return result, err
	}
	result.EventID = c.Event.Meta.ID
	result.StreamVersion = version + 1
	if _, err = tx.Exec(ctx, `INSERT INTO dream.command_results VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, c.Actor, c.Namespace, c.Operation, c.Key, digest[:], result.EventID, result.Sequence, result.StreamVersion); err != nil {
		return result, err
	}
	return result, nil
}
func subject(s core.Subject) string {
	if s.Reference != nil {
		return "reference/" + string(s.Reference.Observer) + "/" + string(s.Reference.LocalID)
	}
	return "principal/" + string(s.Principal)
}
func payloadTable(c graph.PayloadClass) (string, error) {
	switch c {
	case graph.ObservablePayload:
		return "observable_payloads", nil
	case graph.PrivatePayload:
		return "private_payloads", nil
	case graph.ResearchPayload:
		return "research_payloads", nil
	}
	return "", fmt.Errorf("unknown payload class")
}
