package assistance

import (
	"context"

	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
)

// DomainPerspective preserves the source account unchanged. Effective confidence
// is calculated separately from its author confidence and retrieved evidence.
// All entries originate in approved graph capabilities, never planner authority.
type DomainPerspective struct {
	Account  core.RelationshipContext
	Evidence map[core.ID]core.Confidence
}

func (p DomainPerspective) measure(kind core.ID) (float64, bool) {
	for _, m := range p.Account.Measures {
		if m.Kind == kind {
			source, ok := p.Evidence[m.Source]
			frame, frameOK := p.Evidence[p.Account.ContextSource]
			record, recordOK := p.Evidence[p.Account.Account]
			confidence := m.Confidence * source * frame * record
			if !ok || !frameOK || !recordOK || confidence <= 0 {
				return 0, false
			}
			return m.Value * float64(confidence), true
		}
	}
	return 0, false
}
func prepareDomains(r Request, relations []graph.SafeRelation, items []graph.SafeContextItem) (bool, []DomainPerspective, error) {
	if r.Focus == nil || r.Scope == nil {
		return false, nil, ErrInvalid
	}
	if r.Arm == None {
		return true, nil, nil
	}
	// The simple baseline has no graph context. It can follow an explicitly chosen
	// frame and goal, but does not infer trust from the frame/role label.
	if r.Arm == Simple {
		return r.Focus.RoleContext != "" && r.Focus.Account == "", nil, nil
	}
	accounts := []core.RelationshipContext{}
	for _, relation := range relations {
		if relation.State.Context != nil {
			accounts = append(accounts, *relation.State.Context)
		}
	}
	owners := r.Scope.Participants()
	if r.Arm == Single {
		owners = []core.ID{r.User}
	}
	selected := []DomainPerspective{}
	for _, owner := range owners {
		other := r.User
		if owner == r.User {
			other = r.Scope.Target
		}
		focus := *r.Focus
		// A user may explicitly choose their own conflicting account; another
		// observer's competing accounts remain ambiguous unless the frame is unique.
		if owner != r.User {
			focus.Account = ""
		}
		choice, e := core.SelectRelationship(accounts, focus, owner, other, r.At)
		if e != nil {
			return false, nil, e
		}
		if choice.Status != "selected" {
			return false, nil, nil
		}
		evidence := map[core.ID]core.Confidence{}
		for _, id := range append(choice.Account.Sources(), choice.Account.Account) {
			found := false
			for _, item := range items {
				if item.Observer == owner && item.Source == id {
					if item.LearnedAt > r.At || item.OccurredAt > r.At || item.Valid.Start > r.At || item.Valid.End != nil && r.At >= *item.Valid.End {
						return false, nil, ErrDenied
					}
					evidence[id] = item.Confidence
					found = true
					break
				}
			}
			if !found {
				return false, nil, ErrDenied
			}
		}
		selected = append(selected, DomainPerspective{clone(*choice.Account), evidence})
	}
	return len(selected) > 0, selected, nil
}

// DomainPlanner is a fixed offline engineering control, not a validated social
// model. Each observed perspective must independently support the small proposal;
// no averaging turns an adverse or unknown account into inferred consensus.
// Interpersonal willingness is separately enforced by the host before and after it.
type DomainPlanner struct{}

func (DomainPlanner) Plan(ctx context.Context, in Input) (Result, error) {
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	if in.Version != DomainVersion || in.Focus == nil || in.Focus.Validate() != nil || len(in.Relationships) == 0 {
		return Result{}, ErrInvalid
	}
	for _, p := range in.Relationships {
		if p.Account.Version != 2 || p.Account.Validate(p.Account.Observer, p.Account.Other) != nil || p.Account.Domain != in.Focus.Domain || in.Focus.RoleContext != "" && p.Account.RoleContext != in.Focus.RoleContext {
			return Result{}, ErrInvalid
		}
	}
	if in.Goal == Coordinate {
		for _, p := range in.Relationships {
			trust, known := p.measure("trust")
			expectation, hasExpectation := p.measure("expectation")
			outcome, _ := p.measure("prior_outcome")
			if !known || !hasExpectation || trust <= 0 || trust+.25*expectation+.2*outcome <= 0 {
				return waitFor(in.Version, "restraint"), nil
			}
		}
	}
	out := ExplicitPreference(in.Goal, in.User)
	out.Version = in.Version
	if out.Selected != 0 {
		for _, item := range in.Context {
			out.Candidates[out.Selected].Evidence = append(out.Candidates[out.Selected].Evidence, EvidenceRef{item.Observer, item.Source})
		}
	}
	return out, nil
}
