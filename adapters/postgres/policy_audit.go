package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
)

var _ graph.DecisionRecorder = (*Store)(nil)

// RecordPolicyDecision records sanitized operational decisions as ordinary v1
// journal events. No source text/IDs, provider errors, labels or grants are copied.
// Operational random IDs avoid restart collisions; they are not simulation RNG.
func (s *Store) RecordPolicyDecision(ctx context.Context, a graph.PolicyAudit) error {
	if a.Version != 1 || a.Binding.Validate() != nil || (a.Stage != "approve" && a.Stage != "revalidate" && a.Stage != "writer_output" && a.Stage != "view_access") || len(a.Decision.Evidence) > 16 || (a.Decision.Action != "ALLOW" && a.Decision.Action != "WAIT") || a.Decision.Allowed != (a.Decision.Action == "ALLOW") {
		return fmt.Errorf("invalid policy audit")
	}
	for _, e := range a.Decision.Evidence {
		if e.Clause.Validate() != nil || e.SourceOrdinal < -1 || e.SourceOrdinal > 15 {
			return fmt.Errorf("invalid policy audit clause")
		}
	}
	raw, err := json.Marshal(a)
	if err != nil {
		return err
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return err
	}
	id := core.ID("policy:" + hex.EncodeToString(nonce[:]))
	hash := sha256.Sum256([]byte(a.Binding))
	namespace := core.ID("audit:" + hex.EncodeToString(hash[:]))
	c := graph.AppendCommand{Actor: "policy-service", Namespace: namespace, Operation: "policy.audit", Key: id, Class: graph.PrivatePayload, Payload: graph.Payload{Version: 1, Text: string(raw)}, Event: core.Event{Version: 1, Stream: id, Type: "policy.audit.v1", Subject: core.Subject{Principal: "policy-service"}, Meta: core.Metadata{ID: id, Observer: "policy-service", Source: "policy.v1", Sensitivity: core.Restricted, Confidence: 1, Valid: core.Interval{}, RecordedAt: time.Unix(0, 0), Rights: core.Rights{Resource: id}}}}
	_, err = s.Append(ctx, c)
	return err
}
