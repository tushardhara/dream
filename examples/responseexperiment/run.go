// Package responseexperiment composes the real graph/helper host and the native
// domain/recipient policies. All inputs are fictional, offline engineering cases.
package responseexperiment

import (
	"context"
	"fmt"
	"time"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/examples/assistanceclient"
	"github.com/tushardhara/dream/examples/helperexperiment"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/dynamics"
)

type Scene struct {
	Expectation           float64 // Bob's private own expectation; never supplied to the helper
	Observation           string
	ShareReport, Followup bool
	PrivateFear           float64
}
type step struct {
	local   *assistanceclient.Local
	request assistance.Request
	host    assistance.Host
}

func (s *step) Step(ctx context.Context, i int, at core.LogicalTime, seed uint64, record *assistance.Interaction) (assistance.Interaction, error) {
	r := s.request
	r.ID = core.ID(fmt.Sprintf("response-helper-%d", i))
	r.At = at
	r.Seed = seed
	r.Contexts = append(r.Contexts[:0:0], r.Contexts...)
	for j := range r.Contexts {
		r.Contexts[j].Binding = r.ID
		r.Contexts[j].Query.KnownAt = at
		r.Contexts[j].Query.ValidAt = at
		r.Contexts[j].Query.RecordedAsOf = time.Unix(1000, 0).UTC()
	}
	if e := s.local.Register(r); e != nil {
		return assistance.Interaction{}, e
	}
	return s.host.Execute(ctx, r, record)
}
func preference(owner, other core.ID, class core.InteractionClass) core.Boundary {
	id := core.ID(string(owner) + "-response-willing-" + string(class))
	return core.Boundary{Version: core.BoundaryVersion, Meta: core.Metadata{ID: id, Observer: owner, Source: owner, Sensitivity: core.Restricted, Confidence: 1, Supporting: []core.ID{"synthetic-participation"}, RecordedAt: time.Unix(1, 0).UTC(), Rights: core.Rights{Resource: id}}, Principal: owner, With: other, Topic: core.ID(core.PracticalCoordination), Class: class, Decision: core.Willing, Basis: "self_report", OccurredAt: 1, LearnedAt: 1}
}
func World(scene Scene) hws.ResponsiveAssistanceWorld {
	w := hws.ResponsiveAssistanceWorld{}
	for _, owner := range []core.ID{"alice", "bob"} {
		a, _ := behavior.NewDomainActor(owner, 0)
		if owner == "bob" {
			a.Human.Human.Private.Fear = scene.PrivateFear
		}
		w.Actors = append(w.Actors, a)
	}
	base := helperexperiment.World()
	for i := 0; i < 16; i++ {
		f := base.Frames[i%len(base.Frames)]
		at := core.LogicalTime(2 + i*4)
		owner := f.Actor
		other := core.ID("bob")
		if owner == "bob" {
			other = "alice"
		}
		s := f.Situation
		event := s.Observation.Event
		event.Event = core.ID(fmt.Sprintf("response-native-%d", i))
		event.OccurredAt = at
		event.LearnedAt = at
		event.Rights.Resource = event.Event
		if owner == "bob" {
			event.Signals.StatusThreat = scene.PrivateFear
		}
		s.Observation.Event = event
		for j := range s.Observation.Context {
			s.Observation.Context[j].Evidence = event
			s.Observation.Context[j].Evidence.Confidence = 0
		}
		s.Sources = []core.ID{event.Event}
		s.Horizon = at + 3
		s.Offers = nil
		s.Resources = nil
		class := core.Coordination
		kinds := []behavior.Kind{behavior.Invite}
		if (i/2)%2 == 1 {
			class = core.Discussion
			kinds = []behavior.Kind{behavior.Say, behavior.Argue, behavior.Decline, behavior.Apologize}
		}
		for _, kind := range kinds {
			s.Offers = append(s.Offers, behavior.ActionOffer{Kind: kind, Recipient: other, Evidence: []core.ID{event.Event}, Duration: 1})
		}
		expectation := .8
		if owner == "bob" {
			expectation = scene.Expectation
		}
		profile := assistanceclient.DomainProfile(owner, other, core.ID(string(owner)+"-private-response"), core.PracticalCoordination, "everyday", .2, expectation)
		for _, id := range append(profile.Sources(), profile.Account) {
			s.Sources = append(s.Sources, id)
			s.RelationshipEvidence = append(s.RelationshipEvidence, dynamics.Perceived{Actor: owner, Event: id, OccurredAt: 1, LearnedAt: 1, Confidence: 1, Rights: core.Rights{Resource: id, Grants: []core.Grant{{Actor: owner, Recipient: owner, Purpose: "simulation", Operation: core.Read}, {Actor: owner, Recipient: owner, Purpose: "simulation", Operation: core.Derive}}}})
		}
		focus := core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: profile.Domain, RoleContext: profile.RoleContext, Account: profile.Account}
		scope := core.InteractionScope{Version: core.InteractionScopeVersion, Initiator: owner, Target: other, Topic: core.ID(profile.Domain), Class: class}
		input := behavior.DomainSituation{Focus: focus, Accounts: []core.RelationshipContext{profile}, Scoped: behavior.ScopedSituation{Scope: scope, Situation: s, Boundaries: []core.Boundary{preference(owner, other, class), preference(other, owner, class)}}}
		w.Frames = append(w.Frames, hws.ResponseFrame{Actor: owner, At: at, Situation: input, Observation: scene.Observation, Followup: scene.Followup, ShareReport: scene.ShareReport})
	}
	return w
}
func newStep(arm assistance.Arm) (*step, error) {
	// Public reports are fixed across private recipient conditions. The helper's
	// context never contains scene.Expectation, native private state or outcomes.
	focus := core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: core.PracticalCoordination, RoleContext: "everyday"}
	profiles := []core.RelationshipContext{assistanceclient.DomainProfile("alice", "bob", "alice-public", focus.Domain, focus.RoleContext, .5, .5), assistanceclient.DomainProfile("bob", "alice", "bob-public", focus.Domain, focus.RoleContext, .5, .5)}
	local, r, e := assistanceclient.DomainFixture(arm, focus, profiles)
	if e != nil {
		return nil, e
	}
	r.Scope.Class = core.Coordination
	for _, owner := range r.Participants {
		other := core.ID("alice")
		if owner == "alice" {
			other = "bob"
		}
		if e = local.AppendBoundary(owner, preference(owner, other, core.Coordination)); e != nil {
			return nil, e
		}
	}
	engine := &step{local: local, request: r, host: local.Host(assistance.FakePlanner{})}
	return engine, nil
}
func Run(ctx context.Context, scene Scene, arm assistance.Arm, seed uint64, recorded *hws.ResponsiveAssistanceRun) (hws.ResponsiveAssistanceRun, error) {
	engine, e := newStep(arm)
	if e != nil {
		return hws.ResponsiveAssistanceRun{}, e
	}
	world := World(scene)
	manifest := hws.NewResponsiveAssistanceManifest(world, arm, seed)
	return hws.RunResponsiveAssistance(ctx, world, manifest, engine, recorded)
}
