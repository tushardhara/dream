package assistance

import (
	"context"
	"github.com/tushardhara/dream/core"
)

const GroupFlowVersion = "group-assistance.v1"

type GroupRequest struct {
	Version                                      string
	ID, Session, User, Helper, Decision, Purpose core.ID
	At                                           core.LogicalTime
}

func (r GroupRequest) Validate() error {
	for _, id := range []core.ID{r.ID, r.Session, r.User, r.Helper, r.Decision, r.Purpose} {
		if id.Validate() != nil {
			return ErrInvalid
		}
	}
	if r.Version != GroupFlowVersion || r.User == r.Helper || r.At.Validate() != nil {
		return ErrInvalid
	}
	return nil
}

type GroupSnapshot struct {
	Now          core.LogicalTime
	History      core.GroupHistory
	Reservations []core.GroupReservation
	Budget       core.GroupBudget
	Boundaries   []core.Boundary
}

// Visit authenticates an exact request, reads current grants/boundaries/portfolio
// and delivers under the same transaction as changes to them. The helper proposes
// alternatives; it never reserves effort or sends invitations itself.
type GroupJournal interface {
	VisitGroup(context.Context, GroupRequest, func(GroupSnapshot) (GroupResponse, error)) (GroupResponse, error)
}
type GroupAlternative struct {
	ID       core.ID
	Resource core.ID
	Units    int64
	Tasks    []core.GroupTask
}
type GroupResponse struct {
	Version        string
	User, Decision core.ID
	At             core.LogicalTime
	Next           string
	Alternatives   []GroupAlternative
	Perspectives   []core.GroupPerspective
	GlobalWelfare  string
}
type GroupHost struct{ Journal GroupJournal }

func groupSharingGrants(r GroupRequest) []core.Grant {
	grants := []core.Grant{}
	for _, who := range []core.ID{r.Helper, r.User} {
		for _, op := range []core.Operation{core.Read, core.Derive} {
			grants = append(grants, core.Grant{Actor: who, Recipient: who, Purpose: r.Purpose, Operation: op})
		}
	}
	return append(grants, core.Grant{Actor: r.Helper, Recipient: r.User, Purpose: r.Purpose, Operation: core.ShareOnRequest})
}
func GroupBoundariesAllow(r GroupRequest, s GroupSnapshot, d core.GroupDecision) bool {
	if r.Validate() != nil || d.Validate() != nil || core.ValidateBoundaryLog(s.Boundaries) != nil || s.Now < r.At {
		return false
	}
	for _, person := range d.Affected {
		if person == r.User {
			continue
		}
		scope := core.InteractionScope{Version: core.InteractionScopeVersion, Initiator: r.User, Target: person, Topic: core.ID(d.Focus.Domain), Class: core.Coordination}
		v, e := core.EvaluateBoundaries(s.Boundaries, scope, s.Now)
		if e != nil || !v.Allowed {
			return false
		}
	}
	return true
}
func (h GroupHost) Execute(ctx context.Context, r GroupRequest, recorded *GroupResponse) (GroupResponse, error) {
	if r.Validate() != nil || h.Journal == nil {
		return GroupResponse{}, ErrInvalid
	}
	return h.Journal.VisitGroup(ctx, r, func(s GroupSnapshot) (GroupResponse, error) {
		out := GroupResponse{Version: GroupFlowVersion, User: r.User, Decision: r.Decision, At: s.Now, Next: "WAIT", Alternatives: []GroupAlternative{}, Perspectives: []core.GroupPerspective{}, GlobalWelfare: "NOT_AGGREGATED"}
		if ctx.Err() != nil || s.Now < r.At || s.History.Validate() != nil || core.ValidateGroupPortfolio(s.History, s.Reservations, s.Budget) != nil || s.Budget.Validate() != nil || core.ValidateBoundaryLog(s.Boundaries) != nil {
			return GroupResponse{}, ErrDenied
		}
		var d core.GroupDecision
		for _, v := range s.History.Decisions {
			if v.ID == r.Decision {
				d = v
			}
		}
		if d.ID == "" || d.At > r.At {
			return GroupResponse{}, ErrDenied
		}
		affected := false
		for _, p := range d.Affected {
			affected = affected || p == r.User
		}
		if !affected {
			return GroupResponse{}, ErrDenied
		}
		grants := groupSharingGrants(r)
		for _, g := range grants {
			if !d.Meta.Rights.Allows(core.PermissionRequest{Resource: d.Meta.ID, Context: g}) {
				return GroupResponse{}, ErrDenied
			}
		}
		// Private preparation needs only the user's willingness, not group consent.
		peer := d.Affected[0]
		if peer == r.User {
			peer = d.Affected[1]
		}
		private, e := core.EvaluateBoundaries(s.Boundaries, core.InteractionScope{Version: core.InteractionScopeVersion, Initiator: r.User, Target: peer, Topic: core.ID(d.Focus.Domain), Class: core.PrivatePreparation}, s.Now)
		if e != nil || !private.Allowed {
			if recorded != nil && Digest(*recorded) != Digest(out) {
				return GroupResponse{}, ErrDenied
			}
			return out, nil
		}
		for _, member := range d.Affected {
			p, e := core.GroupPerspectiveFor(s.History, d.Group, d.Focus, member, s.Now, grants)
			if e != nil {
				return GroupResponse{}, ErrDenied
			}
			out.Perspectives = append(out.Perspectives, p)
		}
		if GroupBoundariesAllow(r, s, d) {
			for _, o := range d.Options {
				if core.GroupOptionFeasible(s.History, s.Reservations, s.Budget, d.ID, o.ID, s.Now, grants) == nil {
					out.Alternatives = append(out.Alternatives, GroupAlternative{ID: o.ID, Resource: o.Resource, Units: o.Units, Tasks: append([]core.GroupTask{}, o.Tasks...)})
				}
			}
		}
		if len(out.Alternatives) > 0 {
			out.Next = "choose_agreed_option"
		} else {
			out.Next = "private_prepare_or_WAIT"
		}
		if recorded != nil && Digest(*recorded) != Digest(out) {
			return GroupResponse{}, ErrDenied
		}
		return clone(out), nil
	})
}
