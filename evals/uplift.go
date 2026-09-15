package evals

import (
	"fmt"
	"sort"
	"strings"

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
	Person      core.ID      `json:"person"`
	Tier        EvidenceTier `json:"tier"`
	Provenance  string       `json:"provenance"`
	DerivedFrom EvidenceTier `json:"derived_from,omitempty"`
	// Source is the evaluator-owned record this evidence was read from, and
	// Observer is who authored it. Independently attributed later evidence may
	// not be self-declared: it must name a source record authored by the
	// subject at a stated time, so relabelling a system assertion cannot
	// manufacture a higher tier.
	Source   core.ID            `json:"source"`
	Observer core.ID            `json:"observer"`
	At       core.LogicalTime   `json:"at"`
	Metric   string             `json:"metric"`
	Value    core.GroupQuantity `json:"value"`
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
	if o.Tier == AttributedLater {
		if o.Source.Validate() != nil {
			return fmt.Errorf("attributed later evidence needs a source record")
		}
		if o.Observer != o.Person {
			return fmt.Errorf("attributed later evidence must be authored by its subject")
		}
		if o.At < 0 {
			return fmt.Errorf("attributed later evidence needs an observation time")
		}
	}
	return o.Value.Validate(-1, 1)
}

// SourceRecord is an evaluator-owned record that evidence is read FROM. A tier
// is justified by the kind of record behind it, not by what the caller labels
// it: this is what stops a system assertion being relabelled as independently
// attributed later evidence.
type SourceRecord struct {
	ID       core.ID          `json:"id"`
	Kind     string           `json:"kind"`
	Subject  core.ID          `json:"subject"`
	Observer core.ID          `json:"observer"`
	At       core.LogicalTime `json:"at"`
	About    core.ID          `json:"about"`
	Content  string           `json:"content"`
	// Arm is the run this record was collected in. A record collected in one
	// arm is not evidence about another, so evidence may not borrow it.
	Arm Arm `json:"arm"`
	// Metric and Value are the measurement AS COLLECTED. Evidence citing this
	// record must report the same measurement: a claimed value cannot change
	// while its source stays the same.
	Metric string             `json:"metric"`
	Value  core.GroupQuantity `json:"value"`
}

func sameQuantity(a, b core.GroupQuantity) bool {
	if a.Status != b.Status {
		return false
	}
	if a.Value == nil || b.Value == nil {
		return a.Value == nil && b.Value == nil
	}
	return *a.Value == *b.Value
}

// tierRequires names the source-record kind that can justify each tier.
var tierRequires = map[EvidenceTier]string{
	SystemAssertion:        "system_assertion",
	PromptedResponse:       "prompted_response",
	BehaviouralObservation: "observed_choice",
	AttributedLater:        "later_self_report",
}

func (r SourceRecord) Validate() error {
	if r.ID.Validate() != nil || r.Subject.Validate() != nil || r.Observer.Validate() != nil || r.At < 0 || r.Content == "" || r.Metric == "" || !r.Arm.Valid() {
		return fmt.Errorf("invalid source record")
	}
	known := false
	for _, k := range tierRequires {
		known = known || k == r.Kind
	}
	if !known {
		return fmt.Errorf("unknown source record kind")
	}
	// A later self-report is authored by its own subject about an earlier event.
	if r.Kind == "later_self_report" {
		if r.Observer != r.Subject {
			return fmt.Errorf("later self-report must be authored by its subject")
		}
		if r.About.Validate() != nil {
			return fmt.Errorf("later self-report must name the event it is about")
		}
	}
	return nil
}

// resolve binds one piece of evidence to the record it claims to come from and
// checks that the record actually justifies the claimed tier.
func resolve(o TierEvidence, arm Arm, sources map[core.ID]SourceRecord) error {
	rec, ok := sources[o.Source]
	if !ok {
		return fmt.Errorf("evidence cites an unresolved source record")
	}
	if rec.Kind != tierRequires[o.Tier] {
		return fmt.Errorf("source record kind %q does not justify tier %q", rec.Kind, o.Tier)
	}
	if rec.Subject != o.Person || rec.Observer != o.Observer {
		return fmt.Errorf("source record does not attribute this evidence")
	}
	if rec.At != o.At {
		return fmt.Errorf("evidence time disagrees with its source record")
	}
	// A record collected in another arm is not this arm's evidence.
	if rec.Arm != arm {
		return fmt.Errorf("evidence borrows a source record collected in arm %q", rec.Arm)
	}
	// The measurement must be the one the record actually holds.
	if rec.Metric != o.Metric || !sameQuantity(rec.Value, o.Value) {
		return fmt.Errorf("evidence measurement disagrees with its source record")
	}
	if o.Tier == AttributedLater {
		about, ok := sources[rec.About]
		if !ok {
			return fmt.Errorf("later self-report is about an unresolved event")
		}
		// The event reported on must belong to the same run. Checking only the
		// citing record's arm left borrowing possible through the event link.
		// This does not require the event's actor to be the reporter: a valid
		// third-party account of someone else's event stays possible.
		if about.Arm != arm {
			return fmt.Errorf("later self-report is about an event from arm %q", about.Arm)
		}
		if rec.At <= about.At {
			return fmt.Errorf("later self-report is not later than the event it reports")
		}
	}
	return nil
}

