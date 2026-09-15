package evals

import (
	"fmt"
	"sort"

	"github.com/tushardhara/dream/core"
)

// UpliftVersion is the opt-in envelope for matched-arm helpful-AI evaluation.
// It adds no capability to existing evaluation reports and changes no frozen
// policy, codec or replay pin.
const UpliftVersion = "uplift-evaluation.v1"

// Arm is one matched comparison arm. Humans continue acting in every arm: the
// no-assistant arm is a control, never a world in which people stop deciding.
type Arm string

const (
	NoAssistant       Arm = "none"
	SimpleAssistance  Arm = "simple"
	SinglePerspective Arm = "single_perspective"
	MultiPerspective  Arm = "multi_perspective"
)

// Arms is the fixed arm order. Matched worlds and exogenous events are shared
// across arms; only the assistant policy differs.
var Arms = []Arm{NoAssistant, SimpleAssistance, SinglePerspective, MultiPerspective}

func (a Arm) Valid() bool {
	for _, k := range Arms {
		if a == k {
			return true
		}
	}
	return false
}

// EvidenceTier keeps RVE-like tiers distinct. A lower tier never becomes a
// higher one, and no tier here is evidence about real people: every record in
// this package carries SyntheticProvenance.
type EvidenceTier string

const (
	SystemAssertion        EvidenceTier = "system_assertion"
	PromptedResponse       EvidenceTier = "prompted_response"
	BehaviouralObservation EvidenceTier = "behavioural_observation"
	AttributedLater        EvidenceTier = "independently_attributed_later"
)

var tierRank = map[EvidenceTier]int{SystemAssertion: 0, PromptedResponse: 1, BehaviouralObservation: 2, AttributedLater: 3}

// SyntheticProvenance is the only provenance this package accepts. It exists so
// that a synthetic tier can never be recorded as real-human proof.
const SyntheticProvenance = "SYNTHETIC"

// bannedBenefitMetrics can never qualify as benefit, however they are named.
// Engagement is not welfare: more messages, opens, time or attachment are not
// evidence that anyone was helped.
var bannedBenefitMetrics = map[string]bool{
	"action_count": true, "actions": true, "message_count": true, "messages": true,
	"notification_opens": true, "opens": true, "session_time": true, "time_in_app": true,
	"engagement": true, "emotional_attachment": true, "attachment": true,
	"acceptance_rate": true, "invitations_accepted": true, "daily_active": true,
}

// BannedBenefitMetric reports whether a metric name is disqualified as benefit.
func BannedBenefitMetric(name string) bool { return bannedBenefitMetrics[name] }

// TierEvidence is one attributed synthetic observation about one person.
type TierEvidence struct {
	Person      core.ID            `json:"person"`
	Tier        EvidenceTier       `json:"tier"`
	Provenance  string             `json:"provenance"`
	DerivedFrom EvidenceTier       `json:"derived_from,omitempty"`
	Metric      string             `json:"metric"`
	Value       core.GroupQuantity `json:"value"`
}

func (o TierEvidence) Validate() error {
	if o.Person.Validate() != nil || o.Metric == "" {
		return fmt.Errorf("invalid observation subject")
	}
	if _, ok := tierRank[o.Tier]; !ok {
		return fmt.Errorf("invalid evidence tier")
	}
	if o.Provenance != SyntheticProvenance {
		return fmt.Errorf("evidence provenance must be synthetic")
	}
	if o.DerivedFrom != "" {
		rank, ok := tierRank[o.DerivedFrom]
		if !ok {
			return fmt.Errorf("invalid evidence tier")
		}
		if rank < tierRank[o.Tier] {
			return fmt.Errorf("evidence tier promoted above its source")
		}
	}
	if BannedBenefitMetric(o.Metric) {
		return fmt.Errorf("metric cannot qualify as benefit")
	}
	return o.Value.Validate(-1, 1)
}

