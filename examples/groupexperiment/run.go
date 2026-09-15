package groupexperiment

import (
	"context"
	"fmt"
	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/examples/groupclient"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/demo"
)

type Report struct {
	Version, HumanValidity, GlobalWelfare string
	People                                int
	Native                                demo.GroupNativeRun
	Helper                                assistance.GroupResponse
	Reservations                          []core.GroupReservation
	Execution                             string
}

// Run uses a declared fictional preference for shared-care, not a welfare
// optimiser. Only a native Coordinate choice plus every explicit agreement and
// current boundary can reserve it. The helper itself cannot execute actions.
func Run(ctx context.Context, c Case) (Report, error) {
	out := Report{Version: demo.GroupVersion, HumanValidity: "NOT_TESTED", GlobalWelfare: "NOT_AGGREGATED", People: len(c.Native.Scenario.Public.Humans), Execution: "WAIT", Reservations: []core.GroupReservation{}}
	l, e := groupclient.New("group-session", c.Native.History, c.Budget, c.Reservations, c.Boundaries, c.Native.At)
	if e != nil {
		return out, e
	}
	r, e := l.Request(Person(0), "group-request", c.Native.Decision, c.Native.At)
	if e != nil {
		return out, e
	}
	out.Helper, e = l.Host().Execute(ctx, r, nil)
	if e != nil {
		return out, e
	}
	out.Native, e = demo.RunGroups(c.Native, nil)
	if e != nil {
		return out, e
	}
	agreed := false
	for _, o := range out.Helper.Alternatives {
		agreed = agreed || o.ID == "shared-care"
	}
	for _, tr := range out.Native.Traces {
		if tr.Actor != Person(0) {
			continue
		}
		d := tr.Decision.Human
		if agreed && d.Candidates[d.Selected].Offer.Kind == behavior.Coordinate {
			if e = l.Reserve(Person(0), "group-reservation", r.Decision, "shared-care", r.At); e != nil {
				return out, fmt.Errorf("explicit native plan: %w", e)
			}
			out.Execution = "reserved_shared_care"
		}
	}
	out.Reservations = l.Reservations()
	return out, nil
}
