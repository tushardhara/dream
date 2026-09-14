package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
	"sort"
)

func (s *Store) ReadAudit(ctx context.Context, key hws.SnapshotKey, through int64) (hws.AuditPacket, error) {
	var packet hws.AuditPacket
	bundle, e := s.ReadReplay(ctx, key, through)
	if e != nil {
		return packet, e
	}
	tx, e := s.begin(ctx)
	if e != nil {
		return packet, e
	}
	defer tx.Rollback(ctx)
	packet = hws.AuditPacket{Version: "audit.v1", Replay: bundle, Models: []hws.AuditModel{}, Policies: []hws.AuditPolicy{}}
	sc := key.Scope
	var cutoff int64
	if e = tx.QueryRow(ctx, `SELECT seq FROM dream.events WHERE actor=$1 AND namespace=$2 AND stream=$3 AND stream_version=$4`, sc.Actor, sc.Namespace, sc.Run, through).Scan(&cutoff); e != nil {
		return packet, e
	}
	uses := hws.AuditModelUses(bundle)
	sort.Slice(uses, func(i, j int) bool { return uses[i].Key < uses[j].Key })
	seen := map[core.ID]bool{}
	bindings := map[core.ID]bool{}
	for _, use := range uses {
		if seen[use.Key] {
			continue
		}
		seen[use.Key] = true
		var principal, namespace, event core.ID
		var status string
		var attempts int
		if e = tx.QueryRow(ctx, `SELECT principal,memory_namespace,event_id,status,attempt FROM dream.model_requests WHERE actor=$1 AND namespace=$2 AND run=$3 AND key=$4`, sc.Actor, sc.Namespace, sc.Run, use.Key).Scan(&principal, &namespace, &event, &status, &attempts); e != nil {
			return packet, e
		}
		payload, e := readModelPayload(ctx, tx, principal, namespace, event)
		if e != nil {
			return packet, e
		}
		binding, e := hws.ModelPolicyBinding(sc, principal)
		if e != nil {
			return packet, e
		}
		packet.Models = append(packet.Models, hws.AuditModel{Use: use, Intent: payload.Intent, Artifact: payload.Artifact, Status: status, Attempts: attempts, PolicyBinding: binding})
		bindings[binding] = true
	}
	namespaces := []string{}
	for binding := range bindings {
		hash := sha256.Sum256([]byte(binding))
		namespaces = append(namespaces, "audit:"+hex.EncodeToString(hash[:]))
	}
	rows, e := tx.Query(ctx, `SELECT e.seq,p.payload FROM dream.events e JOIN dream.private_payloads p ON(p.actor,p.namespace,p.event_id)=(e.actor,e.namespace,e.id) WHERE e.actor='policy-service' AND e.namespace=ANY($1) AND e.seq<=$2 AND e.envelope->>'type'='policy.audit.v1' AND NOT EXISTS(SELECT 1 FROM dream.tombstones t WHERE(t.actor,t.namespace,t.event_id)=(e.actor,e.namespace,e.id)) ORDER BY e.seq LIMIT 1025`, namespaces, cutoff)
	if e != nil {
		return packet, e
	}
	for rows.Next() {
		var sequence int64
		var raw []byte
		if e = rows.Scan(&sequence, &raw); e != nil {
			rows.Close()
			return packet, e
		}
		var payload graph.Payload
		var audit graph.PolicyAudit
		if json.Unmarshal(raw, &payload) != nil || json.Unmarshal([]byte(payload.Text), &audit) != nil {
			rows.Close()
			return packet, hws.ErrModel
		}
		packet.Policies = append(packet.Policies, hws.AuditPolicy{Sequence: sequence, Decision: audit})
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return packet, e
	}
	// Re-read under the same journal lock, so a revocation between the initial
	// replay read and model/policy assembly cannot escape this response.
	if _, _, e = readSnapshot(ctx, tx, key); e != nil {
		return packet, e
	}
	for _, frame := range bundle.Frames {
		var revoked bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dream.tombstones WHERE actor=$1 AND namespace=$2 AND event_id=$3)`, sc.Actor, sc.Namespace, runtimeID(string(sc.Run), frame.Revision)).Scan(&revoked); e != nil {
			return packet, e
		}
		if revoked {
			return packet, ErrRevoked
		}
	}
	return hws.SealAudit(packet)
}
