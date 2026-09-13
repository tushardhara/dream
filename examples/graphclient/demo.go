// Package graphclient demonstrates an independent, non-simulator host. The
// caller injects durable storage; no world, run, model, or simulated human exists.
package graphclient

import (
	"context"
	"fmt"
	"time"

	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
)

type Store interface {
	graph.MemoryJournal
	graph.EventRevoker
}
type Report struct {
	AliceView, BobView string
	Corrected          bool
	Revoked            bool
	Exported           int
}

// Run uses only fictional statements. It is exercised against a fresh disposable
// PostgreSQL database in TestGraphClientIntegration by make migration-check.
func Run(ctx context.Context, store Store) (Report, error) {
	svc := graph.MemoryService{Journal: store}
	report := Report{}
	record := func(actor, id core.ID, text string) graph.MemoryRecord {
		return graph.MemoryRecord{Event: core.Event{Version: 1, Type: graph.MemoryEventType, Stream: "statements", Subject: core.Subject{Principal: "shared-project"}, OccurredAt: 1, Meta: core.Metadata{ID: id, Observer: actor, Source: actor, Sensitivity: core.Restricted, Confidence: .6, Valid: core.Interval{}, RecordedAt: time.Unix(1, 0), Rights: core.Rights{Resource: id, Grants: []core.Grant{{Actor: actor, Recipient: actor, Purpose: "example", Operation: core.Read}, {Actor: actor, Recipient: actor, Purpose: "example", Operation: core.Derive}, {Actor: actor, Recipient: "report-reader", Purpose: "example", Operation: core.Export}}}}}, Content: graph.MemoryContent{Version: 1, Kind: graph.EpisodicMemory, Text: text, Salience: .5, HalfLife: 100, Learned: []graph.Learned{{Actor: actor, At: 1}}}}
	}
	query := func(actor core.ID) graph.MemoryQuery {
		return graph.MemoryQuery{Scope: graph.MemoryScope{Owner: actor, Namespace: "second-host"}, Actor: actor, Purpose: "example", Subject: core.Subject{Principal: "shared-project"}, ValidAt: 5, KnownAt: 5, RecordedAsOf: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), Limit: 10}
	}
	for _, actor := range []core.ID{"alice", "bob"} {
		if _, err := core.NewPrincipal(actor); err != nil {
			return report, err
		}
		text := "I found the synthetic collaboration supportive"
		if actor == "bob" {
			text = "I found the synthetic collaboration difficult"
		}
		source := record(actor, "source", text)
		q := query(actor)
		if _, err := svc.Put(ctx, q.Scope, "source", 0, source, "example"); err != nil {
			return report, err
		}
		claim := record(actor, "claim", text)
		claim.Content.Kind = graph.ClaimMemory
		claim.Event.Meta.Supporting = []core.ID{"source"}
		if _, err := svc.Put(ctx, q.Scope, "claim", 1, claim, "example"); err != nil {
			return report, err
		}
		views, err := svc.Retrieve(ctx, q)
		if err != nil {
			return report, err
		}
		if len(views) != 2 {
			return report, fmt.Errorf("expected independent source and claim")
		}
		if actor == "alice" {
			report.AliceView = views[0].Record.Content.Text
		} else {
			report.BobView = views[0].Record.Content.Text
		}
	}
	// Correct Alice's source. The old claim is not silently reinterpreted as a new
	// statement and Bob's independently owned perspective does not change.
	q := query("alice")
	correction := record("alice", "corrected", "I now find the synthetic collaboration mixed")
	correction.Supersedes = "source"
	if _, err := svc.Put(ctx, q.Scope, "corrected", 2, correction, "example"); err != nil {
		return report, err
	}
	views, err := svc.Retrieve(ctx, q)
	if err != nil {
		return report, err
	}
	report.Corrected = len(views) == 1 && views[0].Record.Event.Meta.ID == "corrected"
	revoke := record("alice", "revoke", "unused").Event
	revoke.Type = "revoke"
	revoke.Meta.Rights.Grants = nil
	if _, err = graph.RevokeMemory(ctx, store, q.Scope, "revoke", 3, revoke, "source"); err != nil {
		return report, err
	}
	views, err = svc.Retrieve(ctx, q)
	if err != nil {
		return report, err
	}
	report.Revoked = len(views) == 0
	denied, err := svc.Export(ctx, query("bob"), "unauthorized")
	if err != nil {
		return report, err
	}
	if len(denied.Records) != 0 {
		return report, fmt.Errorf("unauthorized export")
	}
	exported, err := svc.Export(ctx, query("bob"), "report-reader")
	if err != nil {
		return report, err
	}
	report.Exported = len(exported.Records)
	if !report.Corrected || !report.Revoked || report.Exported != 2 || report.AliceView == report.BobView {
		return report, fmt.Errorf("second-host acceptance failed")
	}
	return report, nil
}
