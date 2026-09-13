package postgres

import (
	"context"
	"encoding/json"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
	"time"
)

// ReadModelUsage is a trusted storage port. Network exposure must first require
// a current research permit for the exact scope and revalidate before returning.
func (s *Store) ReadModelUsage(ctx context.Context, sc hws.Scope) (hws.ModelUsage, error) {
	var u hws.ModelUsage
	if sc.Validate() != nil {
		return u, hws.ErrCommand
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return u, err
	}
	defer tx.Rollback(ctx)
	if _, _, _, err = loadRuntime(ctx, tx, sc); err != nil {
		return u, err
	}
	err = tx.QueryRow(ctx, `SELECT tokens,spend FROM dream.model_budgets WHERE actor=$1 AND namespace=$2 AND run=$3`, sc.Actor, sc.Namespace, sc.Run).Scan(&u.ReservedTokens, &u.ReservedSpendMicros)
	if err != nil {
		return u, err
	}
	err = tx.QueryRow(ctx, `SELECT coalesce(sum(input_tokens),0),coalesce(sum(output_tokens),0),count(input_tokens),count(*)-count(input_tokens) FROM dream.model_usage WHERE actor=$1 AND namespace=$2 AND run=$3`, sc.Actor, sc.Namespace, sc.Run).Scan(&u.KnownInputTokens, &u.KnownOutputTokens, &u.KnownAttempts, &u.SettledUnknownAttempts)
	if err != nil {
		return u, err
	}
	err = tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE status='running' AND expires>clock_timestamp()),count(*) FILTER(WHERE status='uncertain' OR (status='running' AND expires<=clock_timestamp())) FROM dream.model_requests WHERE actor=$1 AND namespace=$2 AND run=$3`, sc.Actor, sc.Namespace, sc.Run).Scan(&u.RunningAttempts, &u.UncertainRequests)
	return u, err
}

// ReconcileModels never guesses that an expired call was free or a human WAIT.
// It fences expired/cancelled attempts and records that uncertainty atomically.
// The caller retries explicit durable keys through BeginModel; this operation
// neither schedules providers nor resets budgets or runtime leases.
func (s *Store) ReconcileModels(ctx context.Context, sc hws.Scope) (int, error) {
	if sc.Validate() != nil {
		return 0, hws.ErrCommand
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	snap, _, _, err := loadRuntime(ctx, tx, sc)
	if err != nil {
		return 0, err
	}
	stopped := snap.State.Status == "cancelled" || snap.State.Status == "completed" || snap.State.Status == "budget"
	rows, err := tx.Query(ctx, `SELECT key,fence FROM dream.model_requests WHERE actor=$1 AND namespace=$2 AND run=$3 AND status='running' AND (expires<=clock_timestamp() OR $4 OR $5<=clock_timestamp()) ORDER BY key LIMIT 128`, sc.Actor, sc.Namespace, sc.Run, stopped, snap.Deadline)
	if err != nil {
		return 0, err
	}
	type abandoned struct {
		Key   core.ID
		Fence int64
	}
	var items []abandoned
	for rows.Next() {
		var item abandoned
		if err = rows.Scan(&item.Key, &item.Fence); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	for _, item := range items {
		hash, _ := hws.ModelDigest(struct {
			Scope   hws.Scope
			Attempt abandoned
		}{sc, item})
		id := core.ID("recovery:" + hash)
		raw, _ := json.Marshal(struct {
			Version int
			Scope   hws.Scope
			Key     core.ID
			Fence   int64
			Status  string
		}{1, sc, item.Key, item.Fence, "uncertain"})
		command := graph.AppendCommand{Actor: sc.Actor, Namespace: sc.Namespace, Operation: "model.recovery", Key: id, Class: graph.PrivatePayload, Payload: graph.Payload{Version: 1, Text: string(raw)}, Event: core.Event{Version: 1, Stream: id, Type: "model.recovery.v1", Subject: core.Subject{Principal: sc.Actor}, Meta: core.Metadata{ID: id, Observer: sc.Actor, Source: "operations.v1", Sensitivity: core.Restricted, Confidence: 1, Valid: core.Interval{}, RecordedAt: time.Unix(0, 0), Rights: core.Rights{Resource: id}}}}
		if _, err = s.append(ctx, tx, command); err != nil {
			return 0, err
		}
		if _, err = tx.Exec(ctx, `UPDATE dream.model_requests SET status='uncertain',fence=fence+1 WHERE actor=$1 AND namespace=$2 AND run=$3 AND key=$4 AND fence=$5 AND status='running'`, sc.Actor, sc.Namespace, sc.Run, item.Key, item.Fence); err != nil {
			return 0, err
		}
	}
	return len(items), tx.Commit(ctx)
}
