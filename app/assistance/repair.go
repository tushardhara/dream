package assistance

import (
	"context"
	"github.com/tushardhara/dream/core"
)

const RepairFlowVersion = "repair-flow.v1"

// RepairFollowThroughGap is an explicit fixture duration, not a human-validity threshold.
const RepairFollowThroughGap core.LogicalTime = 20

type RepairRequest struct {
	Version                                              string
	ID, Session, Helper, User, Actor, Recipient, Purpose core.ID
	Focus                                                core.RelationshipFocus
	At                                                   core.LogicalTime
}

func (r RepairRequest) Validate() error {
	for _, id := range []core.ID{r.ID, r.Session, r.Helper, r.User, r.Actor, r.Recipient, r.Purpose} {
		if id.Validate() != nil {
			return ErrInvalid
		}
	}
	if r.Version != RepairFlowVersion || r.Actor == r.Recipient || r.User != r.Actor && r.User != r.Recipient || r.Helper == r.Actor || r.Helper == r.Recipient || r.Focus.Validate() != nil || r.Focus.RoleContext == "" || r.Focus.Account != "" || r.At.Validate() != nil {
		return ErrInvalid
	}
	return nil
}

type RepairSnapshot struct {
	Now        core.LogicalTime
	Log        []core.RepairRecord
	Boundaries []core.Boundary
}

// RepairJournal authenticates the exact request and reads current rights/evidence
// under the SAME lock/transaction used for correction, revocation and delivery.
// The callback is bounded pure computation: no model, network or outreach.
type RepairJournal interface {
	Visit(context.Context, RepairRequest, func(RepairSnapshot) (RepairResponse, error)) (RepairResponse, error)
}
type RepairAssessment struct {
	Source     core.ID
	At         core.LogicalTime
	Assessment string
}
type RepairPerspective struct {
	Observer                              core.ID
	Acknowledgements, Apologies, Attempts int
	Fulfilled, Breached                   []core.ID // distinct commitments, never message counts
	Immediate, Later                      *RepairAssessment
}
type RepairResponse struct {
	Version              string
	User                 core.ID
	At                   core.LogicalTime
	Focus                core.RelationshipFocus
	Perspectives         []RepairPerspective
	Expectation          string // permitted recipient evidence only; scoped descriptive history
	Next                 string
	Options              []string
	Evidence             []core.ID
	RepairVerdict        string // NOT_ASSESSED: never a global relationship or forgiveness score
	ConfirmationRequests int    // always zero; no repeated demand for success reports
}
type RepairHost struct{ Journal RepairJournal }

