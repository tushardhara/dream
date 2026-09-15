package behavior

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/dynamics"
)

const RecipientPolicy = "recipient-response.v1"

// ReceivedAction is an observed delivery, not evidence that it was welcome.
// Source is the recipient's permitted observation of this specific action.
type ReceivedAction struct {
	Interaction, Decision, Sender, Recipient core.ID
	Kind                                     core.ID
	EffectAt                                 core.LogicalTime
	Source                                   dynamics.Perceived
}
type RecipientInput struct {
	Delivery     ReceivedAction
	Focus        core.RelationshipFocus
	Context      DisclosureContext
	Current      []dynamics.Perceived
	Availability string // observed or an explicit missing-observation reason
	Phase        string
}
type RecipientDecision struct {
	Version, InputHash string
	Draw               uint64
	Probabilities      [5]float64
	Observation        core.OutcomeObservation
}

// RespondToAction generates an observer-owned synthetic appraisal from this
// recipient's context and private native state. No delivery kind receives a
// positive label or a prescribed positive/negative quota. Nothing is shared here.
func RespondToAction(a ActionActor, in RecipientInput, at core.LogicalTime, draw uint64) (RecipientDecision, error) {
	d := in.Delivery
	owner := a.Drives.Actor
	if a.Validate() != nil || d.Interaction.Validate() != nil || d.Decision.Validate() != nil || d.Kind.Validate() != nil || d.Sender.Validate() != nil || d.Recipient != owner || d.Sender == owner || d.EffectAt < 0 || at < d.EffectAt || in.Focus.Validate() != nil || in.Focus.RoleContext == "" || in.Phase != "immediate" && in.Phase != "later" {
		return RecipientDecision{}, fmt.Errorf("invalid recipient response")
	}
	id := core.ID("response:" + scopedHash(struct {
		Actor ActionActor
		Input RecipientInput
		At    core.LogicalTime
		Draw  uint64
	}{a, in, at, draw})[:32])
	o := core.OutcomeObservation{Version: core.OutcomeObservationVersion, Meta: core.Metadata{ID: id, Observer: owner, Source: owner, Sensitivity: core.Restricted, RecordedAt: time.Unix(1, 0).UTC(), Rights: core.Rights{Resource: id, Grants: []core.Grant{{Actor: owner, Recipient: owner, Purpose: "simulation", Operation: core.Read}, {Actor: owner, Recipient: owner, Purpose: "simulation", Operation: core.Derive}}}}, Interaction: d.Interaction, Action: d.Decision, Participant: owner, Other: d.Sender, Focus: in.Focus, Kind: "observed", Position: "recipient", Phase: in.Phase, Basis: "unobserved", OccurredAt: at, LearnedAt: at, Status: core.Unknown}
	result := RecipientDecision{Version: RecipientPolicy, InputHash: scopedHash(struct {
		Actor ActionActor
		Input RecipientInput
	}{a, in}), Draw: draw}
	if in.Availability != "observed" {
		o.Missing = in.Availability
		if in.Availability == "declined_participation" || in.Availability == "missing_followup" {
			o.Status = core.Censored
		}
		result.Observation = o
		return result, o.Validate()
	}
	if d.Source.Validate(owner, at) != nil || d.Source.OccurredAt < d.EffectAt || in.Context.Observer != owner || in.Context.Recipient != d.Sender || len(in.Current) > 16 {
		return RecipientDecision{}, fmt.Errorf("unavailable recipient observation")
	}
	sources := map[core.ID]bool{}
	metadata := map[core.ID]dynamics.Perceived{}
	for _, p := range in.Current {
		if p.Validate(owner, at) != nil || sources[p.Event] {
			return RecipientDecision{}, fmt.Errorf("invalid recipient context evidence")
		}
		sources[p.Event] = true
		metadata[p.Event] = p
	}
	for _, v := range in.Context.values() {
		if !v.validate(sources) {
			return RecipientDecision{}, fmt.Errorf("unattributed recipient context")
		}
	}
	weighted := func(v ContextValue) float64 { return v.weighted() * float64(metadata[v.Evidence].Confidence) }
	confidence := float64(d.Source.Confidence) * float64(in.Context.RoleExpectation.Confidence) * float64(metadata[in.Context.RoleExpectation.Evidence].Confidence)
	readiness := bounded(.55*weighted(in.Context.RoleExpectation) + .2*weighted(in.Context.Trust) + .15*weighted(in.Context.ExpectedReaction) - .35*weighted(in.Context.Stress) - .2*weighted(in.Context.Fear) + .15*a.Private.Emotion[0] - .3*a.Private.Fear - .2*a.Private.Appraisal[0])
	conflict := math.Abs(weighted(in.Context.RoleExpectation)-weighted(in.Context.Trust)) / 2
	weights := [5]float64{.05 + .7*math.Max(0, readiness), .05 + .7*math.Max(0, -readiness), .2 * (1 - math.Abs(readiness)), .05 + .3*conflict, .05 + .5*(1-confidence)}
	if confidence == 0 {
		weights = [5]float64{0, 0, 0, 0, 1}
	}
	total := 0.0
	for _, w := range weights {
		total += w
	}
	chosen := 4
	fraction := float64(draw>>11) / float64(uint64(1)<<53)
	sum := 0.0
	for i, w := range weights {
		result.Probabilities[i] = w / total
		sum += result.Probabilities[i]
		if fraction < sum && chosen == 4 {
			chosen = i
			fraction = 2
		}
	}
	appraisals := []string{"supportive", "dismissive", "neutral", "mixed", "unresolved"}
	o.Basis = "self_report"
	o.Status = core.Observed
	o.Appraisal = appraisals[chosen]
	o.Meta.Confidence = core.Confidence(confidence)
	o.Meta.Supporting = []core.ID{d.Source.Event}
	for _, v := range in.Context.values() {
		if v.Evidence == "" {
			continue
		}
		found := false
		for _, id := range o.Meta.Supporting {
			found = found || id == v.Evidence
		}
		if !found {
			o.Meta.Supporting = append(o.Meta.Supporting, v.Evidence)
		}
	}
	if chosen != 4 {
		benefit, burden := 0.0, 0.0
		switch chosen {
		case 0:
			benefit = .6
			burden = .1
		case 1:
			benefit = -.4
			burden = .7
		case 3:
			benefit = .4
			burden = .5
		}
		o.Benefit = &benefit
		o.Burden = &burden
	}
	result.Observation = o
	return result, o.Validate()
}

