package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator"
	"github.com/tushardhara/dream/simulator/scenario"
)

var _ hws.SnapshotStore = (*Store)(nil)

func snapshotIdentity(sc hws.Scope, key core.ID) core.ID {
	hash, _ := hws.ModelDigest(struct {
		Scope hws.Scope
		Key   core.ID
	}{sc, key})
	return core.ID("snapshot:" + hash)
}
func addDerivedSource(ctx context.Context, tx pgx.Tx, actor, namespace, event, sourceActor, sourceNamespace, source core.ID) error {
	// Strict journal order prevents cross-scope cycles and makes history immutable.
	var valid bool
	err := tx.QueryRow(ctx, `SELECT c.seq>p.seq AND NOT EXISTS(SELECT 1 FROM dream.tombstones t WHERE(t.actor,t.namespace,t.event_id)=(p.actor,p.namespace,p.id)) FROM dream.events c,dream.events p WHERE(c.actor,c.namespace,c.id)=($1,$2,$3) AND(p.actor,p.namespace,p.id)=($4,$5,$6)`, actor, namespace, event, sourceActor, sourceNamespace, source).Scan(&valid)
	if err != nil || !valid {
		return ErrRevoked
	}
	_, err = tx.Exec(ctx, `INSERT INTO dream.derived_sources VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, actor, namespace, event, sourceActor, sourceNamespace, source)
	return err
}
func readSnapshot(ctx context.Context, tx pgx.Tx, key hws.SnapshotKey) (hws.FrozenState, core.ID, error) {
	var frozen hws.FrozenState
	var id core.ID
	var raw []byte
	var hash string
	sc := key.Scope
	if sc.Validate() != nil || key.ID.Validate() != nil {
		return frozen, id, hws.ErrCommand
	}
	err := tx.QueryRow(ctx, `SELECT h.event_id,h.hash,p.payload FROM dream.snapshot_heads h JOIN dream.snapshot_payloads p USING(actor,namespace,event_id) JOIN dream.artifact_refs a ON(a.actor,a.namespace,a.id)=(h.actor,h.namespace,h.event_id) WHERE h.actor=$1 AND h.namespace=$2 AND h.world=$3 AND h.branch=$4 AND h.run=$5 AND h.id=$6 AND NOT a.invalidated AND NOT EXISTS(SELECT 1 FROM dream.tombstones t WHERE(t.actor,t.namespace,t.event_id)=(h.actor,h.namespace,h.event_id))`, sc.Actor, sc.Namespace, sc.World, sc.Branch, sc.Run, key.ID).Scan(&id, &hash, &raw)
	if err != nil {
		return frozen, id, err
	}
	if key.Hash != "" && key.Hash != hash {
		return frozen, id, hws.ErrCommand
	}
	frozen, err = hws.DecodeSnapshot(raw, hash)
	if err != nil || frozen.Scope != sc {
		return hws.FrozenState{}, id, hws.ErrModel
	}
	return frozen, id, nil
}
func (s *Store) ReadSnapshot(ctx context.Context, key hws.SnapshotKey) (hws.FrozenState, error) {
	if len(key.Hash) != 64 {
		return hws.FrozenState{}, hws.ErrCommand
	}
	tx, e := s.begin(ctx)
	if e != nil {
		return hws.FrozenState{}, e
	}
	defer tx.Rollback(ctx)
	f, _, e := readSnapshot(ctx, tx, key)
	return f, e
}
func (s *Store) CaptureSnapshot(ctx context.Context, sc hws.Scope, key core.ID, revision int64) (hws.SnapshotKey, error) {
	if sc.Validate() != nil || key.Validate() != nil || revision < 1 {
		return hws.SnapshotKey{}, hws.ErrCommand
	}
	tx, e := s.begin(ctx)
	if e != nil {
		return hws.SnapshotKey{}, e
	}
	defer tx.Rollback(ctx)
	frozen, _, e := readSnapshot(ctx, tx, hws.SnapshotKey{Scope: sc, ID: key})
	if e == nil {
		if frozen.Revision != revision {
			return hws.SnapshotKey{}, hws.ErrCommand
		}
		return hws.SnapshotKey{Scope: sc, ID: key, Hash: frozen.Hash}, nil
	}
	// A purged/tampered existing key must not be recreated.
	var exists bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dream.snapshot_heads WHERE actor=$1 AND namespace=$2 AND run=$3 AND id=$4)`, sc.Actor, sc.Namespace, sc.Run, key).Scan(&exists); e != nil {
		return hws.SnapshotKey{}, e
	}
	if exists {
		return hws.SnapshotKey{}, ErrRevoked
	}
	current, parent, _, e := loadRuntime(ctx, tx, sc)
	if e != nil {
		return hws.SnapshotKey{}, e
	}
	if current.Revision != revision {
		return hws.SnapshotKey{}, hws.ErrConflict
	}
	var offset int64
	if e = tx.QueryRow(ctx, `SELECT coalesce(max(seq),0) FROM dream.events`).Scan(&offset); e != nil {
		return hws.SnapshotKey{}, e
	}
	frozen = hws.FrozenState{Version: hws.SnapshotVersion, Scope: sc, Revision: revision, Event: parent, Offset: offset, Engine: current.State.Engine, RNG: current.State.RNG, State: current.State, Memories: []hws.FrozenMemory{}, Models: []hws.ModelUse{}}
	var limits []byte
	var budget hws.ModelBudgetSnapshot
	e = tx.QueryRow(ctx, `SELECT limits,tokens,spend FROM dream.model_budgets WHERE actor=$1 AND namespace=$2 AND run=$3`, sc.Actor, sc.Namespace, sc.Run).Scan(&limits, &budget.Tokens, &budget.Spend)
	if e == nil {
		if json.Unmarshal(limits, &budget.Limits) != nil {
			return hws.SnapshotKey{}, hws.ErrModelBudget
		}
		frozen.ModelBudget = &budget
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return hws.SnapshotKey{}, e
	}
	var ancestor hws.SnapshotKey
	e = tx.QueryRow(ctx, `SELECT h.actor,h.namespace,h.world,h.branch,h.run,h.id,h.hash FROM dream.branch_origins b JOIN dream.snapshot_heads h ON(h.actor,h.namespace,h.event_id)=(b.actor,b.namespace,b.snapshot_event) WHERE b.actor=$1 AND b.namespace=$2 AND b.run=$3`, sc.Actor, sc.Namespace, sc.Run).Scan(&ancestor.Scope.Actor, &ancestor.Scope.Namespace, &ancestor.Scope.World, &ancestor.Scope.Branch, &ancestor.Scope.Run, &ancestor.ID, &ancestor.Hash)
	if e == nil {
		frozen.Ancestor = &ancestor
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return hws.SnapshotKey{}, e
	}
	var scenario scenario.Scenario
	if e = json.Unmarshal(current.State.Genesis.Payload, &scenario); e != nil {
		return hws.SnapshotKey{}, e
	}
	for _, actor := range scenario.Public.Humans {
		scope, _ := (hws.ViewRealm{Scope: sc, Principal: actor.ID}).MemoryScope()
		entries, err := readMemory(ctx, tx, scope)
		if err != nil {
			return hws.SnapshotKey{}, err
		}
		frozen.Memories = append(frozen.Memories, hws.FrozenMemory{Scope: scope, Entries: entries})
	}
	rows, e := tx.Query(ctx, `SELECT r.key,r.principal,r.memory_namespace,r.event_id,r.status FROM dream.model_applications a JOIN dream.model_requests r USING(actor,namespace,run,key) JOIN dream.events e ON(e.actor,e.namespace,e.id)=(a.actor,a.namespace,a.event_id) WHERE a.actor=$1 AND a.namespace=$2 AND a.run=$3 AND e.seq<=$4 ORDER BY r.key LIMIT 129`, sc.Actor, sc.Namespace, sc.Run, offset)
	if e != nil {
		return hws.SnapshotKey{}, e
	}
	type modelSource struct {
		key, principal, namespace, event core.ID
		status                           string
	}
	sources := []modelSource{}
	for rows.Next() {
		var source modelSource
		if e = rows.Scan(&source.key, &source.principal, &source.namespace, &source.event, &source.status); e != nil {
			rows.Close()
			return hws.SnapshotKey{}, e
		}
		sources = append(sources, source)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return hws.SnapshotKey{}, e
	}
	if len(sources) > 128 {
		return hws.SnapshotKey{}, fmt.Errorf("snapshot model budget")
	}
	for _, source := range sources {
		var use hws.ModelUse
		if source.status == "complete" {
			payload, err := readModelPayload(ctx, tx, source.principal, source.namespace, source.event)
			if err != nil || payload.Artifact == nil || payload.Artifact.Validate(payload.Intent.Input) != nil {
				return hws.SnapshotKey{}, ErrRevoked
			}
			use = hws.ModelUse{Key: source.key, Hash: payload.Artifact.Hash}
		} else {
			_, use, e = failedModel(ctx, tx, sc, source.key)
			if e != nil {
				return hws.SnapshotKey{}, e
			}
		}
		frozen.Models = append(frozen.Models, use)
	}
	frozen, e = hws.SealSnapshot(frozen)
	if e != nil {
		return hws.SnapshotKey{}, e
	}
	streamScope := sc
	streamScope.Run = simulator.RunID(snapshotIdentity(sc, key))
	command := runtimeEvent(streamScope, 1, current.State.At, parent, frozen.Hash)
	command.Event.Type = "snapshot.v1"
	command.Operation = "snapshot.capture"
	result, e := s.append(ctx, tx, command)
	if e != nil {
		return hws.SnapshotKey{}, e
	}
	event := result.EventID
	raw, e := json.Marshal(frozen)
	if e != nil {
		return hws.SnapshotKey{}, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO dream.snapshot_heads VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, sc.Actor, sc.Namespace, sc.World, sc.Branch, sc.Run, key, event, frozen.Hash, revision); e != nil {
		return hws.SnapshotKey{}, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO dream.snapshot_payloads VALUES($1,$2,$3,$4)`, sc.Actor, sc.Namespace, event, raw); e != nil {
		return hws.SnapshotKey{}, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO dream.artifact_refs(actor,namespace,id,kind) VALUES($1,$2,$3,'snapshot')`, sc.Actor, sc.Namespace, event); e != nil {
		return hws.SnapshotKey{}, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO dream.artifact_sources VALUES($1,$2,$3,$4)`, sc.Actor, sc.Namespace, event, parent); e != nil {
		return hws.SnapshotKey{}, e
	}
	for _, memory := range frozen.Memories {
		for _, entry := range memory.Entries {
			if entry.Revoked {
				continue
			}
			if e = addDerivedSource(ctx, tx, sc.Actor, sc.Namespace, event, memory.Scope.Owner, memory.Scope.Namespace, entry.Event.Meta.ID); e != nil {
				return hws.SnapshotKey{}, e
			}
		}
	}
	for _, source := range sources {
		if e = addDerivedSource(ctx, tx, sc.Actor, sc.Namespace, event, source.principal, source.namespace, source.event); e != nil {
			return hws.SnapshotKey{}, e
		}
	}
	return hws.SnapshotKey{Scope: sc, ID: key, Hash: frozen.Hash}, tx.Commit(ctx)
}

func (s *Store) ReadReplay(ctx context.Context, key hws.SnapshotKey, through int64) (hws.ReplayBundle, error) {
	if len(key.Hash) != 64 {
		return hws.ReplayBundle{}, hws.ErrCommand
	}
	tx, e := s.begin(ctx)
	if e != nil {
		return hws.ReplayBundle{}, e
	}
	defer tx.Rollback(ctx)
	f, _, e := readSnapshot(ctx, tx, key)
	if e != nil {
		return hws.ReplayBundle{}, e
	}
	if through < f.Revision || through-f.Revision > hws.MaxReplayFrames {
		return hws.ReplayBundle{}, hws.ErrCommand
	}
	bundle := hws.ReplayBundle{Snapshot: f, Frames: []hws.ReplayFrame{}}
	sc := key.Scope
	rows, e := tx.Query(ctx, `SELECT e.stream_version,p.payload,v.payload FROM dream.events e JOIN dream.runtime_payloads p ON(p.actor,p.namespace,p.event_id)=(e.actor,e.namespace,e.id) JOIN dream.research_payloads v ON(v.actor,v.namespace,v.event_id)=(e.actor,e.namespace,e.id) WHERE e.actor=$1 AND e.namespace=$2 AND e.stream=$3 AND e.stream_version>$4 AND e.stream_version<=$5 AND NOT EXISTS(SELECT 1 FROM dream.tombstones t WHERE(t.actor,t.namespace,t.event_id)=(e.actor,e.namespace,e.id)) ORDER BY e.stream_version`, sc.Actor, sc.Namespace, sc.Run, f.Revision, through)
	if e != nil {
		return bundle, e
	}
	defer rows.Close()
	for rows.Next() {
		var revision int64
		var raw, envelope []byte
		if e = rows.Scan(&revision, &raw, &envelope); e != nil {
			return bundle, e
		}
		var payload runtimePayload
		if json.Unmarshal(raw, &payload) != nil || payload.Command == nil {
			return bundle, fmt.Errorf("historical command not recorded; replay unavailable")
		}
		hash, e := DecodePayload(envelope)
		if e != nil {
			return bundle, e
		}
		bundle.Frames = append(bundle.Frames, hws.ReplayFrame{Revision: revision, Before: payload.Before, After: hash.Text, Command: *payload.Command, State: payload.State, Transition: payload.Transition, Model: payload.Model, OperationalStop: payload.OperationalStop})
	}
	if e = rows.Err(); e != nil {
		return bundle, e
	}
	rows.Close()
	if int64(len(bundle.Frames)) != through-f.Revision {
		return bundle, ErrRevoked
	}
	for _, frame := range bundle.Frames {
		var key core.ID
		e = tx.QueryRow(ctx, `SELECT key FROM dream.model_applications WHERE actor=$1 AND namespace=$2 AND run=$3 AND event_id=$4`, sc.Actor, sc.Namespace, sc.Run, runtimeID(string(sc.Run), frame.Revision)).Scan(&key)
		if errors.Is(e, pgx.ErrNoRows) {
			if frame.Model != nil {
				return bundle, hws.ErrModel
			}
			continue
		}
		if e != nil {
			return bundle, e
		}
		if frame.Model == nil || frame.Model.Key != key {
			return bundle, hws.ErrModel
		}
		if frame.Model.Failed {
			_, use, err := failedModel(ctx, tx, sc, key)
			if err != nil || use != *frame.Model {
				return bundle, hws.ErrModel
			}
		} else {
			var principal, namespace, event core.ID
			var status string
			if e = tx.QueryRow(ctx, `SELECT principal,memory_namespace,event_id,status FROM dream.model_requests WHERE actor=$1 AND namespace=$2 AND run=$3 AND key=$4`, sc.Actor, sc.Namespace, sc.Run, key).Scan(&principal, &namespace, &event, &status); e != nil || status != "complete" {
				return bundle, hws.ErrModel
			}
			payload, err := readModelPayload(ctx, tx, principal, namespace, event)
			if err != nil || payload.Artifact == nil || payload.Artifact.Validate(payload.Intent.Input) != nil || payload.Artifact.Hash != frame.Model.Hash {
				return bundle, hws.ErrModel
			}
		}
	}
	raw, e := json.Marshal(bundle)
	if e != nil || len(raw) > 32<<20 {
		return bundle, fmt.Errorf("replay bundle byte budget")
	}
	return bundle, nil
}

func (s *Store) ForkSnapshot(ctx context.Context, spec hws.ForkSpec) (hws.Snapshot, error) {
	if spec.Validate() != nil {
		return hws.Snapshot{}, hws.ErrCommand
	}
	tx, e := s.begin(ctx)
	if e != nil {
		return hws.Snapshot{}, e
	}
	defer tx.Rollback(ctx)
	frozen, sourceEvent, e := readSnapshot(ctx, tx, spec.Source)
	if e != nil {
		return hws.Snapshot{}, e
	}
	requestHash, e := hws.ModelDigest(spec)
	if e != nil {
		return hws.Snapshot{}, e
	}
	sc := spec.Child
	var prior string
	e = tx.QueryRow(ctx, `SELECT request_hash FROM dream.branch_origins WHERE actor=$1 AND namespace=$2 AND run=$3`, sc.Actor, sc.Namespace, sc.Run).Scan(&prior)
	if e == nil {
		if prior != requestHash {
			return hws.Snapshot{}, hws.ErrCommand
		}
		snapshot, _, _, err := loadRuntime(ctx, tx, sc)
		return snapshot, err
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return hws.Snapshot{}, e
	}
	state, e := hws.ForkState(frozen, spec)
	if e != nil {
		return hws.Snapshot{}, e
	}
	parent, _, manifest, e := loadRuntime(ctx, tx, spec.Source.Scope)
	if e != nil || manifest == nil {
		return hws.Snapshot{}, ErrRevoked
	}
	var now time.Time
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return hws.Snapshot{}, e
	}
	deadline := now.Add(spec.MaxDuration)
	if parent.Deadline.Before(deadline) {
		deadline = parent.Deadline
	}
	if !now.Before(deadline) {
		return hws.Snapshot{}, hws.ErrDeadline
	}
	childManifest := *manifest
	childManifest.Scope = sc
	childManifest.MaxDuration = spec.MaxDuration
	manifestHash, e := hws.ModelDigest(childManifest)
	if e != nil {
		return hws.Snapshot{}, e
	}
	hash, e := state.Hash()
	if e != nil {
		return hws.Snapshot{}, e
	}

	originCommand := runtimeEvent(sc, 1, state.At, sourceEvent, requestHash)
	originCommand.Event.Type = "branch.origin.v1"
	origin, e := s.append(ctx, tx, originCommand)
	if e != nil {
		return hws.Snapshot{}, e
	}
	type childMemorySource struct {
		scope graph.MemoryScope
		event core.ID
	}
	memorySources := []childMemorySource{}
	// Materialize only history learned by the fork cutoff, with immutable source
	// references. Parent/sibling writes and their later knowledge are never queried.
	for _, memory := range frozen.Memories {
		childScope, _ := (hws.ViewRealm{Scope: sc, Principal: memory.Scope.Owner}).MemoryScope()
		versions := map[core.ID]int64{}
		copied := map[core.ID]bool{}
		entries := append([]graph.MemoryEntry{}, memory.Entries...)
		sort.Slice(entries, func(i, j int) bool { return entries[i].Sequence < entries[j].Sequence })
		for _, entry := range entries {
			if entry.Revoked || entry.Content == nil || entry.Event.OccurredAt > state.At {
				continue
			}
			known := false
			for _, learned := range entry.Content.Learned {
				known = known || learned.Actor == childScope.Owner && learned.At <= state.At
			}
			if !known {
				continue
			}
			parents := append(append(append([]core.ID{}, entry.Event.Meta.Parents...), entry.Event.Meta.Supporting...), entry.Event.Meta.Contradicting...)
			if entry.Supersedes != "" {
				parents = append(parents, entry.Supersedes)
			}
			closed := true
			for _, id := range parents {
				closed = closed && copied[id]
			}
			if !closed {
				continue
			}
			grant := core.Grant{Actor: childScope.Owner, Recipient: childScope.Owner, Purpose: "simulation", Operation: core.Derive}
			if !entry.Event.Meta.Rights.Allows(core.PermissionRequest{Resource: entry.Event.Meta.ID, Context: grant}) {
				return hws.Snapshot{}, hws.ErrViewDenied
			}
			record := graph.MemoryRecord{Event: entry.Event, Content: *entry.Content, Supersedes: entry.Supersedes}
			payload, err := graph.EncodeMemoryPayload(record)
			if err != nil {
				return hws.Snapshot{}, err
			}
			cmd := graph.AppendCommand{Actor: childScope.Owner, Namespace: childScope.Namespace, Operation: "memory.fork", Key: entry.Event.Meta.ID, ExpectedVersion: versions[entry.Event.Stream], Event: entry.Event, Class: graph.PrivatePayload, Payload: payload, Supersedes: entry.Supersedes, DerivationContext: &grant}
			written, err := s.append(ctx, tx, cmd)
			if err != nil {
				return hws.Snapshot{}, err
			}
			memorySources = append(memorySources, childMemorySource{childScope, written.EventID})
			versions[entry.Event.Stream]++
			copied[entry.Event.Meta.ID] = true
			if e = addDerivedSource(ctx, tx, childScope.Owner, childScope.Namespace, written.EventID, memory.Scope.Owner, memory.Scope.Namespace, entry.Event.Meta.ID); e != nil {
				return hws.Snapshot{}, e
			}
			if e = addDerivedSource(ctx, tx, childScope.Owner, childScope.Namespace, written.EventID, sc.Actor, sc.Namespace, origin.EventID); e != nil {
				return hws.Snapshot{}, e
			}
		}
	}
	command := runtimeEvent(sc, 2, state.At, origin.EventID, hash)
	command.Event.Type = "branch.fork.v1"
	result, e := s.append(ctx, tx, command)
	if e != nil {
		return hws.Snapshot{}, e
	}
	event := result.EventID
	raw, e := json.Marshal(runtimePayload{Manifest: &childManifest, State: state, Fork: &spec})
	if e != nil {
		return hws.Snapshot{}, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO dream.runtime_payloads VALUES($1,$2,$3,$4)`, sc.Actor, sc.Namespace, event, raw); e != nil {
		return hws.Snapshot{}, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO dream.runtime_heads(actor,namespace,world,branch,run,event_id,revision,manifest_hash,deadline) VALUES($1,$2,$3,$4,$5,$6,2,$7,$8)`, sc.Actor, sc.Namespace, sc.World, sc.Branch, sc.Run, event, manifestHash, deadline); e != nil {
		return hws.Snapshot{}, e
	}
	if b := frozen.ModelBudget; b != nil {
		encoded, err := json.Marshal(b.Limits)
		if err != nil {
			return hws.Snapshot{}, err
		}
		if _, e = tx.Exec(ctx, `INSERT INTO dream.model_budgets(actor,namespace,run,limits,tokens,spend) VALUES($1,$2,$3,$4,$5,$6)`, sc.Actor, sc.Namespace, sc.Run, encoded, b.Tokens, b.Spend); e != nil {
			return hws.Snapshot{}, e
		}
	}
	coupling := "independent-branch-streams.v1"
	if spec.PairedExogenous {
		coupling = "common-exogenous-counter.v1"
	}
	if _, e = tx.Exec(ctx, `INSERT INTO dream.branch_origins VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, sc.Actor, sc.Namespace, sc.Run, origin.EventID, sourceEvent, requestHash, spec.Mode, coupling); e != nil {
		return hws.Snapshot{}, e
	}
	if e = associateRuntime(ctx, tx, sc, event, state.At); e != nil {
		return hws.Snapshot{}, e
	}

	for _, source := range memorySources {
		if e = addDerivedSource(ctx, tx, sc.Actor, sc.Namespace, event, source.scope.Owner, source.scope.Namespace, source.event); e != nil {
			return hws.Snapshot{}, e
		}
	}
	return hws.Snapshot{Revision: 2, State: state, Deadline: deadline, Experiment: &hws.ExperimentLabel{Mode: hws.FreshSimulation, Exact: false, Coupling: coupling, Source: spec.Source}}, tx.Commit(ctx)
}
