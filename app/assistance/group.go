package assistance

import (
	"context"
	"github.com/tushardhara/dream/core"
)

const GroupFlowVersion = "group-assistance.v1"

// GroupArmVersion is an OPT-IN envelope adding matched-arm evaluation to the
// group flow. A v1 request carries no arm and behaves exactly as before: this
// version adds a capability, it does not change one. The arms map onto what
// this helper already computes rather than onto invented behaviour — it builds
// one perspective per affected member, so the perspective arms are a real
// difference in what the helper looks at, not a relabelling.
const GroupArmVersion = "group-assistance.v2"

// GroupUsesArms reports whether a group request version carries an arm.
func GroupUsesArms(version string) bool { return version == GroupArmVersion }

type GroupRequest struct {
	Version                                      string
	ID, Session, User, Helper, Decision, Purpose core.ID
	At                                           core.LogicalTime
	// Arm is set only on GroupArmVersion requests and must be empty on v1.
	Arm Arm
}

func (r GroupRequest) Validate() error {
	for _, id := range []core.ID{r.ID, r.Session, r.User, r.Helper, r.Decision, r.Purpose} {
		if id.Validate() != nil {
			return ErrInvalid
		}
	}
	if r.Version != GroupFlowVersion && r.Version != GroupArmVersion || r.User == r.Helper || r.At.Validate() != nil {
		return ErrInvalid
	}
	// An arm on a v1 request would be silently ignored by the v1 path, which is
	// how an evaluation ends up comparing four labels for one policy.
	if GroupUsesArms(r.Version) != r.Arm.Valid() {
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
		out := GroupResponse{Version: r.Version, User: r.User, Decision: r.Decision, At: s.Now, Next: "WAIT", Alternatives: []GroupAlternative{}, Perspectives: []core.GroupPerspective{}, GlobalWelfare: "NOT_AGGREGATED"}
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
		// The no-assistant control returns here having offered nothing: no
		// perspective, no alternative, WAIT. The humans in the native run
		// continue deciding either way — this withholds the assistant, not the
		// people. It is enforced rather than merely produced, so a later change
		// cannot quietly let the control start helping.
		if GroupUsesArms(r.Version) && r.Arm == None {
			if recorded != nil && Digest(*recorded) != Digest(out) {
				return GroupResponse{}, ErrDenied
			}
			return clone(out), nil
		}
		// Which perspectives the helper may take is the arm. Simple assistance
		// takes none and offers only what is feasible; single perspective sees
		// the asking user's own account; multi perspective sees every affected
		// member's. v1 has no arm and takes them all, unchanged.
		for _, member := range d.Affected {
			if GroupUsesArms(r.Version) {
				if r.Arm == Simple {
					break
				}
				if r.Arm == Single && member != r.User {
					continue
				}
			}
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
