package assistanceclient

import (
	"fmt"
	"time"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
)

// DomainFixture composes explicitly authored, synthetic accounts with the real
// graph policy and helper host. Profiles are observations, never consent. The
// caller must append interpersonal preferences separately before execution.
func DomainFixture(arm assistance.Arm, focus core.RelationshipFocus, profiles []core.RelationshipContext) (*Local, assistance.Request, error) {
	r := assistance.Request{Version: assistance.DomainVersion, ID: "domain-request", Helper: "helper", User: "alice", Purpose: "help", Participants: []core.ID{"alice", "bob"}, Arm: arm, Goal: assistance.Coordinate, At: 2, Seed: 7, Focus: &focus, Scope: &core.InteractionScope{Version: core.InteractionScopeVersion, Initiator: "alice", Target: "bob", Topic: core.ID(focus.Domain), Class: core.Discussion}}
	l := New(r.Helper, r.User, r.Goal)
	for _, owner := range r.Participants {
		scope := graph.MemoryScope{Owner: owner, Namespace: "domain-synthetic"}
		entries := []graph.MemoryEntry{}
		sources := []core.ID{}
		seen := map[core.ID]bool{}
		add := func(id core.ID, kind graph.MemoryKind, text string, parents []core.ID) {
			if seen[id] {
				return
			}
			seen[id] = true
			grants := []core.Grant{{Actor: r.Helper, Recipient: r.Helper, Purpose: r.Purpose, Operation: core.Read}, {Actor: owner, Recipient: owner, Purpose: "simulation", Operation: core.Read}, {Actor: owner, Recipient: owner, Purpose: "simulation", Operation: core.Derive}}
			entries = append(entries, graph.MemoryEntry{Sequence: int64(len(entries) + 1), Event: core.Event{Version: 1, Type: graph.MemoryEventType, Stream: "memories", Subject: core.Subject{Principal: owner}, OccurredAt: 1, Meta: core.Metadata{ID: id, Observer: owner, Source: owner, Parents: parents, Sensitivity: core.Restricted, Confidence: 1, Valid: core.Interval{}, RecordedAt: time.Unix(1, 0).UTC(), Rights: core.Rights{Resource: id, Grants: grants}}}, Content: &graph.MemoryContent{Version: 1, Kind: kind, Text: text, Salience: .5, HalfLife: 10, Learned: []graph.Learned{{Actor: owner, At: 1}, {Actor: r.Helper, At: 1}}}})
			sources = append(sources, id)
		}
		for _, profile := range profiles {
			if profile.Observer != owner {
				continue
			}
			if profile.Version != 2 || profile.Validate(owner, profile.Other) != nil {
				return nil, r, fmt.Errorf("invalid synthetic domain account")
			}
			for _, id := range profile.Sources() {
				add(id, graph.EpisodicMemory, "Synthetic account: can you help with this?", nil)
			}
			from, to := core.Subject{Principal: owner}, core.Subject{Principal: profile.Other}
			text, err := graph.EncodeRelation(graph.RelationState{Version: 3, ID: core.ID(string(owner) + "-relationship"), Kind: "edge", From: &from, To: &to, Types: profile.Types, Context: &profile}, owner)
			if err != nil {
				return nil, r, err
			}
			add(profile.Account, graph.RelationshipMemory, text, profile.Sources())
		}
		l.SetMemory(scope, entries)
		if len(sources) > 0 && (arm == assistance.Multi || arm == assistance.Single && owner == r.User) {
			r.Contexts = append(r.Contexts, graph.ContextProposal{Version: 1, Binding: r.ID, Recipient: r.Helper, Operation: core.Read, Mode: graph.ExternalContext, Sources: sources, Query: graph.MemoryQuery{Scope: scope, Actor: r.Helper, Purpose: r.Purpose, Subject: core.Subject{Principal: owner}, ValidAt: r.At, KnownAt: r.At, RecordedAsOf: time.Unix(10, 0).UTC(), Limit: 16}})
		}
	}
	return l, r, l.Register(r)
}

// DomainProfile assigns no coefficients from domain, frame or role names. Its
// numerical observations must be supplied explicitly by the synthetic author.
func DomainProfile(owner, other, account core.ID, domain core.RelationshipDomain, frame core.ID, trust, expectation float64) core.RelationshipContext {
	source := core.ID(string(account) + "-outcome")
	return core.RelationshipContext{Version: 2, Account: account, Observer: owner, Other: other, Domain: domain, RoleContext: frame, ContextSource: core.ID(string(account) + "-frame"), Types: []core.ID{"sibling", "business_partner"}, Valid: core.Interval{}, Measures: []core.RelationshipMeasure{{Kind: "trust", Value: trust, Confidence: 1, Source: source}, {Kind: "expectation", Value: expectation, Confidence: 1, Source: source}}}
}
