package assistance

import (
	"context"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
)

// Temporal permission is deliberately a host gate, not optional planner advice.
// The fixed planner still selects only the explicit-goal templates. Historical
// graph inspection is separate from authorizing an effect at an old logical time.
func prepareTemporal(r Request, items []graph.SafeContextItem) (bool, []core.TemporalContext, error) {
	if r.Temporal == nil || r.Scope == nil {
		return false, nil, ErrInvalid
	}
	if r.Arm == None || r.Arm == Simple {
		return true, nil, nil
	}
	evidence, e := graph.TemporalEvidenceFromApproved(items, r.At)
	if e != nil {
		return false, nil, e
	}
	owners := r.Scope.Participants()
	if r.Arm == Single {
		owners = []core.ID{r.User}
	}
	views := []core.TemporalContext{}
	allowed := true
	for _, owner := range owners {
		other := r.User
		if owner == r.User {
			other = r.Scope.Target
		}
		focus := *r.Temporal
		focus.Observer = owner
		focus.Other = other
		for _, record := range evidence {
			if record.Fact.Observer == owner && (record.Fact.With != other || record.Fact.Channel != focus.Channel) {
				return false, nil, ErrDenied
			}
		}
		view, e := core.InterpretTemporal(focus, evidence, r.At)
		if e != nil {
			return false, nil, e
		}
		views = append(views, view)
		switch r.Goal {
		case Unknown:
			allowed = allowed && view.Recommendation == "clarify"
		case Coordinate:
			allowed = allowed && view.Status == "routine"
		case Listen, Understand:
			allowed = allowed && view.Status != "unknown" && view.Status != "busy"
		case Pause:
			allowed = false
		}
	}
	return allowed && len(views) > 0, views, nil
}

// TemporalPlanner chooses a non-identifying invitation to check current context,
// never an explanation of another person's private circumstance or alleged silence.
type TemporalPlanner struct{}

func (TemporalPlanner) Plan(ctx context.Context, in Input) (Result, error) {
	if in.Version != TemporalVersion || len(in.Temporal) == 0 {
		return Result{}, ErrInvalid
	}
	out, e := (DomainPlanner{}).Plan(ctx, in)
	if e != nil {
		return out, e
	}
	if in.Goal == Unknown && out.Selected != 0 {
		for _, view := range in.Temporal {
			if view.Recommendation != "clarify" {
				return Result{}, ErrDenied
			}
		}
		out.Candidates[out.Selected].Reason = "check_current_context"
	}
	return out, nil
}
