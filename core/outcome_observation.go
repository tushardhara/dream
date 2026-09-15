package core

import (
	"encoding/json"
	"fmt"
	"sort"
)

const OutcomeObservationVersion = "outcome-observation.v1"
const MaxOutcomeObservations = 128

// OutcomeObservation is an attributed account, not an objective welfare fact.
// Expected sender benefit and observed recipient experience remain different
// records. Reply is an explicit causal link, never inferred from action kind.
type OutcomeObservation struct {
	Version                    string   `json:"version"`
	Meta                       Metadata `json:"meta"`
	Interaction, Action, Reply ID
	Participant, Other         ID
	Focus                      RelationshipFocus
	Kind                       string // expected, observed
	Position                   string // sender, recipient; a sender feeling useful is not recipient benefit
	Phase                      string // immediate, later
	Basis                      string // self_report, reported, unobserved
	OccurredAt, LearnedAt      LogicalTime
	Status                     OutcomeStatus
	Appraisal                  string // supportive, dismissive, neutral, mixed, unresolved
	Benefit, Burden            *float64
	Missing                    string // silence, lost_observation, declined_participation, missing_followup
	Supersedes                 ID     `json:",omitempty"`
}

func (o OutcomeObservation) Validate() error {
	if o.Version != OutcomeObservationVersion || o.Meta.Validate() != nil || ids(o.Interaction, o.Action, o.Participant, o.Other) != nil || o.Participant == o.Other || o.Focus.Validate() != nil || o.Focus.RoleContext == "" || o.OccurredAt < 0 || o.LearnedAt < o.OccurredAt || o.Meta.ID == o.Action || o.Meta.ID == o.Interaction {
		return fmt.Errorf("invalid outcome observation")
	}
	if o.Reply != "" && (o.Reply.Validate() != nil || o.Reply == o.Action) || o.Supersedes != "" && (o.Supersedes.Validate() != nil || o.Supersedes == o.Meta.ID) {
		return fmt.Errorf("invalid outcome links")
	}
	if o.Position != "sender" && o.Position != "recipient" {
		return fmt.Errorf("invalid outcome position")
	}
	if o.Kind != "expected" && o.Kind != "observed" || o.Phase != "immediate" && o.Phase != "later" {
		return fmt.Errorf("invalid outcome kind/phase")
	}
	switch o.Basis {
	case "self_report":
		if o.Meta.Observer != o.Participant || o.Meta.Source != o.Participant {
			return fmt.Errorf("foreign self-report")
		}
	case "reported":
		if o.Meta.Observer != o.Other || o.Meta.Source != o.Participant || o.Reply == "" {
			return fmt.Errorf("unattributed reported outcome")
		}
	case "unobserved":
		if o.Status == Observed {
			return fmt.Errorf("missing report asserts observation")
		}
	default:
		return fmt.Errorf("invalid outcome basis")
	}
	if o.Meta.Observer != o.Participant && o.Meta.Observer != o.Other {
		return fmt.Errorf("unrelated outcome observer")
	}
	switch o.Status {
	case Observed:
		if len(o.Meta.Supporting) == 0 || o.Missing != "" {
			return fmt.Errorf("observed outcome needs evidence")
		}
		switch o.Appraisal {
		case "supportive", "dismissive", "neutral", "mixed":
			if o.Benefit == nil || o.Burden == nil {
				return fmt.Errorf("observed dimensions missing")
			}
		case "unresolved":
			if o.Benefit != nil || o.Burden != nil {
				return fmt.Errorf("unresolved outcome asserts benefit")
			}
		default:
			return fmt.Errorf("invalid appraisal")
		}
	case Unknown, Censored:
		if o.Appraisal != "" || o.Benefit != nil || o.Burden != nil {
			return fmt.Errorf("missing outcome asserts appraisal")
		}
		switch o.Missing {
		case "silence", "lost_observation", "declined_participation", "missing_followup":
		default:
			return fmt.Errorf("missing outcome reason")
		}
	default:
		return fmt.Errorf("invalid outcome status")
	}
	for _, v := range []*float64{o.Benefit, o.Burden} {
		if v != nil && !(*v >= -1 && *v <= 1) {
			return fmt.Errorf("invalid outcome dimension")
		}
	}
	return nil
}
func sameOutcomeAccount(a, b OutcomeObservation) bool {
	return a.Meta.Observer == b.Meta.Observer && a.Participant == b.Participant && a.Other == b.Other && a.Interaction == b.Interaction && a.Action == b.Action && a.Kind == b.Kind && a.Position == b.Position && a.Phase == b.Phase && a.Focus.Domain == b.Focus.Domain && a.Focus.RoleContext == b.Focus.RoleContext && a.Focus.Account == b.Focus.Account
}

