package assistance

import (
	"context"
	"github.com/tushardhara/dream/core"
)

// ScopedVersion is opt-in. Version remains the frozen #50 wire/replay contract.
const ScopedVersion = "assistance.v2"
const ClarificationCooldown core.LogicalTime = 10
const MaxClarifications = 2

func knownVersion(v string) bool { return v == Version || v == ScopedVersion }

type PlanningDecision struct {
	Allowed  bool
	Revision string
}
type PlanningGate interface {
	Plan(context.Context, Request) (PlanningDecision, error)
}
type BoundarySnapshot struct {
	Now     core.LogicalTime
	Records []core.Boundary
}

// BoundarySource is trusted policy authority, not graph context for the planner.
// It must supply the complete relevant preferences, including contrary reports
// and current revocation tombstones. Missing authority must return an error.
type BoundarySource interface {
	ReadBoundaries(context.Context, Request) (BoundarySnapshot, error)
}
type BoundaryGate struct {
	Auth    Eligibility
	Source  BoundarySource
	History Journal
}

func (g *BoundaryGate) Check(ctx context.Context, r Request, c Candidate) error {
	if g == nil || g.Auth == nil || g.Auth.Check(ctx, r, c) != nil {
		return ErrDenied
	}
	if r.Version == Version || c.Action == Wait {
		return nil
	}
	d, e := g.Plan(ctx, r)
	if e != nil || !d.Allowed {
		return ErrDenied
	}
	return nil
}
func (g *BoundaryGate) Plan(ctx context.Context, r Request) (PlanningDecision, error) {
	if g == nil || r.Version != ScopedVersion || r.Scope == nil || g.Auth == nil || g.Source == nil || g.History == nil || g.Auth.Check(ctx, r, Candidate{Action: Wait, Reason: "restraint"}) != nil {
		return PlanningDecision{}, ErrDenied
	}
	snapshot, e := g.Source.ReadBoundaries(ctx, r)
	if e != nil || snapshot.Now < r.At {
		return PlanningDecision{}, ErrDenied
	}
	decision, e := core.EvaluateBoundaries(snapshot.Records, *r.Scope, snapshot.Now)
	if e != nil {
		return PlanningDecision{}, ErrDenied
	}
	history, e := g.History.Read(ctx, r)
	if e != nil || len(history) > MaxHistory {
		return PlanningDecision{}, ErrDenied
	}
	recent := []Interaction{}
	for _, old := range history {
		if old.ID != r.ID && old.Scope != nil && Digest(old.Scope) == Digest(r.Scope) && old.Delivered && old.Result.Selected >= 0 && old.Result.Selected < len(old.Result.Candidates) && old.Result.Candidates[old.Result.Selected].Action == Clarify {
			recent = append(recent, old)
		}
	}
	expected := preferenceFor(r).Candidates[preferenceFor(r).Selected]
	if g.Auth.Check(ctx, r, expected) != nil {
		decision = core.BoundaryDecision{Code: "host_refusal"}
	}
	if r.Goal == Pause {
		decision = core.BoundaryDecision{Code: "requested_pause"}
	}
	if r.Goal == Unknown && decision.Allowed {
		if len(recent) >= MaxClarifications {
			decision = core.BoundaryDecision{Code: "clarification_budget"}
		}
		for _, old := range recent {
			if old.At > snapshot.Now || snapshot.Now-old.At < ClarificationCooldown {
				decision = core.BoundaryDecision{Code: "clarification_cooldown"}
			}
		}
	}
	// Time itself is not a replay hash: current interval/expiry is re-evaluated,
	// while unchanged still-valid evidence can support historical mechanical replay.
	revision := Digest(struct {
		Version  string
		Scope    *core.InteractionScope
		Records  []core.Boundary
		Recent   []Interaction
		Decision core.BoundaryDecision
	}{ScopedVersion, r.Scope, snapshot.Records, recent, decision})
	return PlanningDecision{Allowed: decision.Allowed, Revision: revision}, nil
}
func preferenceFor(r Request) Result {
	out := ExplicitPreference(r.Goal, r.User)
	out.Version = r.Version
	return out
}
func waitFor(version, reason string) Result {
	out := waitResult(reason)
	out.Version = version
	return out
}
func (o Result) validPlan(r Request, itemsValid bool, p PlanningDecision) bool {
	if !itemsValid {
		return false
	}
	if r.Version == ScopedVersion {
		if !p.Allowed {
			return Digest(o) == Digest(waitFor(r.Version, "boundary"))
		}
		return o.Candidates[o.Selected].Reason != "boundary"
	}
	return true
}
