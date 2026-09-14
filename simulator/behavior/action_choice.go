package behavior

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/drives"
	"github.com/tushardhara/dream/simulator/dynamics"
)

// A zero ContextValue is unknown, not a guessed midpoint or consent grant.
type ContextValue struct {
	Value      float64         `json:"value"`
	Confidence core.Confidence `json:"confidence"`
	Evidence   core.ID         `json:"evidence,omitempty"`
}

func (v ContextValue) validate(sources map[core.ID]bool) bool {
	return signed(v.Value) && v.Confidence.Validate() == nil && (v.Evidence != "" && sources[v.Evidence] || v.Evidence == "" && v.Value == 0 && v.Confidence == 0)
}
func (v ContextValue) weighted() float64 { return v.Value * float64(v.Confidence) }

type DisclosureContext struct {
	Observer         core.ID      `json:"observer"`
	Recipient        core.ID      `json:"recipient"`
	Role             string       `json:"role,omitempty"`
	Trust            ContextValue `json:"trust"`
	RoleExpectation  ContextValue `json:"role_expectation"`
	Sensitivity      ContextValue `json:"sensitivity"`
	Fear             ContextValue `json:"fear"`
	Pride            ContextValue `json:"pride"`
	Shame            ContextValue `json:"shame"`
	ExpectedReaction ContextValue `json:"expected_reaction"`
	ProtectiveIntent ContextValue `json:"protective_intent"`
	SocialNorm       ContextValue `json:"social_norm"`
	Stress           ContextValue `json:"stress"`
	PriorOutcome     ContextValue `json:"prior_outcome"`
}

func (c DisclosureContext) values() []ContextValue {
	return []ContextValue{c.Trust, c.RoleExpectation, c.Sensitivity, c.Fear, c.Pride, c.Shame, c.ExpectedReaction, c.ProtectiveIntent, c.SocialNorm, c.Stress, c.PriorOutcome}
}
func (c DisclosureContext) readiness() float64 {
	return .3*c.Trust.weighted() + .1*c.RoleExpectation.weighted() - .2*c.Sensitivity.weighted() - .15*c.Fear.weighted() - .1*c.Pride.weighted() - .1*c.Shame.weighted() + .2*c.ExpectedReaction.weighted() + .1*c.ProtectiveIntent.weighted() + .1*c.SocialNorm.weighted() - .15*c.Stress.weighted() + .15*c.PriorOutcome.weighted()
}

// DisclosureGrant is trusted host preparation, not authority a model may mint.
// The service binds it to PolicyService output and revalidates before commit.
type DisclosureGrant struct {
	Recipient core.ID
	Mode      DisclosureMode
	Sources   []core.ID
}
type ActionSituation struct {
	RelationshipEvidence []dynamics.Perceived
	Observation          drives.Observation
	Beliefs              []Belief
	Relationships        []Memory
	Offers               []ActionOffer
	Contexts             []DisclosureContext
	Sources              []core.ID
	Present              []core.ID
	Resources            map[core.ID]int64
	Commitments          []Commitment
	Disclosure           *DisclosureGrant
	Outage               bool
	Horizon              core.LogicalTime
}
type PrivateActionState struct {
	Appraisal  [4]float64         `json:"appraisal"` // threat, opportunity, care, status
	Emotion    [2]float64         `json:"emotion"`   // subjective valence/arousal
	Active     [drives.Count]bool `json:"active"`
	Fear       float64            `json:"fear"`
	Intention  Kind               `json:"intention,omitempty"`
	Suppressed DisclosureMode     `json:"suppressed,omitempty"`
	Rationale  string             `json:"rationale"`
}
type ActionActor struct {
	Drives      drives.State       `json:"drives"`
	AvailableAt core.LogicalTime   `json:"available_at"`
	Contact     string             `json:"contact"`
	Memory      []Memory           `json:"memory"`
	Beliefs     []Belief           `json:"beliefs"`
	Private     PrivateActionState `json:"private"`
}

