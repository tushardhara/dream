package assistanceclient

import (
	"time"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
)

// Fixture contains only explicitly authored synthetic self-reports. The helper
// knows each report at time 1 through an exact read grant. No worlds are required.
func Fixture(arm assistance.Arm, goal assistance.Goal) (*Local, assistance.Request) {
	r := assistance.Request{Version: assistance.Version, ID: "request", Helper: "helper", User: "alice", Purpose: "help", Participants: []core.ID{"alice", "bob"}, Arm: arm, Goal: goal, At: 2, Seed: 7}
	l := New(r.Helper, r.User, goal)
	for _, owner := range r.Participants {
		id := core.ID(string(owner) + "-request")
		scope := graph.MemoryScope{Owner: owner, Namespace: "synthetic"}
		entry := graph.MemoryEntry{Sequence: 1, Event: core.Event{Version: 1, Type: graph.MemoryEventType, Stream: "memories", Subject: core.Subject{Principal: owner}, OccurredAt: 1, Meta: core.Metadata{ID: id, Observer: owner, Source: owner, Sensitivity: core.Restricted, Confidence: .8, Valid: core.Interval{Start: 0}, RecordedAt: time.Unix(1, 0).UTC(), Rights: core.Rights{Resource: id, Grants: []core.Grant{{Actor: r.Helper, Recipient: r.Helper, Purpose: r.Purpose, Operation: core.Read}}}}}, Content: &graph.MemoryContent{Version: 1, Kind: graph.EpisodicMemory, Text: "Synthetic explicit request for practical coordination", Salience: .5, HalfLife: 10, Learned: []graph.Learned{{Actor: owner, At: 1}, {Actor: r.Helper, At: 1}}}}
		l.SetMemory(scope, []graph.MemoryEntry{entry})
		if arm == assistance.Multi || arm == assistance.Single && owner == r.User {
			r.Contexts = append(r.Contexts, graph.ContextProposal{Version: 1, Binding: r.ID, Recipient: r.Helper, Operation: core.Read, Mode: graph.ExternalContext, Sources: []core.ID{id}, Query: graph.MemoryQuery{Scope: scope, Actor: r.Helper, Purpose: r.Purpose, Subject: core.Subject{Principal: owner}, ValidAt: r.At, KnownAt: r.At, RecordedAsOf: time.Unix(10, 0).UTC(), Limit: 16}})
		}
	}
	_ = l.Register(r)
	return l, r
}
