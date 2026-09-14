package evals

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"time"

	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/drives"
	"github.com/tushardhara/dream/simulator/experiment"
)

const AlignmentReportVersion = "hws-alignment-report.v1"
const AlignmentPlanVersion = "hws-alignment-plan.v1"
const alignmentHypothesis = "Observer-owned relationship inputs affect the bounded synthetic action/drive transition; labels alone do not. Identical frozen inputs reproduce the transition."
const alignmentRule = "relationship-mechanics.v1: pairwise Ask probability differences >1e-6; exact label invariance; expectation/history/stress sensitivity; hostile-spouse/supportive-acquaintance reversal; unknown permits Ask/WAIT but not Reveal; appraisal reaches drives; compare selected actions across seeds; compare all repeated trial bytes. These are engineering checks, not behavioral adequacy tests."
const alignmentScope = "Synthetic #42 fixture only: 16 controlled one-step action/drive-state trajectories per seed. No longitudinal, human, cross-model, intervention or real-time study inference."

type AlignmentPlan struct {
	Version             string                              `json:"version"`
	SourceRevision      string                              `json:"source_revision"`
	SourceTree          string                              `json:"source_tree"`
	SourceExtractSHA256 string                              `json:"source_extract_sha256"`
	EvaluatorArtifact   string                              `json:"evaluator_artifact_sha256"`
	GeneratorArtifact   string                              `json:"generator_artifact"`
	Model               string                              `json:"model"`
	DatasetHash         string                              `json:"dataset_hash"`
	RegistryVersion     string                              `json:"registry_version"`
	RegistryHash        string                              `json:"registry_hash"`
	Probe               experiment.RelationshipProbeRequest `json:"probe"`
	Hypothesis          string                              `json:"hypothesis"`
	DecisionRule        string                              `json:"decision_rule"`
	EvidenceScope       string                              `json:"evidence_scope"`
	RegisteredAt        time.Time                           `json:"registered_at"`
}

func NewAlignmentPlan(datasetHash, revision, tree, evaluatorArtifact, generatorArtifact string, seeds []uint64, registeredAt time.Time) (AlignmentPlan, error) {
	p := AlignmentPlan{AlignmentPlanVersion, revision, tree, AlignmentSourceSHA256, evaluatorArtifact, generatorArtifact, experiment.RelationshipExperimentVersion + "/" + behavior.ActionPolicy + "/" + drives.ModelVersion, datasetHash, AlignmentRegistryVersion, AlignmentRegistryHash(), experiment.RelationshipProbeRequest{Version: experiment.RelationshipExperimentVersion, FixtureHash: experiment.RelationshipFixtureHash(), Seeds: append([]uint64{}, seeds...)}, alignmentHypothesis, alignmentRule, alignmentScope, registeredAt.UTC()}
	return p, p.Validate()
}
func (p AlignmentPlan) Validate() error {
	hash := regexp.MustCompile(`^[0-9a-f]{64}$`)
	gitHash := regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
	if p.Version != AlignmentPlanVersion || !gitHash.MatchString(p.SourceRevision) || !gitHash.MatchString(p.SourceTree) || p.SourceExtractSHA256 != AlignmentSourceSHA256 || !hash.MatchString(p.EvaluatorArtifact) || !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(p.GeneratorArtifact) || p.Model != experiment.RelationshipExperimentVersion+"/"+behavior.ActionPolicy+"/"+drives.ModelVersion || !hash.MatchString(p.DatasetHash) || p.RegistryVersion != AlignmentRegistryVersion || p.RegistryHash != AlignmentRegistryHash() || p.Probe.Validate() != nil || p.Hypothesis != alignmentHypothesis || p.DecisionRule != alignmentRule || p.EvidenceScope != alignmentScope || p.RegisteredAt.IsZero() || p.RegisteredAt.Location() != time.UTC {
		return fmt.Errorf("invalid frozen alignment protocol")
	}
	return nil
}
func (p AlignmentPlan) Hash() string { h, _ := Digest(p); return h }