func NewActionActor(id core.ID, at core.LogicalTime) (ActionActor, error) {
	d, e := drives.New(id, at, drives.DefaultSubstrate())
	a := ActionActor{Drives: d, Contact: "engaged", Memory: []Memory{}, Beliefs: []Belief{}, Private: PrivateActionState{Rationale: "initial"}}
	if e != nil {
		return ActionActor{}, e
	}
	return a, a.Validate()
}
func validateActionMemory(ms []Memory, actor core.ID) error {
	if len(ms) > 8 {
		return fmt.Errorf("memory bound")
	}
	seen := map[core.ID]bool{}
	for _, m := range ms {
		if m.Other.Validate() != nil || m.Other == actor || seen[m.Other] || !signed(m.Trust) || !signed(m.Disclosure) || len(m.Evidence) < 1 || len(m.Evidence) > 16 {
			return fmt.Errorf("invalid action memory")
		}
		seen[m.Other] = true
		ids := map[core.ID]bool{}
		for _, id := range m.Evidence {
			if id.Validate() != nil || ids[id] {
				return fmt.Errorf("invalid memory evidence")
			}
			ids[id] = true
		}
	}
	return nil
}
func (a ActionActor) Validate() error {
	if a.Drives.Validate() != nil || a.AvailableAt < 0 || validateActionMemory(a.Memory, a.Drives.Actor) != nil || validateBeliefs(a.Beliefs, a.Drives.Actor) != nil {
		return fmt.Errorf("invalid action actor")
	}
	switch a.Contact {
	case "engaged", "withdrawn", "left":
	default:
		return fmt.Errorf("invalid contact state")
	}
	for _, v := range a.Private.Appraisal {
		if !unit(v) {
			return fmt.Errorf("invalid private appraisal")
		}
	}
	for _, v := range a.Private.Emotion {
		if !signed(v) {
			return fmt.Errorf("invalid private emotion")
		}
	}
	if !unit(a.Private.Fear) || a.Private.Suppressed != "" && !a.Private.Suppressed.Valid() {
		return fmt.Errorf("invalid private fear/disclosure")
	}
	if a.Private.Intention != "" {
		if _, ok := Definition(a.Private.Intention); !ok {
			return fmt.Errorf("unknown intention")
		}
	}
	switch a.Private.Rationale {
	case "initial", "sampled", "constraints_wait", "provider_outage":
	default:
		return fmt.Errorf("unknown bounded rationale")
	}
	return nil
}

type ActionCandidate struct {
	Offer       ActionOffer `json:"offer"`
	Consequence float64     `json:"subjective_consequence"`
	Probability float64     `json:"probability"`
}
type ActionDecision struct {
	ID           core.ID           `json:"id"`
	Actor        core.ID           `json:"actor"`
	Event        core.ID           `json:"event"`
	At           core.LogicalTime  `json:"at"`
	Policy       string            `json:"policy"`
	AppraisalKey string            `json:"appraisal_key"`
	Candidates   []ActionCandidate `json:"candidates"`
	Selected     int               `json:"selected"`
	Draw         uint64            `json:"draw"`
	Explored     bool              `json:"explored"`
	Operational  bool              `json:"operational"`
	Reason       string            `json:"reason"`
	Stages       [14]string        `json:"stages"`
}

