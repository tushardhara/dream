package assistance

import (
	"context"
	"github.com/tushardhara/dream/core"
)

// ExplicitPreference is a fixed-template baseline, with no inferred private
// context or objective of reconciliation. WAIT remains available in every result.
func ExplicitPreference(goal Goal, user core.ID) Result {
	out := waitResult("restraint")
	c := Candidate{Recipient: user}
	switch goal {
	case Unknown:
		c.Action = Clarify
		c.Reason = "goal_unknown"
	case Listen:
		c.Action = Acknowledge
		c.Reason = "explicit_listening"
	case Coordinate:
		c.Action = Propose
		c.Reason = "explicit_coordination"
	default:
		return out
	}
	out.Candidates = append(out.Candidates, c)
	out.Selected = 1
	return out
}

// FakePlanner demonstrates orchestration only, not language understanding or uplift.
// Additional perspectives are separate, permitted observations; no consent, hidden
// feelings or consensus is inferred from their presence.
type FakePlanner struct{}

func (FakePlanner) Plan(ctx context.Context, in Input) (Result, error) {
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
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
