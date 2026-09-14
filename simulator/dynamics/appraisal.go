package dynamics

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/core"
	"math"
	"sort"
)

// Signals are bounded structured observations supplied by a trusted perception
// adapter. They are not inferred from unrestricted prose or hidden fixture labels.
type Signals struct {
	Effort       float64 `json:"effort"`
	Rest         float64 `json:"rest"`
	Scarcity     float64 `json:"scarcity"`
	Opportunity  float64 `json:"opportunity"`
	OtherNeed    float64 `json:"other_need"`
	Support      float64 `json:"support"`
	StatusThreat float64 `json:"status_threat"`
	Inclusion    float64 `json:"inclusion"`
	Exclusion    float64 `json:"exclusion"`
}

func (s Signals) Validate() error {
	for _, v := range []float64{s.Effort, s.Rest, s.Scarcity, s.Opportunity, s.OtherNeed, s.Support, s.StatusThreat, s.Inclusion, s.Exclusion} {
		if !unit(v) {
			return fmt.Errorf("invalid appraisal signal")
		}
	}
	return nil
}

type Perceived struct {
	Event      core.ID          `json:"event"`
	Actor      core.ID          `json:"actor"`
	OccurredAt core.LogicalTime `json:"occurred_at"`
	LearnedAt  core.LogicalTime `json:"learned_at"`
	Confidence core.Confidence  `json:"confidence"`
	Signals    Signals          `json:"signals"`
	Rights     core.Rights      `json:"rights"`
}

func (p Perceived) Validate(actor core.ID, at core.LogicalTime) error {
	if len(p.Rights.Grants) > 128 || p.Event.Validate() != nil || p.Actor != actor || p.Actor.Validate() != nil || p.OccurredAt < 0 || p.LearnedAt < p.OccurredAt || p.LearnedAt > at || p.Confidence.Validate() != nil || p.Signals.Validate() != nil || p.Rights.Resource != p.Event {
		return fmt.Errorf("invalid perceived event, actor or time")
	}
	for _, op := range []core.Operation{core.Read, core.Derive} {
		if !p.Rights.Allows(core.PermissionRequest{Resource: p.Event, Context: core.Grant{Actor: actor, Recipient: actor, Purpose: "simulation", Operation: op}}) {
			return fmt.Errorf("perception requires current self read and derive rights")
		}
	}
	return nil
}

// Rationale carries enum codes and bounded numeric deltas only. No raw event
// text, model trace, hidden labels or unrestricted chain-of-thought is stored.
type Rationale struct {
	Version int            `json:"version"`
	Stage   string         `json:"stage"`
	Key     string         `json:"key"`
	Cause   core.ID        `json:"cause"`
	Codes   []string       `json:"codes"`
	Deltas  [Count]float64 `json:"deltas"`
}

