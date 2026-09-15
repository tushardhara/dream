package assistance

import "github.com/tushardhara/dream/core"

const OutcomeReportVersion = "helper-outcome-report.v1"

// OutcomeReport contains a separately authorized participant account. Private
// model state, supporting source IDs and private lineage never enter this port.
// Expected usefulness remains distinct from the recipient's observed experience.
type OutcomeReport struct {
	Version                                                              string
	ID, Interaction, Action, Reply, Observer, Source, Participant, Other core.ID
	Focus                                                                core.RelationshipFocus
	Kind, Position, Phase, Appraisal, Missing                            string
	Status                                                               core.OutcomeStatus
	OccurredAt, LearnedAt                                                core.LogicalTime
	Confidence                                                           core.Confidence
	Benefit, Burden                                                      *float64
}

// OutcomeAction is a trusted host receipt for an independently chosen human
// action after advice. A helper delivery is not this receipt or an outcome.
type OutcomeAction struct {
	Version                                string
	Interaction, Action, Sender, Recipient core.ID
	At, EffectAt                           core.LogicalTime
	Replies                                []core.ID
}

// OutcomeReports is a current permission projection, not a truth aggregator.
// Each observer/phase remains independent. Revoked corrections cannot resurrect
// old accounts. No report is created merely because the helper delivered a step.
func OutcomeReports(i Interaction, action OutcomeAction, log []core.OutcomeObservation, helper core.ID, at core.LogicalTime) ([]OutcomeReport, error) {
	if i.Result.Version != i.Version || i.ID.Validate() != nil || i.At.Validate() != nil || at.Validate() != nil || action.At > at || i.Scope == nil || i.Scope.Validate() != nil || i.Scope.Initiator != i.User || i.Helper != helper || helper.Validate() != nil || !UsesDomainContext(i.Version) || i.Focus == nil || i.Focus.Validate() != nil || !i.Delivered || i.Result.Selected < 0 || i.Result.Selected >= len(i.Result.Candidates) || core.ValidateOutcomeLog(log) != nil {
		return nil, ErrInvalid
	}
	selected := i.Result.Candidates[i.Result.Selected]
	if selected.Action != Propose || selected.Recipient != i.User || action.Version != OutcomeReportVersion || action.Interaction != i.ID || action.Action.Validate() != nil || action.Sender != i.User || action.Recipient != i.Scope.Target || action.Recipient == i.User || action.At <= i.At || action.EffectAt <= action.At || action.EffectAt-action.At > 1000000 || len(action.Replies) > 16 {
		return nil, ErrInvalid
	}
	replies := map[core.ID]bool{}
	for _, id := range action.Replies {
		if id.Validate() != nil || id == action.Action || replies[id] {
			return nil, ErrInvalid
		}
		replies[id] = true
	}
	out := []OutcomeReport{}
	focus := *i.Focus
	focus.Account = ""
	for _, owner := range []core.ID{i.User, action.Recipient} {
		current, e := core.CurrentOutcomes(log, owner, focus, at, core.Grant{Actor: helper, Recipient: helper, Purpose: "help", Operation: core.Read})
		if e != nil {
			return nil, e
		}
		for _, o := range current {
			if o.Interaction != i.ID {
				continue
			}
			if o.Action != action.Action || (o.Basis != "self_report" && o.Basis != "unobserved") || o.Meta.Observer != o.Participant || o.Meta.Source != o.Participant || o.OccurredAt < action.At || o.Position == "recipient" && o.OccurredAt < action.EffectAt || o.Reply != "" && !replies[o.Reply] {
				return nil, ErrInvalid
			}
			if o.Position == "recipient" && (o.Participant != action.Recipient || o.Other != i.User) || o.Position == "sender" && (o.Participant != i.User || o.Other != action.Recipient) {
				return nil, ErrInvalid
			}
			reportFocus := o.Focus
			reportFocus.Account = ""
			out = append(out, OutcomeReport{Version: OutcomeReportVersion, ID: o.Meta.ID, Interaction: o.Interaction, Action: o.Action, Reply: o.Reply, Observer: o.Meta.Observer, Source: o.Meta.Source, Participant: o.Participant, Other: o.Other, Focus: reportFocus, Kind: o.Kind, Position: o.Position, Phase: o.Phase, Appraisal: o.Appraisal, Missing: o.Missing, Status: o.Status, OccurredAt: o.OccurredAt, LearnedAt: o.LearnedAt, Confidence: o.Meta.Confidence, Benefit: o.Benefit, Burden: o.Burden})
		}
	}
	return clone(out), nil
}
