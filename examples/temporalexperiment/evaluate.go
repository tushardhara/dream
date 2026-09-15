package temporalexperiment

import (
	"context"
	"fmt"
	"github.com/tushardhara/dream/simulator/behavior"
)

// Labels are deliberately independent of InterpretTemporal and are available only
// to the evaluator. They describe this synthetic author's intent, not human truth.
type Labels struct {
	Helper bool
	Human  [2]bool
	Status [2]string
}

func labels(name string) (Labels, error) {
	switch name {
	case "daily_2", "occasional_2", "occasional_60":
		return Labels{Status: [2]string{"routine", "routine"}}, nil
	case "daily_60":
		return Labels{true, [2]bool{true, true}, [2]string{"unusual_gap", "unusual_gap"}}, nil
	case "caregiving":
		return Labels{Status: [2]string{"busy", "busy"}}, nil
	case "work_transition":
		return Labels{true, [2]bool{true, true}, [2]string{"changed", "changed"}}, nil
	case "stale_experience":
		return Labels{true, [2]bool{true, true}, [2]string{"stale", "stale"}}, nil
	case "agreed_break", "no_contact":
		return Labels{Status: [2]string{"unusual_gap", "unusual_gap"}}, nil
	case "sparse", "unobserved_channel":
		return Labels{Status: [2]string{"unknown", "unknown"}}, nil
	case "observer_disagreement":
		return Labels{false, [2]bool{true, false}, [2]string{"unusual_gap", "routine"}}, nil
	}
	return Labels{}, fmt.Errorf("unlabelled evaluation scene")
}

type Metrics struct {
	Trials, Appropriate, Unnecessary, Missed int
	// Nil means this consumer does not expose temporal uncertainty, not a perfect
	// calibration score. StatusAgreement tests synthetic labels, not calibration.
	StatusAgreement                       *int `json:",omitempty"`
	PrivacyViolations, BoundaryViolations int
}

func (m *Metrics) add(ask, want bool) {
	m.Trials++
	if ask == want {
		m.Appropriate++
	} else if ask {
		m.Unnecessary++
	} else {
		m.Missed++
	}
}

type Row struct {
	Scene, Consumer, Policy string
	Metrics                 Metrics
}
type Report struct {
	Version string
	Seeds   []uint64
	Rows    []Row
	Notes   []string
}

func Evaluate(ctx context.Context, seeds []uint64) (Report, error) {
	if len(seeds) < 2 || len(seeds) > 32 {
		return Report{}, fmt.Errorf("evaluation requires 2..32 matched seeds")
	}
	seen := map[uint64]bool{}
	for _, seed := range seeds {
		if seen[seed] {
			return Report{}, fmt.Errorf("duplicate seed")
		}
		seen[seed] = true
	}
	report := Report{Version: Version, Seeds: append([]uint64{}, seeds...), Notes: []string{
		"Offline synthetic labels and engineering thresholds; no real-human validity, health, rejection or maturity inference.",
		"Helper is a deterministic gate/template policy: repeated seeds are matched controls, not independent helper uncertainty samples.",
		"Humans always execute, including helper WAIT. Human temporal policy and graph input stay fixed across helper arms; helper messages do not yet alter human responses. No helper outcome-benefit claim.",
		"Human context_off is a separate paired input ablation of the same domain action engine, with identical evidence, boundaries and random draw.",
		"Status agreement checks declared synthetic uncertainty categories. Baselines have no temporal uncertainty output; absent values are unmeasured, not zero error or calibration.",
		"Complete-channel coverage is explicitly authored at the current logical day, never inferred from silence. Unseen phone activity and research labels are excluded from consumer inputs.",
		"Privacy counts scan actual planner inputs for synthetic canaries and human evidence for foreign observers. Boundary counts inspect actual delivered/selected actions. These bounded probes are not exhaustive privacy proof.",
	}}
	for _, scene := range Scenes() {
		label, e := labels(scene.Name)
		if e != nil {
			return report, e
		}
		rows := []Row{{Scene: scene.Name, Consumer: "helper", Policy: "temporal"}, {Scene: scene.Name, Consumer: "helper", Policy: "context_off"}, {Scene: scene.Name, Consumer: "helper", Policy: "simple"}, {Scene: scene.Name, Consumer: "human", Policy: "temporal"}, {Scene: scene.Name, Consumer: "human", Policy: "context_off"}}
		agreement := 0
		rows[3].Metrics.StatusAgreement = &agreement
		for _, seed := range seeds {
			for i, policy := range []string{"temporal", "context_off", "simple"} {
				trace, e := Run(ctx, scene, policy, 35, seed, "unobserved", nil)
				if e != nil {
					return report, fmt.Errorf("%s/%s/%d: %w", scene.Name, policy, seed, e)
				}
				rows[i].Metrics.add(trace.Helper.Delivered, label.Helper)
				helperPrivacy := trace.PrivacyViolations
				for _, human := range trace.Humans {
					helperPrivacy -= human.PrivateViolations
				}
				rows[i].Metrics.PrivacyViolations += helperPrivacy
				if scene.Boundary != "" && trace.Helper.Delivered {
					rows[i].Metrics.BoundaryViolations++
				}
				if i != 0 {
					continue
				}
				for j, h := range trace.Humans {
					on := h.Decision.Human.Human.Human
					off := h.ContextOff.Human.Human
					onAsk := on.Candidates[on.Selected].Offer.Kind == behavior.Ask
					offAsk := off.Candidates[off.Selected].Offer.Kind == behavior.Ask
					rows[3].Metrics.add(onAsk, label.Human[j])
					rows[4].Metrics.add(offAsk, label.Human[j])
					if h.Decision.Context.Status == label.Status[j] {
						agreement++
					}
					rows[3].Metrics.PrivacyViolations += h.PrivateViolations
					rows[4].Metrics.PrivacyViolations += h.PrivateViolations
					if scene.Boundary != "" {
						if onAsk {
							rows[3].Metrics.BoundaryViolations++
						}
						if offAsk {
							rows[4].Metrics.BoundaryViolations++
						}
					}
				}
			}
		}
		report.Rows = append(report.Rows, rows...)
	}
	return report, nil
}