func actionID(actor, event core.ID) core.ID { return core.ID("action:" + drives.Key(actor, event)) }
func (d ActionDecision) Validate() error {
	if d.Policy != ActionPolicy || d.ID != actionID(d.Actor, d.Event) || d.Actor.Validate() != nil || d.Event.Validate() != nil || d.At < 0 || d.AppraisalKey != drives.Key(d.Actor, d.Event) || len(d.Candidates) < 1 || len(d.Candidates) > 28 || d.Selected < 0 || d.Selected >= len(d.Candidates) || d.Candidates[0].Offer.Kind != Wait {
		return fmt.Errorf("invalid action decision")
	}
	for i, v := range d.Stages {
		want := "done"
		if i == 11 {
			if v != "prepared" && v != "done" {
				return fmt.Errorf("missing execution stage")
			}
			continue
		}
		if i >= 12 {
			want = "pending"
		}
		if d.Operational && (i == 5 || i == 6 || i >= 12) {
			want = "not_applicable"
		}
		if v != want {
			return fmt.Errorf("invalid lifecycle stage %d", i+1)
		}
	}
	if d.Operational != (d.Reason == "provider_outage") || d.Operational && (len(d.Candidates) != 1 || d.Selected != 0) {
		return fmt.Errorf("invalid operational action")
	}
	if d.Reason != "sampled" && d.Reason != "constraints_wait" && d.Reason != "provider_outage" {
		return fmt.Errorf("invalid decision reason")
	}
	total := 0.0
	selected := len(d.Candidates) - 1
	best := 0
	found := false
	u := float64(d.Draw>>11) / float64(uint64(1)<<53)
	seen := map[string]bool{}
	for i, c := range d.Candidates {
		b, _ := json.Marshal(c.Offer)
		if c.Offer.Validate() != nil || !unit(c.Probability) || c.Probability == 0 || !signed(c.Consequence) || seen[string(b)] {
			return fmt.Errorf("invalid action candidate")
		}
		seen[string(b)] = true
		total += c.Probability
		if !found && u < total {
			selected = i
			found = true
		}
		if c.Probability > d.Candidates[best].Probability+1e-12 {
			best = i
		}
	}
	if math.Abs(total-1) > 1e-12 || selected != d.Selected || d.Explored != (best != d.Selected) {
		return fmt.Errorf("invalid action sampling")
	}
	return nil
}
func (s ActionSituation) validate(a ActionActor, at core.LogicalTime) error {
	if a.Validate() != nil || s.Observation.Validate(a.Drives.Actor, at) != nil || s.Horizon < at || s.Horizon-at > 1000000000 || len(s.Offers) > 27 || len(s.Present) > 24 || len(s.Sources) > 16 || len(s.Contexts) > 8 || len(s.Resources) > 1024 || len(s.Commitments) > 16 || validateBeliefs(s.Beliefs, a.Drives.Actor) != nil || validateActionMemory(s.Relationships, a.Drives.Actor) != nil {
		return fmt.Errorf("invalid action situation")
	}
	sources := map[core.ID]bool{}
	for _, id := range s.Sources {
		if id.Validate() != nil || sources[id] {
			return fmt.Errorf("invalid permitted sources")
		}
		sources[id] = true
	}
	if _, e := relationshipEvidence(s, a.Drives.Actor, at); e != nil {
		return e
	}
	if !sources[s.Observation.Event.Event] {
		return fmt.Errorf("event not retrieved")
	}
	for _, cue := range s.Observation.Context {
		if !sources[cue.Evidence.Event] {
			return fmt.Errorf("context not retrieved")
		}
	}
	for _, b := range s.Beliefs {
		if !sources[b.Source] {
			return fmt.Errorf("belief source not retrieved")
		}
	}
	for _, m := range s.Relationships {
		for _, id := range m.Evidence {
			if !sources[id] {
				return fmt.Errorf("relationship not retrieved")
			}
		}
	}
	present := map[core.ID]bool{}
	for _, id := range s.Present {
		if id.Validate() != nil || present[id] {
			return fmt.Errorf("invalid presence")
		}
		present[id] = true
	}
	for id, n := range s.Resources {
		if id.Validate() != nil || n < 0 {
			return fmt.Errorf("invalid resource")
		}
	}
	promises := map[core.ID]bool{}
	for _, c := range s.Commitments {
		if c.Validate() != nil || promises[c.ID] {
			return fmt.Errorf("invalid commitments")
		}
		promises[c.ID] = true
	}
	contexts := map[core.ID]bool{}
	for _, c := range s.Contexts {
		if c.Observer != a.Drives.Actor || c.Recipient.Validate() != nil || c.Recipient == c.Observer || contexts[c.Recipient] {
			return fmt.Errorf("foreign or duplicate disclosure context")
		}
		contexts[c.Recipient] = true
		for _, v := range c.values() {
			if !v.validate(sources) {
				return fmt.Errorf("unpermitted disclosure context")
			}
		}
		switch c.Role {
		case "":
		case "spouse", "sibling", "friend", "acquaintance", "parent", "child", "cofounder", "employee", "manager", "stranger", "sibling_in_law":
			if c.RoleExpectation.Evidence == "" {
				return fmt.Errorf("role requires supplied evidence")
			}
		default:
			return fmt.Errorf("unknown role context")
		}
	}
	if g := s.Disclosure; g != nil {
		if g.Recipient.Validate() != nil || g.Recipient == a.Drives.Actor || !g.Mode.Valid() || len(g.Sources) < 1 || len(g.Sources) > 8 {
			return fmt.Errorf("invalid disclosure grant")
		}
		seen := map[core.ID]bool{}
		for _, id := range g.Sources {
			if !sources[id] || seen[id] {
				return fmt.Errorf("unpermitted disclosure grant")
			}
			seen[id] = true
		}
	}
	offers := map[string]bool{}
	for _, o := range s.Offers {
		b, _ := json.Marshal(o)
		if o.Validate() != nil || offers[string(b)] {
			return fmt.Errorf("invalid proposed action")
		}
		offers[string(b)] = true
		for _, id := range o.Evidence {
			if !sources[id] {
				return fmt.Errorf("action evidence not retrieved")
			}
		}
	}
	return nil
}
func contextFor(s ActionSituation, recipient core.ID) DisclosureContext {
	for _, c := range s.Contexts {
		if c.Recipient == recipient {
			return c
		}
	}
	return DisclosureContext{}
}
func eligibleAction(o ActionOffer, a ActionActor, s ActionSituation, at core.LogicalTime) bool {
	if o.Kind == Wait {
		return true
	}
	if s.Outage || a.AvailableAt > at || o.Duration > s.Horizon-at {
		return false
	}
	if a.Contact == "left" && o.Kind != Reconnect {
		return false
	}
	if o.Kind == Reconnect && a.Contact == "engaged" {
		return false
	}
	if o.Recipient != "" {
		ok := false
		for _, id := range s.Present {
			ok = ok || id == o.Recipient
		}
		if !ok || o.Recipient == a.Drives.Actor {
			return false
		}
	}
	if o.Mode != "" && o.Mode != Silence {
		g := s.Disclosure
		if g == nil || g.Recipient != o.Recipient || g.Mode != o.Mode {
			return false
		}
		allowed := map[core.ID]bool{}
		for _, id := range g.Sources {
			allowed[id] = true
		}
		for _, id := range o.Evidence {
			if !allowed[id] {
				return false
			}
		}
		c := contextFor(s, o.Recipient)
		if o.Mode == Full && (c.Trust.Confidence == 0 || c.ExpectedReaction.Confidence == 0 || c.readiness() < 0) {
			return false
		}
		if o.Mode == Partial && (c.Trust.Confidence == 0 || c.readiness() < -.3) {
			return false
		}
	}
	if o.Kind == Help || o.Kind == Promise {
		if s.Resources[o.Resource] < o.Units {
			return false
		}
	}
	if o.Kind == Promise {
		if o.Due < at+o.Duration || o.Due > s.Horizon || len(s.Commitments) >= 16 {
			return false
		}
		for _, c := range s.Commitments {
			if c.ID == o.Commitment {
				return false
			}
		}
	} else if o.Commitment != "" {
		ok := false
		for _, c := range s.Commitments {
			if c.ID == o.Commitment && c.Actor == a.Drives.Actor && c.Recipient == o.Recipient && c.Status == "pending" {
				ok = o.Kind == BreakPromise || c.Resource == o.Resource && c.Units == o.Units && at+o.Duration <= c.Due
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// ChooseAction performs stages1–11 and prepares stage12. The host commits the
// selected bounded effect; stages13/14 live on the later outcome ledger.
func ChooseAction(a ActionActor, s ActionSituation, at core.LogicalTime, draw uint64) (ActionActor, ActionDecision, error) {
	if e := s.validate(a, at); e != nil {
		return ActionActor{}, ActionDecision{}, e
	}
	raw, _ := json.Marshal(a)
	var next ActionActor
	_ = json.Unmarshal(raw, &next)
	state, r, repeated, e := drives.Appraise(a.Drives, s.Observation, at)
	if e != nil {
		return ActionActor{}, ActionDecision{}, e
	}
	if repeated {
		return ActionActor{}, ActionDecision{}, fmt.Errorf("already selected for appraisal")
	}
	next.Drives = state
	next.Beliefs = append([]Belief{}, s.Beliefs...)
	if s.Outage {
		next.Beliefs = []Belief{}
	}
	sort.Slice(next.Beliefs, func(i, j int) bool { return next.Beliefs[i].Source < next.Beliefs[j].Source })
	sig := s.Observation.Event.Signals
	confidence := float64(s.Observation.Event.Confidence)
	next.Private = PrivateActionState{Appraisal: [4]float64{sig.Exclusion * confidence, sig.Opportunity * confidence, sig.OtherNeed * confidence, sig.StatusThreat * confidence}, Emotion: [2]float64{bounded((sig.Support - sig.Exclusion) * confidence), bounded((sig.StatusThreat + sig.Opportunity) * confidence)}, Fear: sig.StatusThreat * confidence, Rationale: "sampled"}
	for i, v := range state.Variables {
		next.Private.Active[i] = v.Values[0] > .5
	}
	tendencies, e := drives.Response(state)
	if e != nil {
		return ActionActor{}, ActionDecision{}, e
	}
	offers := []ActionOffer{{Kind: Wait, Duration: 1}}
	for _, o := range s.Offers {
		if o.Kind == Wait {
			continue
		}
		if eligibleAction(o, next, s, at) {
			o.Evidence = append([]core.ID{}, o.Evidence...)
			offers = append(offers, o)
		} else if o.Mode != "" {
			next.Private.Suppressed = o.Mode
		}
	}
	sort.Slice(offers[1:], func(i, j int) bool {
		b, _ := json.Marshal(offers[i+1])
		c, _ := json.Marshal(offers[j+1])
		return string(b) < string(c)
	})
	d := ActionDecision{ID: actionID(a.Drives.Actor, s.Observation.Event.Event), Actor: a.Drives.Actor, Event: s.Observation.Event.Event, At: at, Policy: ActionPolicy, AppraisalKey: r.Key, Draw: draw, Operational: s.Outage, Reason: "sampled", Candidates: []ActionCandidate{}}
	for i := range d.Stages {
		d.Stages[i] = "done"
	}
	d.Stages[11] = "prepared"
	d.Stages[12] = "pending"
	d.Stages[13] = "pending"
	if s.Outage {
		d.Reason = "provider_outage"
		d.Stages[5] = "not_applicable"
		d.Stages[6] = "not_applicable"
		d.Stages[12] = "not_applicable"
		d.Stages[13] = "not_applicable"
	} else if len(offers) == 1 {
		d.Reason = "constraints_wait"
	}
	next.Private.Rationale = d.Reason
	total := 0.0
	best := 0
	bestScore := -1.0
	for _, o := range offers {
		trust := 0.0
		for _, m := range next.Memory {
			if m.Other == o.Recipient {
				trust += m.Trust
			}
		}
		for _, m := range s.Relationships {
			if m.Other == o.Recipient {
				trust += m.Trust
			}
		}
		trust = bounded(trust)
		belief := 0.0
		for _, b := range next.Beliefs {
			belief += b.Value * float64(b.Confidence) / 8
		}
		score := tendencies.Wait
		switch o.Kind {
		case Say, Ask, Answer, Coordinate, Invite, Reconnect:
			score = .2 + tendencies.Approach + .15*trust + .1*belief
		case Reveal, PartiallyReveal, Joke:
			score = .2 + tendencies.Approach + .25*trust
		case Lie, Hide:
			score = .1 + tendencies.Defend + .2*next.Private.Fear
		case Challenge, Argue, Complain, Decline:
			score = .2 + tendencies.Defend - .1*trust
		case Apologize, Support, Promise, Help, SeekThirdPartySupport:
			score = .2 + tendencies.Support + .2*trust
		case Withdraw, Delay, Leave, Ignore, BreakPromise, ChangeTopic:
			score = .1 + tendencies.Rest + .1*tendencies.Defend - .1*trust
		}
		if o.Mode != "" {
			c := contextFor(s, o.Recipient)
			sign := 1.0
			if o.Mode == FalseClaim || o.Mode == Omission || o.Mode == Deflection || o.Mode == Silence || o.Mode == TopicChange {
				sign = -1
			}
			score += sign * c.readiness()
		}
		score = math.Max(.01, bounded(score))
		if score > bestScore+1e-12 {
			best = len(d.Candidates)
			bestScore = score
		}
		d.Candidates = append(d.Candidates, ActionCandidate{Offer: o, Consequence: score, Probability: score})
		total += score
	}
	partial := 0.0
	for i := range d.Candidates {
		p := .9*d.Candidates[i].Probability/total + .1/float64(len(d.Candidates))
		if i == len(d.Candidates)-1 {
			p = 1 - partial
		}
		d.Candidates[i].Probability = p
		partial += p
	}
	u := float64(draw>>11) / float64(uint64(1)<<53)
	sum := 0.0
	d.Selected = len(d.Candidates) - 1
	for i, c := range d.Candidates {
		sum += c.Probability
		if u < sum {
			d.Selected = i
			break
		}
	}
	d.Explored = d.Selected != best
	selected := d.Candidates[d.Selected].Offer
	next.Private.Intention = selected.Kind
	if selected.Kind != Wait {
		next.AvailableAt = at + selected.Duration
		switch selected.Kind {
		case Leave:
			next.Contact = "left"
		case Withdraw:
			next.Contact = "withdrawn"
		case Reconnect:
			next.Contact = "engaged"
		}
	}
	if selected.Kind == Hide {
		next.Private.Suppressed = Silence
	}
	if e := next.Validate(); e != nil {
		return ActionActor{}, ActionDecision{}, e
	}
	return next, d, d.Validate()
}

// ActionOutcome keeps later lifecycle stages honest, separate from the next
// decision. Unknown/censored responses never update state or fulfill a promise.
type ActionOutcome struct {
	EffectAt         core.LogicalTime `json:"effect_at"`
	Outcome          Outcome          `json:"outcome"`
	Action           Kind             `json:"action"`
	Commitment       core.ID          `json:"commitment,omitempty"`
	ObservationStage string           `json:"observation_stage"`
	LearningStage    string           `json:"learning_stage"`
}

func (o ActionOutcome) Validate() error {
	if o.Outcome.Validate() != nil || o.EffectAt < o.Outcome.At || o.EffectAt > o.Outcome.Horizon {
		return fmt.Errorf("invalid action outcome")
	}
	if _, ok := Definition(o.Action); !ok {
		return fmt.Errorf("invalid outcome action")
	}
	if o.Commitment != "" && (o.Commitment.Validate() != nil || o.Action != Help && o.Action != Promise && o.Action != BreakPromise) {
		return fmt.Errorf("invalid outcome commitment")
	}
	want := "pending"
	if o.Outcome.Operational {
		want = "not_applicable"
	} else if o.Outcome.Status == core.Observed {
		want = "done"
	} else if o.Outcome.Status == core.Censored {
		want = "censored"
	}
	if o.ObservationStage != want || o.LearningStage != want {
		return fmt.Errorf("outcome lifecycle mismatch")
	}
	return nil
}
func TrackAction(d ActionDecision, horizon core.LogicalTime) (ActionOutcome, error) {
	if d.Validate() != nil || horizon < d.At {
		return ActionOutcome{}, fmt.Errorf("invalid action tracking")
	}
	selected := d.Candidates[d.Selected].Offer
	other := core.ID("")
	if selected.Delivered() {
		other = selected.Recipient
	}
	stage := "pending"
	if d.Operational {
		stage = "not_applicable"
	}
	effectAt := d.At
	if selected.Kind != Wait {
		effectAt += selected.Duration
	}
	o := ActionOutcome{EffectAt: effectAt, Outcome: Outcome{Decision: d.ID, Observer: d.Actor, Other: other, At: d.At, Horizon: horizon, Status: core.Unknown, Operational: d.Operational}, Action: selected.Kind, Commitment: selected.Commitment, ObservationStage: stage, LearningStage: stage}
	return o, o.Validate()
}
func ResolveAction(a ActionActor, o ActionOutcome, e dynamics.Perceived, other core.ID, response string) (ActionActor, ActionOutcome, error) {
	if a.Validate() != nil || o.Validate() != nil || o.Outcome.Status != core.Unknown || o.Outcome.Operational || o.Outcome.Observer != a.Drives.Actor || other != o.Outcome.Other || e.LearnedAt <= o.Outcome.At || e.OccurredAt < o.EffectAt || e.LearnedAt > o.Outcome.Horizon || e.Validate(a.Drives.Actor, e.LearnedAt) != nil {
		return ActionActor{}, ActionOutcome{}, fmt.Errorf("invalid later action evidence")
	}
	out := o
	out.Outcome.Status = core.Observed
	out.Outcome.Response = response
	out.Outcome.Evidence = e.Event
	out.Outcome.LearnedAt = e.LearnedAt
	out.ObservationStage = "done"
	out.LearningStage = "done"
	if out.Validate() != nil {
		return ActionActor{}, ActionOutcome{}, fmt.Errorf("invalid later response")
	}
	raw, _ := json.Marshal(a)
	var next ActionActor
	_ = json.Unmarshal(raw, &next)
	index := -1
	for i, m := range next.Memory {
		if m.Other == other {
			index = i
		}
	}
	if index < 0 {
		if len(next.Memory) >= 8 {
			return ActionActor{}, ActionOutcome{}, fmt.Errorf("memory bound")
		}
		next.Memory = append(next.Memory, Memory{Other: other, Evidence: []core.ID{}})
		index = len(next.Memory) - 1
	}
	m := &next.Memory[index]
	if len(m.Evidence) >= 16 {
		return ActionActor{}, ActionOutcome{}, fmt.Errorf("evidence bound")
	}
	for _, id := range m.Evidence {
		if id == e.Event {
			return ActionActor{}, ActionOutcome{}, fmt.Errorf("already learned response")
		}
	}
	delta := 0.0
	if response == "supportive" {
		delta = .1
	}
	if response == "dismissive" {
		delta = -.1
	}
	m.Trust = bounded(m.Trust + delta*float64(e.Confidence))
	m.Disclosure = bounded(m.Disclosure + .5*delta*float64(e.Confidence))
	m.Evidence = append(m.Evidence, e.Event)
	sort.Slice(next.Memory, func(i, j int) bool { return next.Memory[i].Other < next.Memory[j].Other })
	return next, out, next.Validate()
}
func CensorAction(o ActionOutcome, at core.LogicalTime) (ActionOutcome, error) {
	if o.Validate() != nil {
		return ActionOutcome{}, fmt.Errorf("invalid outcome")
	}
	v, e := Censor(o.Outcome, at)
	if e != nil {
		return ActionOutcome{}, e
	}
	o.Outcome = v
	if !v.Operational {
		o.ObservationStage = "censored"
		o.LearningStage = "censored"
	}
	return o, o.Validate()
}
