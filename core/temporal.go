package core

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

const TemporalFactVersion = "temporal-fact.v1"
const TemporalFocusVersion = "temporal-focus.v1"
const TemporalPolicyVersion = "temporal-context.v1"

// TemporalFocus uses logical days in this opt-in policy. A channel is never a
// claim about all communication; unobserved channels remain unobserved.
type TemporalFocus struct {
	Version                  string
	Observer, Other, Channel ID
}

func (f TemporalFocus) Validate() error {
	if f.Version != TemporalFocusVersion || ids(f.Observer, f.Other, f.Channel) != nil || f.Observer == f.Other {
		return fmt.Errorf("invalid temporal focus")
	}
	return nil
}

// TemporalFact is an attributed statement, not an objective relationship state.
// Source names the permissioned observation/diary/self-report supporting it.
// Life signal names describe voluntarily supplied context, not inferred duties.
type TemporalFact struct {
	Version         string        `json:"version"`
	Account         ID            `json:"account"`
	Observer        ID            `json:"observer"`
	Person          ID            `json:"person"`
	With            ID            `json:"with"`
	Channel         ID            `json:"channel"`
	Source          ID            `json:"source"`
	Kind            string        `json:"kind"`  // circumstance, expectation, contacts
	Basis           string        `json:"basis"` // self_report, observed, hypothesis
	Signal          string        `json:"signal,omitempty"`
	Category        string        `json:"category,omitempty"` // responsibility, transition, preference, experience
	ConfirmedAt     LogicalTime   `json:"confirmed_at"`
	FreshFor        LogicalTime   `json:"fresh_for,omitempty"`
	MinGap          LogicalTime   `json:"min_gap,omitempty"`
	MaxGap          LogicalTime   `json:"max_gap,omitempty"`
	Observations    []LogicalTime `json:"observations,omitempty"`
	WindowStart     LogicalTime   `json:"window_start,omitempty"`
	ObservedThrough LogicalTime   `json:"observed_through,omitempty"`
	CompleteChannel bool          `json:"complete_channel,omitempty"`
}

func (f TemporalFact) Validate() error {
	if f.Version != TemporalFactVersion || ids(f.Account, f.Observer, f.Person, f.With, f.Channel, f.Source) != nil || f.Account == f.Source || f.Observer == f.With || f.Person != f.Observer && f.Person != f.With || f.ConfirmedAt < 0 || f.ConfirmedAt > 1000000000 || f.FreshFor < 0 || f.FreshFor > 3650 {
		return fmt.Errorf("invalid temporal fact")
	}
	if f.Basis != "self_report" && f.Basis != "observed" && f.Basis != "hypothesis" || f.Basis == "self_report" && f.Person != f.Observer {
		return fmt.Errorf("invalid temporal attribution")
	}
	switch f.Kind {
	case "circumstance":
		if f.Basis == "observed" || f.FreshFor < 1 || f.MinGap != 0 || f.MaxGap != 0 || len(f.Observations) != 0 || f.WindowStart != 0 || f.ObservedThrough != 0 || f.CompleteChannel {
			return fmt.Errorf("invalid circumstance")
		}
		if f.Category != "responsibility" && f.Category != "transition" && f.Category != "preference" && f.Category != "experience" {
			return fmt.Errorf("unknown circumstance category")
		}
		if f.Signal != "busy" && f.Signal != "routine_changed" && f.Signal != "available" {
			return fmt.Errorf("unknown circumstance signal")
		}
	case "expectation":
		if f.Basis == "observed" || f.Person != f.Observer || f.FreshFor < 1 || f.MinGap < 1 || f.MaxGap < f.MinGap || f.MaxGap > 3650 || f.Signal != "" || f.Category != "" || len(f.Observations) != 0 || f.WindowStart != 0 || f.ObservedThrough != 0 || f.CompleteChannel {
			return fmt.Errorf("invalid contact preference")
		}
	case "contacts":
		if f.Basis != "observed" || f.Person != f.Observer || f.FreshFor != 0 || f.MinGap != 0 || f.MaxGap != 0 || f.Signal != "" || f.Category != "" || len(f.Observations) > 8 || f.WindowStart < 0 || f.ObservedThrough < f.WindowStart || f.ObservedThrough > 1000000000 {
			return fmt.Errorf("invalid contact observations")
		}
		last := LogicalTime(-1)
		for _, at := range f.Observations {
			if at < f.WindowStart || at > f.ObservedThrough || at <= last {
				return fmt.Errorf("unordered contact observations")
			}
			last = at
		}
	default:
		return fmt.Errorf("unknown temporal kind")
	}
	return nil
}