func (h RepairHost) Execute(ctx context.Context, r RepairRequest, recorded *RepairResponse) (RepairResponse, error) {
	if r.Validate() != nil || h.Journal == nil {
		return RepairResponse{}, ErrInvalid
	}
	return h.Journal.Visit(ctx, r, func(s RepairSnapshot) (RepairResponse, error) {
		if ctx.Err() != nil || s.Now < r.At || core.ValidateRepairLog(s.Log) != nil || core.ValidateBoundaryLog(s.Boundaries) != nil {
			return RepairResponse{}, ErrDenied
		}
		other := r.Actor
		if r.User == r.Actor {
			other = r.Recipient
		}
		out := RepairResponse{Version: RepairFlowVersion, User: r.User, At: r.At, Focus: r.Focus, Perspectives: []RepairPerspective{}, Expectation: "unresolved", Next: "WAIT", Options: []string{}, Evidence: []core.ID{}, RepairVerdict: "NOT_ASSESSED"}
		d, e := core.EvaluateBoundaries(s.Boundaries, core.InteractionScope{Version: core.InteractionScopeVersion, Initiator: r.User, Target: other, Topic: core.ID(r.Focus.Domain), Class: core.PrivatePreparation}, s.Now)
		if e != nil || !d.Allowed {
			return out, nil
		}
		grants := []core.Grant{}
		for _, who := range []core.ID{r.Helper, r.User} {
			for _, op := range []core.Operation{core.Read, core.Derive} {
				grants = append(grants, core.Grant{Actor: who, Recipient: who, Purpose: r.Purpose, Operation: op})
			}
		}
		// Even a summary/count may identify private source history. Reading alone
		// does not authorize disclosing derived context to the requesting participant.
		grants = append(grants, core.Grant{Actor: r.Helper, Recipient: r.User, Purpose: r.Purpose, Operation: core.ShareOnRequest})
		current, e := core.CurrentRepair(s.Log, r.Actor, r.Recipient, r.Focus, r.At, grants)
		if e != nil {
			return RepairResponse{}, ErrDenied
		}
		perspectives := map[core.ID]*RepairPerspective{}
		for _, who := range []core.ID{r.Actor, r.Recipient} {
			perspectives[who] = &RepairPerspective{Observer: who, Fulfilled: []core.ID{}, Breached: []core.ID{}}
		}
		fulfils := map[core.ID]core.LogicalTime{}
		breaches := map[core.ID]core.LogicalTime{}
		lastBreach := core.LogicalTime(-1)
		ownContact := ""
		otherContact := ""
		for _, v := range current {
			out.Evidence = append(out.Evidence, v.Meta.ID)
			p := perspectives[v.Meta.Observer]
			if v.Kind == "action" {
				switch v.Action {
				case "acknowledge":
					p.Acknowledgements++
				case "apologize":
					p.Apologies++
				case "help":
					p.Attempts++
				}
				if v.Action == "withdraw" || v.Action == "leave" || v.Action == "decline" {
					if v.Meta.Observer == r.User {
						ownContact = v.Action
					} else {
						otherContact = v.Action
					}
				}
			}
			if v.Kind == "observation" {
				if v.Finding == "fulfilled" {
					if _, ok := fulfils[v.Commitment]; !ok {
						p.Fulfilled = append(p.Fulfilled, v.Commitment)
					}
					fulfils[v.Commitment] = v.OccurredAt
				}
				if v.Finding == "breached" {
					if _, ok := breaches[v.Commitment]; !ok {
						p.Breached = append(p.Breached, v.Commitment)
					}
					breaches[v.Commitment] = v.OccurredAt
					if v.OccurredAt > lastBreach {
						lastBreach = v.OccurredAt
					}
				}
			}
			if v.Kind == "interpretation" {
				a := &RepairAssessment{Source: v.Meta.ID, At: v.OccurredAt, Assessment: v.Assessment}
				if v.Phase == "immediate" {
					p.Immediate = a
				} else {
					p.Later = a
				}
			}
		}
		periods := map[core.LogicalTime]bool{}
		for id, at := range fulfils {
			if at > lastBreach {
				if _, contrary := breaches[id]; !contrary {
					periods[at] = true
				}
			}
		}
		first, last := r.At, core.LogicalTime(-1)
		for at := range periods {
			if at < first {
				first = at
			}
			if at > last {
				last = at
			}
		}
		switch {
		case len(periods) >= 2 && last-first >= RepairFollowThroughGap:
			out.Expectation = "sustained_follow_through_observed"
		case len(periods) >= 1:
			out.Expectation = "new_follow_through_observed"
		case len(breaches) >= 2:
			out.Expectation = "repeated_breach_observed"
		case len(breaches) == 1:
			out.Expectation = "breach_observed"
		}
		for _, who := range []core.ID{r.Actor, r.Recipient} {
			out.Perspectives = append(out.Perspectives, *perspectives[who])
		}
		// Descriptive history changes an OPTIONAL next step, never a human action or
		// willingness. No global trust update, repair verdict or success question.
		if len(current) > 0 {
			out.Options = []string{"WAIT", "decline", "private_plan", "selective_distance"}
			out.Next = "WAIT"
		}
		if ownContact == "leave" {
			out.Next = "respect_ending"
			out.Options = []string{"WAIT"}
		} else if ownContact == "withdraw" || ownContact == "decline" {
			out.Next = "respect_pause"
			out.Options = []string{"WAIT", "private_plan"}
		} else if otherContact != "" {
			out.Next = "private_support"
			out.Options = []string{"WAIT", "private_plan"}
		} else if out.Expectation == "repeated_breach_observed" {
			out.Next = "consider_selective_distance"
		} else if len(periods) > 0 {
			out.Next = "consider_bounded_plan"
		}
		if recorded != nil && Digest(out) != Digest(*recorded) {
			return RepairResponse{}, ErrDenied
		}
		return clone(out), nil
	})
}
