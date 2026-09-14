package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const BoundaryVersion = "interpersonal-boundary.v1"
const InteractionScopeVersion = "interaction-scope.v1"
const AllTopics ID = "all_topics"

type InteractionClass string

const (
	PrivatePreparation    InteractionClass = "private_preparation"
	Discussion            InteractionClass = "discussion"
	Coordination          InteractionClass = "coordination"
	SummarySharing        InteractionClass = "summary_sharing"
	ThirdPartyInvolvement InteractionClass = "third_party_involvement"
	Clarification         InteractionClass = "clarification"
	AllInteractions       InteractionClass = "all_interactions"
)

func (c InteractionClass) Valid() bool {
	return c == PrivatePreparation || c == Discussion || c == Coordination || c == SummarySharing || c == ThirdPartyInvolvement || c == Clarification
}

// Target is the person affected by the intended interaction, even when it is
// relayed through Via. A relay never replaces the original relationship endpoint.
// These are trusted attributed intent fields, not inferred from arbitrary prose.
type InteractionScope struct {
	Version           string
	Initiator, Target ID
	Topic             ID
	Class             InteractionClass
	Via               ID `json:",omitempty"`
}

func (s InteractionScope) Validate() error {
	if s.Version != InteractionScopeVersion || ids(s.Initiator, s.Target, s.Topic) != nil || s.Initiator == s.Target || s.Topic == AllTopics || !s.Class.Valid() {
		return fmt.Errorf("invalid interpersonal scope")
	}
	if s.Via != "" && (s.Via.Validate() != nil || s.Via == s.Initiator || s.Via == s.Target) || s.Class == ThirdPartyInvolvement && s.Via == "" {
		return fmt.Errorf("invalid relay scope")
	}
	if s.Class == PrivatePreparation && s.Via != "" {
		return fmt.Errorf("private preparation cannot involve a third party")
	}
	return nil
}
func (s InteractionScope) Participants() []ID {
	if s.Class == PrivatePreparation {
		return []ID{s.Initiator}
	}
	out := []ID{s.Initiator, s.Target}
	if s.Via != "" {
		out = append(out, s.Via)
	}
	return out
}

type Willingness string

const (
	Willing            Willingness = "willing"
	Declined           Willingness = "declined"
	TakingBreak        Willingness = "break"
	Ended              Willingness = "ended"
	Dismissed          Willingness = "dismissed"
	WillingnessUnknown Willingness = "unknown"
	PressureSignal     Willingness = "pressure_signal"
)

// Boundary is an observer-owned policy fact, separate from graph data grants.
// Only self_report by Principal can establish affirmative willingness. Other
// observers' interpretations never manufacture that person's consent.
type Boundary struct {
	Version               string
	Meta                  Metadata
	Principal, With       ID
	Topic                 ID
	Class                 InteractionClass
	Decision              Willingness
	Basis                 string // self_report, hypothesis, observed_signal
	Signal                string `json:",omitempty"` // disagreement, uncertain_pressure, credible_pressure, credible_threat
	OccurredAt, LearnedAt LogicalTime
	Supersedes            []ID        `json:",omitempty"`
	Revoked               bool        `json:",omitempty"`
	RevokedAt             LogicalTime `json:",omitempty"`
	RevokedBy             ID          `json:",omitempty"`
}

func (b Boundary) Validate() error {
	if len(b.Meta.Parents) > 8 || len(b.Meta.Contradicting) > 8 || len(b.Meta.Rights.Grants) > 8 {
		return fmt.Errorf("boundary provenance bound")
	}
	if b.Version != BoundaryVersion || b.Meta.Validate() != nil || ids(b.Principal, b.With, b.Topic) != nil || b.Principal == b.With || (!b.Class.Valid() && b.Class != AllInteractions) || b.OccurredAt.Validate() != nil || b.LearnedAt < b.OccurredAt || len(b.Meta.Supporting) < 1 || len(b.Meta.Supporting) > 8 || len(b.Supersedes) > 8 || unique(b.Supersedes) != nil {
		return fmt.Errorf("invalid boundary evidence")
	}
	switch b.Decision {
	case Willing, Declined, TakingBreak, Ended, Dismissed, WillingnessUnknown, PressureSignal:
	default:
		return fmt.Errorf("unknown willingness")
	}
	switch b.Basis {
	case "self_report":
		if b.Meta.Observer != b.Principal || b.Meta.Source != b.Principal {
			return fmt.Errorf("another observer cannot assert self-report")
		}
	case "hypothesis", "observed_signal":
	default:
		return fmt.Errorf("unknown boundary basis")
	}
	if b.Decision == Willing && (b.Topic == AllTopics || b.Class == AllInteractions) {
		return fmt.Errorf("no blanket affirmative consent")
	}
	if b.Decision == TakingBreak && b.Meta.Valid.End == nil || b.Decision == Ended && b.Meta.Valid.End != nil {
		return fmt.Errorf("invalid break or ending interval")
	}
	if b.Decision == PressureSignal {
		if b.Basis != "observed_signal" || b.Signal != "disagreement" && b.Signal != "uncertain_pressure" && b.Signal != "credible_pressure" && b.Signal != "credible_threat" {
			return fmt.Errorf("unknown pressure evidence")
		}
	} else if b.Signal != "" {
		return fmt.Errorf("unexpected pressure signal")
	}
	if b.Revoked {
		if b.RevokedBy != b.Meta.Observer || b.RevokedAt < b.LearnedAt {
			return fmt.Errorf("invalid revocation provenance")
		}
	} else if b.RevokedBy != "" || b.RevokedAt != 0 {
		return fmt.Errorf("unexpected revocation")
	}
	return nil
}

