package assistanceclient

import (
	"fmt"
	"time"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
)

// TemporalFixture authors a bounded synthetic diary/circumstance snapshot. Its
// explicit source grants are data permissions; callers append willingness alone.
func TemporalFixture(arm assistance.Arm, at core.LogicalTime, channel core.ID, facts []core.TemporalFact) (*Local, assistance.Request, error) {
	focus := core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: core.Childcare, RoleContext: "family"}
	profiles := []core.RelationshipContext{DomainProfile("alice", "bob", "alice-care", core.Childcare, "family", .7, .6), DomainProfile("bob", "alice", "bob-care", core.Childcare, "family", .7, .6)}
	base, r, e := DomainFixture(arm, focus, profiles)
	if e != nil {
		return nil, r, e
	}
	local := New(r.Helper, r.User, assistance.Unknown)
	r.Version = assistance.TemporalVersion
	r.ID = "temporal-request"
	r.Goal = assistance.Unknown
	r.At = at
	r.Scope.Class = core.Clarification
	r.Temporal = &core.TemporalFocus{Version: core.TemporalFocusVersion, Observer: r.User, Other: r.Scope.Target, Channel: channel}
	for _, fact := range facts {
		if fact.Observer != "alice" && fact.Observer != "bob" {
			return nil, r, fmt.Errorf("unsupported synthetic observer")
		}
	}
	for scope, original := range base.entries {
		entries := copyValue(original)
		sources := []core.ID{}
		seen := map[core.ID]bool{}
		for _, entry := range entries {
			sources = append(sources, entry.Event.Meta.ID)
			seen[entry.Event.Meta.ID] = true
		}
		appendEntry := func(id core.ID, text string, parents []core.ID) error {
			if seen[id] {
				return fmt.Errorf("duplicate synthetic temporal record")
			}
			seen[id] = true
			entry := copyValue(original[0])
			entry.Sequence = int64(len(entries) + 1)
			entry.Event.Meta.ID = id
			entry.Event.Meta.Rights.Resource = id
			entry.Event.Meta.Parents = parents
			entry.Event.OccurredAt = at
			entry.Content.Kind = graph.EpisodicMemory
			entry.Content.Text = text
			for i := range entry.Content.Learned {
				entry.Content.Learned[i].At = at
			}
			entries = append(entries, entry)
			sources = append(sources, id)
			return nil
		}
		for _, fact := range facts {
			if fact.Observer != scope.Owner {
				continue
			}
			text, e := graph.EncodeTemporal(fact)
			if e != nil {
				return nil, r, e
			}
			if !seen[fact.Source] {
				if e = appendEntry(fact.Source, "Synthetic voluntarily supplied circumstance and observed channel diary", nil); e != nil {
					return nil, r, e
				}
			}
			if e = appendEntry(fact.Account, text, []core.ID{fact.Source}); e != nil {
				return nil, r, e
			}
		}
		local.SetMemory(scope, entries)
		for i := range r.Contexts {
			if r.Contexts[i].Query.Scope == scope {
				r.Contexts[i].Version = 2
				r.Contexts[i].Binding = r.ID
				r.Contexts[i].Sources = sources
				r.Contexts[i].Query.ValidAt = at
				r.Contexts[i].Query.KnownAt = at
				r.Contexts[i].Query.RecordedAsOf = time.Unix(1000, 0).UTC()
			}
		}
	}
	return local, r, local.Register(r)
}

// ContactFacts keeps explicit preference separate from a bounded complete-channel
// diary. CompleteChannel is an authored observation-coverage statement, never
// inferred from silence or generalized to another channel.
func ContactFacts(owner, other core.ID, at, upper core.LogicalTime) []core.TemporalFact {
	source := core.ID(string(owner) + "-temporal-source")
	preference := core.TemporalFact{Version: core.TemporalFactVersion, Account: core.ID(string(owner) + "-rhythm"), Observer: owner, Person: owner, With: other, Channel: "chat", Source: source, Kind: "expectation", Basis: "self_report", FreshFor: 365, MinGap: 1, MaxGap: upper}
	diary := core.TemporalFact{Version: core.TemporalFactVersion, Account: core.ID(string(owner) + "-contacts"), Observer: owner, Person: owner, With: other, Channel: "chat", Source: source, Kind: "contacts", Basis: "observed", Observations: []core.LogicalTime{0, 1, 2, 3}, ObservedThrough: at, CompleteChannel: true}
	return []core.TemporalFact{preference, diary}
}
func LifeFact(owner, other core.ID, signal string, confirmed, fresh core.LogicalTime) core.TemporalFact {
	return core.TemporalFact{Version: core.TemporalFactVersion, Account: core.ID(string(owner) + "-circumstance"), Observer: owner, Person: owner, With: other, Channel: "chat", Source: core.ID(string(owner) + "-temporal-source"), Kind: "circumstance", Basis: "self_report", Category: "responsibility", Signal: signal, ConfirmedAt: confirmed, FreshFor: fresh}
}
