// Package temporalexperiment runs bounded, offline, synthetic temporal controls.
// Research labels are scored after execution and never enter either consumer.
package temporalexperiment

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/examples/assistanceclient"
	"github.com/tushardhara/dream/examples/helperexperiment"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/drives"
	"github.com/tushardhara/dream/simulator/dynamics"
	"github.com/tushardhara/dream/simulator/scenario"
)

const Version = "temporal-experiment.v1"

type Scene struct {
	Name                      string
	Gap                       core.LogicalTime
	Upper                     [2]core.LogicalTime
	Life                      string
	Stale, Sparse, Unobserved bool
	Boundary                  core.Willingness
}

func Scenes() []Scene {
	return []Scene{
		{Name: "daily_2", Gap: 2, Upper: [2]core.LogicalTime{3, 3}},
		{Name: "daily_60", Gap: 60, Upper: [2]core.LogicalTime{3, 3}},
		{Name: "occasional_2", Gap: 2, Upper: [2]core.LogicalTime{90, 90}},
		{Name: "occasional_60", Gap: 60, Upper: [2]core.LogicalTime{90, 90}},
		{Name: "caregiving", Gap: 60, Upper: [2]core.LogicalTime{3, 3}, Life: "busy"},
		{Name: "work_transition", Gap: 60, Upper: [2]core.LogicalTime{90, 90}, Life: "routine_changed"},
		{Name: "stale_experience", Gap: 60, Upper: [2]core.LogicalTime{90, 90}, Life: "available", Stale: true},
		{Name: "agreed_break", Gap: 60, Upper: [2]core.LogicalTime{3, 3}, Boundary: core.TakingBreak},
		{Name: "sparse", Gap: 60, Upper: [2]core.LogicalTime{3, 3}, Sparse: true},
		{Name: "unobserved_channel", Gap: 60, Upper: [2]core.LogicalTime{3, 3}, Unobserved: true},
		{Name: "observer_disagreement", Gap: 60, Upper: [2]core.LogicalTime{3, 90}},
		{Name: "no_contact", Gap: 60, Upper: [2]core.LogicalTime{3, 3}, Boundary: core.Ended},
	}
}

