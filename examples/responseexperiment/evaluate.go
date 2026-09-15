package responseexperiment

import (
	"context"
	"fmt"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
)

type Metrics struct {
	HumanChoices, HumanWaits, HelperSteps, HelperDeliveries, ChosenAdviceActions int
	ExpectedSenderAccounts, ImmediateRecipientAccounts, LaterRecipientAccounts   int
	Supportive, Dismissive, Neutral, Mixed, Unresolved, Unknown, Censored        int
	SharedOutcomeReports, HelperPrivateInputDifferences                          int
}
type Row struct {
	Scene   string
	Arm     assistance.Arm
	Metrics Metrics
}
type Report struct {
	Version string
	Seeds   []uint64
	Rows    []Row
	Notes   []string
}

func Evaluate(ctx context.Context, seeds []uint64) (Report, error) {
	if len(seeds) < 2 || len(seeds) > 32 {
		return Report{}, fmt.Errorf("requires 2..32 matched seeds")
	}
	seen := map[uint64]bool{}
	for _, seed := range seeds {
		if seen[seed] {
			return Report{}, fmt.Errorf("duplicate seed")
		}
		seen[seed] = true
	}
	scenes := []struct {
		name  string
		scene Scene
	}{{"welcome", Scene{Expectation: 1, Observation: "observed", ShareReport: true, Followup: true}}, {"unwanted", Scene{Expectation: -1, Observation: "observed", ShareReport: true, Followup: true}}, {"silence", Scene{Expectation: -1, Observation: "silence", ShareReport: true, Followup: true}}, {"lost_observation", Scene{Expectation: -1, Observation: "lost_observation", ShareReport: true, Followup: true}}, {"declined_participation", Scene{Expectation: -1, Observation: "declined_participation", ShareReport: true, Followup: true}}, {"missing_followup", Scene{Expectation: -1, Observation: "observed", ShareReport: true, Followup: false}}}
	out := Report{Version: "recipient-response-evaluation.v1", Seeds: append([]uint64{}, seeds...), Notes: []string{
		"Offline synthetic engineering cases, not human welfare, treatment effects, calibration or human validity.",
		"All arms run the same domain native policy and recipient policy; helper proposals only offer an option to their requesting person. Only an independently chosen human coordination action reaches the other participant.",
		"Recipient appraisals are stochastic model outputs, not evaluator success labels. No positive/negative ratio is prescribed. Expected sender accounts never count as observed recipient benefit.",
		"Immediate and later records for one action are dependent observations and remain separate. Counts are not independent participant samples; no welfare score averages conflicting perspectives.",
		"Silence/lost observation are unknown; declined participation/missing follow-up are censored. Unresolved observed appraisals have no benefit/burden scalar.",
		"HelperPrivateInputDifferences compares actual helper interaction bytes against the matched welcome run for each arm/seed while only private recipient conditions change. This bounded invariance check is not exhaustive privacy proof.",
		"Reports require an explicit participant sharing choice and current read permission; they omit private model state, source lineage and account IDs. Reports are stored separately and are not automatically fed into the planner.",
		"Finite frames can end with pending advice or delivery; unobserved future experience is not scored. No live provider, real-person data, outreach or deployment.",
	}}
	baselines := map[string]string{}
	for _, entry := range scenes {
		for _, arm := range []assistance.Arm{assistance.None, assistance.Simple, assistance.Single, assistance.Multi} {
			row := Row{Scene: entry.name, Arm: arm}
			m := &row.Metrics
			for _, seed := range seeds {
				run, e := Run(ctx, entry.scene, arm, seed, nil)
				if e != nil {
					return out, e
				}
				key := fmt.Sprintf("%s/%d", arm, seed)
				hash := assistance.Digest(run.Helper)
				if entry.name == "welcome" {
					baselines[key] = hash
				} else if baselines[key] != hash {
					m.HelperPrivateInputDifferences++
				}
				m.HumanChoices += len(run.Humans)
				for _, d := range run.Humans {
					h := d.Human.Human
					if h.Candidates[h.Selected].Offer.Kind == behavior.Wait {
						m.HumanWaits++
					}
				}
				m.HelperSteps += len(run.Helper)
				for _, h := range run.Helper {
					if h.Delivered {
						m.HelperDeliveries++
					}
				}
				m.ChosenAdviceActions += len(run.Actions)
				m.SharedOutcomeReports += len(run.Reports)
				for _, o := range run.Outcomes {
					if o.Kind == "expected" && o.Position == "sender" {
						m.ExpectedSenderAccounts++
						continue
					}
					if o.Position != "recipient" {
						continue
					}
					if o.Phase == "immediate" {
						m.ImmediateRecipientAccounts++
					} else {
						m.LaterRecipientAccounts++
					}
					if o.Status == core.Unknown {
						m.Unknown++
						continue
					}
					if o.Status == core.Censored {
						m.Censored++
						continue
					}
					switch o.Appraisal {
					case "supportive":
						m.Supportive++
					case "dismissive":
						m.Dismissive++
					case "neutral":
						m.Neutral++
					case "mixed":
						m.Mixed++
					case "unresolved":
						m.Unresolved++
					}
				}
			}
			out.Rows = append(out.Rows, row)
		}
	}
	return out, nil
}