type AlignmentFinding struct {
	ID           string          `json:"id"`
	Status       AlignmentStatus `json:"status"`
	Scope        string          `json:"scope"`
	EvidenceHash string          `json:"evidence_hash"`
	Explanation  string          `json:"explanation"`
	// Three-valued enum, never a truthy interpretation of a "passing falsifier".
	FalsifierTriggered string `json:"falsifier_triggered,omitempty"`
}
type MechanicsFinding struct {
	Name        string          `json:"name"`
	Status      AlignmentStatus `json:"status"`
	Explanation string          `json:"explanation"`
}
type AlignmentEvidence struct {
	First  *experiment.RelationshipProbe `json:"first,omitempty"`
	Repeat *experiment.RelationshipProbe `json:"repeat,omitempty"`
}
type AlignmentReport struct {
	Version            string             `json:"version"`
	Plan               AlignmentPlan      `json:"plan"`
	PlanHash           string             `json:"plan_hash"`
	Registry           []SourceCriterion  `json:"registry"`
	Evidence           AlignmentEvidence  `json:"evidence"`
	EvidenceHash       string             `json:"evidence_hash"`
	Findings           []AlignmentFinding `json:"findings"`
	Mechanics          []MechanicsFinding `json:"engineering_mechanics"`
	SyntheticOnly      bool               `json:"synthetic_only"`
	HumanValidity      AlignmentStatus    `json:"real_human_validity"`
	CrossModelTransfer AlignmentStatus    `json:"cross_model_transfer"`
	RealStudy          string             `json:"real_30_day_study"`
	Hash               string             `json:"hash"`
}

// Receipt time is outside the logical report and its hash. The same frozen plan
// can reproduce identical report bytes while retaining both actual run times.
type AlignmentReceipt struct {
	Version           string    `json:"version"`
	PlanHash          string    `json:"plan_hash"`
	ReportHash        string    `json:"report_hash"`
	EvaluatorArtifact string    `json:"evaluator_artifact_sha256"`
	EvaluatedAt       time.Time `json:"evaluated_at"`
}

func (r AlignmentReceipt) Verify(report AlignmentReport) error {
	if r.Version != "hws-alignment-receipt.v1" || r.PlanHash != report.PlanHash || r.ReportHash != report.Hash || r.EvaluatorArtifact != report.Plan.EvaluatorArtifact || r.EvaluatedAt.IsZero() || r.EvaluatedAt.Before(report.Plan.RegisteredAt) || r.EvaluatedAt.Location() != time.UTC {
		return fmt.Errorf("invalid evaluator execution receipt")
	}
	return nil
}

// The generator port has no dataset, labels, hypothesis or scoring-rule field.
type RelationshipGenerator interface {
	ProbeRelationships(context.Context, experiment.RelationshipProbeRequest) (experiment.RelationshipProbe, error)
}

func RunAlignment(ctx context.Context, dataset Dataset, plan AlignmentPlan, g RelationshipGenerator, now time.Time) (AlignmentReport, AlignmentReceipt, error) {
	if plan.Validate() != nil || dataset.Validate(now) != nil || dataset.Hash != plan.DatasetHash || now.Before(plan.RegisteredAt) {
		return AlignmentReport{}, AlignmentReceipt{}, fmt.Errorf("alignment preregistration/dataset binding")
	}
	var evidence AlignmentEvidence
	// Detach before each independent invocation: a generator cannot mutate the
	// evaluator's frozen plan or acquire a private dataset through an alias.
	generate := func() (*experiment.RelationshipProbe, error) {
		raw, _ := json.Marshal(plan.Probe)
		var request experiment.RelationshipProbeRequest
		_ = json.Unmarshal(raw, &request)
		p, e := g.ProbeRelationships(ctx, request)
		if e != nil {
			return nil, e
		}
		if e = p.Validate(plan.Probe); e != nil {
			return nil, e
		}
		return &p, nil
	}
	var e error
	if evidence.First, e = generate(); e != nil {
		return AlignmentReport{}, AlignmentReceipt{}, e
	}
	if evidence.Repeat, e = generate(); e != nil {
		return AlignmentReport{}, AlignmentReceipt{}, e
	}
	report, e := BuildAlignmentReport(plan, evidence)
	if e != nil {
		return report, AlignmentReceipt{}, e
	}
	receipt := AlignmentReceipt{"hws-alignment-receipt.v1", report.PlanHash, report.Hash, plan.EvaluatorArtifact, now.UTC()}
	return report, receipt, report.Verify(plan, dataset, receipt)
}