// PersonOutcome is one affected person's separately attributed result. Benefit
// and burden are quantities that distinguish observed zero from unknown, so an
// unobserved burden is never read as no burden.
type PersonOutcome struct {
	Person          core.ID            `json:"person"`
	Benefit         core.GroupQuantity `json:"benefit"`
	Burden          core.GroupQuantity `json:"burden"`
	Appropriateness core.GroupQuantity `json:"appropriateness"`
	// BurdenReduction is a reduction in burden: the opposite of Burden and on
	// its own scale. It is kept separate so a reduction can never be recorded
	// as a burden, or a burden inferred from its absence.
	BurdenReduction    core.GroupQuantity `json:"burden_reduction"`
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
	// "missing" and "censored" mean an instrument existed and its value did not
	// arrive, which is adverse. "not_instrumented" means this consumer has no
	// delayed-outcome instrument at all: that is ignorance, not harm, and is
	// reported as uncertainty. Collapsing the two would manufacture harm
	// findings out of a consumer that simply never measured.
	case "unresolved", "missing", "censored", "resolved", "not_instrumented":
	default:
		return fmt.Errorf("invalid delayed outcome disposition")
	}
	for _, q := range []core.GroupQuantity{p.Benefit, p.Burden, p.Appropriateness} {
		if q.Validate(-1, 1) != nil {
			return fmt.Errorf("invalid outcome quantity")
		}
	}
	// BurdenReduction carries the source scale of the record it came from
	// (core.OrdinaryExperience allows -100..100). Forcing it into the -1..1
	// outcome range rejected values the source contract considers valid.
	if p.BurdenReduction.Validate(MinBurdenReduction, MaxBurdenReduction) != nil {
		return fmt.Errorf("invalid burden reduction quantity")
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

// Stream is one versioned RNG stream a run drew from, named by the domain it
// serves. See ArmRun.Streams for why independence is per domain, not per arm.
type Stream struct {
	Domain string `json:"domain"`
	Seed   string `json:"seed"`
}

type ArmRun struct {
	Arm  Arm    `json:"arm"`
	Seed uint64 `json:"seed"`
	// WorldHash and ExogenousHash are receipts of the generation this arm
	// actually ran on, and are shared across arms by design: a matched
	// comparison uses the same initial world and the same exogenous events, so
	// the assistant policy is the only difference.
	WorldHash     string `json:"world_hash"`
	ExogenousHash string `json:"exogenous_hash"`
	// Streams are the versioned RNG streams this arm drew from. #58 requires
	// them to be independent AND the worlds and exogenous events to be matched,
	// which are only compatible under one reading: independence is across
	// DOMAINS — human decisions, exogenous events and the helper draw from
	// separate versioned streams so one cannot perturb another — while the
	// streams themselves are identical across arms. Splitting them per arm
	// would give each arm different exogenous events, contradicting the
	// matching in the same sentence. The repository's own generator settles it:
	// NewAssistanceManifest derives its human, exogenous and helper seeds
	// without reference to the arm.
	Streams []Stream `json:"rng_streams"`
	// PolicyHash is the receipt of what this arm's policy actually produced. It
	// is the one identifier that may differ between arms, and when two arms
	// share it they did the same thing: no uplift can be read between them.
	PolicyHash string          `json:"policy_hash"`
	Scenario   core.ID         `json:"scenario"`
	Outcomes   []PersonOutcome `json:"outcomes"`
	HelperActs int             `json:"helper_actions"`
}

func (a ArmRun) Validate() error {
	if !a.Arm.Valid() {
		return fmt.Errorf("unknown comparison arm")
	}
	if a.WorldHash == "" || a.ExogenousHash == "" || a.PolicyHash == "" || a.Scenario.Validate() != nil {
		return fmt.Errorf("invalid arm run identity")
	}
	if len(a.Streams) == 0 {
		return fmt.Errorf("arm records no rng stream")
	}
	domains, seeds := map[string]bool{}, map[string]bool{}
	for _, st := range a.Streams {
		if st.Domain == "" || st.Seed == "" {
			return fmt.Errorf("invalid rng stream receipt")
		}
		if domains[st.Domain] {
			return fmt.Errorf("duplicate rng stream domain %q", st.Domain)
		}
		// Two domains drawing the identical seed are one stream under two
		// names, which is the coupling the independence requirement forbids.
		if seeds[st.Seed] {
			return fmt.Errorf("rng stream domain %q is not independent of another", st.Domain)
		}
		domains[st.Domain], seeds[st.Seed] = true, true
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
	Version  string  `json:"version"`
	Scenario core.ID `json:"scenario"`
	// Family is the required #58 scenario family this comparison executes. It
	// is declared explicitly and checked against RequiredFamilies, never
	// inferred from the scenario name: a scenario whose name happens to
	// contain a family string is not evidence that the family was executed.
	Family string `json:"family"`
	Seed   uint64 `json:"seed"`
	// Affected is defined independently of any arm. Every arm must report an
	// outcome for exactly these people, so a third party's cost cannot be
	// dropped from one arm and vanish from the comparison.
	Affected []core.ID `json:"affected"`
	// Sources is the evaluator-owned ledger every piece of evidence must resolve
	// against. Evidence that cites nothing real is not evidence.
	Sources []SourceRecord `json:"sources"`
	Runs    []ArmRun       `json:"runs"`
}

func (c Comparison) Validate() error {
	if c.Version != UpliftVersion || c.Scenario.Validate() != nil || len(c.Affected) < 2 {
		return fmt.Errorf("invalid comparison envelope")
	}
	if !RequiredFamily(c.Family) {
		return fmt.Errorf("comparison does not declare a required scenario family")
	}
	sources := map[core.ID]SourceRecord{}
	for _, rec := range c.Sources {
		if e := rec.Validate(); e != nil {
			return fmt.Errorf("source ledger: %w", e)
		}
		if _, dup := sources[rec.ID]; dup {
			return fmt.Errorf("duplicate source record")
		}
		sources[rec.ID] = rec
	}
	roster := map[core.ID]bool{}
	for _, id := range c.Affected {
		if id.Validate() != nil || roster[id] {
			return fmt.Errorf("invalid affected roster")
		}
		roster[id] = true
	}
	// Arms that were actually executed. A comparison must carry the
	// no-assistant control and at least one candidate; an arm with no
	// implementation is reported as not run rather than fabricated.
	if len(c.Runs) < 2 || len(c.Runs) > len(Arms) {
		return fmt.Errorf("comparison must run the control and at least one candidate")
	}
	control := false
	for _, r := range c.Runs {
		control = control || r.Arm == NoAssistant
	}
	if !control {
		return fmt.Errorf("comparison must include the no-assistant control")
	}
	seenArm := map[Arm]bool{}
	world, exogenous, streams := "", "", ""
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
		// Every arm accounts for every affected person, including explicitly
		// missing outcomes. Omission is not permitted.
		if len(r.Outcomes) != len(c.Affected) {
			return fmt.Errorf("arm omits an affected person")
		}
		for _, o := range r.Outcomes {
			if !roster[o.Person] {
				return fmt.Errorf("arm reports a person outside the affected roster")
			}
			for _, ob := range o.Observations {
				if e := resolve(ob, r.Arm, sources); e != nil {
					return fmt.Errorf("evidence for %s in arm %s: %w", o.Person, r.Arm, e)
				}
			}
		}
		if world == "" {
			world, exogenous, streams = r.WorldHash, r.ExogenousHash, streamKey(r.Streams)
		}
		// Matched design: identical initial world and exogenous events.
		if r.WorldHash != world || r.ExogenousHash != exogenous {
			return fmt.Errorf("arms do not share the matched initial world")
		}
		// Common random numbers. An earlier version of this contract required
		// the opposite — a distinct stream per arm — which is wrong for a
		// matched design and was satisfiable only by labelling the streams
		// differently. Drawing different numbers per arm reintroduces exactly
		// the variance the matching exists to remove, so an arm that does not
		// share every stream is not a matched arm.
		if streamKey(r.Streams) != streams {
			return fmt.Errorf("arms do not share the matched rng streams")
		}
	}
	return nil
}

// UpliftFinding is the honest conclusion of a comparison. It is never
// positive by construction: absence of evidence is reported as such.
type UpliftFinding struct {
	Version   string   `json:"version"`
	Scenario  core.ID  `json:"scenario"`
	Baseline  Arm      `json:"baseline"`
	Candidate Arm      `json:"candidate"`
	Status    Status   `json:"status"`
	Evidence  string   `json:"evidence"`
	Harms     []string `json:"harm_qualifications,omitempty"`
	// Uncertainty names what was not measured. It qualifies every finding,
	// including not-tested ones, so absence of evidence stays visible.
	Uncertainty []string `json:"uncertainty,omitempty"`
	// Margin is the observed spread of per-person paired margins, clustered by
	// independent world unit. It is descriptive over a small bounded fixture,
	// not a calibrated confidence interval, and says so in its method.
	Margin        *Interval `json:"margin,omitempty"`
	SyntheticOnly bool      `json:"synthetic_only"`
	HumanValidity Status    `json:"real_human_validity"`
}

// Burden reduction is reported on its source scale, not the -1..1 outcome scale.
const (
	MinBurdenReduction = -100
	MaxBurdenReduction = 100
)

// MinIndependentUnits is the smallest number of independent scenario/world
// units that may support any reported difference between arms.
const MinIndependentUnits = 2

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
	// The estimand is matched within a world unit AND within a person. A world
	// is identified by what generated it, never by its label; a pair exists only
	// where the SAME person was actually observed in both arms. Different
	// people's levels are not a per-person improvement.
	type pair struct{ base, cand []float64 }
	units := map[string]map[core.ID]*pair{}
	harms, unknowns := []string{}, []string{}
	// identical names the scenarios in which the two arms produced the same
	// policy output; identicalCount and compared count comparisons, not
	// scenario names, so a scenario run at several seeds is counted once per
	// seed on both sides.
	identical := []string{}
	identicalCount, compared := 0, 0
	attributed := 0
	for _, c := range cs {
		if e := c.Validate(); e != nil {
			return UpliftFinding{}, e
		}
		scenarios[c.Scenario] = true
		present := map[Arm]bool{}
		for _, r := range c.Runs {
			present[r.Arm] = true
		}
		if !present[baseline] || !present[candidate] {
			continue
		}
		// Two arms that produced the same policy output did the same thing in
		// this scenario. Whatever the levels say, the difference between them
		// is not attributable to the assistant policy, so it is recorded as an
		// identical-arm scenario and can never be read as uplift.
		policy := map[Arm]string{}
		for _, r := range c.Runs {
			if r.Arm == baseline || r.Arm == candidate {
				policy[r.Arm] = r.PolicyHash
			}
		}
		if policy[baseline] == policy[candidate] {
			identical = append(identical, string(c.Scenario))
			identicalCount++
		}
		compared++
		for _, r := range c.Runs {
			if r.Arm != baseline && r.Arm != candidate {
				continue
			}
			key := r.WorldHash + "\x00" + r.ExogenousHash
			if units[key] == nil {
				units[key] = map[core.ID]*pair{}
			}
			for _, o := range r.Outcomes {
				if units[key][o.Person] == nil {
					units[key][o.Person] = &pair{}
				}
				if r.Arm == candidate {
					if o.BoundaryViolations > 0 {
						harms = append(harms, fmt.Sprintf("%s: %d boundary violation(s)", o.Person, o.BoundaryViolations))
					}
					if o.Unwanted > 0 {
						harms = append(harms, fmt.Sprintf("%s: %d unwanted intervention(s)", o.Person, o.Unwanted))
					}
					if o.Benefit.Status == core.Observed && o.Benefit.Value != nil && *o.Benefit.Value < 0 {
						harms = append(harms, fmt.Sprintf("%s: observed negative benefit", o.Person))
					}
					if o.Burden.Status == core.Observed && o.Burden.Value != nil && *o.Burden.Value > 0 {
						harms = append(harms, fmt.Sprintf("%s: observed burden", o.Person))
					}
					if o.DelayedOutcome == "missing" || o.DelayedOutcome == "censored" {
						harms = append(harms, fmt.Sprintf("%s: %s outcome", o.Person, o.DelayedOutcome))
					}
					if o.DelayedOutcome == "unresolved" {
						unknowns = append(unknowns, fmt.Sprintf("%s: outcome unresolved", o.Person))
					}
					if o.DelayedOutcome == "not_instrumented" {
						unknowns = append(unknowns, fmt.Sprintf("%s: this consumer has no delayed-outcome instrument", o.Person))
					}
					if o.Benefit.Status != core.Observed {
						unknowns = append(unknowns, fmt.Sprintf("%s: benefit not observed", o.Person))
					}
					if o.Burden.Status != core.Observed {
						unknowns = append(unknowns, fmt.Sprintf("%s: burden not observed", o.Person))
					}
				}
				for _, ob := range o.Observations {
					if ob.Tier != AttributedLater || ob.Value.Status != core.Observed {
						continue
					}
					attributed++
					if r.Arm == baseline {
						units[key][o.Person].base = append(units[key][o.Person].base, *ob.Value.Value)
					} else {
						units[key][o.Person].cand = append(units[key][o.Person].cand, *ob.Value.Value)
					}
				}
			}
		}
	}
	if len(scenarios) == 1 {
		for sc := range scenarios {
			out.Scenario = sc
		}
	}
	// Qualifications are attached before any decision, so a refused or
	// not-tested result still reports what was observed and what was not.
	sort.Strings(harms)
	// An arm pair that ran identically is stated as uncertainty even when the
	// result is not-tested for some other reason, so the reader is never left
	// to assume the two arms actually differed.
	sort.Strings(identical)
	for _, sc := range dedupe(identical) {
		unknowns = append(unknowns, fmt.Sprintf("%s and %s produced identical policy output in %s", baseline, candidate, sc))
	}
	sort.Strings(unknowns)
	out.Harms = dedupe(harms)
	out.Uncertainty = dedupe(unknowns)

	pairedUnits, pairedPeople, oneArmed := 0, 0, 0
	margins := []float64{}
	for _, people := range units {
		unitHasPair := false
		for _, pr := range people {
			switch {
			case len(pr.base) > 0 && len(pr.cand) > 0:
				pairedPeople++
				unitHasPair = true
				sort.Float64s(pr.base)
				sort.Float64s(pr.cand)
				margins = append(margins, pr.cand[0]-pr.base[len(pr.base)-1])
			case len(pr.base) > 0 || len(pr.cand) > 0:
				oneArmed++
			}
		}
		if unitHasPair {
			pairedUnits++
		}
	}
	if pairedPeople == 0 {
		out.Status = NotTested
		out.Evidence = fmt.Sprintf("no person was observed in both arms (%d one-armed person-observation(s), %d attributed observation(s))", oneArmed, attributed)
		return out, nil
	}
	if len(harms) > 0 {
		out.Status = Inconclusive
		out.Evidence = fmt.Sprintf("%s shows harm or unobservable outcomes; no uplift may be reported (%s)", candidate, strings.Join(out.Harms, "; "))
		return out, nil
	}
	// Every scenario in which both arms ran produced the same policy output, so
	// the two arms are the same intervention under two labels. Any difference
	// in the levels is noise, and no uplift is attributable to the policy.
	if compared > 0 && identicalCount == compared {
		out.Status = NotTested
		out.Evidence = fmt.Sprintf("%s and %s produced identical policy output in every scenario compared (%d); there is no policy difference to attribute an effect to", baseline, candidate, compared)
		return out, nil
	}
	if oneArmed > 0 {
		out.Status = Inconclusive
		out.Evidence = fmt.Sprintf("%d person-observation(s) exist in only one arm; missingness is not evidence of a difference", oneArmed)
		return out, nil
	}
	// Uncertainty is computed AT the independent world unit, not over pooled
	// per-person margins: each unit contributes one summary, and the spread is
	// taken across those unit summaries. Pooling people would treat several
	// measurements of one world as independent evidence.
	unitMargins := []float64{}
	favouring, against := 0, 0
	for _, people := range units {
		inUnit := []float64{}
		for _, pr := range people {
			if len(pr.base) == 0 || len(pr.cand) == 0 {
				continue
			}
			sort.Float64s(pr.base)
			sort.Float64s(pr.cand)
			inUnit = append(inUnit, pr.cand[0]-pr.base[len(pr.base)-1])
		}
		if len(inUnit) == 0 {
			continue
		}
		sort.Float64s(inUnit)
		// One unit, one summary: its weakest per-person margin, so a unit cannot
		// be carried by its most favourable person.
		unitMargins = append(unitMargins, inUnit[0])
		if inUnit[0] > 0 {
			favouring++
		} else {
			against++
		}
	}
	sort.Float64s(unitMargins)
	sort.Float64s(margins)
	out.Margin = &Interval{Low: unitMargins[0], High: unitMargins[len(unitMargins)-1], Units: pairedUnits,
		Method: fmt.Sprintf("spread of per-unit summaries across %d independent world unit(s); each unit contributes its weakest per-person paired margin; %d unit(s) favour %s, %d do not; descriptive over a bounded fixture, not a calibrated interval", pairedUnits, favouring, candidate, against)}
	// A difference is reported only when EVERY independent unit agrees. One unit
	// pointing the other way is disagreement between worlds, not uplift.
	if against > 0 {
		out.Status = Inconclusive
		out.Evidence = fmt.Sprintf("independent world units disagree: %d favour %s, %d do not (%d observations); no uplift established", favouring, candidate, against, attributed)
		return out, nil
	}
	if pairedUnits < MinIndependentUnits {
		out.Status = Inconclusive
		out.Evidence = fmt.Sprintf("%d independent world unit(s) with a paired person; at least %d are required before any difference is reported", pairedUnits, MinIndependentUnits)
		return out, nil
	}
	if margins[0] <= 0 {
		out.Status = Inconclusive
		out.Evidence = fmt.Sprintf("per-person paired margins overlap or disagree across %d unit(s) (%d pairs, %d observations); no uplift established", pairedUnits, pairedPeople, attributed)
		return out, nil
	}
	out.Status = Pass
	out.Evidence = fmt.Sprintf("every one of %d per-person pairs separates without overlap across %d independent world units (%d observations)", pairedPeople, pairedUnits, attributed)
	return out, nil
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
	ArmsExecuted                []Arm           `json:"arms_executed"`
	Findings                    []UpliftFinding `json:"findings"`
	// People carries the per-person evidence the findings are qualified by, so
	// burden, unwanted interventions, boundary violations and unobservable
	// outcomes survive into the emitted result instead of being summarised away.
	People           []PersonRecord     `json:"per_person"`
	ScenarioCoverage []ScenarioCoverage `json:"scenario_coverage"`
	// Executed is the frozen manifest of arms that actually ran. Coverage is
	// derived from it, so a reader can audit the claim rather than trust it.
	Executed    []ExecutedUnit `json:"executed_manifest"`
	Limitations []string       `json:"limitations"`
}