// PersonOutcome is one affected person's separately attributed result. Benefit
// and burden are quantities that distinguish observed zero from unknown, so an
// unobserved burden is never read as no burden.
type PersonOutcome struct {
	Person             core.ID            `json:"person"`
	Benefit            core.GroupQuantity `json:"benefit"`
	Burden             core.GroupQuantity `json:"burden"`
	Appropriateness    core.GroupQuantity `json:"appropriateness"`
	Unwanted           int                `json:"unwanted_interventions"`
	BoundaryViolations int                `json:"boundary_violations"`
	DelayedOutcome     string             `json:"delayed_outcome"`
	Acted              bool               `json:"person_acted"`
	Observations       []TierEvidence     `json:"observations"`
}

func (p PersonOutcome) Validate() error {
	if p.Person.Validate() != nil || p.Unwanted < 0 || p.BoundaryViolations < 0 {
		return fmt.Errorf("invalid person outcome")
	}
	switch p.DelayedOutcome {
	case "unresolved", "missing", "censored", "resolved":
	default:
		return fmt.Errorf("invalid delayed outcome disposition")
	}
	for _, q := range []core.GroupQuantity{p.Benefit, p.Burden, p.Appropriateness} {
		if q.Validate(-1, 1) != nil {
			return fmt.Errorf("invalid outcome quantity")
		}
	}
	for _, o := range p.Observations {
		if e := o.Validate(); e != nil {
			return fmt.Errorf("observation for %s: %w", p.Person, e)
		}
		if o.Person != p.Person {
			return fmt.Errorf("observation attributed to another person")
		}
	}
	return nil
}

// ArmRun is one arm executed against a matched world with its own RNG stream.
type ArmRun struct {
	Arm           Arm             `json:"arm"`
	Seed          uint64          `json:"seed"`
	WorldHash     string          `json:"world_hash"`
	ExogenousHash string          `json:"exogenous_hash"`
	RNGStream     string          `json:"rng_stream"`
	Scenario      core.ID         `json:"scenario"`
	Outcomes      []PersonOutcome `json:"outcomes"`
	HelperActs    int             `json:"helper_actions"`
}

func (a ArmRun) Validate() error {
	if !a.Arm.Valid() {
		return fmt.Errorf("unknown comparison arm")
	}
	if a.WorldHash == "" || a.ExogenousHash == "" || a.RNGStream == "" || a.Scenario.Validate() != nil {
		return fmt.Errorf("invalid arm run identity")
	}
	if len(a.Outcomes) == 0 {
		return fmt.Errorf("arm reports no affected people")
	}
	if a.Arm == NoAssistant && a.HelperActs != 0 {
		return fmt.Errorf("no-assistant arm performed helper actions")
	}
	acted := false
	seen := map[core.ID]bool{}
	for _, o := range a.Outcomes {
		if e := o.Validate(); e != nil {
			return fmt.Errorf("outcome in arm %s: %w", a.Arm, e)
		}
		if seen[o.Person] {
			return fmt.Errorf("duplicate person in arm")
		}
		seen[o.Person] = true
		acted = acted || o.Acted
	}
	// A control arm in which nobody acts is a broken control, not a result.
	if !acted {
		return fmt.Errorf("arm makes every human wait")
	}
	return nil
}

// Comparison is a matched set of arms over one scenario and seed.
type Comparison struct {
	Version  string   `json:"version"`
	Scenario core.ID  `json:"scenario"`
	Seed     uint64   `json:"seed"`
	Runs     []ArmRun `json:"runs"`
}

