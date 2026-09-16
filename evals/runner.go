package evals

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"time"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/experiment"
)

// BatchGenerator receives only detached, label-blind requests. The production
// offline host implements this port in a networkless process with no dataset mount.
type BatchGenerator interface {
	Generate(context.Context, []experiment.Request) ([]experiment.Projection, error)
}
type Config struct {
	GeneratorArtifact   string             `json:"generator_artifact"`
	Version             string             `json:"version"`
	DatasetHash         string             `json:"dataset_hash"`
	Generator           string             `json:"generator"`
	Policy              string             `json:"policy"`
	Prompt              string             `json:"prompt"`
	Seeds               []uint64           `json:"seeds"`
	Horizons            []core.LogicalTime `json:"horizons"`
	Dimensions          []string           `json:"dimensions"`
	BootstrapSeed       uint64             `json:"bootstrap_seed"`
	BootstrapReplicates int                `json:"bootstrap_replicates"`
}

func (c Config) Validate() error {
	if !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(c.GeneratorArtifact) || c.Version != "evaluation-config.v1" || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(c.DatasetHash) || c.Generator != experiment.Version || c.Policy != behavior.Policy || c.Prompt != "none-typed-offline.v1" || len(c.Seeds) < 2 || len(c.Seeds) > 32 || len(c.Horizons) < 1 || len(c.Horizons) > 8 || len(c.Dimensions) < 1 || len(c.Dimensions) > 11 || c.BootstrapReplicates < 100 || c.BootstrapReplicates > 2000 {
		return fmt.Errorf("invalid/preregistered evaluator configuration")
	}
	seen := map[uint64]bool{}
	for _, s := range c.Seeds {
		if seen[s] {
			return fmt.Errorf("duplicate seed")
		}
		seen[s] = true
	}
	hs := map[core.LogicalTime]bool{}
	for _, h := range c.Horizons {
		if h < 1 || h > experiment.MaxHorizon || hs[h] {
			return fmt.Errorf("invalid horizon")
		}
		hs[h] = true
	}
	ds := map[string]bool{}
	for _, d := range c.Dimensions {
		if ds[d] || (!actionDimension(d) && d != "latent_trust" && d != "latent_motive") {
			return fmt.Errorf("invalid dimension")
		}
		ds[d] = true
	}
	return nil
}

type Status string

const (
	Pass         Status = "pass"
	Fail         Status = "fail"
	Inconclusive Status = "inconclusive"
	NotTested    Status = "not-tested"
)

type Finding struct {
	ID       string `json:"id"`
	Status   Status `json:"status"`
	Evidence string `json:"evidence"`
}
type Interval struct {
	Low    float64 `json:"low"`
	High   float64 `json:"high"`
	Units  int     `json:"independent_components"`
	Method string  `json:"method"`
}
type Metric struct {
	Variant         experiment.Variant `json:"variant"`
	Split           Split              `json:"split"`
	Dimension       string             `json:"dimension"`
	Horizon         core.LogicalTime   `json:"horizon"`
	Total           int                `json:"total"`
	Resolved        int                `json:"resolved"`
	Missing         int                `json:"missing"`
	Censored        int                `json:"censored"`
	NotTaken        int                `json:"not_taken"`
	Unresolved      int                `json:"unresolved"`
	Abstentions     int                `json:"abstained_seed_predictions"`
	SeedPredictions int                `json:"seed_predictions"`
	Scored          int                `json:"scored_cases"`
	ResolutionRate  *float64           `json:"resolution_rate"`
	Brier           *float64           `json:"brier"`
	BrierCI         *Interval          `json:"brier_ci"`
	BaseRate        *float64           `json:"train_base_rate"`
	BaseRateBrier   *float64           `json:"base_rate_brier"`
	SelectedBySeed  []float64          `json:"selected_rate_by_seed"`
	SelectionCI     *Interval          `json:"selection_ci"`
	Status          Status             `json:"score_availability"`
}
type Report struct {
	PreregistrationHash string    `json:"preregistration_hash,omitempty"`
	Version             string    `json:"version"`
	DatasetHash         string    `json:"dataset_hash"`
	Config              Config    `json:"config"`
	ConfigHash          string    `json:"config_hash"`
	PredictionsHash     string    `json:"predictions_hash"`
	SyntheticOnly       bool      `json:"synthetic_only"`
	HumanValidity       Status    `json:"real_human_validity"`
	Metrics             []Metric  `json:"metrics"`
	Falsifiers          []Finding `json:"falsifiers"`
	Limitations         []string  `json:"limitations"`
	Hash                string    `json:"hash"`
}

