// Package helperexperiment composes a small offline experiment; labels and private
// simulated states never enter the reusable helper host.
package helperexperiment

import (
	"context"
	"fmt"
	"time"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/examples/assistanceclient"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/drives"
	"github.com/tushardhara/dream/simulator/dynamics"
)

type steps struct {
	local   *assistanceclient.Local
	request assistance.Request
	host    assistance.Host
}

func (s *steps) Step(ctx context.Context, i int, at core.LogicalTime, seed uint64, record *assistance.Interaction) (assistance.Interaction, error) {
	r := s.request
	r.ID = core.ID(fmt.Sprintf("helper-%d", i))
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
func World() hws.AssistanceWorld {
	a, _ := behavior.NewActionActor("alice", 0)
	b, _ := behavior.NewActionActor("bob", 0)
	w := hws.AssistanceWorld{Actors: []behavior.ActionActor{a, b}}
	kinds := []behavior.Kind{behavior.Say, behavior.Argue, behavior.Help, behavior.Wait}
	for i := 0; i < 8; i++ {
		actor, other := core.ID("alice"), core.ID("bob")
		if i%2 == 1 {
			actor, other = other, actor
		}
		at := core.LogicalTime(2 + i*4)
		id := core.ID(fmt.Sprintf("human-event-%d", i))
		rights := core.Rights{Resource: id, Grants: []core.Grant{{Actor: actor, Recipient: actor, Purpose: "simulation", Operation: core.Read}, {Actor: actor, Recipient: actor, Purpose: "simulation", Operation: core.Derive}}}
		p := dynamics.Perceived{Rights: rights, Event: id, Actor: actor, OccurredAt: at, LearnedAt: at, Confidence: 1, Signals: dynamics.Signals{Opportunity: .5, Support: .2}}
		observation := drives.Observation{Event: p}
		for j := range observation.Context {
			observation.Context[j] = drives.Cue{Evidence: dynamics.Perceived{Rights: rights, Event: id, Actor: actor, OccurredAt: at, LearnedAt: at}}
		}
		kind := kinds[(i/2)%len(kinds)]
		offer := behavior.ActionOffer{Kind: kind, Duration: 1}
		if kind != behavior.Wait {
			offer.Recipient = other
			offer.Evidence = []core.ID{id}
		}
		if kind == behavior.Help {
			offer.Resource = "time"
			offer.Units = 1
		}
		situation := behavior.ActionSituation{Observation: observation, Sources: []core.ID{id}, Offers: []behavior.ActionOffer{offer}, Present: []core.ID{actor, other}, Resources: map[core.ID]int64{"time": 10}, Horizon: at + 2}
		w.Frames = append(w.Frames, hws.HumanFrame{Actor: actor, At: at, Situation: situation})
	}
	return w
}
func Run(ctx context.Context, arm assistance.Arm, goal assistance.Goal, seed uint64, recorded *hws.AssistanceRun) (hws.AssistanceRun, error) {
	local, r := assistanceclient.Fixture(arm, goal)
	step := &steps{local: local, request: r, host: local.Host(assistance.FakePlanner{})}
	world := World()
	manifest := hws.NewAssistanceManifest(world, arm, seed)
	return hws.RunAssistance(ctx, world, manifest, step, recorded)
}