func Key(actor, event core.ID) string {
	b, _ := json.Marshal([]string{ModelVersion, string(actor), string(event)})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func perceptionDigest(p Perceived) (string, error) {
	if p.Confidence == 0 {
		p.Confidence = 0
	}
	for _, v := range []*float64{&p.Signals.Effort, &p.Signals.Rest, &p.Signals.Scarcity, &p.Signals.Opportunity, &p.Signals.OtherNeed, &p.Signals.Support, &p.Signals.StatusThreat, &p.Signals.Inclusion, &p.Signals.Exclusion} {
		if *v == 0 {
			*v = 0
		}
	}

	p.Rights.Grants = append([]core.Grant{}, p.Rights.Grants...)
	sort.Slice(p.Rights.Grants, func(i, j int) bool {
		a, _ := json.Marshal(p.Rights.Grants[i])
		b, _ := json.Marshal(p.Rights.Grants[j])
		return string(a) < string(b)
	})
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

// Appraise is the sole owner of appraisal.v1. Event identity, actor and stage
// form the transition key, so #11 must consume this result, not appraise it again.
// The bounded ledger never evicts; exhaustion denies instead of enabling replay.
func Appraise(s State, p Perceived, at core.LogicalTime) (State, Rationale, bool, error) {
	if err := s.Validate(); err != nil {
		return State{}, Rationale{}, false, err
	}
	if at < s.At {
		return State{}, Rationale{}, false, fmt.Errorf("retroactive appraisal")
	}
	if err := p.Validate(s.Actor, at); err != nil {
		return State{}, Rationale{}, false, err
	}
	digest, err := perceptionDigest(p)
	if err != nil {
		return State{}, Rationale{}, false, err
	}
	key := Key(s.Actor, p.Event)
	for _, r := range s.Applied {
		if r.Event == p.Event {
			if r.Digest != digest {
				return State{}, Rationale{}, false, fmt.Errorf("appraisal idempotency conflict")
			}
			return clone(s), Rationale{Version: 1, Stage: ModelVersion, Key: key, Cause: p.Event, Codes: []string{"already_applied"}}, true, nil
		}
	}
	if len(s.Applied) >= MaxReceipts {
		return State{}, Rationale{}, false, fmt.Errorf("appraisal ledger budget exhausted")
	}
	out, err := Advance(s, at)
	if err != nil {
		return State{}, Rationale{}, false, err
	}
	x := p.Signals
	f := out.Variables[Fatigue].Level
	raw := [Count]float64{x.Effort - x.Rest + .25*x.Scarcity*x.Effort, x.Scarcity - x.Opportunity + .25*f*x.Effort, x.OtherNeed*(1-.5*f) + .3*x.Support - .3*x.Effort*f, x.StatusThreat + .4*x.Scarcity*x.StatusThreat - .25*x.Support, x.Exclusion - x.Inclusion + .25*x.StatusThreat - .3*x.Support, .4*x.StatusThreat + .4*x.Exclusion + .2*x.Scarcity + .3*x.StatusThreat*x.Exclusion - .2*x.Support - .1*x.Rest}
	result := Rationale{Version: 1, Stage: ModelVersion, Key: key, Cause: p.Event, Codes: []string{}, Deltas: [Count]float64{}}
	gain := (.1 + .9*out.Substrate.Reactivity) * out.Substrate.Plasticity * float64(p.Confidence)
	for i, value := range raw {
		before := out.Variables[i]
		delta := math.Max(-1, math.Min(1, value)) * definitions[i].Gain * gain
		level := quant(before.Level + delta)
		confidence := before.Confidence
		if delta != 0 {
			confidence = core.Confidence(quant(float64(before.Confidence)*(1-gain) + float64(p.Confidence)*gain))
		}
		out.Variables[i] = Variable{Level: level, Anchor: level, Confidence: confidence, AnchorConfidence: confidence, AnchorAt: at}
		result.Deltas[i] = math.Round((level-before.Level)*1e9) / 1e9
		if result.Deltas[i] == 0 {
			result.Deltas[i] = 0
		}
	}
	for _, c := range []struct {
		on   bool
		code string
	}{{x.Effort > 0, "effort"}, {x.Rest > 0, "recovery"}, {x.Scarcity > 0 || x.Opportunity > 0, "resources"}, {x.OtherNeed > 0, "other_need"}, {x.Support > 0 || x.Inclusion > 0, "support"}, {x.StatusThreat > 0 || x.Exclusion > 0, "social_threat"}, {x.StatusThreat*x.Exclusion > 0 || x.Scarcity*x.Effort > 0, "compound"}} {
		if c.on {
			result.Codes = append(result.Codes, c.code)
		}
	}
	if err := addCause(&out, p.Event); err != nil {
		return State{}, Rationale{}, false, err
	}
	out.Applied = append(out.Applied, Receipt{Event: p.Event, Digest: digest})
	sort.Slice(out.Applied, func(i, j int) bool { return out.Applied[i].Event < out.Applied[j].Event })
	return out, result, false, out.Validate()
}

func (r Rationale) Validate() error {
	if r.Version != 1 || r.Stage != ModelVersion || r.Cause.Validate() != nil || len(r.Key) != 64 || len(r.Codes) > 7 {
		return fmt.Errorf("invalid structured rationale")
	}
	for _, c := range r.Key {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return fmt.Errorf("invalid rationale key")
		}
	}
	seen := map[string]bool{}
	for _, code := range r.Codes {
		switch code {
		case "effort", "recovery", "resources", "other_need", "support", "social_threat", "compound", "already_applied":
		default:
			return fmt.Errorf("unknown rationale code")
		}
		if seen[code] {
			return fmt.Errorf("duplicate rationale code")
		}
		seen[code] = true
	}
	for _, d := range r.Deltas {
		if math.IsNaN(d) || math.IsInf(d, 0) || d < -1 || d > 1 {
			return fmt.Errorf("invalid rationale delta")
		}
	}
	if seen["already_applied"] {
		if len(r.Codes) != 1 {
			return fmt.Errorf("duplicate appraisal rationale mixed with new appraisal")
		}
		for _, d := range r.Deltas {
			if d != 0 {
				return fmt.Errorf("duplicate appraisal cannot apply delta")
			}
		}
	}
	return nil
}
