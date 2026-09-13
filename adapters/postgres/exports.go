package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
)

func exportRealm(sc hws.Scope, caller core.ID) (core.ID, error) {
	if sc.Validate() != nil || caller.Validate() != nil {
		return "", hws.ErrViewDenied
	}
	digest, err := hws.ModelDigest(struct {
		Scope  hws.Scope
		Caller core.ID
	}{sc, caller})
	return core.ID("exports:" + digest), err
}
func (s *Store) SaveExport(ctx context.Context, submission hws.ExportSubmission) error {
	if submission.Version != 1 || submission.ID.Validate() != nil || len(submission.Request) == 0 || len(submission.Request) > 16384 || len(submission.Manifest) == 0 || len(submission.Manifest) > 4096 {
		return hws.ErrCommand
	}
	namespace, err := exportRealm(submission.Scope, submission.Caller)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(submission)
	if err != nil {
		return err
	}
	id := submission.ID
	command := graph.AppendCommand{Actor: submission.Scope.Actor, Namespace: namespace, Operation: "export.submit", Key: id, Class: graph.PrivatePayload, Payload: graph.Payload{Version: 1, Text: string(raw)}, Event: core.Event{Version: 1, Stream: id, Type: "api.export.v1", Subject: core.Subject{Principal: submission.Caller}, Meta: core.Metadata{ID: id, Observer: submission.Scope.Actor, Source: "api.v1", Sensitivity: core.Restricted, Confidence: 1, Valid: core.Interval{}, RecordedAt: time.Unix(0, 0), Rights: core.Rights{Resource: id}}}}
	_, err = s.Append(ctx, command)
	return err
}
func (s *Store) LoadExport(ctx context.Context, sc hws.Scope, caller, id core.ID) (hws.ExportSubmission, error) {
	var out hws.ExportSubmission
	namespace, err := exportRealm(sc, caller)
	if err != nil || id.Validate() != nil {
		return out, hws.ErrViewDenied
	}
	var raw []byte
	tx, err := s.beginRead(ctx)
	if err != nil {
		return hws.ExportSubmission{}, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `SELECT p.payload FROM dream.private_payloads p JOIN dream.events e ON(e.actor,e.namespace,e.id)=(p.actor,p.namespace,p.event_id) WHERE p.actor=$1 AND p.namespace=$2 AND p.event_id=$3 AND e.envelope->>'type'='api.export.v1' AND NOT EXISTS(SELECT 1 FROM dream.tombstones t WHERE(t.actor,t.namespace,t.event_id)=(e.actor,e.namespace,e.id))`, sc.Actor, namespace, id).Scan(&raw)
	if err != nil {
		return out, hws.ErrViewDenied
	}
	var payload graph.Payload
	if json.Unmarshal(raw, &payload) != nil || json.Unmarshal([]byte(payload.Text), &out) != nil || out.Version != 1 || out.Scope != sc || out.Caller != caller || out.ID != id {
		return hws.ExportSubmission{}, hws.ErrViewDenied
	}
	return out, nil
}