// World uses the scenario migration and actor projections. Genesis contains only
// day-zero statements; later contacts/life reports are explicit scheduled inputs.
// One synthetic tick is one logical day in this opt-in experiment.
func World(s Scene, age int, seed uint64, hidden string) (scenario.Scenario, error) {
	w := scenario.Scenario{Version: scenario.Version, World: scenario.World{ID: "temporal-world", Seed: seed, Horizon: 3 + s.Gap + 3}, Public: scenario.Public{Humans: []scenario.Human{{ID: "alice", Name: "Synthetic Alice", Age: 35}, {ID: "bob", Name: "Synthetic Bob", Age: 35}}}, Research: scenario.Research{Labels: []scenario.Label{{ID: "research-label", Text: "RESEARCH_CANARY"}}}}
	contexts := map[core.ID][]core.TemporalFact{}
	for i, owner := range []core.ID{"alice", "bob"} {
		other := core.ID("alice")
		if owner == "alice" {
			other = "bob"
		}
		source := core.ID(string(owner) + "-temporal-source")
		f := scenario.Fact{ID: source, Observer: owner, Subject: owner, Text: "Voluntary synthetic context", Confidence: 1, Grants: []core.Grant{{Actor: owner, Recipient: owner, Purpose: "simulation", Operation: core.Read}}}
		w.Actors = append(w.Actors, scenario.Actor{ID: owner, Facts: []scenario.Fact{f}, Knowledge: []scenario.Knowledge{{Record: source, LearnedAt: 0}}})
		facts := assistanceclient.ContactFacts(owner, other, 0, s.Upper[i])
		facts[1].Observations = []core.LogicalTime{0}
		contexts[owner] = facts
		for day := core.LogicalTime(1); day <= 3; day++ {
			w.Future = append(w.Future, scenario.Scheduled{ID: core.ID(fmt.Sprintf("%s-contact-%d", owner, day)), At: day, Kind: "observation", Actor: owner, Text: "observed_chat_contact"})
		}
		if s.Life != "" {
			confirmed := core.LogicalTime(2 + s.Gap)
			if s.Stale {
				confirmed = 0
			}
			life := assistanceclient.LifeFact(owner, other, s.Life, confirmed, 30)
			if s.Life == "routine_changed" {
				life.Category = "transition"
			}
			if s.Stale {
				life.Category = "experience"
				contexts[owner] = append(contexts[owner], life)
			} else {
				encoded, e := life.Encode()
				if e != nil {
					return w, e
				}
				w.Future = append(w.Future, scenario.Scheduled{ID: core.ID(string(owner) + "-life-event"), At: confirmed, Kind: "observation", Actor: owner, Text: string(encoded)})
			}
		}
		if age != 0 {
			w.Public.Humans[i].Age = age
		}
	}
	// An unobserved phone channel is research-only: changing its text cannot be
	// used as evidence of either contact or absence in the permitted chat diary.
	w.Future = append(w.Future, scenario.Scheduled{ID: "hidden-phone", At: 3 + s.Gap, Kind: "observation", Actor: "bob", Text: "PRIVATE_PHONE_" + hidden})
	unknown := []core.ID{}
	if age == 0 {
		unknown = []core.ID{"alice", "bob"}
	}
	return scenario.WithTemporalContexts(w, contexts, unknown)
}
func snapshots(w scenario.Scenario, s Scene) ([]core.TemporalFact, error) {
	out := []core.TemporalFact{}
	for _, owner := range []core.ID{"alice", "bob"} {
		view, e := w.View(owner)
		if e != nil {
			return nil, e
		}
		facts := view.Temporal
		diary := -1
		for i, f := range facts {
			if f.Kind == "contacts" {
				diary = i
			}
		}
		if diary < 0 {
			return nil, fmt.Errorf("missing authored diary")
		}
		for _, event := range w.Future {
			if event.Actor != owner || event.At > 3+s.Gap {
				continue
			}
			if event.Text == "observed_chat_contact" {
				facts[diary].Observations = append(facts[diary].Observations, event.At)
			} else if strings.HasPrefix(event.Text, "{") {
				var f core.TemporalFact
				if json.Unmarshal([]byte(event.Text), &f) != nil || f.Observer != owner || f.ConfirmedAt != event.At || f.Validate() != nil {
					return nil, fmt.Errorf("invalid scheduled report")
				}
				facts = append(facts, f)
			}
		}
		// This is a new, explicitly authored coverage statement at the query time,
		// not extrapolation from the last observed contact or an old source.
		facts[diary].ObservedThrough = 3 + s.Gap
		facts[diary].CompleteChannel = !s.Sparse && !s.Unobserved
		if s.Sparse {
			facts[diary].Observations = []core.LogicalTime{3}
		}
		out = append(out, facts...)
	}
	return out, nil
}

type HumanTrace struct {
	Actor             core.ID
	Decision          behavior.TemporalDecision
	ContextOff        behavior.DomainDecision
	Final             behavior.TemporalActor
	InputHash         string
	PrivateViolations int
}
type Trace struct {
	Version, Scene, Policy                              string
	Seed                                                uint64
	Helper                                              assistance.Interaction
	Humans                                              []HumanTrace
	PlannerCalls, PrivacyViolations, BoundaryViolations int
}
type capture struct{ Calls, Privacy int }

