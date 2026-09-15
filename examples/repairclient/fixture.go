package repairclient

import (
	"context"
	"fmt"
	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
)

type ScenarioReport struct {
	Name           string
	Periods        []assistance.RepairResponse
	Pause, Ending  assistance.RepairResponse
	Actions        []string
	RemainingHours int64
	EvidenceHash   string
}
type Report struct {
	Version, Evidence, HumanValidity string
	Scenarios                        []ScenarioReport
}

func Fixture() (*Local, error) {
	l := New("repair-demo")
	for _, actor := range []core.ID{"alice", "bob"} {
		if e := l.Boundary(actor, core.Willing, 0); e != nil {
			return nil, e
		}
	}
	return l, nil
}
func response(ctx context.Context, l *Local, id core.ID, at core.LogicalTime) (assistance.RepairResponse, error) {
	r, e := l.Request("bob", id, at)
	if e != nil {
		return assistance.RepairResponse{}, e
	}
	out, e := l.Host().Execute(ctx, r, nil)
	if e != nil {
		return out, e
	}
	replay, e := l.Host().Execute(ctx, r, &out)
	if e != nil || assistance.Digest(replay) != assistance.Digest(out) {
		return out, assistance.ErrInvalid
	}
	return out, nil
}

// Period executes an authored command schedule, not a stochastic human model.
// Both fixtures begin with a breach. Later repair requires actual resource use
// and an independently submitted recipient observation; no success is inferred.
func Period(l *Local, index int, follow bool) error {
	at := core.LogicalTime(1 + index*20)
	prefix := fmt.Sprintf("p%d-", index)
	id := func(s string) core.ID { return core.ID(prefix + s) }
	ga, gb := Permissions("alice", "alice", "bob"), Permissions("bob", "alice", "bob")
	kind := "apologize"
	if follow {
		kind = "acknowledge"
	}
	if e := l.Act("alice", ActionCommand{ID: id("ack"), Kind: kind, At: at, Grants: ga}); e != nil {
		return e
	}
	if e := l.Submit("bob", id("immediate"), id("ack"), "interpretation", "", "immediate", "eased", at+2, gb); e != nil {
		return e
	}
	due := at + 12
	if e := l.Act("alice", ActionCommand{ID: id("promise"), Kind: "promise", Commitment: id("commitment"), Resource: "hours", Units: 1, Due: &due, At: at + 3, Grants: ga}); e != nil {
		return e
	}
	action, finding, later := "break_promise", "breached", "worse"
	if follow {
		action, finding, later = "help", "fulfilled", "mixed"
	}
	c := ActionCommand{ID: id("practical"), Kind: action, Commitment: id("commitment"), At: at + 5, Grants: ga}
	if follow {
		c.Resource = "hours"
		c.Units = 1
	}
	if e := l.Act("alice", c); e != nil {
		return e
	}
	if e := l.Submit("bob", id("observed"), id("practical"), "observation", finding, "", "", at+7, gb); e != nil {
		return e
	}
	if e := l.Submit("alice", id("sender-later"), id("practical"), "interpretation", "", "later", "eased", at+8, ga); e != nil {
		return e
	}
	return l.Submit("bob", id("recipient-later"), id("practical"), "interpretation", "", "later", later, at+9, gb)
}
func Scenario(ctx context.Context, follow bool) (*Local, ScenarioReport, error) {
	l, e := Fixture()
	if e != nil {
		return nil, ScenarioReport{}, e
	}
	out := ScenarioReport{Name: "repeated_apology_breach", Periods: []assistance.RepairResponse{}, Actions: []string{}}
	if follow {
		out.Name = "acknowledgement_follow_through"
	}
	for i := 0; i < 3; i++ {
		if e := Period(l, i, follow && i > 0); e != nil {
			return l, out, e
		}
		r, e := response(ctx, l, core.ID(fmt.Sprintf("period-%d", i)), core.LogicalTime(12+i*20))
		if e != nil {
			return l, out, e
		}
		out.Periods = append(out.Periods, r)
	}
	gb := Permissions("bob", "alice", "bob")
	for i, kind := range []string{"decline", "withdraw", "leave", "wait"} {
		if e := l.Act("bob", ActionCommand{ID: core.ID("choice-" + kind), Kind: kind, At: core.LogicalTime(62 + i*2), Grants: gb}); e != nil {
			return l, out, e
		}
		if i == 1 {
			out.Pause, e = response(ctx, l, "pause", 65)
			if e != nil {
				return l, out, e
			}
		}
	}
	out.Ending, e = response(ctx, l, "ending", 70)
	if e != nil {
		return l, out, e
	}
	log := l.Evidence()
	for _, r := range log {
		if r.Kind == "action" {
			out.Actions = append(out.Actions, r.Action)
		}
	}
	out.RemainingHours = l.Remaining("alice")
	out.EvidenceHash = assistance.Digest(log)
	return l, out, nil
}
func Run(ctx context.Context) (Report, error) {
	out := Report{Version: assistance.RepairFlowVersion, Evidence: "authored_synthetic_commands_and_reports", HumanValidity: "NOT_TESTED", Scenarios: []ScenarioReport{}}
	for _, follow := range []bool{false, true} {
		_, r, e := Scenario(ctx, follow)
		if e != nil {
			return Report{}, e
		}
		out.Scenarios = append(out.Scenarios, r)
	}
	return out, nil
}
