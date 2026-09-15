package hws

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/dynamics"
)

const ResponsiveAssistanceVersion = "helper-experiment.v2"

type ResponseFrame struct {
	Actor                 core.ID
	At                    core.LogicalTime
	Situation             behavior.DomainSituation
	Observation           string
	Followup, ShareReport bool // explicit simulator participant choices, never helper choices
}
type ResponsiveAssistanceWorld struct {
	Actors []behavior.DomainActor
	Frames []ResponseFrame
}
type ResponsiveAssistanceRun struct {
	Actions           []assistance.OutcomeAction
	Manifest          AssistanceManifest
	Humans            []behavior.DomainDecision
	Helper            []assistance.Interaction
	Responses         []behavior.RecipientDecision
	Outcomes          []core.OutcomeObservation
	Reports           []assistance.OutcomeReport
	Final             []behavior.DomainActor
	FirstIntervention int
}

func NewResponsiveAssistanceManifest(w ResponsiveAssistanceWorld, arm assistance.Arm, seed uint64) AssistanceManifest {
	return AssistanceManifest{Version: ResponsiveAssistanceVersion, WorldVersion: behavior.DomainPolicy, HelperVersion: assistance.DomainVersion, FixtureHash: assistance.Digest(w), Arm: arm, HumanSeed: streamDraw("human", seed, 0), ExogenousSeed: streamDraw("exogenous", seed, 0), HelperSeed: streamDraw("helper", seed, 0)}
}

type responseDelivery struct {
	Interaction, Action, Sender, Recipient core.ID
	Kind                                   behavior.Kind
	At                                     core.LogicalTime
}

func ownResponseLog(log []core.OutcomeObservation, owner core.ID) []core.OutcomeObservation {
	out := []core.OutcomeObservation{}
	for _, o := range log {
		if o.Meta.Observer == owner {
			out = append(out, o)
		}
	}
	return out
}
func appendResponseOutcome(out *ResponsiveAssistanceRun, o core.OutcomeObservation) error {
	if _, e := core.AppendOutcome(ownResponseLog(out.Outcomes, o.Meta.Observer), o); e != nil {
		return e
	}
	out.Outcomes = append(out.Outcomes, o)
	return nil
}
func responseProof(owner, id core.ID, at core.LogicalTime, confidence core.Confidence) dynamics.Perceived {
	return dynamics.Perceived{Actor: owner, Event: id, OccurredAt: at, LearnedAt: at, Confidence: confidence, Rights: core.Rights{Resource: id, Grants: []core.Grant{{Actor: owner, Recipient: owner, Purpose: "simulation", Operation: core.Read}, {Actor: owner, Recipient: owner, Purpose: "simulation", Operation: core.Derive}}}}
}
func responseProfile(f ResponseFrame) (core.RelationshipContext, error) {
	selected, e := core.SelectRelationship(f.Situation.Accounts, f.Situation.Focus, f.Actor, f.Situation.Scoped.Scope.Target, f.At)
	if e != nil || selected.Status != "selected" {
		return core.RelationshipContext{}, assistance.ErrInvalid
	}
	return *selected.Account, nil
}

