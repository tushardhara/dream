package ordinaryclient

import (
	"fmt"
	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
	"time"
)

func Boundary(person, other, topic core.ID, decision core.Willingness, at core.LogicalTime) core.Boundary {
	id := core.ID(fmt.Sprintf("ordinary-boundary:%s:%s:%s:%d", person, topic, decision, at))
	return core.Boundary{Version: core.BoundaryVersion, Meta: core.Metadata{ID: id, Observer: person, Source: person, Sensitivity: core.Restricted, Confidence: 1, Supporting: []core.ID{"explicit-ordinary-fixture"}, Valid: core.Interval{Start: at}, RecordedAt: time.Unix(int64(at), 0).UTC(), Rights: core.Rights{Resource: id}}, Principal: person, With: other, Topic: topic, Class: core.Coordination, Decision: decision, Basis: "self_report", OccurredAt: at, LearnedAt: at}
}
func groupMeta(id, owner core.ID) core.Metadata {
	gs := []core.Grant{}
	seen := map[core.Grant]bool{}
	for _, person := range []core.ID{owner, "a", "helper"} {
		for _, op := range []core.Operation{core.Read, core.Derive} {
			g := core.Grant{Actor: person, Recipient: person, Purpose: "help", Operation: op}
			if !seen[g] {
				seen[g] = true
				gs = append(gs, g)
			}
		}
	}
	gs = append(gs, core.Grant{Actor: "helper", Recipient: "a", Purpose: "help", Operation: core.ShareOnRequest})
	return core.Metadata{ID: id, Observer: owner, Source: owner, Sensitivity: core.Restricted, Confidence: .8, Valid: core.Interval{}, RecordedAt: time.Unix(0, 0).UTC(), Rights: core.Rights{Resource: id, Grants: gs}}
}

// FixtureGroup holds care obligations constant across two ways to coordinate.
// The slot plan spends one coordination-time unit; manual matching spends three.
// Both reserve exactly one care unit from EACH explicitly consenting participant.
func FixtureGroup(start, end core.LogicalTime) (core.GroupHistory, core.GroupBudget, []core.Boundary) {
	focus := core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: core.PracticalCoordination, RoleContext: "home"}
	tasks := []core.GroupTask{{Task: "care", Owner: "a", Units: 1}, {Task: "care", Owner: "b", Units: 1}}
	d := core.GroupDecision{Version: core.GroupHistoryVersion, Meta: groupMeta("ordinary-plan-receipt", "a"), ID: "ordinary-plan", Group: "household", Intention: "arrange-call", Focus: focus, At: 0, Window: core.Interval{Start: start, End: &end}, Members: []core.ID{"a", "b"}, Affected: []core.ID{"a", "b"}, Invited: []core.ID{"a", "b"}, Required: []core.GroupRequirement{{Task: "care", Units: 2}}, Options: []core.GroupOption{{ID: "manual", Resource: "coordination-time", Units: 3, Tasks: copyValue(tasks)}, {ID: "slot", Resource: "coordination-time", Units: 1, Tasks: copyValue(tasks)}}}
	h := core.GroupHistory{Version: core.GroupHistoryVersion, Decisions: []core.GroupDecision{d}, Accounts: []core.GroupAccount{}}
	for _, person := range []core.ID{"a", "b"} {
		m := groupMeta(core.ID("plan-account:"+string(person)), person)
		m.Supporting = []core.ID{d.Meta.ID}
		h.Accounts = append(h.Accounts, core.GroupAccount{Version: core.GroupHistoryVersion, Meta: m, Group: d.Group, Decision: d.ID, Member: person, Focus: focus, Phase: "planning", Membership: "member", Participation: "unknown", Intention: d.Intention, ApprovedOptions: []core.ID{"manual", "slot"}, Capacity: core.ObservedGroupQuantity(4), Effort: core.UnknownGroupQuantity(), Benefit: core.UnknownGroupQuantity(), Burden: core.UnknownGroupQuantity(), NormStance: "unspecified"})
	}
	b := core.GroupBudget{Shared: map[core.ID]int64{"coordination-time": 10}, Personal: map[core.ID]int64{"a": 4, "b": 4}}
	boundaries := []core.Boundary{Boundary("a", "b", "coordination", core.Willing, 0), Boundary("b", "a", "coordination", core.Willing, 0)}
	return h, b, boundaries
}
func Fixture(kind core.OrdinaryKind) (*Local, assistance.OrdinaryRequest, error) {
	h, b, boundaries := FixtureGroup(2, 9)
	activity := core.ID(kind)
	boundaries = append(boundaries, Boundary("a", "b", activity, core.Willing, 0), Boundary("b", "a", activity, core.Willing, 0))
	l, e := New("ordinary-session", "helper", []core.ID{"a", "b"}, h, b, nil, boundaries)
	if e != nil {
		return nil, assistance.OrdinaryRequest{}, e
	}
	end := core.LogicalTime(200)
	for _, person := range []core.ID{"a", "b"} {
		p := core.OrdinaryPreference{Version: core.OrdinaryVersion, Source: core.ID("pref:" + string(person) + ":" + string(kind)), Person: person, Activity: activity, Kind: kind, Window: core.Interval{End: &end}, Choice: "wanted", Availability: "available", ExpectedBenefit: core.ObservedGroupQuantity(.6), MaxEffort: core.ObservedGroupQuantity(4)}
		if e = l.PutPreference(person, p, []core.ID{"a"}, 0); e != nil {
			return nil, assistance.OrdinaryRequest{}, e
		}
	}
	author := core.ID("")
	if kind == core.OrdinaryAppreciation || kind == core.OrdinaryMemory {
		author = "b"
		words := "Thank you for making tea."
		if kind == core.OrdinaryMemory {
			words = "We played chess after dinner."
		}
		story := core.OrdinaryStory{Version: core.OrdinaryVersion, Source: "story", Author: author, Activity: activity, Kind: kind, Origin: "participant_authored", Words: words}
		if e = l.PutStory(author, story, core.Public, []core.ID{"a"}, nil, 0); e != nil {
			return nil, assistance.OrdinaryRequest{}, e
		}
	}
	req := assistance.OrdinaryRequest{ID: "opportunity", Participants: []core.ID{"a", "b"}, Activity: activity, Kind: kind, Arm: "permitted_context", At: 1, Window: core.Interval{Start: 2, End: h.Decisions[0].Window.End}, Effort: map[core.ID]int64{"a": 1, "b": 1}}
	if kind == core.OrdinaryCoordination {
		req.Requested = true
		req.PlanDecision, req.PlanOption = "ordinary-plan", "slot"
	}
	req, e = l.Request("a", req, author)
	return l, req, e
}