// TemporalEvidence carries current, approved metadata separately from the claim.
// Graph/human hosts must enforce source and account permissions before projecting
// this value. Valid is historical fact validity, not a freshness deadline.
type TemporalEvidence struct {
	Reporter, SourceReporter          ID
	SourceOccurredAt, SourceLearnedAt LogicalTime
	Fact                              TemporalFact
	OccurredAt, LearnedAt             LogicalTime
	Valid                             Interval
	Confidence, SourceConfidence      Confidence
}

func (e TemporalEvidence) Validate(at LogicalTime) error {
	f := e.Fact
	if ids(e.Reporter, e.SourceReporter) != nil || f.Basis == "self_report" && (e.Reporter != f.Person || e.SourceReporter != f.Person) || e.SourceLearnedAt > e.LearnedAt || e.SourceOccurredAt < 0 || e.SourceLearnedAt < e.SourceOccurredAt || e.SourceLearnedAt > at || f.ConfirmedAt > e.SourceOccurredAt || f.Kind == "contacts" && f.ObservedThrough > e.SourceOccurredAt || f.Validate() != nil || e.Valid.Validate() != nil || e.OccurredAt < 0 || e.LearnedAt < e.OccurredAt || e.LearnedAt > at || e.Confidence.Validate() != nil || e.SourceConfidence.Validate() != nil || f.ConfirmedAt > e.OccurredAt || f.Kind == "contacts" && f.ObservedThrough > e.OccurredAt {
		return fmt.Errorf("invalid temporal evidence/time")
	}
	return nil
}

type ContactRhythm struct {
	Basis          string // explicit, estimated, unknown
	MinGap, MaxGap LogicalTime
	Confidence     Confidence
	Samples        int
}
type TemporalContext struct {
	Version                  string
	Observer, Other, Channel ID
	Rhythm                   ContactRhythm
	GapKnown                 bool
	ObservedGap              LogicalTime // time since a permitted observation in this channel only
	Status                   string      // unknown, routine, unusual_gap, busy, changed, stale
	Applicability            string      // unknown, current, stale
	ApplicabilityConfidence  Confidence
	Recommendation           string // wait, clarify
	Evidence                 []ID
}