func (c Comparison) Validate() error {
	if c.Version != UpliftVersion || c.Scenario.Validate() != nil {
		return fmt.Errorf("invalid comparison envelope")
	}
	if len(c.Runs) != len(Arms) {
		return fmt.Errorf("comparison must run every arm")
	}
	seenArm := map[Arm]bool{}
	streams := map[string]Arm{}
	world, exogenous := "", ""
	for _, r := range c.Runs {
		if e := r.Validate(); e != nil {
			return fmt.Errorf("arm run: %w", e)
		}
		if seenArm[r.Arm] {
			return fmt.Errorf("duplicate comparison arm")
		}
		seenArm[r.Arm] = true
		if r.Scenario != c.Scenario || r.Seed != c.Seed {
			return fmt.Errorf("arm run does not belong to this comparison")
		}
		if world == "" {
			world, exogenous = r.WorldHash, r.ExogenousHash
		}
		// Matched design: identical initial world and exogenous events.
		if r.WorldHash != world || r.ExogenousHash != exogenous {
			return fmt.Errorf("arms do not share the matched initial world")
		}
		// Independent versioned RNG streams: a shared stream couples the arms.
		if other, ok := streams[r.RNGStream]; ok {
			return fmt.Errorf("arms %s and %s share an rng stream", other, r.Arm)
		}
		streams[r.RNGStream] = r.Arm
	}
	return nil
}

// UpliftFinding is the honest conclusion of a comparison. It is never
// positive by construction: absence of evidence is reported as such.
type UpliftFinding struct {
	Version       string  `json:"version"`
	Scenario      core.ID `json:"scenario"`
	Baseline      Arm     `json:"baseline"`
	Candidate     Arm     `json:"candidate"`
	Status        Status  `json:"status"`
	Evidence      string  `json:"evidence"`
	SyntheticOnly bool    `json:"synthetic_only"`
	HumanValidity Status  `json:"real_human_validity"`
}

// CompareArms compares a candidate arm against a baseline over matched
// comparisons. It reports NotTested when no independently attributed later
// evidence exists, Inconclusive when attributed evidence does not separate the
// arms, and never reports uplift from helper activity alone.
func CompareArms(cs []Comparison, baseline, candidate Arm) (UpliftFinding, error) {
	out := UpliftFinding{Version: UpliftVersion, Baseline: baseline, Candidate: candidate, Status: NotTested, SyntheticOnly: true, HumanValidity: NotTested, Evidence: "no independently attributed later evidence"}
	if !baseline.Valid() || !candidate.Valid() || baseline == candidate {
		return UpliftFinding{}, fmt.Errorf("invalid arm comparison")
	}
	if len(cs) == 0 {
		return UpliftFinding{}, fmt.Errorf("no comparisons supplied")
	}
	scenarios := map[core.ID]bool{}
	var base, cand []float64
	attributed := 0
	for _, c := range cs {
		if e := c.Validate(); e != nil {
			return UpliftFinding{}, e
		}
		scenarios[c.Scenario] = true
		for _, r := range c.Runs {
			if r.Arm != baseline && r.Arm != candidate {
				continue
			}
			for _, o := range r.Outcomes {
				for _, ob := range o.Observations {
					// Only independently attributed later evidence can support
					// an uplift claim. Assertions and prompted answers cannot.
					if ob.Tier != AttributedLater || ob.Value.Status != core.Observed {
						continue
					}
					attributed++
					if r.Arm == baseline {
						base = append(base, *ob.Value.Value)
					} else {
						cand = append(cand, *ob.Value.Value)
					}
				}
			}
		}
	}
	if len(scenarios) == 1 {
		for s := range scenarios {
			out.Scenario = s
		}
	}
	if attributed == 0 || len(base) == 0 || len(cand) == 0 {
		return out, nil
	}
	sort.Float64s(base)
	sort.Float64s(cand)
	if mean(cand) > mean(base) {
		out.Status = Pass
		out.Evidence = fmt.Sprintf("attributed later evidence separates arms (%d observations)", attributed)
		return out, nil
	}
	out.Status = Inconclusive
	out.Evidence = fmt.Sprintf("attributed later evidence does not favour %s (%d observations)", candidate, attributed)
	return out, nil
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	t := 0.0
	for _, x := range v {
		t += x
	}
	return t / float64(len(v))
}

