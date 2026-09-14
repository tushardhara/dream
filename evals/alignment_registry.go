package evals

import "reflect"

// These are the supplied #43 revision-2 extracts, not a certification of the
// unavailable full private PRD. Source wording and machine names are separate.
const AlignmentRegistryVersion = "hws-exit-falsifier-registry.v1"
const AlignmentSourceSHA256 = "f7ddaf8ff06587abd8f845a4b70e87894cb7442dd97dbafedb64df09c88e7037"

type AlignmentStatus string

const (
	AlignmentPass         AlignmentStatus = "PASS"
	AlignmentFail         AlignmentStatus = "FAIL"
	AlignmentInconclusive AlignmentStatus = "INCONCLUSIVE"
	AlignmentNotTested    AlignmentStatus = "NOT_TESTED"
)

type SourceCriterion struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Kind                 string   `json:"kind"`
	SourceSection        string   `json:"source_section"`
	SourceWording        string   `json:"source_wording"`
	EvidenceRequirements []string `json:"evidence_requirements"`
}

// Constructors return fresh slices, including nested requirements. Callers
// cannot rename entries or change the registry used by subsequent reports.
func ExitCriteria() []SourceCriterion {
	return []SourceCriterion{
		{"hws19.01", "persistence", "exit", "§19", "Persistence — humans remain recognizably themselves over time.", []string{"preregistered longitudinal identity adequacy", "independent behavioral observations"}},
		{"hws19.02", "plasticity", "exit", "§19", "Plasticity — states and behaviour change in response to life events.", []string{"controlled event/state contrasts", "behavioral adequacy beyond deterministic fixtures"}},
		{"hws19.03", "relational_specificity", "exit", "§19", "Relational specificity — behaviour differs across relationships.", []string{"same actor/event/seed relationship comparisons", "label invariance and history/expectation ablations", "behavioral adequacy beyond fixed coefficients"}},
		{"hws19.04", "hidden_state", "exit", "§19", "Hidden state — internal experience can differ from public communication.", []string{"paired private state and observable communication", "permissioned independently evaluated outcomes"}},
		{"hws19.05", "partial_observability", "exit", "§19", "Partial observability — humans form incorrect beliefs about one another.", []string{"observer belief versus independently held truth", "belief error and uncertainty measurements"}},
		{"hws19.06", "memory", "exit", "§19", "Memory — past events alter future interpretation.", []string{"controlled retained-history ablations", "behavioral adequacy beyond fixed coefficients"}},
		{"hws19.07", "slow_dynamics", "exit", "§19", "Slow dynamics — trust, resentment and expectations accumulate.", []string{"preregistered longitudinal accumulation", "time-scale and behavioral adequacy"}},
		{"hws19.08", "emergence", "exit", "§19", "Emergence — group behaviour is not entirely scripted.", []string{"group trajectories and scripted baselines", "independent emergence decision rule"}},
		{"hws19.09", "stochasticity", "exit", "§19", "Stochasticity — identical initial worlds support multiple plausible trajectories.", []string{"same initial conditions and different preregistered seeds", "meaningful selected-outcome variation", "independent plausibility assessment"}},
		{"hws19.10", "reproducibility", "exit", "§19", "Reproducibility — seed and versioned configuration reproduce a trajectory.", []string{"independent repeated generation of the declared bounded trajectory", "exact source/config/model/seed/dataset/version binding"}},
		{"hws19.11", "counterfactual_branching", "exit", "§19", "Counterfactual branching — states can be forked into alternative futures.", []string{"authorized forks from the same recorded state", "isolated lineage and inherited budgets/deadlines"}},
		{"hws19.12", "ground_truth_isolation", "exit", "§19", "Ground-truth isolation — external systems cannot access simulator-only state.", []string{"adversarial external-boundary experiments", "simulator truth and evaluator labels remain inaccessible"}},
		// Revision 2 explicitly corrects the initial ticket's future-path gloss.
		{"hws19.13", "calibration_readiness", "exit", "§19", "real human data can be imported and compared.", []string{"separately authorized real-human ingestion path", "consented human data import and comparison"}},
	}
}

func PrimaryFalsifiers() []SourceCriterion {
	return []SourceCriterion{
		{"hws20.01", "prompt_dominance", "falsifier", "§20", "behaviour is primarily prompt/persona driven rather than state driven;", []string{"preregistered prompt/persona versus state ablations", "behavioral predictive adequacy"}},
		{"hws20.02", "hidden_state_no_value", "falsifier", "§20", "hidden state adds little predictive value;", []string{"held-out predictive comparison with and without hidden state", "independent labels and adequate statistical power"}},
		{"hws20.03", "no_relational_specificity", "falsifier", "§20", "agents do not behave differently across relationships;", []string{"controlled observer-owned relationship comparisons", "behavioral adequacy beyond fixed coefficients"}},
		{"hws20.04", "personality_stereotype_collapse", "falsifier", "§20", "long simulations collapse into personality stereotypes;", []string{"preregistered long-run trajectories", "independent stereotype-collapse assessment"}},
		{"hws20.05", "unnatural_agreement", "falsifier", "§20", "groups become systematically and unnaturally agreeable;", []string{"group agreement baselines", "independent human plausibility assessment"}},
		{"hws20.06", "unrealistic_memory_determinism", "falsifier", "§20", "memory creates unrealistic determinism;", []string{"memory and seed ablations", "independent realism assessment"}},
		{"hws20.07", "no_alternative_futures", "falsifier", "§20", "different seeds do not produce meaningful alternative futures;", []string{"preregistered seeds with fixed initial conditions", "selected outcomes rather than RNG/hash differences", "independent meaningfulness assessment"}},
		{"hws20.08", "human_calibration_failure", "falsifier", "§20", "behaviour cannot be calibrated against held-out human data;", []string{"authorized held-out human dataset", "frozen calibration protocol and uncertainty"}},
		{"hws20.09", "incompatible_model_laws", "falsifier", "§20", "different simulator models produce irreconcilably incompatible behavioural laws;", []string{"at least two independently specified simulator models", "preregistered cross-model behavioral-law comparison"}},
		{"hws20.10", "no_human_prediction_transfer", "falsifier", "§20", "simulator improvements do not transfer to real-human prediction.", []string{"authorized held-out human outcomes", "preregistered simulator-improvement transfer study"}},
	}
}

func AlignmentRegistry() []SourceCriterion {
	return append(ExitCriteria(), PrimaryFalsifiers()...)
}

func AlignmentRegistryHash() string {
	h, _ := Digest(AlignmentRegistry())
	return h
}

func sameAlignmentRegistry(entries []SourceCriterion) bool {
	return reflect.DeepEqual(entries, AlignmentRegistry())
}
