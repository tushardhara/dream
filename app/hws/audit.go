package hws

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
)

// AuditPacket is an explicitly authorized research export, not telemetry. Its
// bounded replay window contains restricted synthetic state and model context.
type AuditModel struct {
	Use           ModelUse       `json:"use"`
	Intent        ModelIntent    `json:"intent"`
	Artifact      *ModelArtifact `json:"artifact,omitempty"`
	Status        string         `json:"status"`
	Attempts      int            `json:"attempts"`
	PolicyBinding core.ID        `json:"policy_binding"`
}
type AuditPolicy struct {
	Sequence int64             `json:"sequence"`
	Decision graph.PolicyAudit `json:"decision"`
}
type AuditPacket struct {
	Version  string        `json:"version"`
	Replay   ReplayBundle  `json:"replay"`
	Models   []AuditModel  `json:"models"`
	Policies []AuditPolicy `json:"policies"`
	Hash     string        `json:"hash"`
}
type AuditStore interface {
	ReadAudit(context.Context, SnapshotKey, int64) (AuditPacket, error)
}

func ModelPolicyBinding(scope Scope, principal core.ID) (core.ID, error) {
	return (ViewGrant{Caller: principal, Realm: ViewRealm{Scope: scope, Principal: principal}, Kind: ActorViewKind, Purpose: "simulation"}).key()
}
func AuditModelUses(bundle ReplayBundle) []ModelUse {
	uses := append([]ModelUse{}, bundle.Snapshot.Models...)
	for _, frame := range bundle.Frames {
		if frame.Model != nil {
			uses = append(uses, *frame.Model)
		}
	}
	return uses
}
func SealAudit(packet AuditPacket) (AuditPacket, error) {
	packet.Hash = ""
	hash, e := ModelDigest(packet)
	if e != nil {
		return AuditPacket{}, e
	}
	packet.Hash = hash
	return packet, VerifyAudit(packet, hash)
}

// VerifyAudit needs an independently retained expected hash. It verifies the
// bounded recorded trajectory and linked typed model artifacts; it is not proof
// against an administrator rewriting both history and the expected checkpoint.
func VerifyAudit(packet AuditPacket, expected string) error {
	if packet.Version != "audit.v1" || len(expected) != 64 || packet.Hash != expected || len(packet.Models) > 256 || len(packet.Policies) > 1024 {
		return fmt.Errorf("audit envelope/budget")
	}
	copy := packet
	copy.Hash = ""
	hash, e := ModelDigest(copy)
	if e != nil || hash != expected {
		return fmt.Errorf("audit hash mismatch")
	}
	if _, e = Replay(packet.Replay, RecordedReplay); e != nil {
		return e
	}
	uses := AuditModelUses(packet.Replay)
	needed := map[core.ID]ModelUse{}
	for _, u := range uses {
		if previous, ok := needed[u.Key]; ok && previous != u {
			return ErrModel
		}
		needed[u.Key] = u
	}
	seen := map[core.ID]bool{}
	bindings := map[core.ID]bool{}
	for _, m := range packet.Models {
		if m.Intent.Validate() != nil || m.Intent.Scope != packet.Replay.Snapshot.Scope || m.Intent.Key != m.Use.Key || needed[m.Use.Key] != m.Use || seen[m.Use.Key] || m.Attempts < 1 || m.Attempts > 3 {
			return ErrModel
		}
		binding, e := ModelPolicyBinding(m.Intent.Scope, m.Intent.Principal)
		if e != nil || binding != m.PolicyBinding {
			return ErrModel
		}
		bindings[binding] = true
		seen[m.Use.Key] = true
		if m.Use.Failed {
			if m.Artifact != nil || (m.Status != "unavailable" && m.Status != "rate_limited") {
				return ErrModel
			}
			failed, _ := ModelDigest(struct {
				Version int
				Intent  ModelIntent
				Status  string
				Attempt int
			}{1, m.Intent, m.Status, m.Attempts})
			if failed != m.Use.Hash {
				return ErrModel
			}
		} else if m.Status != "complete" || m.Artifact == nil || m.Artifact.Validate(m.Intent.Input) != nil || m.Artifact.Hash != m.Use.Hash {
			return ErrModel
		}
	}
	if len(seen) != len(needed) {
		return fmt.Errorf("missing model audit")
	}
	var sequence int64
	for _, p := range packet.Policies {
		if p.Sequence <= sequence || p.Decision.Version != 1 || !bindings[p.Decision.Binding] {
			return fmt.Errorf("invalid policy audit linkage")
		}
		sequence = p.Sequence
	}
	for _, m := range packet.Models {
		contextHash, _ := ModelDigest(m.Intent.Input.Context)
		linked := false
		for _, p := range packet.Policies {
			d := p.Decision
			if d.Binding == m.PolicyBinding && d.Stage == "revalidate" && d.Decision.Allowed && d.Decision.Action == "ALLOW" && len(d.Decision.Evidence) > 0 && d.ContextRevision == m.Intent.MemoryRevision && d.ContextHash == contextHash && d.KnownAt == m.Intent.At {
				linked = true
			}
		}
		if !linked {
			return fmt.Errorf("model policy context proof unavailable")
		}
	}
	raw, e := json.Marshal(packet)
	if e != nil || len(raw) > 32<<20 {
		return fmt.Errorf("audit byte budget")
	}
	return nil
}