// UpliftSummary is the compiled, machine-checkable statement of what this
// evaluation does and does not establish. Every tier here is synthetic.
type UpliftSummary struct {
	Version                     string          `json:"version"`
	SyntheticOnly               bool            `json:"synthetic_only"`
	HumanValidity               Status          `json:"real_human_validity"`
	LiveProviderSemanticQuality Status          `json:"live_provider_semantic_quality"`
	CrossModelTransfer          Status          `json:"cross_model_transfer"`
	RealThirtyDayStudy          Status          `json:"real_30_day_study"`
	BannedBenefitMetrics        []string        `json:"metrics_disqualified_as_benefit"`
	Comparisons                 int             `json:"comparisons"`
	Arms                        []Arm           `json:"arms"`
	Findings                    []UpliftFinding `json:"findings"`
	Limitations                 []string        `json:"limitations"`
}

// UpliftFixture is a bounded, deterministic synthetic comparison set. It is a
// fixture, not a study: no person in it is real and no result here transfers to
// real people.
func UpliftFixture() []Comparison {
	q := func(v float64) core.GroupQuantity { return core.ObservedGroupQuantity(v) }
	mk := func(scenario core.ID, seed uint64, later map[Arm]float64) Comparison {
		c := Comparison{Version: UpliftVersion, Scenario: scenario, Seed: seed}
		for i, a := range Arms {
			people := []PersonOutcome{}
			for p, id := range []core.ID{"person:01", "person:02"} {
				o := PersonOutcome{Person: id, Benefit: q(.1), Burden: UnknownIfAbsent(a, p), Appropriateness: q(.4),
					DelayedOutcome: []string{"resolved", "unresolved"}[p], Acted: true,
					Observations: []TierEvidence{{Person: id, Tier: BehaviouralObservation, Provenance: SyntheticProvenance, Metric: "observed_choice", Value: q(.1)}}}
				if v, ok := later[a]; ok && p == 0 {
					o.Observations = append(o.Observations, TierEvidence{Person: id, Tier: AttributedLater, Provenance: SyntheticProvenance, Metric: "reported_benefit", Value: q(v)})
				}
				people = append(people, o)
			}
			c.Runs = append(c.Runs, ArmRun{Arm: a, Seed: seed, WorldHash: "world-" + string(scenario), ExogenousHash: "exo-" + string(scenario),
				RNGStream: fmt.Sprintf("%s/%s/%d", scenario, a, i), Scenario: scenario, Outcomes: people})
		}
		return c
	}
	return []Comparison{
		// No independently attributed later evidence at all: NOT_TESTED.
		mk("scenario:ordinary", 11, nil),
		// Attributed later evidence that does NOT favour the assisted arm.
		mk("scenario:repair", 23, map[Arm]float64{NoAssistant: .6, MultiPerspective: .2}),
	}
}

// UnknownIfAbsent keeps an unobserved burden unknown rather than zero.
func UnknownIfAbsent(a Arm, person int) core.GroupQuantity {
	if a == NoAssistant || person == 1 {
		return core.UnknownGroupQuantity()
	}
	return core.ObservedGroupQuantity(.3)
}

// SummariseUplift evaluates every candidate arm against the no-assistant
// control and reports the result honestly, including when there is none.
func SummariseUplift(cs []Comparison) (UpliftSummary, error) {
	s := UpliftSummary{Version: UpliftVersion, SyntheticOnly: true, HumanValidity: NotTested,
		LiveProviderSemanticQuality: NotTested, CrossModelTransfer: NotTested, RealThirtyDayStudy: NotTested,
		Comparisons: len(cs), Arms: Arms,
		Limitations: []string{
			"Authored synthetic fixtures; no real person is represented.",
			"No live or paid provider was called; no semantic quality is measured.",
			"Engagement-style metrics are disqualified as benefit by contract, not by convention.",
			"A synthetic evidence tier never becomes real-human proof.",
		}}
	for name := range bannedBenefitMetrics {
		s.BannedBenefitMetrics = append(s.BannedBenefitMetrics, name)
	}
	sort.Strings(s.BannedBenefitMetrics)
	for _, candidate := range Arms {
		if candidate == NoAssistant {
			continue
		}
		f, e := CompareArms(cs, NoAssistant, candidate)
		if e != nil {
			return UpliftSummary{}, e
		}
		s.Findings = append(s.Findings, f)
	}
	return s, nil
}
