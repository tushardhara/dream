package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
)

var _ hws.ModelStore = (*Store)(nil)

type modelPayload struct {
	Intent   hws.ModelIntent    `json:"intent"`
	Artifact *hws.ModelArtifact `json:"artifact,omitempty"`
}

func (s *Store) ConfigureModels(ctx context.Context, sc hws.Scope, limits hws.ModelLimits) error {
	if sc.Validate() != nil || limits.Validate() != nil {
		return hws.ErrModelBudget
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, _, _, err = loadRuntime(ctx, tx, sc); err != nil {
		return err
	}
	raw, _ := json.Marshal(limits)
	var existing []byte
	err = tx.QueryRow(ctx, `SELECT limits FROM dream.model_budgets WHERE actor=$1 AND namespace=$2 AND run=$3`, sc.Actor, sc.Namespace, sc.Run).Scan(&existing)
	if err == nil {
		if string(existing) != string(raw) {
			return hws.ErrModelBudget
		}
		return tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	// A versioned journal event precedes the durable budget configuration.
	idHash, _ := hws.ModelDigest(struct {
		Scope  hws.Scope
		Limits hws.ModelLimits
	}{sc, limits})
	id := core.ID("mb:" + idHash)
	c := graph.AppendCommand{Actor: sc.Actor, Namespace: sc.Namespace, Operation: "model.budget", Key: id, Class: graph.ResearchPayload, Payload: graph.Payload{Version: 1, Text: string(raw)}, Event: core.Event{Version: 1, Stream: id, Type: "model.budget.v1", Subject: core.Subject{Principal: sc.Actor}, Meta: core.Metadata{ID: id, Observer: sc.Actor, Source: sc.Actor, Sensitivity: core.Restricted, Confidence: 1, Valid: core.Interval{}, RecordedAt: time.Unix(0, 0), Rights: core.Rights{Resource: id}}}}
	if _, err = s.append(ctx, tx, c); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO dream.model_budgets(actor,namespace,run,limits) VALUES($1,$2,$3,$4)`, sc.Actor, sc.Namespace, sc.Run, raw); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func modelEvent(i hws.ModelIntent, version int64, parent core.ID, status string) graph.AppendCommand {
	streamHash, _ := hws.ModelDigest(struct {
		Scope          hws.Scope
		Principal, Key core.ID
	}{i.Scope, i.Principal, i.Key})
	stream := core.ID("model:" + streamHash)
	idHash, _ := hws.ModelDigest(struct {
		Stream  core.ID
		Version int64
	}{stream, version})
	id := core.ID("model:" + idHash)
	grant := core.Grant{Actor: i.Principal, Recipient: i.Principal, Purpose: "simulation", Operation: core.Derive}
	parents := []core.ID{parent}
	if parent == "" {
		parents = nil
		for _, item := range i.Input.Context {
			parents = append(parents, item.Source)
		}
	}
	return graph.AppendCommand{Actor: i.Principal, Namespace: i.MemoryScope.Namespace, Operation: "model.attempt", Key: id, ExpectedVersion: version - 1, Class: graph.PrivatePayload, Payload: graph.Payload{Version: 1, Text: status}, DerivationContext: &grant, Event: core.Event{Version: 1, Stream: stream, Type: "model.attempt.v1", Subject: core.Subject{Principal: i.Principal}, Meta: core.Metadata{ID: id, Observer: i.Principal, Source: i.Principal, Sensitivity: core.Restricted, Confidence: 1, Valid: core.Interval{}, RecordedAt: time.Unix(0, 0), Parents: parents, Rights: core.Rights{Resource: id, Grants: []core.Grant{grant}}}}}
}
func readModelPayload(ctx context.Context, tx pgx.Tx, actor, namespace, id core.ID) (modelPayload, error) {
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT p.payload FROM dream.model_payloads p WHERE actor=$1 AND namespace=$2 AND event_id=$3 AND NOT EXISTS(SELECT 1 FROM dream.tombstones t WHERE(t.actor,t.namespace,t.event_id)=(p.actor,p.namespace,p.event_id))`, actor, namespace, id).Scan(&raw)
	if err != nil {
		return modelPayload{}, ErrRevoked
	}
	var p modelPayload
	if json.Unmarshal(raw, &p) != nil || p.Intent.Validate() != nil {
		return modelPayload{}, hws.ErrModel
	}
	return p, nil
}
func (s *Store) BeginModel(ctx context.Context, i hws.ModelIntent, limits hws.ModelLimits) (hws.ModelAttempt, error) {
	if i.Validate() != nil || limits.Validate() != nil {
		return hws.ModelAttempt{}, hws.ErrModel
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return hws.ModelAttempt{}, err
	}
	defer tx.Rollback(ctx)
	sc := i.Scope
	snap, _, _, err := loadRuntime(ctx, tx, sc)
	if err != nil {
		return hws.ModelAttempt{}, err
	}
	var current time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&current); err != nil {
		return hws.ModelAttempt{}, err
	}
	if !current.Before(snap.Deadline) || snap.State.Status == "cancelled" || snap.State.Status == "completed" || snap.State.Status == "budget" {
		return hws.ModelAttempt{}, hws.ErrDeadline
	}
	if err = checkModelSources(ctx, tx, i, snap.State.At); err != nil {
		return hws.ModelAttempt{}, err
	}
	digest, _ := hws.ModelDigest(i)
	var existingDigest, status string
	var eventID core.ID
	var attempt int
	var fence, version int64
	var expires time.Time
	err = tx.QueryRow(ctx, `SELECT digest,status,event_id,attempt,fence,event_version,expires FROM dream.model_requests WHERE actor=$1 AND namespace=$2 AND run=$3 AND key=$4`, sc.Actor, sc.Namespace, sc.Run, i.Key).Scan(&existingDigest, &status, &eventID, &attempt, &fence, &version, &expires)
	if err == nil {
		if digest != existingDigest {
			return hws.ModelAttempt{}, hws.ErrCommand
		}
		payload, e := readModelPayload(ctx, tx, i.Principal, i.MemoryScope.Namespace, eventID)
		if e != nil {
			return hws.ModelAttempt{}, e
		}
		if status == "complete" {
			if payload.Artifact == nil || payload.Artifact.Validate(i.Input) != nil {
				return hws.ModelAttempt{}, hws.ErrModel
			}
			return hws.ModelAttempt{Intent: i, Number: attempt, Fence: fence, Complete: payload.Artifact}, tx.Commit(ctx)
		}
		if status == "running" && current.Before(expires) {
			return hws.ModelAttempt{}, hws.ErrModelBusy
		}
		if status == "refused" || status == "malformed" {
			return hws.ModelAttempt{}, hws.ErrModel
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return hws.ModelAttempt{}, err
	}
	if attempt >= limits.MaxAttempts {
		return hws.ModelAttempt{}, hws.ErrModelUncertain
	}
	var encoded []byte
	var tokens, spend int64
	if err = tx.QueryRow(ctx, `SELECT limits,tokens,spend FROM dream.model_budgets WHERE actor=$1 AND namespace=$2 AND run=$3`, sc.Actor, sc.Namespace, sc.Run).Scan(&encoded, &tokens, &spend); err != nil {
		return hws.ModelAttempt{}, err
	}
	expected, _ := json.Marshal(limits)
	if string(expected) != string(encoded) {
		return hws.ModelAttempt{}, hws.ErrModelBudget
	}
	var active int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM dream.model_requests WHERE actor=$1 AND namespace=$2 AND run=$3 AND status='running' AND expires>clock_timestamp()`, sc.Actor, sc.Namespace, sc.Run).Scan(&active); err != nil {
		return hws.ModelAttempt{}, err
	}
	if active >= limits.MaxInFlight {
		return hws.ModelAttempt{}, hws.ErrModelBusy
	}
	if tokens+limits.AttemptTokens > limits.Tokens || spend+limits.AttemptSpendMicros > limits.SpendMicros {
		return hws.ModelAttempt{}, hws.ErrModelBudget
	}
	result, err := s.append(ctx, tx, modelEvent(i, version+1, eventID, "running"))
	if err != nil {
		return hws.ModelAttempt{}, err
	}
	raw, _ := json.Marshal(modelPayload{Intent: i})
	if _, err = tx.Exec(ctx, `INSERT INTO dream.model_payloads VALUES($1,$2,$3,$4)`, i.Principal, i.MemoryScope.Namespace, result.EventID, raw); err != nil {
		return hws.ModelAttempt{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE dream.model_budgets SET tokens=tokens+$4,spend=spend+$5 WHERE actor=$1 AND namespace=$2 AND run=$3`, sc.Actor, sc.Namespace, sc.Run, limits.AttemptTokens, limits.AttemptSpendMicros); err != nil {
		return hws.ModelAttempt{}, err
	}
	attemptDeadline := current.Add(limits.Timeout)
	if snap.Deadline.Before(attemptDeadline) {
		attemptDeadline = snap.Deadline
	}
	_, err = tx.Exec(ctx, `INSERT INTO dream.model_requests VALUES($1,$2,$3,$4,$5,$6,$7,$8,'running',$9,$10,$11,$12) ON CONFLICT(actor,namespace,run,key) DO UPDATE SET event_id=excluded.event_id,status=excluded.status,attempt=excluded.attempt,fence=excluded.fence,expires=excluded.expires,event_version=excluded.event_version`, sc.Actor, sc.Namespace, sc.Run, i.Key, i.Principal, i.MemoryScope.Namespace, result.EventID, digest, attempt+1, fence+1, attemptDeadline, version+1)
	if err != nil {
		return hws.ModelAttempt{}, err
	}
	return hws.ModelAttempt{Intent: i, Number: attempt + 1, Fence: fence + 1, Deadline: attemptDeadline}, tx.Commit(ctx)
}
func (s *Store) FinishModel(ctx context.Context, a hws.ModelAttempt, artifact *hws.ModelArtifact, status hws.ProviderStatus) error {
	i := a.Intent
	if i.Validate() != nil || a.Number < 1 || a.Fence < 1 {
		return hws.ErrModel
	}
	if artifact != nil {
		if status != hws.ProviderOK || artifact.Validate(i.Input) != nil {
			return hws.ErrModel
		}
	} else if status != hws.ProviderRefused && status != hws.ProviderMalformed && status != hws.ProviderRateLimited && status != hws.ProviderUnavailable {
		return hws.ErrModel
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	sc := i.Scope
	snap, _, _, err := loadRuntime(ctx, tx, sc)
	if err != nil {
		return err
	}
	var eventID core.ID
	var fence, version int64
	var storedDigest, currentStatus string
	var active bool
	err = tx.QueryRow(ctx, `SELECT event_id,fence,event_version,digest,status,expires>clock_timestamp() FROM dream.model_requests WHERE actor=$1 AND namespace=$2 AND run=$3 AND key=$4`, sc.Actor, sc.Namespace, sc.Run, i.Key).Scan(&eventID, &fence, &version, &storedDigest, &currentStatus, &active)
	if err != nil {
		return err
	}
	digest, _ := hws.ModelDigest(i)
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	if artifact != nil && (!now.Before(snap.Deadline) || !active || snap.State.Status == "cancelled" || snap.State.Status == "completed" || snap.State.Status == "budget") {
		return hws.ErrModelUncertain
	}
	if artifact != nil {
		if err = checkModelSources(ctx, tx, i, snap.State.At); err != nil {
			return err
		}
	}
	if a.Fence != fence || digest != storedDigest || currentStatus != "running" {
		return hws.ErrConflict
	}
	if _, err = readModelPayload(ctx, tx, i.Principal, i.MemoryScope.Namespace, eventID); err != nil {
		return err
	}
	label := string(status)
	if artifact != nil {
		label = "complete"
	}
	result, err := s.append(ctx, tx, modelEvent(i, version+1, eventID, label))
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(modelPayload{Intent: i, Artifact: artifact})
	if len(raw) > 65536 {
		return fmt.Errorf("model artifact budget")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO dream.model_payloads VALUES($1,$2,$3,$4)`, i.Principal, i.MemoryScope.Namespace, result.EventID, raw); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE dream.model_requests SET event_id=$5,status=$6,event_version=$7 WHERE actor=$1 AND namespace=$2 AND run=$3 AND key=$4`, sc.Actor, sc.Namespace, sc.Run, i.Key, result.EventID, label, version+1); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// validateModelUse runs inside the canonical transition transaction. No callback
// or network operation occurs here; one stored artifact can apply only once.
func validateModelUse(ctx context.Context, tx pgx.Tx, sc hws.Scope, use hws.ModelUse) error {
	var principal, namespace, eventID core.ID
	var status string
	err := tx.QueryRow(ctx, `SELECT principal,memory_namespace,event_id,status FROM dream.model_requests WHERE actor=$1 AND namespace=$2 AND run=$3 AND key=$4`, sc.Actor, sc.Namespace, sc.Run, use.Key).Scan(&principal, &namespace, &eventID, &status)
	if err != nil || status != "complete" {
		return hws.ErrModel
	}
	p, err := readModelPayload(ctx, tx, principal, namespace, eventID)
	if err != nil {
		return err
	}
	if p.Artifact == nil || p.Artifact.Validate(p.Intent.Input) != nil || p.Artifact.Hash != use.Hash {
		return hws.ErrModel
	}
	snap, _, _, err := loadRuntime(ctx, tx, sc)
	if err != nil {
		return err
	}
	if err = checkModelSources(ctx, tx, p.Intent, snap.State.At); err != nil {
		return err
	}
	var applied bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dream.model_applications WHERE actor=$1 AND namespace=$2 AND run=$3 AND key=$4)`, sc.Actor, sc.Namespace, sc.Run, use.Key).Scan(&applied); err != nil {
		return err
	}
	if applied {
		return hws.ErrConflict
	}
	return nil
}

func checkModelSources(ctx context.Context, tx pgx.Tx, i hws.ModelIntent, at core.LogicalTime) error {
	if at != i.At {
		return hws.ErrModel
	}
	entries, err := readMemory(ctx, tx, i.MemoryScope)
	if err != nil {
		return err
	}
	digest, err := hws.ModelDigest(entries)
	if err != nil || digest != i.MemoryRevision {
		return hws.ErrModel
	}
	return nil
}