// RunResponsiveAssistance keeps the helper's port free of human private state.
// All arms run the same domain human policy and recipient policy. Delivery only
// adds an optional affordance; separately observed experience trains later choices.
// Reporting is an explicit participant choice followed by a current data gate.
func RunResponsiveAssistance(ctx context.Context, world ResponsiveAssistanceWorld, m AssistanceManifest, helper AssistanceStep, recorded *ResponsiveAssistanceRun) (ResponsiveAssistanceRun, error) {
	if m.Version != ResponsiveAssistanceVersion || m.WorldVersion != behavior.DomainPolicy || m.HelperVersion != assistance.DomainVersion || !m.Arm.Valid() || m.FixtureHash != assistance.Digest(world) || len(world.Actors) < 2 || len(world.Actors) > 8 || len(world.Frames) < 1 || len(world.Frames) > 16 || helper == nil {
		return ResponsiveAssistanceRun{}, assistance.ErrInvalid
	}
	if recorded != nil && (assistance.Digest(recorded.Manifest) != assistance.Digest(m) || len(recorded.Helper) != len(world.Frames)) {
		return ResponsiveAssistanceRun{}, assistance.ErrInvalid
	}
	raw, _ := json.Marshal(world)
	var owned ResponsiveAssistanceWorld
	_ = json.Unmarshal(raw, &owned)
	indices := map[core.ID]int{}
	for i, a := range owned.Actors {
		owner := a.Human.Human.Drives.Actor
		if a.Validate() != nil {
			return ResponsiveAssistanceRun{}, assistance.ErrInvalid
		}
		if _, ok := indices[owner]; ok {
			return ResponsiveAssistanceRun{}, assistance.ErrInvalid
		}
		indices[owner] = i
	}
	out := ResponsiveAssistanceRun{Manifest: m, FirstIntervention: -1}
	pending := []responseDelivery{}
	later := map[core.ID]responseDelivery{}
	proofs := map[core.ID][]dynamics.Perceived{}
	advice := map[core.ID]assistance.Interaction{}
	previous := core.LogicalTime(0)
	for i, f := range owned.Frames {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		index, ok := indices[f.Actor]
		if !ok || f.At < previous || f.Situation.Scoped.Scope.Initiator != f.Actor {
			return out, assistance.ErrInvalid
		}
		previous = f.At
		profile, e := responseProfile(f)
		if e != nil {
			return out, e
		}
		s := f.Situation
		current := append(append([]dynamics.Perceived{}, s.Scoped.Situation.RelationshipEvidence...), proofs[f.Actor]...)
		actor, e := behavior.LearnDomainObservations(owned.Actors[index], ownResponseLog(out.Outcomes, f.Actor), profile, f.At, current)
		if e != nil {
			return out, e
		}
		for _, bucket := range actor.Memories {
			if bucket.Domain != profile.Domain || bucket.RoleContext != profile.RoleContext {
				continue
			}
			for _, memory := range bucket.Memory {
				for _, id := range memory.Evidence {
					for _, p := range proofs[f.Actor] {
						if p.Event == id {
							s.Scoped.Situation.Sources = append(s.Scoped.Situation.Sources, id)
							s.Scoped.Situation.RelationshipEvidence = append(s.Scoped.Situation.RelationshipEvidence, p)
						}
					}
				}
			}
		}
		var incoming *responseDelivery
		for j, d := range pending {
			if d.Recipient == f.Actor && d.At <= f.At {
				copy := d
				incoming = &copy
				pending = append(pending[:j], pending[j+1:]...)
				break
			}
		}
		var suggestion *assistance.Interaction
		if h, ok := advice[f.Actor]; ok && h.At < f.At && h.Scope.Target == s.Scoped.Scope.Target && h.Scope.Class == core.Coordination && h.Scope.Topic == s.Scoped.Scope.Topic && h.Focus.Domain == profile.Domain && h.Focus.RoleContext == profile.RoleContext && s.Scoped.Scope.Class == core.Coordination {
			copy := h
			suggestion = &copy
			s.Scoped.Situation.Offers = append(s.Scoped.Situation.Offers, behavior.ActionOffer{Kind: behavior.Coordinate, Recipient: h.Scope.Target, Evidence: []core.ID{s.Scoped.Situation.Observation.Event.Event}, Duration: 1})
			if out.FirstIntervention < 0 {
				out.FirstIntervention = i
			}
			delete(advice, f.Actor)
		}

		next, d, e := behavior.ChooseDomainAction(actor, s, f.At, streamDraw("human-choice.v2", m.HumanSeed, i))
		if e != nil {
			return out, e
		}
		out.Humans = append(out.Humans, d)
		owned.Actors[index] = next
		human := d.Human.Human
		selected := human.Candidates[human.Selected].Offer
		respond := func(delivery responseDelivery, phase string) error {
			contextInput := f.Situation.Scoped.Situation
			if delivery.Sender == profile.Other {
				var err error
				contextInput, err = behavior.ApplyDomainRelationship(contextInput, profile, f.Actor, f.At)
				if err != nil {
					return err
				}
			}
			appraisal := behavior.DisclosureContext{Observer: f.Actor, Recipient: delivery.Sender}
			for _, c := range contextInput.Contexts {
				if c.Recipient == delivery.Sender {
					appraisal = c
				}
			}
			availability := f.Observation
			if phase == "later" && !f.Followup {
				availability = "missing_followup"
			}
			evidence := responseProof(f.Actor, core.ID("received:"+string(delivery.Action)+":"+phase), f.At, .8)
			focus := core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: profile.Domain, RoleContext: profile.RoleContext, Account: profile.Account}
			input := behavior.RecipientInput{Delivery: behavior.ReceivedAction{Interaction: delivery.Interaction, Decision: delivery.Action, Sender: delivery.Sender, Recipient: f.Actor, Kind: core.ID(delivery.Kind), EffectAt: delivery.At, Source: evidence}, Focus: focus, Context: appraisal, Current: contextInput.RelationshipEvidence, Availability: availability, Phase: phase}
			rd, err := behavior.RespondToAction(next.Human.Human, input, f.At, streamDraw("recipient.v1/"+string(delivery.Action)+"/"+phase, m.HumanSeed, i))
			if err != nil {
				return err
			}
			if phase == "immediate" && selected.Delivered() && selected.Recipient == delivery.Sender {
				rd.Observation.Reply = human.ID
			}
			if f.ShareReport {
				for _, h := range out.Helper {
					if h.ID == delivery.Interaction {
						rd.Observation.Meta.Rights.Grants = append(rd.Observation.Meta.Rights.Grants, core.Grant{Actor: h.Helper, Recipient: h.Helper, Purpose: "help", Operation: core.Read})
					}
				}
			}
			if err = appendResponseOutcome(&out, rd.Observation); err != nil {
				return err
			}
			out.Responses = append(out.Responses, rd)
			proofs[f.Actor] = append(proofs[f.Actor], evidence, responseProof(f.Actor, rd.Observation.Meta.ID, f.At, rd.Observation.Meta.Confidence))
			return nil
		}
		if old, ok := later[f.Actor]; ok {
			if e = respond(old, "later"); e != nil {
				return out, e
			}
			delete(later, f.Actor)
		}
		if incoming != nil {
			if e = respond(*incoming, "immediate"); e != nil {
				return out, e
			}
			later[f.Actor] = *incoming
		}
		if selected.Delivered() {
			interactionID := core.ID("native-interaction:" + string(human.ID))
			if suggestion != nil && selected.Kind == behavior.Coordinate && selected.Recipient == suggestion.Scope.Target {
				interactionID = suggestion.ID
				out.Actions = append(out.Actions, assistance.OutcomeAction{Version: assistance.OutcomeReportVersion, Interaction: interactionID, Action: human.ID, Sender: f.Actor, Recipient: selected.Recipient, At: f.At})
			}
			pending = append(pending, responseDelivery{Interaction: interactionID, Action: human.ID, Sender: f.Actor, Recipient: selected.Recipient, Kind: selected.Kind, At: f.At + selected.Duration})
			id := core.ID("expected:" + string(human.ID))
			value := core.OutcomeObservation{Version: core.OutcomeObservationVersion, Meta: core.Metadata{ID: id, Observer: f.Actor, Source: f.Actor, Sensitivity: core.Restricted, Confidence: .4, RecordedAt: time.Unix(1, 0).UTC(), Supporting: []core.ID{f.Situation.Scoped.Situation.Observation.Event.Event}, Rights: responseProof(f.Actor, id, f.At, .4).Rights}, Interaction: interactionID, Action: human.ID, Participant: f.Actor, Other: selected.Recipient, Focus: core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: profile.Domain, RoleContext: profile.RoleContext, Account: profile.Account}, Kind: "expected", Position: "sender", Phase: "immediate", Basis: "self_report", OccurredAt: f.At, LearnedAt: f.At, Status: core.Observed, Appraisal: "unresolved"}
			if next.Human.Human.Private.Appraisal[1] > next.Human.Human.Private.Appraisal[0] {
				b, c := .4, .1
				value.Appraisal = "supportive"
				value.Benefit = &b
				value.Burden = &c
			}
			if f.ShareReport && suggestion != nil && interactionID == suggestion.ID {
				value.Meta.Rights.Grants = append(value.Meta.Rights.Grants, core.Grant{Actor: suggestion.Helper, Recipient: suggestion.Helper, Purpose: "help", Operation: core.Read})
			}
			if e = appendResponseOutcome(&out, value); e != nil {
				return out, e
			}
		}

		var record *assistance.Interaction
		if recorded != nil {
			record = &recorded.Helper[i]
		}
		interaction, e := helper.Step(ctx, i, f.At, streamDraw("helper-choice.v2", m.HelperSeed, i), record)
		if e != nil {
			return out, e
		}
		if interaction.Version != m.HelperVersion || interaction.Arm != m.Arm || interaction.At != f.At || interaction.Seed != streamDraw("helper-choice.v2", m.HelperSeed, i) || len(interaction.Result.Candidates) == 0 || interaction.Result.Selected < 0 || interaction.Result.Selected >= len(interaction.Result.Candidates) {
			return out, assistance.ErrInvalid
		}
		if _, ok := indices[interaction.Helper]; ok {
			return out, assistance.ErrInvalid
		}
		if _, ok := indices[interaction.User]; !ok {
			return out, assistance.ErrInvalid
		}
		choice := interaction.Result.Candidates[interaction.Result.Selected]
		if m.Arm == assistance.None && (interaction.Delivered || choice.Action != assistance.Wait) {
			return out, assistance.ErrInvalid
		}
		if interaction.Delivered && choice.Action == assistance.Propose {
			if choice.Recipient != interaction.User || interaction.Scope == nil || interaction.Scope.Initiator != interaction.User || interaction.Focus == nil {
				return out, assistance.ErrInvalid
			}
			if _, ok := indices[interaction.Scope.Target]; !ok {
				return out, assistance.ErrInvalid
			}
			advice[interaction.User] = interaction
		}

		out.Helper = append(out.Helper, interaction)
	}
	for _, action := range out.Actions {
		for _, h := range out.Helper {
			if h.ID != action.Interaction {
				continue
			}
			reports, e := assistance.OutcomeReports(h, action, out.Outcomes, h.Helper, previous)
			if e != nil {
				return out, e
			}
			out.Reports = append(out.Reports, reports...)
		}
	}

	out.Final = owned.Actors
	if recorded != nil && assistance.Digest(out) != assistance.Digest(*recorded) {
		return out, fmt.Errorf("responsive helper replay changed")
	}
	return out, nil
}
