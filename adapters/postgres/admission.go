package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
)

// RuntimeReady checks the actual session's reachable roles, not a claimed role
// string. A login able to SET ROLE to an owner/superuser is also rejected.
func (s *Store) RuntimeReady(ctx context.Context) error {
	if s == nil || s.db == nil {
		return hws.ErrViewDenied
	}
	var safe bool
	err := s.db.QueryRow(ctx, `SELECT current_user=session_user
 AND (SELECT ready FROM dream.recovery_gate WHERE singleton)
 AND (SELECT max(version) FROM dream.schema_versions)=6
 AND pg_has_role(session_user,'dream_writer','USAGE')
 AND NOT EXISTS(SELECT 1 FROM pg_roles r WHERE pg_has_role(session_user,r.oid,'MEMBER') AND (r.rolsuper OR r.rolbypassrls OR r.rolcreaterole OR r.rolcreatedb))
 AND NOT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='dream' AND pg_has_role(session_user,c.relowner,'MEMBER'))
 AND (SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='dream' AND c.relname IN ('observable_payloads','private_payloads','research_payloads','runtime_payloads','reader_scopes','model_payloads','snapshot_payloads') AND c.relrowsecurity AND c.relforcerowsecurity)=7`).Scan(&safe)
	if err != nil || !safe {
		return hws.ErrViewDenied
	}
	return nil
}

// AdmitRequest reserves one request permanently in the versioned journal before
// dispatch. Errors/retries still consume admission; idempotent runtime commands
// remain separately deduplicated. A fresh Store/process cannot reset counters.
func (s *Store) AdmitRequest(ctx context.Context, b hws.RequestBudget) error {
	if b.Validate() != nil {
		return hws.ErrAdmission
	}
	binding, err := hws.ModelDigest(struct {
		Credential core.ID
		Scope      hws.Scope
	}{b.Credential, b.Scope})
	if err != nil {
		return err
	}
	namespace := core.ID("admission:" + binding)
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var total, minute int64
	var now time.Time
	err = tx.QueryRow(ctx, `SELECT count(*),coalesce(max(recorded_at),clock_timestamp()) FROM dream.events WHERE actor=$1 AND namespace=$2 AND stream='requests'`, b.Scope.Actor, namespace).Scan(&total, &now)
	if err != nil {
		return err
	}
	err = tx.QueryRow(ctx, `SELECT greatest(clock_timestamp(),$1::timestamptz)`, now).Scan(&now)
	if err != nil {
		return err
	}
	err = tx.QueryRow(ctx, `SELECT count(*) FROM dream.events WHERE actor=$1 AND namespace=$2 AND stream='requests' AND stream_version >= (SELECT min(stream_version) FROM dream.events WHERE actor=$1 AND namespace=$2 AND stream='requests' AND recorded_at>=date_trunc('minute',$3::timestamptz))`, b.Scope.Actor, namespace, now).Scan(&minute)
	if err != nil {
		return err
	}
	if total >= int64(b.Total) || minute >= int64(b.PerMinute) {
		return hws.ErrAdmission
	}
	id := core.ID(fmt.Sprintf("request:%d", total+1))
	command := graph.AppendCommand{Actor: b.Scope.Actor, Namespace: namespace, Operation: "api.admission", Key: id, ExpectedVersion: total, Class: graph.PrivatePayload, Payload: graph.Payload{Version: 1, Text: "request admitted"}, Event: core.Event{Version: 1, Stream: "requests", Type: "api.admission.v1", Subject: core.Subject{Principal: b.Scope.Actor}, Meta: core.Metadata{ID: id, Observer: b.Scope.Actor, Source: "api.v1", Sensitivity: core.Restricted, Confidence: 1, Valid: core.Interval{}, RecordedAt: now, Rights: core.Rights{Resource: id}}}}
	if _, err = s.append(ctx, tx, command); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
