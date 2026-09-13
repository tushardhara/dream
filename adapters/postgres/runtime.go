package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"time"
)

var _ hws.RuntimeStore = (*Store)(nil)

type runtimePayload struct {
	Manifest   *hws.Manifest  `json:"manifest,omitempty"`
	State      rt.State       `json:"state"`
	Transition *rt.Transition `json:"transition,omitempty"`
}

func runtimeID(run string, revision int64) core.ID {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s/%d", run, revision)))
	return core.ID("rt:" + hex.EncodeToString(sum[:]))
}
func runtimeEvent(scope hws.Scope, revision int64, at core.LogicalTime, parent core.ID, text string) graph.AppendCommand {
	id := runtimeID(string(scope.Run), revision)
	grant := core.Grant{Actor: scope.Actor, Recipient: scope.Actor, Purpose: scope.Namespace, Operation: core.Derive}
	c := graph.AppendCommand{Actor: scope.Actor, Namespace: scope.Namespace, Operation: "runtime.commit", Key: id, ExpectedVersion: revision - 1, Class: graph.ResearchPayload, Payload: graph.Payload{Version: 1, Text: text}, DerivationContext: &grant,
		Event: core.Event{Version: 1, Stream: core.ID(scope.Run), Type: "runtime.commit", Subject: core.Subject{Principal: scope.Actor}, OccurredAt: at, Meta: core.Metadata{ID: id, Observer: scope.Actor, Source: scope.Actor, Sensitivity: core.Restricted, Confidence: 1, Valid: core.Interval{Start: at}, RecordedAt: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), Rights: core.Rights{Resource: id, Grants: []core.Grant{grant}}}}}
	if parent != "" {
		c.Event.Meta.Parents = []core.ID{parent}
	} else {
		c.Event.Type = "scenario.genesis"
	}
	return c
}
func (s *Store) CreateRun(ctx context.Context, m hws.Manifest) (hws.Snapshot, error) {
	if err := m.Validate(); err != nil {
		return hws.Snapshot{}, err
	}
	state, err := rt.New(m.Genesis, m.Budget)
	if err != nil {
		return hws.Snapshot{}, err
	}
	b, err := json.Marshal(m)
	if err != nil {
		return hws.Snapshot{}, err
	}
	sum := sha256.Sum256(b)
	digest := hex.EncodeToString(sum[:])
	sc := m.Scope
	tx, err := s.begin(ctx)
	if err != nil {
		return hws.Snapshot{}, err
	}
	defer tx.Rollback(ctx)
	var existing string
	err = tx.QueryRow(ctx, `SELECT manifest_hash FROM dream.runtime_heads WHERE actor=$1 AND namespace=$2 AND world=$3 AND branch=$4`, sc.Actor, sc.Namespace, sc.World, sc.Branch).Scan(&existing)
	if err == nil {
		if existing != digest {
			return hws.Snapshot{}, hws.ErrCommand
		}
		snap, _, _, err := loadRuntime(ctx, tx, sc)
		return snap, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return hws.Snapshot{}, err
	}
	hash, err := state.Hash()
	if err != nil {
		return hws.Snapshot{}, err
	}
	result, err := s.append(ctx, tx, runtimeEvent(sc, 1, 0, "", hash))
	if err != nil {
		return hws.Snapshot{}, err
	}
	payload, err := json.Marshal(runtimePayload{Manifest: &m, State: state})
	if err != nil {
		return hws.Snapshot{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO dream.runtime_payloads VALUES($1,$2,$3,$4)`, sc.Actor, sc.Namespace, result.EventID, payload); err != nil {
		return hws.Snapshot{}, err
	}
	var deadline time.Time
	err = tx.QueryRow(ctx, `INSERT INTO dream.runtime_heads(actor,namespace,world,branch,run,event_id,revision,manifest_hash,deadline) VALUES($1,$2,$3,$4,$5,$6,1,$7,clock_timestamp()+$8::bigint*interval '1 microsecond') RETURNING deadline`, sc.Actor, sc.Namespace, sc.World, sc.Branch, sc.Run, result.EventID, digest, m.MaxDuration.Microseconds()).Scan(&deadline)
	if err != nil {
		return hws.Snapshot{}, err
	}
	if err = associateRuntime(ctx, tx, sc, result.EventID, 0); err != nil {
		return hws.Snapshot{}, err
	}
	return hws.Snapshot{Revision: 1, State: state, Deadline: deadline}, tx.Commit(ctx)
}
func associateRuntime(ctx context.Context, tx pgx.Tx, sc hws.Scope, id core.ID, at core.LogicalTime) error {
	_, err := tx.Exec(ctx, `INSERT INTO dream.simulator_associations VALUES($1,$2,$3,$4,$5,$6,$7)`, sc.Actor, sc.Namespace, id, sc.Run, sc.Branch, sc.Actor, at)
	return err
}
func loadRuntime(ctx context.Context, tx pgx.Tx, sc hws.Scope) (hws.Snapshot, core.ID, *hws.Manifest, error) {
	var snap hws.Snapshot
	var id core.ID
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT h.revision,h.event_id,h.deadline,p.payload FROM dream.runtime_heads h JOIN dream.runtime_payloads p ON(p.actor,p.namespace,p.event_id)=(h.actor,h.namespace,h.event_id) WHERE h.actor=$1 AND h.namespace=$2 AND h.world=$3 AND h.branch=$4 AND h.run=$5 AND NOT EXISTS(SELECT 1 FROM dream.tombstones t WHERE(t.actor,t.namespace,t.event_id)=(h.actor,h.namespace,h.event_id))`, sc.Actor, sc.Namespace, sc.World, sc.Branch, sc.Run).Scan(&snap.Revision, &id, &snap.Deadline, &raw)
	if err != nil {
		return snap, "", nil, err
	}
	var payload runtimePayload
	if err = json.Unmarshal(raw, &payload); err != nil {
		return snap, "", nil, err
	}
	snap.State = payload.State
	return snap, id, payload.Manifest, snap.State.Validate()
}
func (s *Store) LoadRun(ctx context.Context, sc hws.Scope) (hws.Snapshot, error) {
	if err := sc.Validate(); err != nil {
		return hws.Snapshot{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return hws.Snapshot{}, err
	}
	defer tx.Rollback(ctx)
	snap, _, _, err := loadRuntime(ctx, tx, sc)
	return snap, err
}
func validTTL(holder core.ID, ttl time.Duration) error {
	if holder.Validate() != nil || ttl < time.Millisecond || ttl > time.Minute {
		return fmt.Errorf("lease requires holder and 1ms..1min TTL")
	}
	return nil
}
func (s *Store) Acquire(ctx context.Context, sc hws.Scope, holder core.ID, ttl time.Duration) (hws.Lease, error) {
	return s.lease(ctx, sc, hws.Lease{Holder: holder}, ttl, false)
}
func (s *Store) Renew(ctx context.Context, sc hws.Scope, lease hws.Lease, ttl time.Duration) (hws.Lease, error) {
	return s.lease(ctx, sc, lease, ttl, true)
}
func (s *Store) lease(ctx context.Context, sc hws.Scope, l hws.Lease, ttl time.Duration, renew bool) (hws.Lease, error) {
	if err := sc.Validate(); err != nil {
		return hws.Lease{}, err
	}
	if err := validTTL(l.Holder, ttl); err != nil {
		return hws.Lease{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return hws.Lease{}, err
	}
	defer tx.Rollback(ctx)
	// Lock the branch row BEFORE checking wall time so a wait cannot grant an
	// already-expired claim. No journal/global lock is held by this operation.
	var fence int64
	var holder *string
	var expires *time.Time
	err = tx.QueryRow(ctx, `SELECT fence,holder,expires FROM dream.runtime_heads WHERE actor=$1 AND namespace=$2 AND world=$3 AND branch=$4 AND run=$5 FOR UPDATE`, sc.Actor, sc.Namespace, sc.World, sc.Branch, sc.Run).Scan(&fence, &holder, &expires)
	if err != nil {
		return hws.Lease{}, err
	}
	current, _, _, err := loadRuntime(ctx, tx, sc)
	if err != nil {
		return hws.Lease{}, err
	}
	if current.State.Status == "cancelled" || current.State.Status == "completed" || current.State.Status == "budget" {
		return hws.Lease{}, fmt.Errorf("terminal run cannot claim lease")
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return hws.Lease{}, err
	}
	if renew {
		if holder == nil || *holder != string(l.Holder) || fence != l.Fence || expires == nil || !expires.After(now) {
			return hws.Lease{}, hws.ErrLease
		}
	} else {
		if expires != nil && expires.After(now) {
			return hws.Lease{}, hws.ErrLease
		}
		fence++
	}
	var next time.Time
	err = tx.QueryRow(ctx, `UPDATE dream.runtime_heads SET holder=$6,fence=$7,expires=clock_timestamp()+$8::bigint*interval '1 microsecond' WHERE actor=$1 AND namespace=$2 AND world=$3 AND branch=$4 AND run=$5 RETURNING expires`, sc.Actor, sc.Namespace, sc.World, sc.Branch, sc.Run, l.Holder, fence, ttl.Microseconds()).Scan(&next)
	if err != nil {
		return hws.Lease{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO dream.runtime_lease_audit VALUES($1,$2,$3,$4,$5,clock_timestamp(),$6,1)`, sc.Actor, sc.Namespace, sc.Run, fence, l.Holder, next); err != nil {
		return hws.Lease{}, err
	}
	return hws.Lease{Holder: l.Holder, Fence: fence, Expires: next}, tx.Commit(ctx)
}
func readOperation(ctx context.Context, tx pgx.Tx, sc hws.Scope, key core.ID) (*hws.Operation, error) {
	var request, receipt []byte
	op := &hws.Operation{}
	err := tx.QueryRow(ctx, `SELECT o.request,o.digest,o.receipt FROM dream.runtime_operations o JOIN dream.runtime_heads h ON(h.actor,h.namespace,h.run)=(o.actor,o.namespace,o.run) WHERE o.actor=$1 AND o.namespace=$2 AND o.run=$3 AND o.key=$4 AND h.world=$5 AND h.branch=$6 AND NOT EXISTS(SELECT 1 FROM dream.tombstones t WHERE(t.actor,t.namespace,t.event_id)=(h.actor,h.namespace,h.event_id))`, sc.Actor, sc.Namespace, sc.Run, key, sc.World, sc.Branch).Scan(&request, &op.Digest, &receipt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(request, &op.Command); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(receipt, &op.Receipt); err != nil {
		return nil, err
	}
	return op, nil
}
func (s *Store) Operation(ctx context.Context, sc hws.Scope, key core.ID) (*hws.Operation, error) {
	if err := sc.Validate(); err != nil {
		return nil, err
	}
	if err := key.Validate(); err != nil {
		return nil, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	return readOperation(ctx, tx, sc, key)
}
func (s *Store) CommitRun(ctx context.Context, c hws.Commit) (hws.Receipt, error) {
	if err := c.Scope.Validate(); err != nil {
		return hws.Receipt{}, err
	}
	if err := c.Key.Validate(); err != nil {
		return hws.Receipt{}, err
	}
	digest, err := hws.CommandDigest(c.Command)
	if err != nil {
		return hws.Receipt{}, err
	}
	if err = c.State.Validate(); err != nil {
		return hws.Receipt{}, err
	}
	sc := c.Scope
	// Preserve #4's global journal commit order. This covers only the short write
	// phase; handlers and cross-world computation never hold this lock.
	tx, err := s.begin(ctx)
	if err != nil {
		return hws.Receipt{}, err
	}
	defer tx.Rollback(ctx)
	var holder *string
	var fence int64
	var expires *time.Time
	err = tx.QueryRow(ctx, `SELECT holder,fence,expires FROM dream.runtime_heads WHERE actor=$1 AND namespace=$2 AND world=$3 AND branch=$4 AND run=$5 FOR UPDATE`, sc.Actor, sc.Namespace, sc.World, sc.Branch, sc.Run).Scan(&holder, &fence, &expires)
	if err != nil {
		return hws.Receipt{}, err
	}
	op, err := readOperation(ctx, tx, sc, c.Key)
	if err != nil {
		return hws.Receipt{}, err
	}
	if op != nil {
		if op.Digest != digest {
			return hws.Receipt{}, hws.ErrCommand
		}
		if op.Receipt.Done {
			return op.Receipt, nil
		}
	}
	snap, parent, manifest, err := loadRuntime(ctx, tx, sc)
	if err != nil {
		return hws.Receipt{}, err
	}
	oldGenesis, _ := json.Marshal(snap.State.Genesis)
	newGenesis, _ := json.Marshal(c.State.Genesis)
	if !bytes.Equal(oldGenesis, newGenesis) || c.State.Budget != snap.State.Budget || c.State.Engine != snap.State.Engine || c.State.RNG != snap.State.RNG || c.State.At < snap.State.At || c.State.Step < snap.State.Step || c.State.Step > snap.State.Step+1 || c.State.Events < snap.State.Events || c.State.Events > snap.State.Events+1 {
		return hws.Receipt{}, fmt.Errorf("invalid immutable or monotonic transition")
	}
	if snap.Revision != c.Expected {
		return hws.Receipt{}, hws.ErrConflict
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return hws.Receipt{}, err
	}
	if holder == nil || *holder != string(c.Lease.Holder) || fence != c.Lease.Fence || expires == nil || !expires.After(now) {
		return hws.Receipt{}, hws.ErrLease
	}
	var active core.ID
	err = tx.QueryRow(ctx, `SELECT key FROM dream.runtime_operations WHERE actor=$1 AND namespace=$2 AND run=$3 AND NOT done`, sc.Actor, sc.Namespace, sc.Run).Scan(&active)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return hws.Receipt{}, err
	}
	if active != "" && active != c.Key && c.Command.Kind != "pause" && c.Command.Kind != "cancel" {
		return hws.Receipt{}, hws.ErrConflict
	}
	if c.CheckDeadlineOnly && now.Before(snap.Deadline) {
		return hws.Receipt{}, hws.ErrConflict
	}
	if !now.Before(snap.Deadline) {
		c.State, err = snap.State.Clone()
		if err != nil {
			return hws.Receipt{}, err
		}
		c.State.Status = "budget"
		c.Transition = nil
		c.Done = true
	}
	hash, err := c.State.Hash()
	if err != nil {
		return hws.Receipt{}, err
	}
	revision := snap.Revision + 1
	result, err := s.append(ctx, tx, runtimeEvent(sc, revision, c.State.At, parent, hash))
	if err != nil {
		return hws.Receipt{}, err
	}
	payload, err := json.Marshal(runtimePayload{Manifest: manifest, State: c.State, Transition: c.Transition})
	if err != nil {
		return hws.Receipt{}, err
	}
	if len(payload) > 4<<20 {
		return hws.Receipt{}, fmt.Errorf("runtime payload size budget")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO dream.runtime_payloads VALUES($1,$2,$3,$4)`, sc.Actor, sc.Namespace, result.EventID, payload); err != nil {
		return hws.Receipt{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE dream.runtime_heads SET event_id=$6,revision=$7 WHERE actor=$1 AND namespace=$2 AND world=$3 AND branch=$4 AND run=$5`, sc.Actor, sc.Namespace, sc.World, sc.Branch, sc.Run, result.EventID, revision); err != nil {
		return hws.Receipt{}, err
	}
	receipt := hws.Receipt{Revision: revision, Hash: hash, Status: c.State.Status, Done: c.Done}
	raw, err := json.Marshal(receipt)
	if err != nil {
		return hws.Receipt{}, err
	}
	request, err := json.Marshal(c.Command)
	if err != nil {
		return hws.Receipt{}, err
	}
	if active != "" && active != c.Key {
		interrupted := receipt
		interrupted.Status = "interrupted"
		interrupted.Done = true
		interruptedRaw, _ := json.Marshal(interrupted)
		if _, err = tx.Exec(ctx, `UPDATE dream.runtime_operations SET done=true,receipt=$4 WHERE actor=$1 AND namespace=$2 AND run=$3 AND NOT done`, sc.Actor, sc.Namespace, sc.Run, interruptedRaw); err != nil {
			return hws.Receipt{}, err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO dream.runtime_operations VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(actor,namespace,run,key) DO UPDATE SET receipt=EXCLUDED.receipt,done=EXCLUDED.done`, sc.Actor, sc.Namespace, sc.Run, c.Key, request, digest, raw, c.Done); err != nil {
		return hws.Receipt{}, err
	}
	if err = associateRuntime(ctx, tx, sc, result.EventID, c.State.At); err != nil {
		return hws.Receipt{}, err
	}
	var valid, withinDeadline bool
	if err = tx.QueryRow(ctx, `SELECT expires>clock_timestamp(),deadline>clock_timestamp() FROM dream.runtime_heads WHERE actor=$1 AND namespace=$2 AND world=$3 AND branch=$4 AND run=$5 AND holder=$6 AND fence=$7`, sc.Actor, sc.Namespace, sc.World, sc.Branch, sc.Run, c.Lease.Holder, c.Lease.Fence).Scan(&valid, &withinDeadline); err != nil {
		return hws.Receipt{}, err
	}
	if !valid {
		return hws.Receipt{}, hws.ErrLease
	}
	if !withinDeadline && c.State.Status != "budget" {
		return hws.Receipt{}, hws.ErrDeadline
	}
	return receipt, tx.Commit(ctx)
}