// ValidateBoundaryLog preserves conflicting independent accounts. Corrections may
// supersede only earlier facts from the same observer, principal and exact scope.
func ValidateBoundaryLog(records []Boundary) error {
	if len(records) > 64 {
		return fmt.Errorf("boundary log bound")
	}
	seen := map[ID]Boundary{}
	for _, b := range records {
		if b.Validate() != nil {
			return fmt.Errorf("invalid boundary log")
		}
		if _, ok := seen[b.Meta.ID]; ok {
			return fmt.Errorf("duplicate boundary id")
		}
		for _, id := range b.Supersedes {
			old, ok := seen[id]
			if !ok || old.Meta.Observer != b.Meta.Observer || old.Principal != b.Principal || old.With != b.With || old.Topic != b.Topic || old.Class != b.Class || old.Basis != b.Basis || old.LearnedAt > b.LearnedAt {
				return fmt.Errorf("invalid boundary correction")
			}
		}
		seen[b.Meta.ID] = b
	}
	return nil
}

type BoundaryDecision struct {
	Allowed bool
	Code    string
}

func deniedBoundary(code string) BoundaryDecision { return BoundaryDecision{Code: code} }

// EvaluateBoundaries consumes a complete trusted policy snapshot. It emits only
// a bounded gate decision, never private reasons/source identities for a planner.
// Revocation is current: backdated replay cannot restore a withdrawn preference.
func EvaluateBoundaries(records []Boundary, scope InteractionScope, at LogicalTime) (BoundaryDecision, error) {
	if scope.Validate() != nil || at.Validate() != nil || ValidateBoundaryLog(records) != nil {
		return BoundaryDecision{}, fmt.Errorf("invalid boundary query")
	}
	classes := []InteractionClass{scope.Class}
	if scope.Via != "" {
		for _, class := range []InteractionClass{Discussion, ThirdPartyInvolvement} {
			if class != scope.Class {
				classes = append(classes, class)
			}
		}
	}
	for _, class := range classes {
		query := scope
		query.Class = class
		d := evaluateBoundaryClass(records, query, at)
		if !d.Allowed {
			return d, nil
		}
	}
	return BoundaryDecision{Allowed: true, Code: "explicit_current_preferences"}, nil
}

func evaluateBoundaryClass(records []Boundary, scope InteractionScope, at LogicalTime) BoundaryDecision {
	superseded := map[ID]bool{}
	for _, b := range records {
		if b.LearnedAt <= at {
			for _, id := range b.Supersedes {
				superseded[id] = true
			}
		}
	}
	for _, principal := range scope.Participants() {
		cutoff := LogicalTime(-1)
		affirmative := []Boundary{}
		dismissals := 0
		for _, b := range records {
			// Each participant owns a perspective on the original pair. For a relay,
			// the relay's own willingness is additionally required toward the initiator.
			other := scope.Initiator
			if principal == scope.Initiator {
				other = scope.Target
			}
			if b.Principal != principal || b.With != other || b.Topic != scope.Topic && b.Topic != AllTopics || b.Class != scope.Class && b.Class != AllInteractions && b.Decision != PressureSignal || superseded[b.Meta.ID] {
				continue
			}
			if b.Revoked || b.Meta.Rights.Revoked {
				if b.Basis == "self_report" {
					withdrawn := b.RevokedAt
					if b.Meta.Rights.Revoked && withdrawn == 0 {
						withdrawn = at
					}
					if withdrawn > cutoff {
						cutoff = withdrawn
					}
				}
				continue
			}
			if b.LearnedAt > at || b.Meta.Valid.Start > at {
				continue
			}
			expired := b.Meta.Valid.End != nil && at >= *b.Meta.Valid.End
			if b.Decision == PressureSignal {
				if !expired && b.Signal != "disagreement" && scope.Class != PrivatePreparation {
					return deniedBoundary("pressure_or_uncertainty")
				}
				continue
			}
			if b.Basis != "self_report" {
				continue
			}
			if b.Decision == Willing {
				if !expired {
					affirmative = append(affirmative, b)
				}
				continue
			}
			// An expired pause/refusal does not revive consent from before the pause.
			if expired {
				if *b.Meta.Valid.End > cutoff {
					cutoff = *b.Meta.Valid.End
				}
				continue
			}
			if b.Decision == Dismissed {
				dismissals++
				continue
			}
			if b.Decision == WillingnessUnknown {
				return deniedBoundary("unknown")
			}
			return deniedBoundary("refused_or_paused")
		}
		if dismissals > 1 {
			return deniedBoundary("repeated_dismissal")
		}
		if dismissals == 1 {
			return deniedBoundary("dismissed")
		}
		granted := false
		for _, b := range affirmative {
			granted = granted || b.LearnedAt > cutoff
		}
		if !granted {
			return deniedBoundary("unknown")
		}
	}
	return BoundaryDecision{Allowed: true, Code: "explicit_current_preferences"}
}
func EncodeBoundaryLog(records []Boundary) ([]byte, error) {
	if ValidateBoundaryLog(records) != nil {
		return nil, fmt.Errorf("invalid boundaries")
	}
	b, e := json.Marshal(struct {
		Version string
		Records []Boundary
	}{BoundaryVersion, records})
	if e != nil || len(b) > 131072 {
		return nil, fmt.Errorf("boundary wire bound")
	}
	return b, nil
}
func DecodeBoundaryLog(raw []byte) ([]Boundary, error) {
	if len(raw) > 131072 {
		return nil, fmt.Errorf("boundary wire bound")
	}
	var value struct {
		Version string
		Records []Boundary
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&value) != nil || d.Decode(new(any)) != io.EOF || value.Version != BoundaryVersion || ValidateBoundaryLog(value.Records) != nil {
		return nil, fmt.Errorf("invalid boundary encoding")
	}
	return value.Records, nil
}