func SealReport(r Report) (Report, error) { r.Hash = ""; h, e := Digest(r); r.Hash = h; return r, e }
func (r Report) Verify() error {
	copy, e := SealReport(r)
	if e != nil || r.Hash != copy.Hash || r.Version != "evaluation-report.v1" || !r.SyntheticOnly || r.HumanValidity != NotTested || r.Config.Validate() != nil || r.DatasetHash != r.Config.DatasetHash {
		return fmt.Errorf("invalid report")
	}
	hash := regexp.MustCompile(`^[0-9a-f]{64}$`)
	h, e := Digest(r.Config)
	if e != nil || h != r.ConfigHash || !hash.MatchString(r.PredictionsHash) {
		return fmt.Errorf("config/prediction digest mismatch")
	}
	if len(r.Metrics) != len(experiment.Variants())*3*len(r.Config.Horizons)*len(r.Config.Dimensions) {
		return fmt.Errorf("incomplete metrics")
	}
	seen := map[string]bool{}
	unit := func(v float64) bool { return !math.IsNaN(v) && v >= 0 && v <= 1 }
	for _, m := range r.Metrics {
		key := fmt.Sprintf("%s/%s/%s/%d", m.Variant, m.Split, m.Dimension, m.Horizon)
		if seen[key] || !m.Variant.Valid() || (m.Split != Train && m.Split != Calibration && m.Split != Holdout) {
			return fmt.Errorf("invalid/duplicate metric")
		}
		seen[key] = true
		allowed := false
		for _, d := range r.Config.Dimensions {
			for _, h := range r.Config.Horizons {
				allowed = allowed || (d == m.Dimension && h == m.Horizon)
			}
		}
		if !allowed || m.Total < 0 || m.Total > 64 || m.Resolved < 0 || m.Missing < 0 || m.Censored < 0 || m.NotTaken < 0 || m.Unresolved < 0 || m.Resolved+m.Missing+m.Censored+m.NotTaken+m.Unresolved != m.Total || m.Scored < 0 || m.Scored > m.Resolved || m.SeedPredictions != m.Total*len(r.Config.Seeds) || m.Abstentions < 0 || m.Abstentions > m.SeedPredictions {
			return fmt.Errorf("invalid metric counts")
		}
		if (m.Total == 0) != (m.ResolutionRate == nil) || (m.ResolutionRate != nil && (!unit(*m.ResolutionRate) || *m.ResolutionRate != float64(m.Resolved)/float64(m.Total))) {
			return fmt.Errorf("invalid resolution rate")
		}
		if (m.Scored == 0) != (m.Brier == nil) {
			return fmt.Errorf("invented/missing score")
		}
		for _, p := range []*float64{m.Brier, m.BaseRate, m.BaseRateBrier} {
			if p != nil && !unit(*p) {
				return fmt.Errorf("invalid score")
			}
		}
		for _, ci := range []*Interval{m.BrierCI, m.SelectionCI} {
			if ci != nil && (!unit(ci.Low) || !unit(ci.High) || ci.Low > ci.High || ci.Units < 2 || ci.Units > m.Total || ci.Method != "paired-connected-component-bootstrap-95.v1") {
				return fmt.Errorf("invalid uncertainty interval")
			}
		}
		if m.BrierCI != nil && m.Brier == nil || m.BaseRateBrier != nil && (m.BaseRate == nil || m.Brier == nil) {
			return fmt.Errorf("score without supporting data")
		}
		expected := Inconclusive
		if m.BrierCI != nil && m.BaseRateBrier != nil {
			expected = Pass
		}
		if m.Status != expected {
			return fmt.Errorf("optimistic score availability")
		}
		if len(m.SelectedBySeed) != len(r.Config.Seeds) {
			return fmt.Errorf("missing seed distribution")
		}
		for _, v := range m.SelectedBySeed {
			if v != -1 && !unit(v) {
				return fmt.Errorf("invalid seed frequency")
			}
		}
	}
	return nil
}

func pointer(v float64) *float64 { return &v }
func label(c Case, d string, h core.LogicalTime) Label {
	for _, l := range c.Labels {
		if l.Dimension == d && l.Horizon == h {
			return l
		}
	}
	return Label{Dimension: d, Horizon: h, Status: Missing}
}

// Components conservatively join any shared family, person or group. Seeds and
// repeated people are never counted as independent samples for confidence bounds.
func components(cs []Case) []int {
	parents := make([]int, len(cs))
	for i := range parents {
		parents[i] = i
	}
	var root func(int) int
	root = func(i int) int {
		if parents[i] != i {
			parents[i] = root(parents[i])
		}
		return parents[i]
	}
	seen := map[string]int{}
	for i, c := range cs {
		keys := []string{"f/" + string(c.Family)}
		for _, p := range c.People {
			keys = append(keys, "p/"+string(p))
		}
		for _, g := range c.Groups {
			keys = append(keys, "g/"+string(g))
		}
		for _, k := range keys {
			if j, ok := seen[k]; ok {
				parents[root(i)] = root(j)
			} else {
				seen[k] = i
			}
		}
	}
	for i := range parents {
		parents[i] = root(i)
	}
	return parents
}

