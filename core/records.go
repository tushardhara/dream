package core

import (
	"fmt"
	"time"
)

type Sensitivity string

const (
	Public     Sensitivity = "public"
	Restricted Sensitivity = "restricted"
)

// Metadata is observer-owned provenance, not omniscient truth. Parents are
// derived-from record IDs; Evidence names supporting source evidence separately.
type Metadata struct {
	ID            ID                     `json:"id"`
	Observer      ID                     `json:"observer"`
	Source        ID                     `json:"source"`
	Sensitivity   Sensitivity            `json:"sensitivity"`
	Parents       []ID                   `json:"parents"`
	Supporting    []ID                   `json:"supporting"`
	Contradicting []ID                   `json:"contradicting"`
	Confidence    Confidence             `json:"confidence"`
	Probability   *CalibratedProbability `json:"probability,omitempty"`
	Valid         Interval               `json:"valid"`
	RecordedAt    time.Time              `json:"recorded_at"`
	Rights        Rights                 `json:"rights"`
}

func (m Metadata) Validate() error {
	if err := ids(m.ID, m.Observer, m.Source); err != nil {
		return err
	}
	if m.Sensitivity != Public && m.Sensitivity != Restricted {
		return fmt.Errorf("unknown sensitivity")
	}
	for _, list := range [][]ID{m.Parents, m.Supporting, m.Contradicting} {
		if err := unique(list); err != nil {
			return err
		}
		for _, id := range list {
			if id == m.ID {
				return fmt.Errorf("self provenance reference")
			}
		}
	}
	for _, a := range m.Supporting {
		for _, b := range m.Contradicting {
			if a == b {
				return fmt.Errorf("same evidence supports and contradicts one record")
			}
		}
	}
	if err := m.Confidence.Validate(); err != nil {
		return err
	}
	if m.Probability != nil {
		if err := m.Probability.Validate(); err != nil {
			return err
		}
	}
	if err := m.Valid.Validate(); err != nil {
		return err
	}
	if err := recorded(m.RecordedAt); err != nil {
		return err
	}
	if err := m.Rights.Validate(); err != nil {
		return err
	}
	if m.Rights.Resource != m.ID {
		return fmt.Errorf("rights resource mismatch")
	}
	return nil
}

type Evidence struct {
	Meta        Metadata `json:"meta"`
	Description string   `json:"description"`
}

func (v Evidence) Validate() error {
	if err := v.Meta.Validate(); err != nil {
		return err
	}
	return text(v.Description)
}

// Event scope is a generic stream, with no mandatory simulation identifiers.
type Event struct {
	Meta       Metadata    `json:"meta"`
	Version    uint32      `json:"version"`
	Stream     ID          `json:"stream"`
	Type       ID          `json:"type"`
	Subject    Subject     `json:"subject"`
	OccurredAt LogicalTime `json:"occurred_at"`
}

func (v Event) Validate() error {
	if err := v.Meta.Validate(); err != nil {
		return err
	}
	if v.Version != 1 {
		return fmt.Errorf("unsupported event version")
	}
	if err := ids(v.Stream, v.Type); err != nil {
		return err
	}
	if err := v.OccurredAt.Validate(); err != nil {
		return err
	}
	return v.Subject.ValidateFor(v.Meta.Observer)
}

type Claim struct {
	Meta        Metadata `json:"meta"`
	Subject     Subject  `json:"subject"`
	Proposition string   `json:"proposition"`
}

func (v Claim) Validate() error {
	if err := v.Meta.Validate(); err != nil {
		return err
	}
	if len(v.Meta.Supporting)+len(v.Meta.Contradicting) == 0 {
		return fmt.Errorf("claim needs evidence")
	}
	if err := v.Subject.ValidateFor(v.Meta.Observer); err != nil {
		return err
	}
	return text(v.Proposition)
}

type Hypothesis struct {
	Meta        Metadata `json:"meta"`
	Subject     Subject  `json:"subject"`
	Proposition string   `json:"proposition"`
}

func (v Hypothesis) Validate() error {
	if err := v.Meta.Validate(); err != nil {
		return err
	}
	if err := v.Subject.ValidateFor(v.Meta.Observer); err != nil {
		return err
	}
	return text(v.Proposition)
}

type Relationship struct {
	Meta Metadata `json:"meta"`
	From Subject  `json:"from"`
	To   Subject  `json:"to"`
	Kind ID       `json:"kind"`
}