// PersonRecord is one person's result in one arm of one comparison.
type PersonRecord struct {
	Scenario           core.ID            `json:"scenario"`
	Seed               uint64             `json:"seed"`
	WorldHash          string             `json:"world_hash"`
	Arm                Arm                `json:"arm"`
	Person             core.ID            `json:"person"`
	Benefit            core.GroupQuantity `json:"benefit"`
	Burden             core.GroupQuantity `json:"burden"`
	BurdenReduction    core.GroupQuantity `json:"burden_reduction"`
	Appropriateness    core.GroupQuantity `json:"appropriateness"`
	Unwanted           int                `json:"unwanted_interventions"`
	BoundaryViolations int                `json:"boundary_violations"`
	DelayedOutcome     string             `json:"delayed_outcome"`
	Acted              bool               `json:"person_acted"`
	Evidence           []EvidenceRef      `json:"evidence"`
}

// EvidenceRef carries one measurement as it was recorded: the tier and source
// record behind it, and the measured value and status themselves. Exporting
// only provenance metadata would make an unknown or discordant later report
// serialize identically to an observed one, so a report consumer could not
// audit the data a comparison actually used.
type EvidenceRef struct {
	Tier   EvidenceTier       `json:"tier"`
	Source core.ID            `json:"source"`
	Metric string             `json:"metric"`
	At     core.LogicalTime   `json:"at"`
	Value  core.GroupQuantity `json:"value"`
	Status core.OutcomeStatus `json:"status"`
}