// Fixed xorshift64* stream; statistical resampling is distinct from choice seeds.
func next(s *uint64) uint64 {
	if *s == 0 {
		*s = 0x9e3779b97f4a7c15
	}
	x := *s
	x ^= x >> 12
	x ^= x << 25
	x ^= x >> 27
	*s = x
	return x * 2685821657736338717
}
func interval(values map[int][]float64, c Config) *Interval {
	keys := []int{}
	for k, v := range values {
		if len(v) > 0 {
			keys = append(keys, k)
		}
	}
	sort.Ints(keys)
	if len(keys) < 2 {
		return nil
	}
	samples := make([]float64, c.BootstrapReplicates)
	rng := c.BootstrapSeed
	for b := range samples {
		sum := 0.0
		n := 0
		for range keys {
			k := keys[int(next(&rng)%uint64(len(keys)))]
			for _, v := range values[k] {
				sum += v
				n++
			}
		}
		samples[b] = sum / float64(n)
	}
	sort.Float64s(samples)
	return &Interval{Low: samples[int(.025*float64(len(samples)-1))], High: samples[int(.975*float64(len(samples)-1))], Units: len(keys), Method: "paired-connected-component-bootstrap-95.v1"}
}

// splitIntegrity recomputes the frozen-split property from the dataset actually
// supplied, independently of Dataset.Validate. Before #84 this row was a
// constant inside the Falsifiers literal: it read "pass" whatever the run
// computed, so removing the family/person/group crossing check would not have
// changed it, and a reader of bin/evaluation-report.json saw a measured-looking
// verdict that was a string. Recomputing here rather than trusting the preflight
// means the row stays derived even if that preflight stops being sound.
//
// The Pass evidence string is unchanged, so an unmodified run still produces a
// byte-identical report.
func splitIntegrity(d Dataset, c Config) Finding {
	if c.DatasetHash != d.Hash {
		return Finding{"frozen_split_integrity", Fail, "configured dataset hash does not match the supplied dataset"}
	}
	assigned := map[string]Split{}
	for _, row := range d.Cases {
		for i, keys := range [][]core.ID{{row.Family}, row.People, row.Groups} {
			for _, id := range keys {
				key := fmt.Sprintf("%d/%s", i, id)
				if previous, ok := assigned[key]; ok && previous != row.Split {
					return Finding{"frozen_split_integrity", Fail, fmt.Sprintf("%s appears in both the %s and %s splits", id, previous, row.Split)}
				}
				assigned[key] = row.Split
			}
		}
	}
	return Finding{"frozen_split_integrity", Pass, "dataset hash and family/person/group/time validation before generation"}
}

