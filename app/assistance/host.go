package assistance

import (
	"context"

	"github.com/tushardhara/dream/app/graph"
)

type Planner interface {
	Plan(context.Context, Input) (Result, error)
}

// Eligibility is trusted host policy, independent of graph data-read permission.
// Implementations authenticate the helper/user/session and explicit goal selection.
type Eligibility interface {
	Check(context.Context, Request, Candidate) error
}

// Journal owns the helper history and atomic delivery boundary. Commit must invoke
// validate under the same protection as permission/revocation updates and delivery,
// reject conflicting ID reuse, and append at most once. It must not invoke a model
// or perform live outreach. A failed commit must have no delivery or history effect.
// Read returns only this helper/user's history. Deployable persistence is deferred;
// #50 supplies a bounded offline host implementation and exercises this contract.
type Journal interface {
	Read(context.Context, Request) ([]Interaction, error)
	Commit(context.Context, Request, Interaction, func(context.Context) error) error
}
type Host struct {
	Policy      *graph.PolicyService
	Eligibility Eligibility
	Journal     Journal
	Planner     Planner
}

func waitResult(reason string) Result {
	return Result{Version: Version, Candidates: []Candidate{{Action: Wait, Reason: reason}}, Selected: 0}
}

// Execute uses identical current-access gates for fresh, recorded and idempotent
// paths. recorded is a result to replay, never permission to deliver. It is bound
// to the complete request digest; no provider fallback exists on replay failure.
func (h Host) Execute(ctx context.Context, request Request, recorded *Interaction) (Interaction, error) {
	r := clone(request)
	if r.Validate() != nil {
		return Interaction{}, ErrInvalid
	}
	if h.Eligibility == nil || h.Journal == nil {
		return Interaction{}, ErrDenied
	}
	baseline := Candidate{Action: Wait, Reason: "restraint"}
	if ctx.Err() != nil || h.Eligibility.Check(ctx, r, baseline) != nil {
		return Interaction{}, ErrDenied
	}
	planning := PlanningDecision{Allowed: true}
	var gate PlanningGate
	if UsesScopedBoundaries(r.Version) {
		var ok bool
		gate, ok = h.Eligibility.(PlanningGate)
		if !ok {
			return Interaction{}, ErrDenied
		}
		var e error
		planning, e = gate.Plan(ctx, r)
		if e != nil || len(planning.Revision) != 64 {
			return Interaction{}, ErrDenied
		}
	}
	caps := make([]graph.ApprovedContext, 0, len(r.Contexts))
	var items []graph.SafeContextItem
	var relations []graph.SafeRelation
	for _, p := range r.Contexts {
		if !planning.Allowed {
			break
		}
		if h.Policy == nil {
			return Interaction{}, ErrDenied
		}
		cap, d, e := h.Policy.Approve(ctx, p)
		if e != nil || !d.Allowed {
			return Interaction{}, ErrDenied
		}
		safe, d, e := h.Policy.Revalidate(ctx, cap, r.ID)
		if e != nil || !d.Allowed {
			return Interaction{}, ErrDenied
		}
		if r.Version == DomainVersion {
			projected, err := safe.Relations()
			if err != nil {
				return Interaction{}, ErrDenied
			}
			relations = append(relations, projected...)
		}
		caps = append(caps, cap)
		items = append(items, safe.Items()...)
	}
	if len(items) > 16 || len(Digest(items)) != 64 {
		return Interaction{}, ErrInvalid
	}
	contextKnown := true
	var perspectives []DomainPerspective
	if r.Version == DomainVersion && planning.Allowed {
		var err error
		contextKnown, perspectives, err = prepareDomains(r, relations, items)
		if err != nil {
			return Interaction{}, ErrDenied
		}
	}
	// Duplicate references in different namespaces must not create ambiguous output lineage.
	for i, item := range items {
		for _, before := range items[:i] {
			if item.Observer == before.Observer && item.Source == before.Source {
				return Interaction{}, ErrInvalid
			}
		}
	}
	revalidate := func(ctx context.Context, c Candidate) error {
		if UsesScopedBoundaries(r.Version) {
			current, e := gate.Plan(ctx, r)
			if e != nil || current != planning || !planning.Allowed && c.Action != Wait {
				return ErrDenied
			}
		}
		if ctx.Err() != nil || h.Eligibility.Check(ctx, r, c) != nil {
			return ErrDenied
		}
		for _, cap := range caps {
			if _, d, e := h.Policy.Revalidate(ctx, cap, r.ID); e != nil || !d.Allowed {
				return ErrDenied
			}
		}
		return nil
	}
	history, e := h.Journal.Read(ctx, r)
	if e != nil || len(history) > MaxHistory {
		return Interaction{}, ErrDenied
	}
	hash := Digest(r)
	for _, old := range history {
		if !knownVersion(old.Version) || old.Helper != r.Helper || old.User != r.User || old.At.Validate() != nil {
			return Interaction{}, ErrDenied
		}
		if old.ID == r.ID {
			if !old.matches(r, items, planning, contextKnown) || recorded != nil && Digest(*recorded) != Digest(old) {
				return Interaction{}, ErrInvalid
			}
			if revalidate(ctx, old.Result.Candidates[old.Result.Selected]) != nil {
				return Interaction{}, ErrDenied
			}
			return clone(old), nil
		}
	}
	if len(history) == MaxHistory {
		return Interaction{}, ErrDenied
	}
	result := waitFor(r.Version, "restraint")
	if recorded != nil {
		old := clone(*recorded)
		if !old.matches(r, items, planning, contextKnown) {
			return Interaction{}, ErrInvalid
		}
		result = old.Result
	} else if !planning.Allowed {
		result = waitFor(r.Version, "boundary")
	} else if !contextKnown {
		result = waitFor(r.Version, "relationship_context")
	} else if r.Arm == None {
		result = waitFor(r.Version, "disabled")
	} else if r.Arm == Simple {
		result = preferenceFor(r)
	} else if h.Planner != nil {
		// History is mechanical only. Old evidence/source refs are not re-exposed to a
		// planner without fresh permission. No request hashes enter the provider input.
		safeHistory := []Interaction{}
		for _, old := range history {
			if old.At <= r.At && old.Version == r.Version && Digest(old.Scope) == Digest(r.Scope) && Digest(old.Focus) == Digest(r.Focus) {
				safeHistory = append(safeHistory, clone(old))
			}
		}
		for i := range safeHistory {
			safeHistory[i].RequestHash = ""
			safeHistory[i].EvidenceHash = ""
			safeHistory[i].BoundaryHash = ""
			safeHistory[i].Scope = nil
			safeHistory[i].Focus = nil
			for j := range safeHistory[i].Result.Candidates {
				safeHistory[i].Result.Candidates[j].Evidence = nil
			}
		}
		input := Input{Focus: clone(r.Focus), Relationships: clone(perspectives), Scope: clone(r.Scope), Version: r.Version, ID: r.ID, Helper: r.Helper, User: r.User, Arm: r.Arm, Goal: r.Goal, At: r.At, Seed: r.Seed, Context: clone(items), History: safeHistory}
		result, e = h.Planner.Plan(ctx, input)
		if e != nil {
			result = waitFor(r.Version, "unavailable")
		}
	}
	if !result.validPlan(r, result.validate(r, items), planning, contextKnown) {
		return Interaction{}, ErrInvalid
	}
	selected := result.Candidates[result.Selected]
	if revalidate(ctx, selected) != nil {
		return Interaction{}, ErrDenied
	}
	out := Interaction{Focus: clone(r.Focus), Scope: clone(r.Scope), BoundaryHash: planning.Revision, EvidenceHash: Digest(items), Arm: r.Arm, Seed: r.Seed, Version: r.Version, ID: r.ID, Helper: r.Helper, User: r.User, At: r.At, RequestHash: hash, Result: clone(result), Delivered: selected.Action != Wait}
	if h.Journal.Commit(ctx, r, out, func(commitCtx context.Context) error { return revalidate(commitCtx, selected) }) != nil {
		return Interaction{}, ErrDenied
	}
	return clone(out), nil
}

func (old Interaction) matches(r Request, items []graph.SafeContextItem, planning PlanningDecision, contextKnown bool) bool {
	return old.Version == r.Version && Digest(old.Focus) == Digest(r.Focus) && old.BoundaryHash == planning.Revision && Digest(old.Scope) == Digest(r.Scope) && old.ID == r.ID && old.Helper == r.Helper && old.User == r.User && old.At == r.At && old.Arm == r.Arm && old.Seed == r.Seed && old.RequestHash == Digest(r) && old.EvidenceHash == Digest(items) && old.Result.validPlan(r, old.Result.validate(r, items), planning, contextKnown) && old.Delivered == (old.Result.Candidates[old.Result.Selected].Action != Wait)
}
