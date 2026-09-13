// Package behavior owns bounded synthetic choices and observer-specific outcome
// learning. It is not a production intervention or relationship recommendation policy.
package behavior

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/dynamics"
)

const Policy = "human-actions.v1"
const MaxOutcomes = 16

type Kind string

const (
	Wait              Kind = "wait"
	Observe           Kind = "observe"
	Ask               Kind = "ask"
	SelfDisclose      Kind = "self_disclose"
	Help              Kind = "help"
	Decline           Kind = "decline"
	Invite            Kind = "invite"
	BreakPromise      Kind = "break_promise"
	ThirdPartySupport Kind = "third_party_support"
)

// Registry is the executable ticket-named registry, not a claim about missing
// source attachments. Unknown actions never silently become another action.
func Registry() []Kind {
	return []Kind{Wait, Observe, Ask, SelfDisclose, Help, Decline, Invite, BreakPromise, ThirdPartySupport}
}
func (k Kind) Valid() bool {
	for _, x := range Registry() {
		if x == k {
			return true
		}
	}
	return false
}
func unit(x float64) bool       { return !math.IsNaN(x) && !math.IsInf(x, 0) && x >= 0 && x <= 1 }
func signed(x float64) bool     { return !math.IsNaN(x) && !math.IsInf(x, 0) && x >= -1 && x <= 1 }
func bounded(x float64) float64 { return math.Round(math.Max(-1, math.Min(1, x))*1e9) / 1e9 }

type Belief struct {
	Source     core.ID         `json:"source"`
	Observer   core.ID         `json:"observer"`
	Code       string          `json:"code"`
	Value      float64         `json:"value"`
	Confidence core.Confidence `json:"confidence"`
}
type Memory struct {
	Other      core.ID   `json:"other"`
	Trust      float64   `json:"trust"`
	Disclosure float64   `json:"disclosure"`
	Evidence   []core.ID `json:"evidence"`
}
type Actor struct {
	State       dynamics.State   `json:"state"`
	AvailableAt core.LogicalTime `json:"available_at"`
	Memory      []Memory         `json:"memory"`
	Beliefs     []Belief         `json:"beliefs"`
}
type Offer struct {
	Kind       Kind             `json:"kind"`
	Recipient  core.ID          `json:"recipient,omitempty"`
	Resource   core.ID          `json:"resource,omitempty"`
	Units      int64            `json:"units,omitempty"`
	Duration   core.LogicalTime `json:"duration"`
	Commitment core.ID          `json:"commitment,omitempty"`
}
type Candidate struct {
	Offer       Offer   `json:"offer"`
	Consequence float64 `json:"subjective_consequence"`
	Probability float64 `json:"probability"`
}
type Decision struct {
	ID           core.ID          `json:"id"`
	Actor        core.ID          `json:"actor"`
	Event        core.ID          `json:"event"`
	At           core.LogicalTime `json:"at"`
	Policy       string           `json:"policy"`
	Candidates   []Candidate      `json:"candidates"`
	Selected     int              `json:"selected"`
	Draw         uint64           `json:"draw"`
	Explored     bool             `json:"explored"`
	Operational  bool             `json:"operational"`
	Reason       string           `json:"reason"`
	AppraisalKey string           `json:"appraisal_key"`
}
type Outcome struct {
	Decision    core.ID            `json:"decision"`
	Observer    core.ID            `json:"observer"`
	Other       core.ID            `json:"other,omitempty"`
	At          core.LogicalTime   `json:"at"`
	Horizon     core.LogicalTime   `json:"horizon"`
	Status      core.OutcomeStatus `json:"status"`
	Response    string             `json:"response,omitempty"`
	Evidence    core.ID            `json:"evidence,omitempty"`
	LearnedAt   core.LogicalTime   `json:"learned_at,omitempty"`
	Operational bool               `json:"operational"`
}
type Commitment struct {
	ID        core.ID          `json:"id"`
	Actor     core.ID          `json:"actor"`
	Recipient core.ID          `json:"recipient"`
	Resource  core.ID          `json:"resource"`
	Units     int64            `json:"units"`
	Due       core.LogicalTime `json:"due"`
	Status    string           `json:"status"`
}