// ValidateOutcomeLog validates append order and immutable correction identity.
// Concurrent contradictory accounts are preserved; selection does not average them.
func ValidateOutcomeLog(log []OutcomeObservation) error {
	if len(log) > MaxOutcomeObservations {
		return fmt.Errorf("outcome ledger bound")
	}
	seen := map[ID]OutcomeObservation{}
	for _, o := range log {
		if o.Validate() != nil {
			return fmt.Errorf("invalid outcome ledger entry")
		}
		if _, ok := seen[o.Meta.ID]; ok {
			return fmt.Errorf("duplicate outcome identity")
		}
		if o.Supersedes != "" {
			old, ok := seen[o.Supersedes]
			if !ok || !sameOutcomeAccount(old, o) || o.LearnedAt < old.LearnedAt || o.Meta.RecordedAt.Before(old.Meta.RecordedAt) {
				return fmt.Errorf("invalid outcome correction")
			}
		}
		seen[o.Meta.ID] = o
	}
	return nil
}
func AppendOutcome(log []OutcomeObservation, o OutcomeObservation) ([]OutcomeObservation, error) {
	out := append(append([]OutcomeObservation{}, log...), o)
	if ValidateOutcomeLog(out) != nil {
		return nil, fmt.Errorf("invalid outcome append")
	}
	raw, _ := json.Marshal(out)
	var owned []OutcomeObservation
	if json.Unmarshal(raw, &owned) != nil {
		return nil, fmt.Errorf("outcome copy")
	}
	return owned, nil
}

// CurrentOutcomes resolves only already-learned corrections. Current rights still
// govern historical views, and revoking a correction never resurrects its target.
// Caller supplies exact data scope; receipt/delivery grants no permission.
func CurrentOutcomes(log []OutcomeObservation, observer ID, focus RelationshipFocus, at LogicalTime, grant Grant) ([]OutcomeObservation, error) {
	if ValidateOutcomeLog(log) != nil || observer.Validate() != nil || focus.Validate() != nil || focus.RoleContext == "" || at < 0 || grant.Validate() != nil {
		return nil, fmt.Errorf("invalid outcome projection")
	}
	superseded := map[ID]bool{}
	for _, o := range log {
		if o.LearnedAt <= at && o.Supersedes != "" {
			superseded[o.Supersedes] = true
		}
	}
	out := []OutcomeObservation{}
	for _, o := range log {
		if o.Meta.Observer != observer || o.Focus.Domain != focus.Domain || o.Focus.RoleContext != focus.RoleContext || focus.Account != "" && o.Focus.Account != focus.Account || o.LearnedAt > at || o.OccurredAt > at || superseded[o.Meta.ID] || o.Meta.Valid.Start > at || o.Meta.Valid.End != nil && at >= *o.Meta.Valid.End {
			continue
		}
		if !o.Meta.Rights.Allows(PermissionRequest{Resource: o.Meta.ID, Context: grant}) {
			continue
		}
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LearnedAt != out[j].LearnedAt {
			return out[i].LearnedAt < out[j].LearnedAt
		}
		return out[i].Meta.ID < out[j].Meta.ID
	})
	raw, _ := json.Marshal(out)
	var owned []OutcomeObservation
	_ = json.Unmarshal(raw, &owned)
	return owned, nil
}