func (c *capture) Plan(ctx context.Context, in assistance.Input) (assistance.Result, error) {
	c.Calls++
	raw, _ := json.Marshal(in)
	for _, canary := range []string{"PRIVATE_PHONE_", "RESEARCH_CANARY"} {
		if strings.Contains(string(raw), canary) {
			c.Privacy++
		}
	}
	return (assistance.FakePlanner{}).Plan(ctx, in)
}
func Run(ctx context.Context, s Scene, policy string, age int, seed uint64, hidden string, recorded *Trace) (Trace, error) {
	w, e := World(s, age, seed, hidden)
	if e != nil {
		return Trace{}, e
	}
	engine := scenario.Engine{ScenarioVersion: 1, Capabilities: []core.ID{"genesis.v1", "schedule.v1", "temporal-context.v1"}}
	if _, e = w.Genesis(engine); e != nil {
		return Trace{}, e
	}
	facts, e := snapshots(w, s)
	if e != nil {
		return Trace{}, e
	}
	arm := assistance.Multi
	if policy == "simple" {
		arm = assistance.Simple
	} else if policy != "temporal" && policy != "context_off" {
		return Trace{}, fmt.Errorf("unknown policy")
	}
	at := 3 + s.Gap
	local, r, e := assistanceclient.TemporalFixture(assistance.Multi, at, "chat", facts)
	if e != nil {
		return Trace{}, e
	}
	humanRequest := r
	r.Arm = arm
	if arm == assistance.Simple {
		r.Contexts = nil
	}
	// Preserve the same evidence and human policy across helper arms. The v3
	// baseline removes only the helper's temporal interpretation/projection version.
	if policy == "context_off" {
		r.Version = assistance.DomainVersion
		r.Temporal = nil
		for i := range r.Contexts {
			r.Contexts[i].Version = 1
		}

	}
	r.Seed = seed
	r.ID = core.ID(fmt.Sprintf("temporal-%s-%d", policy, seed))
	for i := range r.Contexts {
		r.Contexts[i].Binding = r.ID
	}
	if e = local.Register(r); e != nil {
		return Trace{}, e
	}
	boundaries := []core.Boundary{}
	for _, owner := range []core.ID{"alice", "bob"} {
		other := core.ID("alice")
		if owner == "alice" {
			other = "bob"
		}
		id := core.ID(string(owner) + "-willing")
		decision := s.Boundary
		if decision == "" {
			decision = core.Willing
		}
		b := core.Boundary{Version: core.BoundaryVersion, Meta: core.Metadata{ID: id, Observer: owner, Source: owner, Sensitivity: core.Restricted, Confidence: 1, Supporting: []core.ID{"synthetic-choice"}, RecordedAt: time.Unix(1, 0).UTC(), Rights: core.Rights{Resource: id}}, Principal: owner, With: other, Topic: r.Scope.Topic, Class: core.Clarification, Decision: decision, Basis: "self_report", OccurredAt: 1, LearnedAt: 1}
		if decision == core.TakingBreak {
			end := at + 10
			b.Meta.Valid.End = &end
		}
		boundaries = append(boundaries, b)
		if e = local.AppendBoundary(owner, b); e != nil {
			return Trace{}, e
		}
	}
	planner := &capture{}
	var prior *assistance.Interaction
	if recorded != nil {
		prior = &recorded.Helper
	}
	helper, e := local.Host(planner).Execute(ctx, r, prior)
	if e != nil {
		return Trace{}, e
	}
	out := Trace{Version: Version, Scene: s.Name, Policy: policy, Seed: seed, Helper: helper, PlannerCalls: planner.Calls, PrivacyViolations: planner.Privacy}
	// Counting violations examines actual emitted actions against independently
	// authored boundaries; it is not a constant assigned to a successful run.
	if s.Boundary != "" && helper.Delivered {
		out.BoundaryViolations++
	}
	for i, owner := range []core.ID{"alice", "bob"} {
		human, e := humanInput(ctx, local, humanRequest, boundaries, owner, i)
		if e != nil {
			return Trace{}, e
		}
		actor, e := behavior.NewTemporalActor(owner, 0)
		if e != nil {
			return Trace{}, e
		}
		sum := sha256.Sum256([]byte(fmt.Sprintf("temporal-draw.v1/%d/%s", seed, owner)))
		draw := binary.BigEndian.Uint64(sum[:8])
		final, d, e := behavior.ChooseTemporalAction(actor, human, at, draw)
		if e != nil {
			return Trace{}, e
		}
		_, off, e := behavior.ChooseDomainAction(actor.Human, human.Domain, at, draw)
		if e != nil {
			return Trace{}, e
		}
		trace := HumanTrace{Actor: owner, Decision: d, ContextOff: off, Final: final, InputHash: assistance.Digest(human)}
		for _, p := range human.Domain.Scoped.Situation.RelationshipEvidence {
			if p.Actor != owner {
				trace.PrivateViolations++
			}
		}
		if s.Boundary != "" && d.Human.Human.Human.Candidates[d.Human.Human.Human.Selected].Offer.Kind == behavior.Ask {
			out.BoundaryViolations++
		}
		out.PrivacyViolations += trace.PrivateViolations
		out.Humans = append(out.Humans, trace)
	}
	if recorded != nil {
		// Recorded execution correctly skips planner generation; compare the actual
		// interaction and human results, not the live diagnostic call count.
		out.PlannerCalls = recorded.PlannerCalls
		if assistance.Digest(out) != assistance.Digest(*recorded) {
			return Trace{}, fmt.Errorf("temporal replay changed")
		}
	}
	return out, nil
}