// BuildAlignmentReport can also record an unrun or insufficient experiment.
// Missing evidence never defaults to PASS. Validity is structural/reproducible
// evidence integrity, not cryptographic proof that an external study occurred.
func BuildAlignmentReport(plan AlignmentPlan, evidence AlignmentEvidence) (AlignmentReport, error) {
	if plan.Validate() != nil {
		return AlignmentReport{}, fmt.Errorf("invalid frozen plan")
	}
	if evidence.Repeat != nil && evidence.First == nil {
		return AlignmentReport{}, fmt.Errorf("repeat without first experiment")
	}
	for _, p := range []*experiment.RelationshipProbe{evidence.First, evidence.Repeat} {
		if p != nil {
			if e := p.Validate(plan.Probe); e != nil {
				return AlignmentReport{}, e
			}
		}
	}
	// Detach report ownership from caller-owned nested evidence and plan slices.
	raw, _ := json.Marshal(struct {
		Plan     AlignmentPlan
		Evidence AlignmentEvidence
	}{plan, evidence})
	var owned struct {
		Plan     AlignmentPlan
		Evidence AlignmentEvidence
	}
	_ = json.Unmarshal(raw, &owned)
	r := AlignmentReport{Version: AlignmentReportVersion, Plan: owned.Plan, PlanHash: plan.Hash(), Registry: AlignmentRegistry(), Evidence: owned.Evidence, SyntheticOnly: true, HumanValidity: AlignmentNotTested, CrossModelTransfer: AlignmentNotTested, RealStudy: "NOT_RUN"}
	r.EvidenceHash, _ = Digest(r.Evidence)
	r.Mechanics = relationshipMechanics(r.Evidence)
	for _, entry := range r.Registry {
		f := AlignmentFinding{ID: entry.ID, Status: AlignmentNotTested, Scope: "source claim; no relevant adequacy experiment", EvidenceHash: r.EvidenceHash, Explanation: "No relevant preregistered experiment is represented by this evidence."}
		if entry.Kind == "falsifier" {
			f.FalsifierTriggered = "unknown"
		}
		if r.Evidence.First != nil {
			switch entry.ID {
			case "hws19.02", "hws19.03", "hws19.06", "hws19.09", "hws20.03", "hws20.06", "hws20.07":
				f.Status = AlignmentInconclusive
				f.Scope = "synthetic mechanism experiment; behavioral adequacy unresolved"
				f.Explanation = "Controlled fixture experiments ran; fixed coefficients, one-step transitions and seed variation cannot resolve behavioral adequacy or human plausibility. Falsifier non-observation is not evidence it is false."
			case "hws19.10":
				f.Status = AlignmentInconclusive
				f.Scope = "engineering only: repeated bounded one-step action/drive-state trajectories under this exact frozen protocol"
				f.Explanation = "One experiment ran; a second independent invocation is required to assess reproducibility."
				if r.Evidence.Repeat != nil {
					f.Status = AlignmentFail
					f.Explanation = "The repeated frozen experiment produced different trajectory evidence."
					if reflect.DeepEqual(r.Evidence.First, r.Evidence.Repeat) {
						f.Status = AlignmentPass
						f.Explanation = "Two generation invocations reproduced every recorded action/drive-state trial byte for this bounded fixture. No general-world or human-validity claim."
					}
				}
			}
		}
		if entry.ID == "hws19.13" {
			f.Explanation = "Real human import/comparison is not authorized or performed. Synthetic schema/import mechanics do not satisfy this source criterion; a future path is only a limitation."
		}
		if entry.ID == "hws20.08" || entry.ID == "hws20.10" {
			f.Explanation = "No authorized held-out human dataset or human transfer experiment; synthetic data cannot resolve this falsifier."
		}
		if entry.ID == "hws20.09" {
			f.Explanation = "No independently specified second simulator model or preregistered cross-model study; variants of this reference implementation are not that evidence."
		}
		r.Findings = append(r.Findings, f)
	}
	r.Hash, _ = Digest(r)
	return r, nil
}

// expected is the independently retained preregistration, not a plan recovered
// from the report being checked. Resealing altered source/config/evidence claims
// cannot replace it. Execution authenticity still requires trusted provenance.
func (r AlignmentReport) Verify(expected AlignmentPlan, dataset Dataset, receipt AlignmentReceipt) error {
	if receipt.Verify(r) != nil || dataset.Validate(receipt.EvaluatedAt) != nil || dataset.Hash != expected.DatasetHash {
		return fmt.Errorf("missing or invalid dataset/execution evidence")
	}

	if expected.Validate() != nil || !reflect.DeepEqual(r.Plan, expected) || r.PlanHash != expected.Hash() || !sameAlignmentRegistry(r.Registry) {
		return fmt.Errorf("registry/source/frozen-plan binding mismatch")
	}
	canonical, e := BuildAlignmentReport(expected, r.Evidence)
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(r, canonical) {
		return fmt.Errorf("unsupported, omitted, forged or corrupted alignment verdict")
	}
	return nil
}