// ScenarioCoverage states, per required #58 scenario family, whether this
// evaluation actually executes it. A family that is not executed is reported
// as not covered rather than silently omitted.
type ScenarioCoverage struct {
	Family  string `json:"family"`
	Covered bool   `json:"covered"`
	Note    string `json:"note"`
}

// UpliftFixture is a bounded, deterministic synthetic comparison set. It is a
// fixture, not a study: no person in it is real and no result here transfers to
// real people.
func UpliftFixture() []Comparison {
	q := func(v float64) core.GroupQuantity { return core.ObservedGroupQuantity(v) }
	mk := func(scenario core.ID, family string, seed uint64, later map[Arm]float64) Comparison {
		c := Comparison{Version: UpliftVersion, Scenario: scenario, Family: family, Seed: seed, Affected: []core.ID{"person:01", "person:02"}}
		for _, a := range Arms {
			for _, id := range []core.ID{"person:01", "person:02"} {
				c.Sources = append(c.Sources, SourceRecord{ID: core.ID("event:" + string(scenario) + ":" + string(a) + ":" + string(id)),
					Kind: "observed_choice", Subject: id, Observer: id, At: 0, Arm: a,
					Metric: "observed_choice", Value: q(.1),
					Content: "the fixture event later reports are about"})
			}
		}
		for i, a := range Arms {
			people := []PersonOutcome{}
			for p, id := range []core.ID{"person:01", "person:02"} {
				o := PersonOutcome{Person: id, Benefit: q(.1), Burden: UnknownIfAbsent(a, p), Appropriateness: q(.4),
					BurdenReduction: core.UnknownGroupQuantity(),
					DelayedOutcome:  []string{"resolved", "unresolved"}[p], Acted: true,
					Observations: []TierEvidence{{Person: id, Tier: BehaviouralObservation, Provenance: SyntheticProvenance,
						Source: core.ID("event:" + string(scenario) + ":" + string(a) + ":" + string(id)), Observer: id, At: 0, Metric: "observed_choice", Value: q(.1)}}}
				if v, ok := later[a]; ok && p == 0 {
					o.Observations = append(o.Observations, TierEvidence{Person: id, Tier: AttributedLater, Provenance: SyntheticProvenance,
						Source: core.ID(fmt.Sprintf("experience:%s:%s", scenario, a)), Observer: id, At: 5, Metric: "reported_benefit", Value: q(v)})
					c.Sources = append(c.Sources, SourceRecord{ID: core.ID(fmt.Sprintf("experience:%s:%s", scenario, a)),
						Kind: "later_self_report", Subject: id, Observer: id, At: 5, Arm: a,
						Metric: "reported_benefit", Value: q(v),
						About: core.ID("event:" + string(scenario) + ":" + string(a) + ":" + string(id)), Content: "synthetic later self-report"})
				}
				people = append(people, o)
			}
			// Matched design: world, exogenous events and every rng stream are
			// shared across arms; only the policy receipt differs.
			c.Runs = append(c.Runs, ArmRun{Arm: a, Seed: seed, WorldHash: "world-" + string(scenario), ExogenousHash: "exo-" + string(scenario),
				Streams: []Stream{
					{Domain: "human", Seed: fmt.Sprintf("human/%s/%d", scenario, seed)},
					{Domain: "exogenous", Seed: fmt.Sprintf("exogenous/%s/%d", scenario, seed)},
					{Domain: "helper", Seed: fmt.Sprintf("helper/%s/%d", scenario, seed)},
				},
				PolicyHash: fmt.Sprintf("policy-%s-%d", a, i),
				Scenario:   scenario, Outcomes: people})
		}
		return c
	}
	return []Comparison{
		// No independently attributed later evidence at all: NOT_TESTED.
		mk("scenario:ordinary_joy", "ordinary_joy", 11, nil),
		// Attributed later evidence that does NOT favour the assisted arm.
		mk("scenario:repair", "repair", 23, map[Arm]float64{NoAssistant: .6, MultiPerspective: .2}),
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
	// The ticket requires comparison against simple assistance, not only against
	// the no-assistant control: a policy must beat doing something simple, not
	// merely beat doing nothing.
	executed := map[Arm]bool{}
	for _, c := range cs {
		for _, r := range c.Runs {
			executed[r.Arm] = true
		}
	}
	s.ArmsExecuted = []Arm{}
	for _, a := range Arms {
		if executed[a] {
			s.ArmsExecuted = append(s.ArmsExecuted, a)
		}
	}
	for _, baseline := range []Arm{NoAssistant, SimpleAssistance} {
		for _, candidate := range Arms {
			if candidate == baseline || (baseline == SimpleAssistance && candidate == NoAssistant) {
				continue
			}
			if !executed[baseline] || !executed[candidate] {
				s.Findings = append(s.Findings, UpliftFinding{Version: UpliftVersion, Baseline: baseline, Candidate: candidate,
					Status: NotTested, SyntheticOnly: true, HumanValidity: NotTested,
					Evidence: "arm not executed in this run; no implementation was fabricated for it"})
				continue
			}
			f, e := CompareArms(cs, baseline, candidate)
			if e != nil {
				return UpliftSummary{}, e
			}
			s.Findings = append(s.Findings, f)
		}
	}
	for _, c := range cs {
		for _, r := range c.Runs {
			for _, o := range r.Outcomes {
				refs := []EvidenceRef{}
				for _, ob := range o.Observations {
					refs = append(refs, EvidenceRef{Tier: ob.Tier, Source: ob.Source, Metric: ob.Metric,
						At: ob.At, Value: ob.Value, Status: ob.Value.Status})
				}
				s.People = append(s.People, PersonRecord{Scenario: c.Scenario, Seed: c.Seed, WorldHash: r.WorldHash,
					Arm: r.Arm, Person: o.Person, Benefit: o.Benefit, Burden: o.Burden,
					BurdenReduction: o.BurdenReduction, Appropriateness: o.Appropriateness,
					Unwanted: o.Unwanted, BoundaryViolations: o.BoundaryViolations,
					DelayedOutcome: o.DelayedOutcome, Acted: o.Acted, Evidence: refs})
			}
		}
	}
	s.Executed = ExecutedManifest(cs)
	s.ScenarioCoverage = coverage(cs)
	return s, nil
}

// RequiredFamilies are the #58 scenario families the manifest must account for.
var RequiredFamilies = []string{
	"ordinary_joy", "wanted_unwanted_help", "selective_boundaries",
	"conflict_goals", "role_domain_trust", "life_changes", "repair", "group_burden",
}

// RequiredFamily reports whether name is one of the required #58 families.
// Matching is exact: substring containment is not membership.
func RequiredFamily(name string) bool {
	for _, f := range RequiredFamilies {
		if f == name {
			return true
		}
	}
	return false
}

// ExecutedUnit is one arm of one comparison that actually ran and produced
// outcomes for real people in the roster. The manifest is assembled from these
// units alone, so nothing enters coverage by being named.
type ExecutedUnit struct {
	Family   string  `json:"family"`
	Scenario core.ID `json:"scenario"`
	Seed     uint64  `json:"seed"`
	Arm      Arm     `json:"arm"`
	People   int     `json:"people_with_outcomes"`
	// PolicyHash lets a reader recompute which arms actually did the same
	// thing, instead of taking the findings' word for it, and Streams lets
	// them recheck that the arms were matched and the domains independent.
	PolicyHash string   `json:"policy_hash"`
	Streams    []Stream `json:"rng_streams"`
}

// ExecutedManifest is the frozen record of what this run actually executed,
// in a stable order. It is the only thing coverage is computed from.
func ExecutedManifest(cs []Comparison) []ExecutedUnit {
	out := []ExecutedUnit{}
	for _, c := range cs {
		for _, r := range c.Runs {
			if len(r.Outcomes) == 0 {
				continue
			}
			out = append(out, ExecutedUnit{Family: c.Family, Scenario: c.Scenario,
				Seed: c.Seed, Arm: r.Arm, People: len(r.Outcomes), PolicyHash: r.PolicyHash,
				Streams: r.Streams})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Family != out[j].Family {
			return out[i].Family < out[j].Family
		}
		if out[i].Scenario != out[j].Scenario {
			return out[i].Scenario < out[j].Scenario
		}
		if out[i].Seed != out[j].Seed {
			return out[i].Seed < out[j].Seed
		}
		return out[i].Arm < out[j].Arm
	})
	return out
}

// coverage credits a family only when the executed manifest contains a real
// comparison for it: the no-assistant control AND at least one candidate arm,
// both with outcomes, within a single scenario and seed. A lone arm is not a
// comparison, and a family named by a scenario string that executed nothing is
// reported NOT COVERED.
func coverage(cs []Comparison) []ScenarioCoverage {
	type unit struct {
		scenario core.ID
		seed     uint64
	}
	control := map[string]map[unit]bool{}
	candidate := map[string]map[unit]bool{}
	// Families outside RequiredFamilies need no filter here: the report loop
	// below iterates RequiredFamilies, so an unrecognised family is never read.
	for _, u := range ExecutedManifest(cs) {
		side := candidate
		if u.Arm == NoAssistant {
			side = control
		}
		if side[u.Family] == nil {
			side[u.Family] = map[unit]bool{}
		}
		side[u.Family][unit{u.Scenario, u.Seed}] = true
	}
	out := []ScenarioCoverage{}
	for _, f := range RequiredFamilies {
		covered, arms := false, 0
		for k := range control[f] {
			if candidate[f][k] {
				covered = true
			}
		}
		for range candidate[f] {
			arms++
		}
		note := "executed through the real consumer path: control and at least one candidate arm produced outcomes"
		switch {
		case covered:
		case arms > 0:
			note = "NOT COVERED: candidate arms executed but no matched no-assistant control in the same scenario and seed"
		default:
			note = "NOT COVERED: no executed comparison for this family in this run"
		}
		out = append(out, ScenarioCoverage{Family: f, Covered: covered, Note: note})
	}
	return out
}

// streamKey is the order-independent identity of a run's stream set, so two
// arms that drew the same streams in a different order still match.
func streamKey(in []Stream) string {
	parts := []string{}
	for _, s := range in {
		parts = append(parts, s.Domain+"="+s.Seed)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