func humanInput(ctx context.Context, local *assistanceclient.Local, r assistance.Request, boundaries []core.Boundary, owner core.ID, index int) (behavior.TemporalSituation, error) {
	other := core.ID("alice")
	if owner == "alice" {
		other = "bob"
	}
	q := r.Contexts[index].Query
	q.Actor = owner
	q.Purpose = "simulation"
	selected, e := (graph.MemoryService{Journal: local}).Retrieve(ctx, q)
	if e != nil {
		return behavior.TemporalSituation{}, e
	}
	frame := helperexperiment.World().Frames[index]
	input := behavior.TemporalSituation{Reporters: map[core.ID]core.ID{}, Focus: core.TemporalFocus{Version: core.TemporalFocusVersion, Observer: owner, Other: other, Channel: "chat"}, Domain: behavior.DomainSituation{Focus: *r.Focus, Scoped: behavior.ScopedSituation{Scope: *r.Scope, Boundaries: boundaries, Situation: frame.Situation}}}
	input.Domain.Scoped.Scope.Initiator = owner
	input.Domain.Scoped.Scope.Target = other
	p := input.Domain.Scoped.Situation.Observation.Event
	p.OccurredAt = r.At
	p.LearnedAt = r.At
	input.Domain.Scoped.Situation.Observation.Event = p
	for j := range input.Domain.Scoped.Situation.Observation.Context {
		cue := p
		cue.Confidence = 0
		input.Domain.Scoped.Situation.Observation.Context[j] = drives.Cue{Evidence: cue}
	}
	input.Domain.Scoped.Situation.Horizon = r.At + 3
	input.Domain.Scoped.Situation.Offers = []behavior.ActionOffer{{Kind: behavior.Ask, Recipient: other, Duration: 1, Evidence: []core.ID{p.Event}}}
	records := map[core.ID]graph.MemoryRecord{}
	perceived := map[core.ID]dynamics.Perceived{}
	for _, pick := range selected {
		rec := pick.Record
		learned := core.LogicalTime(-1)
		for _, v := range rec.Content.Learned {
			if v.Actor == owner {
				learned = v.At
			}
		}
		meta := dynamics.Perceived{Event: rec.Event.Meta.ID, Actor: rec.Event.Meta.Observer, OccurredAt: rec.Event.OccurredAt, LearnedAt: learned, Confidence: rec.Event.Meta.Confidence, Rights: rec.Event.Meta.Rights}
		input.Domain.Scoped.Situation.Sources = append(input.Domain.Scoped.Situation.Sources, meta.Event)
		input.Domain.Scoped.Situation.RelationshipEvidence = append(input.Domain.Scoped.Situation.RelationshipEvidence, meta)
		records[meta.Event] = rec
		perceived[meta.Event] = meta
		input.Reporters[meta.Event] = rec.Event.Meta.Source
		if rec.Content.Kind == graph.RelationshipMemory {
			v, e := graph.DecodeRelation(rec)
			if e != nil {
				return input, e
			}
			input.Domain.Accounts = append(input.Domain.Accounts, *v.Context)
		}
	}
	for _, pick := range selected {
		rec := pick.Record
		if !strings.HasPrefix(rec.Content.Text, "temporal.v1:") {
			continue
		}
		f, e := graph.DecodeTemporal(rec)
		if e != nil {
			return input, e
		}
		p := perceived[f.Account]
		source, ok := perceived[f.Source]
		if !ok {
			return input, fmt.Errorf("unavailable temporal source")
		}
		input.Evidence = append(input.Evidence, core.TemporalEvidence{Fact: f, Reporter: rec.Event.Meta.Source, SourceReporter: records[f.Source].Event.Meta.Source, SourceOccurredAt: source.OccurredAt, SourceLearnedAt: source.LearnedAt, OccurredAt: p.OccurredAt, LearnedAt: p.LearnedAt, Valid: rec.Event.Meta.Valid, Confidence: p.Confidence, SourceConfidence: source.Confidence})
	}
	return input, nil
}