func (v Relationship) Validate() error {
	if err := v.Meta.Validate(); err != nil {
		return err
	}
	if err := v.From.ValidateFor(v.Meta.Observer); err != nil {
		return err
	}
	if err := v.To.ValidateFor(v.Meta.Observer); err != nil {
		return err
	}
	return v.Kind.Validate()
}

type Group struct {
	Meta    Metadata  `json:"meta"`
	Members []Subject `json:"members"`
}

func (v Group) Validate() error {
	if err := v.Meta.Validate(); err != nil {
		return err
	}
	if len(v.Members) == 0 {
		return fmt.Errorf("group needs members")
	}
	seen := map[string]bool{}
	for _, s := range v.Members {
		if err := s.ValidateFor(v.Meta.Observer); err != nil {
			return err
		}
		key := subjectKey(s)
		if seen[key] {
			return fmt.Errorf("duplicate group member")
		}
		seen[key] = true
	}
	return nil
}

type Memory struct {
	Meta    Metadata `json:"meta"`
	Content string   `json:"content"`
}

func (v Memory) Validate() error {
	if err := v.Meta.Validate(); err != nil {
		return err
	}
	return text(v.Content)
}

type OpenLoop struct {
	Meta        Metadata     `json:"meta"`
	Description string       `json:"description"`
	Due         *LogicalTime `json:"due,omitempty"`
	Closed      bool         `json:"closed"`
}

func (v OpenLoop) Validate() error {
	if err := v.Meta.Validate(); err != nil {
		return err
	}
	if v.Due != nil && *v.Due < v.Meta.Valid.Start {
		return fmt.Errorf("due before valid start")
	}
	return text(v.Description)
}

type Intent struct {
	Meta        Metadata `json:"meta"`
	Actor       ID       `json:"actor"`
	Description string   `json:"description"`
}

func (v Intent) Validate() error {
	if err := v.Meta.Validate(); err != nil {
		return err
	}
	if err := v.Actor.Validate(); err != nil {
		return err
	}
	return text(v.Description)
}

type Proposal struct {
	Meta   Metadata `json:"meta"`
	Intent ID       `json:"intent"`
	Action ID       `json:"action"`
}

func (v Proposal) Validate() error {
	if err := v.Meta.Validate(); err != nil {
		return err
	}
	return ids(v.Intent, v.Action)
}

type DecisionKind string

const (
	Act  DecisionKind = "ACT"
	Wait DecisionKind = "WAIT"
)

type Decision struct {
	Meta     Metadata     `json:"meta"`
	Actor    ID           `json:"actor"`
	Kind     DecisionKind `json:"kind"`
	Proposal ID           `json:"proposal,omitempty"`
	Reason   string       `json:"reason"`
}

func (v Decision) Validate() error {
	if err := v.Meta.Validate(); err != nil {
		return err
	}
	if err := v.Actor.Validate(); err != nil {
		return err
	}
	switch v.Kind {
	case Act:
		if err := v.Proposal.Validate(); err != nil {
			return err
		}
	case Wait:
		if v.Proposal != "" {
			return fmt.Errorf("WAIT cannot select a proposal")
		}
	default:
		return fmt.Errorf("unknown decision")
	}
	return text(v.Reason)
}

type OutcomeStatus string

const (
	Observed OutcomeStatus = "observed"
	Unknown  OutcomeStatus = "unknown"
	Censored OutcomeStatus = "censored"
)

type Outcome struct {
	Meta              Metadata      `json:"meta"`
	Decision          ID            `json:"decision"`
	AffectedObserver  ID            `json:"affected_observer"`
	ImmediateResponse string        `json:"immediate_response,omitempty"`
	Horizon           LogicalTime   `json:"horizon"`
	Status            OutcomeStatus `json:"status"`
}

func (v Outcome) Validate() error {
	if err := v.Meta.Validate(); err != nil {
		return err
	}
	if err := ids(v.Decision, v.AffectedObserver); err != nil {
		return err
	}
	if v.Horizon < v.Meta.Valid.Start {
		return fmt.Errorf("horizon before valid start")
	}
	switch v.Status {
	case Observed:
		if len(v.Meta.Supporting) == 0 {
			return fmt.Errorf("observed outcome needs evidence")
		}
		return text(v.ImmediateResponse)
	case Unknown, Censored:
		if v.ImmediateResponse != "" {
			return fmt.Errorf("unobserved outcome cannot assert response")
		}
	default:
		return fmt.Errorf("unknown outcome status")
	}
	return nil
}
