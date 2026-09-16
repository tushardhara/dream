// Package groupexperiment composes native simulated choices with a reusable
// permissioned helper. The core history is authored synthetic evidence, not a
// claim about real groups, popularity, caregiving norms or welfare.
package groupexperiment

import (
	"fmt"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/demo"
	"time"
)

type Case struct {
	Native       demo.GroupNativeInput
	Budget       core.GroupBudget
	Reservations []core.GroupReservation
	Boundaries   []core.Boundary
}

func Person(i int) core.ID { return core.ID(fmt.Sprintf("person:%02d", i+1)) }
func Focus() core.RelationshipFocus {
	return core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: core.PracticalCoordination, RoleContext: "everyday"}
}
func grants(owner, requester core.ID, all []core.ID, root bool) []core.Grant {
	out := []core.Grant{}
	seen := map[core.Grant]bool{}
	add := func(g core.Grant) {
		if !seen[g] {
			seen[g] = true
			out = append(out, g)
		}
	}
	for _, who := range []core.ID{owner, requester, "helper"} {
		for _, op := range []core.Operation{core.Read, core.Derive} {
			add(core.Grant{Actor: who, Recipient: who, Purpose: "help", Operation: op})
		}
	}
	add(core.Grant{Actor: "helper", Recipient: requester, Purpose: "help", Operation: core.ShareOnRequest})
	sim := []core.ID{owner}
	if root {
		sim = all
	}
	for _, who := range sim {
		for _, op := range []core.Operation{core.Read, core.Derive} {
			add(core.Grant{Actor: who, Recipient: who, Purpose: "simulation", Operation: op})
		}
	}
	return out
}
func meta(id, owner core.ID, at core.LogicalTime, all []core.ID, root bool) core.Metadata {
	return core.Metadata{ID: id, Observer: owner, Source: owner, Sensitivity: core.Restricted, Confidence: .8, Valid: core.Interval{Start: at}, RecordedAt: time.Unix(int64(at), 0).UTC(), Rights: core.Rights{Resource: id, Grants: grants(owner, Person(0), all, root)}}
}
func Boundary(owner, other core.ID, class core.InteractionClass, decision core.Willingness, at core.LogicalTime) core.Boundary {
	id := core.ID(fmt.Sprintf("boundary:%s:%s:%s:%s", owner, other, class, decision))
	return core.Boundary{Version: core.BoundaryVersion, Meta: core.Metadata{ID: id, Observer: owner, Source: owner, Sensitivity: core.Restricted, Confidence: 1, Supporting: []core.ID{"explicit-fixture-participation"}, Valid: core.Interval{Start: at}, RecordedAt: time.Unix(int64(at), 0).UTC(), Rights: core.Rights{Resource: id}}, Principal: owner, With: other, Topic: core.ID(Focus().Domain), Class: class, Decision: decision, Basis: "self_report", OccurredAt: at, LearnedAt: at}
}
func Fixture(people int, seed uint64, omitted bool, careBurden float64) (Case, error) {
	sc, e := demo.ResponsiveScenario(people, 1, seed)
	if e != nil {
		return Case{}, e
	}
	all := []core.ID{}
	budget := core.GroupBudget{Shared: map[core.ID]int64{"shared-time": 6}, Personal: map[core.ID]int64{}}
	for i := 0; i < people; i++ {
		all = append(all, Person(i))
		budget.Personal[Person(i)] = 4
	}
	h := core.GroupHistory{Version: core.GroupHistoryVersion, Decisions: []core.GroupDecision{}, Accounts: []core.GroupAccount{}}
	for period := 0; period < 4; period++ {
		at := core.LogicalTime(1 + 10*period)
		start, end := at+9, at+14
		if period == 2 {
			end = 45
		}
		if period == 3 {
			at, start, end = 32, 33, 43
		}
		id := core.ID(fmt.Sprintf("group-decision:%d", period))
		receipt := core.ID(fmt.Sprintf("group-receipt:%d", period))
		invited := append([]core.ID{}, all...)
		if omitted && period < 2 {
			invited = invited[:len(invited)-1]
		}
		d := core.GroupDecision{Version: core.GroupHistoryVersion, Meta: meta(receipt, Person(0), at, all, true), ID: id, Group: "friends", Intention: "shared-outing", Focus: Focus(), At: at, Window: core.Interval{Start: start, End: &end}, Members: append([]core.ID{}, all...), Affected: append([]core.ID{}, all...), Invited: invited, Required: []core.GroupRequirement{{Task: "care", Units: 4}}, Options: []core.GroupOption{
			{ID: "solo-care", Resource: "shared-time", Units: 3, Tasks: []core.GroupTask{{Task: "care", Owner: Person(people - 1), Units: 4}}},
			{ID: "shared-care", Resource: "shared-time", Units: 2, Tasks: []core.GroupTask{{Task: "care", Owner: Person(people - 1), Units: 2}, {Task: "care", Owner: Person(people - 2), Units: 2}}},
			{ID: "covered-care", Resource: "shared-time", Units: 2, Tasks: []core.GroupTask{{Task: "care", Owner: Person(people - 2), Units: 4}}},
		}}
		h.Decisions = append(h.Decisions, d)
		for i, member := range all {
			observedAt := start + 1
			phase := "experienced"
			if period == 3 {
				observedAt = 32
				phase = "planning"
			}
			a := core.GroupAccount{Version: core.GroupHistoryVersion, Meta: meta(core.ID(fmt.Sprintf("account:%d:%d", period, i)), member, observedAt, all, false), Group: d.Group, Decision: d.ID, Member: member, Focus: d.Focus, OccurredAt: observedAt, LearnedAt: observedAt, Phase: phase, Membership: "member", Participation: "unknown", Intention: d.Intention, Capacity: core.ObservedGroupQuantity(4), Effort: core.UnknownGroupQuantity(), Benefit: core.UnknownGroupQuantity(), Burden: core.UnknownGroupQuantity(), NormStance: "unspecified"}
			a.Meta.Supporting = []core.ID{receipt}
			if phase == "planning" {
				a.ApprovedOptions = []core.ID{"solo-care", "shared-care", "covered-care"}
			} else {
				a.Participation = "attended"
				a.CoPresent = append([]core.ID{}, invited...)
				a.NormStance = "supports"
				a.Effort = core.ObservedGroupQuantity(0)
				a.Benefit = core.ObservedGroupQuantity(.5)
				a.Burden = core.ObservedGroupQuantity(0)
				if omitted && period < 2 && i == people-1 {
					a.Participation = "not_invited"
					a.CoPresent = nil
					a.NormStance = "disputes"
				}
				if i == people-1 {
					a.Effort = core.ObservedGroupQuantity(careBurden)
					a.Burden = core.ObservedGroupQuantity(careBurden)
				}
			}
			h.Accounts = append(h.Accounts, a)
		}
	}
	boundaries := []core.Boundary{Boundary(Person(0), Person(1), core.PrivatePreparation, core.Willing, 0)}
	for i := 1; i < people; i++ {
		boundaries = append(boundaries, Boundary(Person(0), Person(i), core.Coordination, core.Willing, 0), Boundary(Person(i), Person(0), core.Coordination, core.Willing, 0))
	}
	c := Case{Native: demo.GroupNativeInput{Version: demo.GroupVersion, Scenario: sc, History: h, Decision: "group-decision:3", At: 33, Theme: "joy"}, Budget: budget, Reservations: []core.GroupReservation{}, Boundaries: boundaries}
	if e = h.Validate(); e != nil {
		return c, e
	}
	if e = core.ValidateBoundaryLog(boundaries); e != nil {
		return c, e
	}
	return c, budget.Validate()
}
