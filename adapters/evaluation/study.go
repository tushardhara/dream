package evaluation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tushardhara/dream/adapters/postgres"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/evals"
)

// StudyJournal persists protocol metadata/report hashes, never evaluator labels.
// It is a trusted offline evaluator host capability, not a public API endpoint.
type StudyJournal struct{ db *pgxpool.Pool }

func NewStudyJournal(db *pgxpool.Pool) (*StudyJournal, error) {
	if db == nil || db.Config().MaxConns < 2 {
		return nil, fmt.Errorf("study journal requires a bounded pool with at least two connections")
	}
	return &StudyJournal{db: db}, nil
}
func studyStream(scope evals.StudyScope) (core.ID, error) {
	if e := scope.Validate(); e != nil {
		return "", e
	}
	hash, e := evals.Digest(scope)
	return core.ID("study:" + hash), e
}
func studyID(stream core.ID, sequence int) core.ID {
	return core.ID(fmt.Sprintf("%s:%d", stream, sequence))
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func studyLoad(ctx context.Context, q queryer, scope evals.StudyScope) ([]evals.StudyEvent, error) {
	stream, e := studyStream(scope)
	if e != nil {
		return nil, e
	}
	rows, e := q.Query(ctx, `SELECT e.stream_version,e.id,p.payload,EXISTS(SELECT 1 FROM dream.tombstones t WHERE(t.actor,t.namespace,t.event_id)=(e.actor,e.namespace,e.id)) FROM dream.events e LEFT JOIN dream.research_payloads p ON(p.actor,p.namespace,p.event_id)=(e.actor,e.namespace,e.id) WHERE e.actor=$1 AND e.namespace=$2 AND e.stream=$3 ORDER BY e.stream_version LIMIT 62`, scope.Owner, scope.Namespace, stream)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	events := []evals.StudyEvent{}
	for rows.Next() {
		var seq int
		var id core.ID
		var raw []byte
		var revoked bool
		if e = rows.Scan(&seq, &id, &raw, &revoked); e != nil {
			return nil, e
		}
		var payload graph.Payload
		var event evals.StudyEvent
		if revoked || seq != len(events)+1 || id != studyID(stream, seq) || json.Unmarshal(raw, &payload) != nil || payload.Validate() != nil || json.Unmarshal([]byte(payload.Text), &event) != nil || event.Sequence != seq {
			return nil, fmt.Errorf("unavailable/corrupt study event")
		}
		events = append(events, event)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if len(events) > 0 {
		state, e := evals.RebuildStudy(events)
		if e != nil || state.Plan.Scope != scope {
			return nil, fmt.Errorf("invalid scoped study journal")
		}
	}
	return events, nil
}
func (j *StudyJournal) Load(ctx context.Context, scope evals.StudyScope) ([]evals.StudyEvent, error) {
	if e := postgres.New(j.db).RuntimeReady(ctx); e != nil {
		return nil, e
	}
	return studyLoad(ctx, j.db, scope)
}
func (j *StudyJournal) Append(ctx context.Context, scope evals.StudyScope, expected int, event evals.StudyEvent) error {
	if e := postgres.New(j.db).RuntimeReady(ctx); e != nil {
		return e
	}
	stream, e := studyStream(scope)
	if e != nil {
		return e
	}
	gate, e := j.db.Begin(ctx)
	if e != nil {
		return e
	}
	defer gate.Rollback(ctx)
	// This per-study gate serializes strict CAS before the existing journal's
	// idempotent Append. It is closed before returning, and before any provider call.
	if _, e = gate.Exec(ctx, `SELECT pg_advisory_xact_lock(17017,hashtext($1))`, string(stream)); e != nil {
		return e
	}
	events, e := studyLoad(ctx, gate, scope)
	if e != nil {
		return e
	}
	if len(events) != expected || event.Sequence != expected+1 {
		return fmt.Errorf("study reservation already committed or journal changed")
	}
	next := append(events, event)
	state, e := evals.RebuildStudy(next)
	if e != nil || state.Plan.Scope != scope {
		return fmt.Errorf("invalid study transition")
	}
	raw, e := json.Marshal(event)
	if e != nil || len(raw) > 4096 {
		return fmt.Errorf("study event payload budget")
	}
	id := studyID(stream, event.Sequence)
	grant := core.Grant{Actor: scope.Owner, Recipient: scope.Owner, Purpose: "study-protocol", Operation: core.Derive}
	read := grant
	read.Operation = core.Read
	meta := core.Metadata{ID: id, Observer: scope.Owner, Source: "study-protocol.v1", Sensitivity: core.Restricted, Confidence: 1, Valid: core.Interval{}, RecordedAt: event.At, Rights: core.Rights{Resource: id, Grants: []core.Grant{read, grant}}}
	command := graph.AppendCommand{Actor: scope.Owner, Namespace: scope.Namespace, Operation: "study.protocol", Key: id, ExpectedVersion: int64(expected), Class: graph.ResearchPayload, Payload: graph.Payload{Version: 1, Text: string(raw)}, Event: core.Event{Version: 1, Stream: stream, Type: core.ID("study." + event.Kind + ".v1"), Subject: core.Subject{Principal: scope.Owner}, Meta: meta}}
	if expected > 0 {
		command.Event.Meta.Parents = []core.ID{studyID(stream, expected)}
		command.DerivationContext = &grant
	}
	if _, e = postgres.New(j.db).Append(ctx, command); e != nil {
		return e
	}
	return gate.Commit(ctx)
}