func trialProbability(t experiment.RelationshipTrial, k behavior.Kind) float64 {
	for _, c := range t.Decision.Candidates {
		if c.Offer.Kind == k {
			return c.Probability
		}
	}
	return 0
}
func relationshipMechanics(e AlignmentEvidence) []MechanicsFinding {
	names := []string{"relational_context_sensitivity", "label_invariance", "expectation_history_stress_ablations", "history_reversal", "unknown_permission_boundary", "relationship_appraisal", "event_changes_drive_state", "selected_action_seed_variation", "repeated_generation"}
	out := []MechanicsFinding{}
	for _, name := range names {
		out = append(out, MechanicsFinding{Name: name, Status: AlignmentNotTested, Explanation: "No relevant probe executed."})
	}
	if e.First == nil {
		return out
	}
	bySeed := map[uint64]map[string]experiment.RelationshipTrial{}
	for _, t := range e.First.Trials {
		if bySeed[t.Seed] == nil {
			bySeed[t.Seed] = map[string]experiment.RelationshipTrial{}
		}
		bySeed[t.Seed][t.Case] = t
	}
	checks := [7]bool{true, true, true, true, true, true, true}
	selected := map[behavior.Kind]bool{}
	for _, seed := range e.First.Request.Seeds {
		rows := bySeed[seed]
		roles := []string{"spouse", "sibling", "friend", "acquaintance"}
		for i, a := range roles {
			for _, b := range roles[:i] {
				checks[0] = checks[0] && math.Abs(trialProbability(rows[a], behavior.Ask)-trialProbability(rows[b], behavior.Ask)) > 1e-6
			}
		}
		for _, label := range []string{"sibling", "friend", "acquaintance", "manager"} {
			checks[1] = checks[1] && reflect.DeepEqual(rows["spouse"].Decision, rows["label_"+label].Decision)
		}
		for _, kind := range []string{"expectation", "prior_outcome", "stress"} {
			checks[2] = checks[2] && !reflect.DeepEqual(rows["ablation_baseline"].Decision.Candidates, rows["ablate_"+kind].Decision.Candidates)
		}
		checks[3] = checks[3] && trialProbability(rows["hostile_spouse"], behavior.Reveal) == 0 && trialProbability(rows["supportive_acquaintance"], behavior.Reveal) > 0
		checks[4] = checks[4] && trialProbability(rows["unknown"], behavior.Reveal) == 0 && trialProbability(rows["unknown"], behavior.Ask) > 0 && trialProbability(rows["unknown"], behavior.Wait) > 0
		checks[5] = checks[5] && rows["ablation_baseline"].AfterDrivesHash != rows["without_appraisal"].AfterDrivesHash
		checks[6] = checks[6] && rows["spouse"].BeforeDrivesHash != rows["spouse"].AfterDrivesHash
		d := rows["spouse"].Decision
		selected[d.Candidates[d.Selected].Offer.Kind] = true
	}
	for i, pass := range checks {
		out[i].Status = AlignmentFail
		out[i].Explanation = "Declared synthetic mechanics check failed; this is not a human-realism verdict."
		if pass {
			out[i].Status = AlignmentPass
			out[i].Explanation = "Declared controlled synthetic mechanics check supported; behavioral adequacy remains unresolved."
		}
	}
	out[7].Status = AlignmentInconclusive
	out[7].Explanation = "These seeds did not show different selected spouse-case actions; absence in a small sample does not establish deterministic behavior."
	if len(selected) > 1 {
		out[7].Status = AlignmentPass
		out[7].Explanation = "Fixed initial inputs produced different selected actions across preregistered seeds; hashes/RNG draws alone are not counted. Plausibility is not established."
	}
	out[8].Status = AlignmentInconclusive
	out[8].Explanation = "Only one generation invocation is present."
	if e.Repeat != nil {
		out[8].Status = AlignmentFail
		out[8].Explanation = "Repeated trial bytes differ."
		if reflect.DeepEqual(e.First, e.Repeat) {
			out[8].Status = AlignmentPass
			out[8].Explanation = "All bounded trial bytes reproduced under the frozen protocol."
		}
	}
	return out
}