// OutcomeLearning rebuilds the bounded current memory rather than applying a
// second delta on correction. Expected benefit and missing outcomes never train.
// Immediate/later records remain in the ledger; the latest observed phase for an
// action informs the current projection. At most eight recent actions per peer
// contribute, with old evidence retained in the caller's immutable ledger.
func OutcomeLearning(log []core.OutcomeObservation, owner core.ID, focus core.RelationshipFocus, at core.LogicalTime, current []dynamics.Perceived) ([]Memory, error) {
	active, e := core.CurrentOutcomes(log, owner, focus, at, core.Grant{Actor: owner, Recipient: owner, Purpose: "simulation", Operation: core.Derive})
	if e != nil {
		return nil, e
	}
	if len(current) > 256 {
		return nil, fmt.Errorf("outcome source bound")
	}
	metadata := map[core.ID]dynamics.Perceived{}
	for _, p := range current {
		if _, ok := metadata[p.Event]; ok || p.Validate(owner, at) != nil {
			return nil, fmt.Errorf("invalid current outcome evidence")
		}
		metadata[p.Event] = p
	}
	phases := map[core.ID]map[string][]core.OutcomeObservation{}
	for _, o := range active {
		if o.Kind != "observed" || o.Position != "recipient" || o.Status != core.Observed {
			continue
		}
		proof, ok := metadata[o.Meta.ID]
		if !ok || proof.Confidence != o.Meta.Confidence || proof.OccurredAt != o.OccurredAt || proof.LearnedAt != o.LearnedAt {
			continue
		}
		permitted := true
		for _, id := range o.Meta.Supporting {
			if _, ok := metadata[id]; !ok {
				permitted = false
			}
		}
		if !permitted {
			continue
		}
		if phases[o.Action] == nil {
			phases[o.Action] = map[string][]core.OutcomeObservation{}
		}
		phases[o.Action][o.Phase] = append(phases[o.Action][o.Phase], o)
	}
	latest := map[core.ID]core.OutcomeObservation{}
	for action, accounts := range phases {
		selected := accounts["immediate"]
		if len(accounts["later"]) > 0 {
			selected = accounts["later"]
		}
		if len(selected) == 1 {
			latest[action] = selected[0]
		}
	}
	groups := map[core.ID][]core.OutcomeObservation{}
	for _, o := range latest {
		other := o.Other
		if owner != o.Participant {
			other = o.Participant
		}
		groups[other] = append(groups[other], o)
	}
	if len(groups) > 8 {
		return nil, fmt.Errorf("outcome peer bound")
	}
	out := []Memory{}
	for other, records := range groups {
		sort.Slice(records, func(i, j int) bool {
			if records[i].LearnedAt != records[j].LearnedAt {
				return records[i].LearnedAt < records[j].LearnedAt
			}
			return records[i].Meta.ID < records[j].Meta.ID
		})
		if len(records) > 8 {
			records = records[len(records)-8:]
		}
		m := Memory{Other: other, Evidence: []core.ID{}}
		for _, o := range records {
			delta := 0.0
			if o.Appraisal == "supportive" {
				delta = .1
			} else if o.Appraisal == "dismissive" {
				delta = -.1
			}
			m.Trust += delta * float64(o.Meta.Confidence)
			m.Disclosure += .5 * delta * float64(o.Meta.Confidence)
			m.Evidence = append(m.Evidence, o.Meta.ID)
		}
		m.Trust = bounded(m.Trust)
		m.Disclosure = bounded(m.Disclosure)
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Other < out[j].Other })
	return out, validateActionMemory(out, owner)
}
