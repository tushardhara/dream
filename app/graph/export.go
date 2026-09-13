package graph

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tushardhara/dream/core"
)

type EventRevoker interface {
	Revoke(context.Context, AppendCommand, core.ID) (AppendResult, error)
}

// RevokeMemory delegates the atomic versioned event/tombstone/purge operation.
// Authentication of the scope's owner is a host responsibility, like Put.
func RevokeMemory(ctx context.Context, store EventRevoker, scope MemoryScope, key core.ID, expected int64, event core.Event, root core.ID) (AppendResult, error) {
	if store == nil || scope.Validate() != nil || event.Meta.Observer != scope.Owner || event.Type != "revoke" {
		return AppendResult{}, fmt.Errorf("invalid memory revocation")
	}
	c := AppendCommand{Actor: scope.Owner, Namespace: scope.Namespace, Operation: "memory.revoke", Key: key, ExpectedVersion: expected, Event: event, Class: PrivatePayload, Payload: Payload{Version: 1, Text: "explicit source revocation"}}
	if err := c.Validate(); err != nil {
		return AppendResult{}, err
	}
	return store.Revoke(ctx, c, root)
}

type MemoryExport struct {
	Version   uint32         `json:"version"`
	Scope     MemoryScope    `json:"scope"`
	Actor     core.ID        `json:"actor"`
	Recipient core.ID        `json:"recipient"`
	Purpose   core.ID        `json:"purpose"`
	Records   []MemoryRecord `json:"records"`
}

// Export uses one fresh snapshot and requires explicit export permission on the
// record and complete provenance, in addition to normal actor retrieval rights.
// It returns a bounded transient artifact, not a stored snapshot or ongoing grant.
// Bytes already delivered to a caller cannot be recalled by a later revocation.
func (s MemoryService) Export(ctx context.Context, q MemoryQuery, recipient core.ID) (MemoryExport, error) {
	out := MemoryExport{Version: 1, Scope: q.Scope, Actor: q.Actor, Recipient: recipient, Purpose: q.Purpose, Records: []MemoryRecord{}}
	if q.Validate() != nil || recipient.Validate() != nil || s.Journal == nil {
		return MemoryExport{}, fmt.Errorf("invalid memory export")
	}
	entries, err := s.Journal.ReadMemory(ctx, q.Scope)
	if err != nil {
		return MemoryExport{}, err
	}
	idx, err := RebuildMemory(q.Scope, entries)
	if err != nil {
		return MemoryExport{}, err
	}
	done := map[core.ID]bool{}
	allowed := map[core.ID]bool{}
	var canExport func(core.ID) bool
	canExport = func(id core.ID) bool {
		if done[id] {
			return allowed[id]
		}
		done[id] = true
		e, ok := idx.byID[id]
		if !ok || e.Revoked || e.Content == nil || !e.Event.Meta.Rights.Allows(core.PermissionRequest{Resource: id, Context: core.Grant{Actor: q.Actor, Recipient: recipient, Purpose: q.Purpose, Operation: core.Export}}) {
			return false
		}
		sources := memorySources(e.record())
		if e.Supersedes != "" {
			sources = append(sources, e.Supersedes)
		}
		for _, id := range sources {
			if !canExport(id) {
				return false
			}
		}
		allowed[id] = true
		return true
	}
	for _, pick := range idx.selectMatchingMemory(q, func(r MemoryRecord) bool { return canExport(r.Event.Meta.ID) }) {
		if canExport(pick.ID) {
			out.Records = append(out.Records, idx.byID[pick.ID].record())
		}
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return MemoryExport{}, err
	}
	var detached MemoryExport
	if err = json.Unmarshal(raw, &detached); err != nil {
		return MemoryExport{}, err
	}
	return detached, nil
}
