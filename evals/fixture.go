package evals

import (
	"fmt"
	"time"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/dynamics"
	"github.com/tushardhara/dream/simulator/experiment"
)

// SyntheticFixture is a reproducible format/engineering fixture. Labels follow
// an arbitrary documented alternation, never the generator's predictions.
func SyntheticFixture() (Dataset, Config, error) {
	d := Dataset{Version: "evaluation-dataset.v1", ID: "synthetic-independent.v1"}
	// Distinct declared scenario templates per split, not the same template with
	// different RNG seeds or renamed people. These remain toy synthetic families.
	families := []struct {
		id      core.ID
		signals dynamics.Signals
	}{
		{"care-need", dynamics.Signals{OtherNeed: .6}},
		{"care-support", dynamics.Signals{Support: .8, OtherNeed: .2}},
		{"care-effort", dynamics.Signals{Effort: .9, OtherNeed: .7}},
		{"care-recovery", dynamics.Signals{Rest: .7, Inclusion: .4}},
		{"status-threat", dynamics.Signals{StatusThreat: .8}},
		{"status-opportunity", dynamics.Signals{Opportunity: .8, StatusThreat: .2}},
		{"status-repair", dynamics.Signals{Support: .6, StatusThreat: .5}},
		{"status-exclusion", dynamics.Signals{Exclusion: .8, StatusThreat: .6}},
		{"resource-scarcity", dynamics.Signals{Scarcity: .9}},
		{"resource-exchange", dynamics.Signals{Opportunity: .6, Scarcity: .4, Effort: .3}},
		{"compound-pressure", dynamics.Signals{Scarcity: .8, Effort: .9, Exclusion: .7}},
		{"compound-recovery", dynamics.Signals{Rest: .9, Support: .7, Inclusion: .8}},
	}

	for si, split := range []Split{Train, Calibration, Holdout} {
		for n := 0; n < 4; n++ {
			prefix := fmt.Sprintf("fixture:%d:%d", si, n)
			id := core.ID(prefix)
			a := core.ID(prefix + ":a")
			b := core.ID(prefix + ":b")
			other := core.ID(prefix + ":c")
			at := core.LogicalTime(100+si*100+n*10) * 24 * dynamics.Hour
			addSource := func(suffix string, kind SourceKind, subject core.ID, when core.LogicalTime) core.ID {
				sourceID := core.ID(prefix + ":" + suffix)
				observer := a
				if kind == SelfReport || kind == PrivateSurvey {
					observer = subject
				}
				d.Sources = append(d.Sources, Source{ID: sourceID, Kind: kind, Subject: subject, Observer: observer, OccurredAt: when, LearnedAt: when, Synthetic: true, Provenance: core.ID(prefix + ":synthetic-provenance"), Consent: Consent{Generate: kind != LaterAction, Subject: subject, Purpose: "evaluation", Evaluate: true, Retain: true, Expires: time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)}, Text: "synthetic format fixture"})
				return sourceID
			}
			mem := addSource("memory", SelfReport, a, at-2*dynamics.Hour)
			rel := addSource("relationship", PrivateSurvey, a, at-2*dynamics.Hour)
			belief := addSource("belief", PublicStatement, b, at-2*dynamics.Hour)
			state, e := dynamics.New(a, at-2*dynamics.Hour, dynamics.DefaultSubstrate())
			if e != nil {
				return Dataset{}, Config{}, e
			}
			state, e = dynamics.Intervene(state, dynamics.Fatigue, float64(n)/4, at-2*dynamics.Hour, addSource("intervention", Observation, a, at-2*dynamics.Hour))
			if e != nil {
				return Dataset{}, Config{}, e
			}
			situation := func(suffix string, when core.LogicalTime) behavior.Situation {
				event := addSource(suffix, Observation, a, when)
				return behavior.Situation{Perceived: dynamics.Perceived{Event: event, Actor: a, OccurredAt: when, LearnedAt: when, Confidence: .7, Signals: families[si*4+n].signals, Rights: core.Rights{Resource: event, Grants: []core.Grant{{Actor: a, Recipient: a, Purpose: "simulation", Operation: core.Read}, {Actor: a, Recipient: a, Purpose: "simulation", Operation: core.Derive}}}}, Present: []core.ID{a, b, other}, Horizon: at + 10*dynamics.Hour, Resources: map[core.ID]int64{"time": 2}, Offers: []behavior.Offer{{Kind: behavior.Ask, Recipient: b, Duration: 1}, {Kind: behavior.Help, Recipient: b, Resource: "time", Units: 1, Duration: 1}, {Kind: behavior.Invite, Recipient: other, Duration: 1}}, Relationships: []behavior.Memory{{Other: b, Trust: .6, Disclosure: .2, Evidence: []core.ID{rel}}, {Other: other, Trust: -.7, Disclosure: -.2, Evidence: []core.ID{rel}}}, Beliefs: []behavior.Belief{{Source: mem, Observer: a, Code: "supported", Value: .5, Confidence: .6}, {Source: belief, Observer: a, Code: "contradicted", Value: -.7, Confidence: .5}}}
			}
			history := situation("history", at-dynamics.Hour)
			current := situation("current", at)
			c := Case{ID: id, Family: families[si*4+n].id, People: []core.ID{a, b, other}, Groups: []core.ID{core.ID(prefix + ":group")}, Split: split, Input: experiment.Input{Version: experiment.Version, ID: id, Initial: behavior.Actor{State: state, Memory: []behavior.Memory{{Other: b, Trust: .8, Disclosure: .4, Evidence: []core.ID{mem}}}}, History: []experiment.Observation{{At: at - dynamics.Hour, Situation: history, Draw: 0}}, Current: current, AsOf: at, Horizon: dynamics.Hour}}
			for _, h := range []core.LogicalTime{dynamics.Hour, 2 * dynamics.Hour} {
				source := addSource(fmt.Sprintf("outcome:%d", h), LaterAction, a, at+h)
				yes := n%2 == 0
				no := !yes
				c.Labels = append(c.Labels, Label{Dimension: "wait", Horizon: h, Status: Observed, Value: &yes, Source: source, Annotation: "EVALUATOR_LABEL_CANARY"}, Label{Dimension: "help", Horizon: h, Status: Observed, Value: &no, Source: source}, Label{Dimension: "latent_trust", Horizon: h, Status: Unresolved}, Label{Dimension: "ask", Horizon: h, Status: []OutcomeStatus{Missing, Censored, NotTaken, Missing}[n]})
			}
			if n == 3 {
				c.Input.Current.Horizon = at + dynamics.Hour
			} // horizon 2 must abstain, including baseline.
			d.Cases = append(d.Cases, c)
		}
	}
	sealed, e := SealDataset(d)
	if e != nil {
		return Dataset{}, Config{}, e
	}
	cfg := Config{GeneratorArtifact: "sha256:0000000000000000000000000000000000000000000000000000000000000000", Version: "evaluation-config.v1", DatasetHash: sealed.Hash, Generator: experiment.Version, Policy: behavior.Policy, Prompt: "none-typed-offline.v1", Seeds: []uint64{11, 23, 47, 89, 101, 131, 173, 199}, Horizons: []core.LogicalTime{dynamics.Hour, 2 * dynamics.Hour}, Dimensions: []string{"wait", "help", "ask", "latent_trust"}, BootstrapSeed: 20260913, BootstrapReplicates: 1000}
	return sealed, cfg, nil
}