// Situation contains only trusted, currently permitted perception and retrieved
// evidence. A raw model response, free prose, or hidden label is never a command.
type Situation struct {
	Perceived           dynamics.Perceived
	Relationships       []Memory
	Beliefs             []Belief
	Offers              []Offer
	Present             []core.ID
	Resources           map[core.ID]int64
	Commitments         []Commitment
	DisclosureRecipient core.ID // nonempty only after own-fiction disclosure policy
	Outage              bool    // trusted gateway classification; permission errors must fail upstream
	Horizon             core.LogicalTime
}

func (a Actor) Validate() error {
	if a.State.Validate() != nil || a.AvailableAt < 0 || len(a.Memory) > 8 || len(a.Beliefs) > 8 {
		return fmt.Errorf("invalid behavior actor")
	}
	seen := map[core.ID]bool{}
	for _, m := range a.Memory {
		if m.Other.Validate() != nil || m.Other == a.State.Actor || seen[m.Other] || !signed(m.Trust) || !signed(m.Disclosure) || len(m.Evidence) == 0 || len(m.Evidence) > 16 {
			return fmt.Errorf("invalid observer memory")
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
	return validateBeliefs(a.Beliefs, a.State.Actor)
}
func validateBeliefs(bs []Belief, actor core.ID) error {
	if len(bs) > 8 {
		return fmt.Errorf("belief budget")
	}
	seen := map[core.ID]bool{}
	for _, b := range bs {
		if b.Source.Validate() != nil || b.Observer != actor || !signed(b.Value) || b.Confidence.Validate() != nil || seen[b.Source] {
			return fmt.Errorf("invalid subjective belief")
		}
		seen[b.Source] = true
		switch b.Code {
		case "supported", "contradicted", "uncertain":
		default:
			return fmt.Errorf("unknown interpretation")
		}
	}
	return nil
}
func (o Offer) validate() error {
	if !o.Kind.Valid() || o.Duration < 1 || o.Duration > 1000000 {
		return fmt.Errorf("invalid action kind/time")
	}
	if o.Kind == Wait || o.Kind == Observe {
		if o.Recipient != "" {
			return fmt.Errorf("self action recipient")
		}
	} else if o.Recipient.Validate() != nil {
		return fmt.Errorf("missing recipient")
	}
	if o.Kind == Help {
		if o.Resource.Validate() != nil || o.Units <= 0 || o.Units > 1000000 {
			return fmt.Errorf("help requires explicit bounded resource effect")
		}
	} else if o.Resource != "" || o.Units != 0 {
		return fmt.Errorf("utterance cannot allocate resources")
	}
	if o.Commitment != "" && (o.Commitment.Validate() != nil || (o.Kind != Help && o.Kind != BreakPromise)) {
		return fmt.Errorf("invalid commitment action")
	}
	if o.Kind == BreakPromise && o.Commitment == "" {
		return fmt.Errorf("missing promise")
	}
	return nil
}
func (c Commitment) Validate() error {
	if c.ID.Validate() != nil || c.Actor.Validate() != nil || c.Recipient.Validate() != nil || c.Actor == c.Recipient || c.Resource.Validate() != nil || c.Units < 1 || c.Units > 1000000 || c.Due < 0 {
		return fmt.Errorf("invalid commitment")
	}
	switch c.Status {
	case "pending", "fulfilled", "broken":
		return nil
	}
	return fmt.Errorf("unknown commitment status")
}
func safe(o Offer, a Actor, s Situation, at core.LogicalTime) bool {
	if o.Kind == Wait {
		return true
	}
	if a.AvailableAt > at || o.Duration > s.Horizon-at {
		return false
	}
	if o.Recipient != "" {
		present := false
		for _, id := range s.Present {
			present = present || id == o.Recipient
		}
		if !present || o.Recipient == a.State.Actor {
			return false
		}
	}
	if o.Kind == SelfDisclose && s.DisclosureRecipient != o.Recipient {
		return false
	}
	if o.Kind == Help && s.Resources[o.Resource] < o.Units {
		return false
	}
	if o.Commitment != "" {
		match := false
		for _, c := range s.Commitments {
			if c.ID == o.Commitment && c.Actor == a.State.Actor && c.Recipient == o.Recipient && c.Status == "pending" {
				match = o.Kind == BreakPromise || (at+o.Duration <= c.Due && c.Resource == o.Resource && c.Units == o.Units)
			}
		}
		if !match {
			return false
		}
	}
	return true
}

// Choose calls the sole appraisal owner once, consumes its resulting state and
// then samples an explicit epsilon mixture. Draws are uniform 53-bit fractions;
// probabilities are policy weights, never model confidence or calibration claims.
func Choose(a Actor, s Situation, at core.LogicalTime, draw uint64) (Actor, Decision, error) {
	fail := func() (Actor, Decision, error) { return Actor{}, Decision{}, fmt.Errorf("invalid cognitive situation") }
	if a.Validate() != nil || s.Perceived.Validate(a.State.Actor, at) != nil || s.Horizon < at || s.Horizon-at > 1000000000 || len(s.Offers) > 16 || len(s.Present) > 24 || len(s.Resources) > 1024 || len(s.Commitments) > 16 || validateBeliefs(s.Beliefs, a.State.Actor) != nil {
		return fail()
	}
	for _, id := range s.Present {
		if id.Validate() != nil {
			return fail()
		}
	}
	for id, n := range s.Resources {
		if id.Validate() != nil || n < 0 {
			return fail()
		}
	}
	for _, c := range s.Commitments {
		if c.Validate() != nil {
			return fail()
		}
	}
	// Retrieved edge values remain separate from outcome-learned memory.
	relationActor := a
	relationActor.Memory = s.Relationships
	if relationActor.Validate() != nil {
		return fail()
	}
	// Validate even filtered-out proposals: unknown actions are not hidden by WAIT.
	for _, o := range s.Offers {
		if o.validate() != nil {
			return fail()
		}
	}
	state, rationale, repeated, err := dynamics.Appraise(a.State, s.Perceived, at)
	if err != nil {
		return Actor{}, Decision{}, err
	}
	if repeated {
		return Actor{}, Decision{}, fmt.Errorf("event already appraised; cannot select another action")
	}
	// Deep copy before updating memory/beliefs; a failed transition never mutates caller state.
	raw, _ := json.Marshal(a)
	var next Actor
	_ = json.Unmarshal(raw, &next)
	next.State = state
	next.Beliefs = append([]Belief{}, s.Beliefs...)
	sort.Slice(next.Beliefs, func(i, j int) bool { return next.Beliefs[i].Source < next.Beliefs[j].Source })
	t, err := dynamics.Response(state)
	if err != nil {
		return Actor{}, Decision{}, err
	}
	offers := []Offer{{Kind: Wait, Duration: 1}}
	seen := map[string]bool{}
	for _, o := range s.Offers {
		if o.Kind == Wait {
			continue
		}
		b, _ := json.Marshal(o)
		if seen[string(b)] {
			return fail()
		}
		seen[string(b)] = true
		if !s.Outage && safe(o, next, s, at) {
			offers = append(offers, o)
		}
	}
	sort.Slice(offers[1:], func(i, j int) bool {
		b, _ := json.Marshal(offers[i+1])
		c, _ := json.Marshal(offers[j+1])
		return string(b) < string(c)
	})
	d := Decision{ID: core.ID("decision:" + dynamics.Key(a.State.Actor, s.Perceived.Event)), Actor: a.State.Actor, Event: s.Perceived.Event, At: at, Policy: Policy, Draw: draw, AppraisalKey: rationale.Key, Operational: s.Outage, Reason: "sampled", Candidates: []Candidate{}}
	if s.Outage {
		d.Reason = "provider_outage"
	} else if len(offers) == 1 {
		d.Reason = "constraints_wait"
	}
	total := 0.0
	best := 0
	bestScore := -1.0
	for _, o := range offers {
		trust, disclosure := 0.0, 0.0
		for _, m := range next.Memory {
			if m.Other == o.Recipient {
				trust = m.Trust
				disclosure = m.Disclosure
			}
		}
		for _, m := range s.Relationships {
			if m.Other == o.Recipient {
				trust = bounded(trust + m.Trust)
				disclosure = bounded(disclosure + m.Disclosure)
			}
		}
		belief := 0.0
		for _, b := range next.Beliefs {
			belief += b.Value * float64(b.Confidence) / 8
		}
		score := t.Wait
		switch o.Kind {
		case Observe:
			score = .2 + t.Rest
		case Ask:
			score = .2 + t.Approach + .2*belief
		case SelfDisclose:
			score = .2 + t.Approach + .4*trust + .3*disclosure
		case Help:
			score = .2 + t.Support + .3*trust
		case Decline:
			score = .2 + t.Defend - .2*trust
		case Invite:
			score = .2 + t.Approach + .3*trust
		case BreakPromise:
			score = .1 + t.Rest - .3*trust
		case ThirdPartySupport:
			score = .2 + t.Approach + .2*t.Defend
		}
		score = bounded(math.Max(.01, score))
		if score > bestScore+1e-12 {
			best = len(d.Candidates)
			bestScore = score
		}
		d.Candidates = append(d.Candidates, Candidate{Offer: o, Consequence: bounded(score), Probability: score})
		total += score
	}
	for i := range d.Candidates {
		d.Candidates[i].Probability = .9*d.Candidates[i].Probability/total + .1/float64(len(d.Candidates))
	}
	partial := 0.0
	for i := 0; i < len(d.Candidates)-1; i++ {
		partial += d.Candidates[i].Probability
	}
	d.Candidates[len(d.Candidates)-1].Probability = 1 - partial
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
	if d.Candidates[d.Selected].Offer.Kind != Wait {
		next.AvailableAt = at + d.Candidates[d.Selected].Offer.Duration
	}
	return next, d, d.Validate()
}
func (d Decision) Validate() error {
	if d.ID != core.ID("decision:"+dynamics.Key(d.Actor, d.Event)) || d.Actor.Validate() != nil || d.Event.Validate() != nil || d.At < 0 || d.Policy != Policy || d.AppraisalKey != dynamics.Key(d.Actor, d.Event) || len(d.Candidates) < 1 || len(d.Candidates) > 17 || d.Selected < 0 || d.Selected >= len(d.Candidates) || d.Candidates[0].Offer.Kind != Wait {
		return fmt.Errorf("invalid decision")
	}
	switch d.Reason {
	case "sampled", "constraints_wait", "provider_outage":
	default:
		return fmt.Errorf("invalid reason")
	}
	if d.Operational != (d.Reason == "provider_outage") || (d.Operational && (len(d.Candidates) != 1 || d.Selected != 0)) {
		return fmt.Errorf("invalid operational WAIT")
	}
	total := 0.0
	selected := len(d.Candidates) - 1
	u := float64(d.Draw>>11) / float64(uint64(1)<<53)
	found := false
	for i, c := range d.Candidates {
		if c.Offer.validate() != nil || !unit(c.Probability) || c.Probability == 0 || !signed(c.Consequence) {
			return fmt.Errorf("invalid candidate")
		}
		total += c.Probability
		if !found && u < total {
			selected = i
			found = true
		}
	}
	best := 0
	for i, c := range d.Candidates {
		if c.Probability > d.Candidates[best].Probability+1e-12 {
			best = i
		}
	}
	if d.Explored != (d.Selected != best) {
		return fmt.Errorf("invalid explored flag")
	}
	if math.Abs(total-1) > 1e-12 || selected != d.Selected {
		return fmt.Errorf("invalid selection probability/draw")
	}
	return nil
}
func Track(d Decision, horizon core.LogicalTime) (Outcome, error) {
	if d.Validate() != nil || horizon < d.At {
		return Outcome{}, fmt.Errorf("invalid outcome horizon")
	}
	return Outcome{Decision: d.ID, Observer: d.Actor, Other: d.Candidates[d.Selected].Offer.Recipient, At: d.At, Horizon: horizon, Status: core.Unknown, Operational: d.Operational}, nil
}
func (o Outcome) Validate() error {
	if o.Decision.Validate() != nil || o.Observer.Validate() != nil || o.At < 0 || o.Horizon < o.At || (o.Other != "" && o.Other.Validate() != nil) {
		return fmt.Errorf("invalid outcome")
	}
	switch o.Status {
	case core.Unknown, core.Censored:
		if o.Response != "" || o.Evidence != "" || o.LearnedAt != 0 {
			return fmt.Errorf("unobserved response")
		}
	case core.Observed:
		if o.Evidence.Validate() != nil || o.LearnedAt < o.At || o.LearnedAt > o.Horizon || o.Other == "" {
			return fmt.Errorf("invalid outcome evidence/time")
		}
		switch o.Response {
		case "supportive", "dismissive", "neutral":
		default:
			return fmt.Errorf("unknown response")
		}
	default:
		return fmt.Errorf("invalid outcome status")
	}
	return nil
}

// Resolve requires actual later observer-owned evidence. Passing a horizon alone
// cannot fabricate success; censorship and unknown never train state. A second
// resolution is rejected, including a conflicting response after restart.
func Resolve(a Actor, o Outcome, e dynamics.Perceived, other core.ID, response string) (Actor, Outcome, error) {
	if a.Validate() != nil || o.Validate() != nil || o.Status != core.Unknown || o.Observer != a.State.Actor || o.Operational || other != o.Other || e.Actor != o.Observer || e.LearnedAt <= o.At || e.LearnedAt > o.Horizon || e.Validate(a.State.Actor, e.LearnedAt) != nil {
		return Actor{}, Outcome{}, fmt.Errorf("invalid outcome resolution")
	}
	nextOutcome := o
	nextOutcome.Status = core.Observed
	nextOutcome.Response = response
	nextOutcome.Evidence = e.Event
	nextOutcome.LearnedAt = e.LearnedAt
	if nextOutcome.Validate() != nil {
		return Actor{}, Outcome{}, fmt.Errorf("invalid response")
	}
	raw, _ := json.Marshal(a)
	var next Actor
	_ = json.Unmarshal(raw, &next)
	index := -1
	for i, m := range next.Memory {
		if m.Other == other {
			index = i
		}
	}
	if index < 0 {
		if len(next.Memory) >= 8 {
			return Actor{}, Outcome{}, fmt.Errorf("memory budget")
		}
		next.Memory = append(next.Memory, Memory{Other: other, Evidence: []core.ID{}})
		index = len(next.Memory) - 1
	}
	m := &next.Memory[index]
	if len(m.Evidence) >= 16 {
		return Actor{}, Outcome{}, fmt.Errorf("outcome evidence budget")
	}
	for _, id := range m.Evidence {
		if id == e.Event {
			return Actor{}, Outcome{}, fmt.Errorf("response already learned")
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
	return next, nextOutcome, next.Validate()
}
func Censor(o Outcome, at core.LogicalTime) (Outcome, error) {
	if o.Validate() != nil || o.Status != core.Unknown || at < o.Horizon {
		return Outcome{}, fmt.Errorf("cannot censor")
	}
	o.Status = core.Censored
	return o, nil
}
