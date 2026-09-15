package ordinaryexperiment

import (
	"context"
	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/simulator/behavior"
	"testing"
)

func TestNativeOrdinaryFamilies(t *testing.T) {
	declines, social, reservations, adverse, unknown := 0, 0, 0, 0, 0
	for _, family := range Families {
		for seed := uint64(0); seed < 8; seed++ {
			var generic Report
			for _, arm := range Arms {
				r, e := Run(context.Background(), family, arm, seed)
				if e != nil {
					t.Fatal(family, arm, seed, e)
				}
				if len(r.Opportunities) != Frames || r.HumanValidity != "NOT_TESTED" || r.GlobalWelfare != "NOT_AGGREGATED" {
					t.Fatal("report contract")
				}
				waits, daily := 0, 0
				for _, op := range r.Opportunities {
					if op.Helper.Action == "WAIT" {
						waits++
						if op.Selected != behavior.Wait {
							t.Fatal("unsupported offer entered native policy")
						}
					}
					if arm == "generic" && op.Helper.Quote != nil {
						t.Fatal("generic attributed participant words")
					}
				}
				if waits <= Frames/2 {
					t.Fatal("healthy life was repeatedly interrupted", family, arm, seed, waits)
				}
				if arm == "none" || family == "quiet" || family == "unknown_benefit" {
					if waits != Frames {
						t.Fatal("quiet/unknown forced activity")
					}
				}
				for _, tr := range r.Traces {
					if tr.MemoryBefore != tr.MemoryAfter {
						t.Fatal("ordinary choice penalized relationship memory", tr.Stage)
					}
					if tr.Stage == "daily-life" {
						daily++
						if selected(tr) != behavior.Wait {
							social++
						}
					}
					if selected(tr) == behavior.Decline {
						declines++
						for _, op := range r.Opportunities {
							if op.Frame > tr.Frame && op.Helper.Activity == "simple_activity" && op.Helper.Action != "WAIT" {
								t.Fatal("declined ritual proposed again")
							}
						}
					}
				}
				if daily != 2*Frames {
					t.Fatal("independent human life stopped")
				}
				for _, x := range r.Experiences {
					if x.Participation == "unwelcome" {
						adverse++
					}
					if x.Participation == "unknown" {
						unknown++
						if x.Benefit.Value != nil {
							t.Fatal("unknown became zero")
						}
					}
				}
				reservations += len(r.Reservations)
				if arm == "generic" {
					generic = r
				}
				if arm == "permitted_context" {
					if assistance.Digest(generic.Traces) != assistance.Digest(r.Traces) || assistance.Digest(generic.Experiences) != assistance.Digest(r.Experiences) || assistance.Digest(generic.Reservations) != assistance.Digest(r.Reservations) {
						t.Fatal("undeclared context advantage")
					}
				}
			}
		}
	}
	if declines == 0 || social == 0 || reservations == 0 || adverse == 0 || unknown == 0 {
		t.Fatal("missing bounded positive/adverse consumer controls", declines, social, reservations, adverse, unknown)
	}
	t.Log("native declines/daily social/reservations/adverse/unknown", declines, social, reservations, adverse, unknown)
}
func TestNativeOrdinaryDeterministicAndCancelled(t *testing.T) {
	a, e := Run(context.Background(), "daily", "permitted_context", 3)
	if e != nil {
		t.Fatal(e)
	}
	b, e := Run(context.Background(), "daily", "permitted_context", 3)
	if e != nil || assistance.Digest(a) != assistance.Digest(b) {
		t.Fatal("nondeterministic", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = Run(ctx, "daily", "generic", 3); e == nil {
		t.Fatal("cancelled work continued")
	}
}