func Run(ctx context.Context, d Dataset, c Config, g BatchGenerator, now time.Time) (Report, error) {
	if d.Validate(now) != nil || c.Validate() != nil || c.DatasetHash != d.Hash || g == nil {
		return Report{}, fmt.Errorf("evaluation preflight failed")
	}
	// Recomputed rather than asserted, and checked before any generation runs, so
	// a dataset whose splits leak cannot produce a report claiming they do not.
	split := splitIntegrity(d, c)
	if split.Status != Pass {
		return Report{}, fmt.Errorf("frozen split integrity failed: %s", split.Evidence)
	}
	// Detach all input memory before giving it to the generator, including fakes.
	requests := []experiment.Request{}
	for _, v := range experiment.Variants() {
		for _, h := range c.Horizons {
			for _, row := range d.Cases {
				for _, seed := range c.Seeds {
					input := row.Input
					input.Horizon = h
					requests = append(requests, experiment.Request{Input: input, Variant: v, Seed: seed})
				}
			}
		}
	}
	if len(requests) > 8192 {
		return Report{}, fmt.Errorf("generation request budget")
	}
	raw, e := json.Marshal(requests)
	if e != nil {
		return Report{}, e
	}
	var detached []experiment.Request
	if e = json.Unmarshal(raw, &detached); e != nil {
		return Report{}, e
	}
	predictions, e := g.Generate(ctx, detached)
	if e != nil {
		return Report{}, e
	}
	if len(predictions) != len(requests) {
		return Report{}, fmt.Errorf("incomplete predictions")
	}
	for i, p := range predictions {
		requestHash, e := experiment.RequestDigest(requests[i])
		if e != nil || p.RequestHash != requestHash {
			return Report{}, fmt.Errorf("prediction request binding mismatch")
		}
		if p.Validate() != nil || p.Variant != requests[i].Variant || p.Horizon != requests[i].Input.Horizon {
			return Report{}, fmt.Errorf("invalid/misaligned prediction")
		}
	}
	r := Report{Version: "evaluation-report.v1", DatasetHash: d.Hash, Config: c, SyntheticOnly: true, HumanValidity: NotTested}
	r.ConfigHash, _ = Digest(c)
	r.PredictionsHash, _ = Digest(predictions)
	component := components(d.Cases)
	for vi, v := range experiment.Variants() {
		for hi, h := range c.Horizons {
			for _, split := range []Split{Train, Calibration, Holdout} {
				for _, dimension := range c.Dimensions {
					m := Metric{Variant: v, Split: split, Dimension: dimension, Horizon: h, Status: Inconclusive, SelectedBySeed: make([]float64, len(c.Seeds))}
					trainN := 0
					trainY := 0.0
					for _, row := range d.Cases {
						l := label(row, dimension, h)
						if row.Split == Train && l.Status == Observed {
							trainN++
							if *l.Value {
								trainY++
							}
						}
					}
					if trainN > 0 {
						m.BaseRate = pointer(trainY / float64(trainN))
					}
					losses := map[int][]float64{}
					selections := map[int][]float64{}
					sumLoss := 0.0
					baseLoss := 0.0
					seedN := make([]int, len(c.Seeds))
					for ci, row := range d.Cases {
						if row.Split != split {
							continue
						}
						m.Total++
						l := label(row, dimension, h)
						switch l.Status {
						case Observed:
							m.Resolved++
						case Missing:
							m.Missing++
						case Censored:
							m.Censored++
						case NotTaken:
							m.NotTaken++
						case Unresolved:
							m.Unresolved++
						}
						mean := 0.0
						n := 0
						selected := 0.0
						for si := range c.Seeds {
							idx := ((vi*len(c.Horizons)+hi)*len(d.Cases)+ci)*len(c.Seeds) + si
							p := predictions[idx]
							m.SeedPredictions++
							if p.Abstained {
								m.Abstentions++
								continue
							}
							n++
							seedN[si]++
							if string(p.Candidates[p.Selected].Offer.Kind) == dimension {
								m.SelectedBySeed[si]++
								selected++
							}
							prob := 0.0
							for _, candidate := range p.Candidates {
								if string(candidate.Offer.Kind) == dimension {
									prob += candidate.Probability
								}
							}
							mean += prob
						}
						if n > 0 {
							selections[component[ci]] = append(selections[component[ci]], selected/float64(n))
						}
						if l.Status != Observed || n == 0 {
							continue
						}
						mean /= float64(n)
						y := 0.0
						if *l.Value {
							y = 1
						}
						loss := math.Pow(mean-y, 2)
						sumLoss += loss
						m.Scored++
						losses[component[ci]] = append(losses[component[ci]], loss)
						if m.BaseRate != nil {
							baseLoss += math.Pow(*m.BaseRate-y, 2)
						}
					}
					if m.Total > 0 {
						m.ResolutionRate = pointer(float64(m.Resolved) / float64(m.Total))
					}
					if m.Scored > 0 {
						m.Brier = pointer(sumLoss / float64(m.Scored))
						m.BrierCI = interval(losses, c)
						if m.BaseRate != nil {
							m.BaseRateBrier = pointer(baseLoss / float64(m.Scored))
						}
					}
					for si := range seedN {
						if seedN[si] > 0 {
							m.SelectedBySeed[si] /= float64(seedN[si])
						} else {
							m.SelectedBySeed[si] = -1
						}
					}
					m.SelectionCI = interval(selections, c)
					// This is score availability, never scientific adequacy or an improvement gate.
					if m.BrierCI != nil && m.BaseRateBrier != nil {
						m.Status = Pass
					}
					r.Metrics = append(r.Metrics, m)
				}
			}
		}
	}
	r.Falsifiers = []Finding{split, {"calibration_readiness", Inconclusive, "per-dimension/horizon synthetic scores; no owner-approved adequacy threshold"}, {"behavioral_ablations", Inconclusive, "five preregistered variants; differences are descriptive, not a friendliness/conflict objective"}, {"partial_observation_roll_forward", Inconclusive, "conditional on no unseen events and fixed affordances; abstentions counted"}, {"complete_hws_source_falsifiers", NotTested, "legacy evaluation-report.v1 has seven format-scoped findings; the complete supplied source registry is reported separately by hws-alignment-report.v1"}, {"cross_model_transfer", NotTested, "only offline reference generator; no independent model data"}, {"real_human_validity", NotTested, "synthetic formats only; no authorized human dataset"}}
	r.Limitations = []string{"Synthetic action annotations are engineering fixtures, not human or latent truth.", "Confidence intervals resample connected family/person/group components, not seeds; tiny fixtures do not establish adequacy.", "No production policy is changed; owner-approved promotion is a separate signed event.", "No paid provider, private dataset or live study was run."}
	return SealReport(r)
}