// InterpretTemporal keeps every observer separate. Sparse observations and an
// incomplete channel diary cannot establish absence of contact. No output names
// rejection, relationship deterioration, hidden activity or numerical maturity.
func InterpretTemporal(focus TemporalFocus, records []TemporalEvidence, at LogicalTime) (TemporalContext, error) {
	if focus.Validate() != nil || at < 0 || at > 1000000000 || len(records) > 16 {
		return TemporalContext{}, fmt.Errorf("invalid temporal input")
	}
	out := TemporalContext{Version: TemporalPolicyVersion, Observer: focus.Observer, Other: focus.Other, Channel: focus.Channel, Rhythm: ContactRhythm{Basis: "unknown"}, Status: "unknown", Applicability: "unknown", Recommendation: "wait", Evidence: []ID{}}
	seen := map[[2]ID]bool{}
	prefs := []TemporalEvidence{}
	diaries := []TemporalEvidence{}
	life := []TemporalEvidence{}
	for _, e := range records {
		if e.Validate(at) != nil {
			return TemporalContext{}, fmt.Errorf("invalid temporal record")
		}
		f := e.Fact
		key := [2]ID{f.Observer, f.Account}
		if seen[key] {
			return TemporalContext{}, fmt.Errorf("duplicate temporal account")
		}
		seen[key] = true
		if f.Observer != focus.Observer || f.With != focus.Other || f.Channel != focus.Channel || e.Valid.Start > at || e.Valid.End != nil && at >= *e.Valid.End {
			continue
		}
		out.Evidence = append(out.Evidence, f.Account, f.Source)
		if f.Basis == "hypothesis" || float64(e.Confidence*e.SourceConfidence) < .5 {
			continue
		}
		switch f.Kind {
		case "expectation":
			prefs = append(prefs, e)
		case "contacts":
			diaries = append(diaries, e)
		case "circumstance":
			life = append(life, e)
		}
	}
	// Multiple current diaries/preferences are competing accounts, not additive
	// evidence. Corrections must be resolved by the authorized graph projection.
	if len(diaries) != 1 || len(prefs) > 1 {
		return out, nil
	}
	diary := diaries[0]
	obs := diary.Fact.Observations
	if len(prefs) == 1 {
		p := prefs[0]
		out.Rhythm = ContactRhythm{Basis: "explicit", MinGap: p.Fact.MinGap, MaxGap: p.Fact.MaxGap, Confidence: p.Confidence * p.SourceConfidence}
	} else if len(obs) >= 4 && diary.Fact.CompleteChannel {
		gaps := []int64{}
		for i := 1; i < len(obs); i++ {
			gaps = append(gaps, int64(obs[i]-obs[i-1]))
		}
		sort.Slice(gaps, func(i, j int) bool { return gaps[i] < gaps[j] })
		// A wide spread is irregular, not an invitation to force regular contact.
		if gaps[len(gaps)-1] <= 1825 && gaps[len(gaps)-1] <= 4*gaps[0] {
			out.Rhythm = ContactRhythm{Basis: "estimated", MinGap: LogicalTime(gaps[0]), MaxGap: LogicalTime(2 * gaps[len(gaps)-1]), Confidence: Confidence(.5 * float64(diary.Confidence*diary.SourceConfidence)), Samples: len(obs)}
		}
	}
	if len(obs) == 0 || !diary.Fact.CompleteChannel || diary.Fact.ObservedThrough < at || out.Rhythm.Basis == "unknown" {
		return out, nil
	}
	out.GapKnown = true
	out.ObservedGap = at - obs[len(obs)-1]
	out.Status = "routine"
	out.Applicability = "current"
	out.ApplicabilityConfidence = out.Rhythm.Confidence
	if out.ObservedGap > out.Rhythm.MaxGap {
		out.Status = "unusual_gap"
		out.Recommendation = "clarify"
	}
	stale := false
	changed := false
	busy := false
	if len(prefs) == 1 {
		p := prefs[0]
		age := at - p.Fact.ConfirmedAt
		out.ApplicabilityConfidence = Confidence(float64(out.Rhythm.Confidence) * math.Exp2(-float64(age)/float64(p.Fact.FreshFor)))
		stale = age > p.Fact.FreshFor
	}
	for _, e := range life {
		age := at - e.Fact.ConfirmedAt
		confidence := float64(e.Confidence*e.SourceConfidence) * math.Exp2(-float64(age)/float64(e.Fact.FreshFor))
		if confidence < float64(out.ApplicabilityConfidence) {
			out.ApplicabilityConfidence = Confidence(confidence)
		}
		if age > e.Fact.FreshFor {
			stale = true
			continue
		}
		busy = busy || e.Fact.Signal == "busy"
		if e.Fact.Signal == "routine_changed" {
			if len(prefs) == 1 {
				changed = changed || prefs[0].Fact.ConfirmedAt < e.Fact.ConfirmedAt
			} else {
				changed = changed || obs[0] < e.Fact.ConfirmedAt
			}
		}
	}
	if stale {
		out.Status = "stale"
		out.Applicability = "stale"
		out.Recommendation = "clarify"
	}
	if changed {
		out.Status = "changed"
		out.Applicability = "unknown"
		out.ApplicabilityConfidence = 0
		out.Recommendation = "clarify"
	}
	if busy {
		out.Status = "busy"
		out.Recommendation = "wait"
	}
	return out, nil
}

func (f TemporalFact) Encode() ([]byte, error) {
	if f.Validate() != nil {
		return nil, fmt.Errorf("invalid temporal fact")
	}
	b, e := json.Marshal(f)
	if e != nil || len(b) > 4096 {
		return nil, fmt.Errorf("temporal fact byte bound")
	}
	return b, nil
}
