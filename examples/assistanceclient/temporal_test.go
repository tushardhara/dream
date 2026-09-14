package assistanceclient

import (
	"context"
	"testing"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
)

func temporalFacts(at, upper core.LogicalTime) []core.TemporalFact {
	return append(ContactFacts("alice", "bob", at, upper), ContactFacts("bob", "alice", at, upper)...)
}
func temporalFixture(t *testing.T, arm assistance.Arm, at core.LogicalTime, facts []core.TemporalFact) (*Local, assistance.Request) {
	t.Helper()
	l, r, e := TemporalFixture(arm, at, "chat", facts)
	if e != nil {
		t.Fatal(e)
	}
	consent(t, l, r)
	return l, r
}
func TestTemporalHelperConsumesLifeRhythmStalenessAndUncertainty(t *testing.T) {
	for _, name := range []string{"daily_2", "daily_60", "occasional_60", "busy", "changed", "stale", "sparse", "unobserved_channel", "conflicting_observers"} {
		t.Run(name, func(t *testing.T) {
			at, upper := core.LogicalTime(63), core.LogicalTime(3)
			if name == "daily_2" {
				at = 5
			}
			if name == "occasional_60" || name == "changed" || name == "stale" {
				upper = 90
			}
			facts := temporalFacts(at, upper)
			switch name {
			case "busy":
				facts = append(facts, LifeFact("bob", "alice", "busy", 60, 30))
			case "changed":
				life := LifeFact("alice", "bob", "routine_changed", 60, 30)
				life.Category = "transition"
				facts = append(facts, life)
			case "stale":
				facts = append(facts, LifeFact("alice", "bob", "available", 1, 30), LifeFact("bob", "alice", "available", 1, 30))
			case "sparse":
				for i := range facts {
					if facts[i].Kind == "contacts" {
						facts[i].CompleteChannel = false
						facts[i].Observations = []core.LogicalTime{3}
					}
				}
			case "unobserved_channel":
				for i := range facts {
					if facts[i].Kind == "contacts" {
						facts[i].CompleteChannel = false
					}
				}
			case "conflicting_observers":
				facts[2].MaxGap = 90
			}
			// For changed context both observers independently supply the transition.
			if name == "changed" {
				life := LifeFact("bob", "alice", "routine_changed", 60, 30)
				life.Category = "transition"
				facts = append(facts, life)
			}
			l, r := temporalFixture(t, assistance.Multi, at, facts)
			calls := 0
			out, e := l.Host(planFunc(func(ctx context.Context, in assistance.Input) (assistance.Result, error) {
				calls++
				if len(in.Temporal) != 2 || in.Temporal[0].Observer == in.Temporal[1].Observer {
					t.Fatal("temporal perspectives collapsed")
				}
				return (assistance.FakePlanner{}).Plan(ctx, in)
			})).Execute(context.Background(), r, nil)
			want := name == "daily_60" || name == "changed" || name == "stale"
			if e != nil || out.Delivered != want || calls != map[bool]int{true: 1, false: 0}[want] {
				t.Fatal("temporal input ignored", out, e, calls)
			}
			if !want && out.Result.Candidates[out.Result.Selected].Reason != "temporal_context" {
				t.Fatal("temporal restraint not a host gate", out)
			}
		})
	}
}

func TestTemporalHelperPermissionChangesAndReplay(t *testing.T) {
	for _, stage := range []string{"planning", "commit", "external", "stored"} {
		for _, part := range []core.ID{"bob-circumstance", "bob-temporal-source"} {
			t.Run(stage+"/"+string(part), func(t *testing.T) {
				facts := append(temporalFacts(63, 90), LifeFact("alice", "bob", "available", 1, 30), LifeFact("bob", "alice", "available", 1, 30))
				l, r := temporalFixture(t, assistance.Multi, 63, facts)
				h := l.Host(assistance.FakePlanner{})
				var prior *assistance.Interaction
				if stage == "external" || stage == "stored" {
					record, e := h.Execute(context.Background(), r, nil)
					if e != nil || !record.Delivered {
						t.Fatal(e)
					}
					if stage == "external" {
						prior = &record
						l, r = temporalFixture(t, assistance.Multi, 63, facts)
						h = l.Host(nil)
					}
				}
				revoke := func() {
					for scope, entries := range l.entries {
						for i := range entries {
							if entries[i].Event.Meta.ID == part {
								entries[i].Event.Meta.Rights.Revoked = true
							}
						}
						l.SetMemory(scope, entries)
					}
				}
				switch stage {
				case "planning":
					h.Planner = planFunc(func(ctx context.Context, in assistance.Input) (assistance.Result, error) {
						revoke()
						return (assistance.FakePlanner{}).Plan(ctx, in)
					})
				case "commit":
					h.Journal = beforeCommit{Local: l, before: revoke}
				default:
					revoke()
				}
				before := l.Deliveries()
				if _, e := h.Execute(context.Background(), r, prior); e == nil || l.Deliveries() != before {
					t.Fatal("revoked temporal evidence authorized effect")
				}
			})
		}
	}
}
func TestTemporalHelperPrivateUpdatesDoNotEnterOtherAccountsOrExplanations(t *testing.T) {
	facts := temporalFacts(63, 3)
	facts = append(facts, LifeFact("bob", "alice", "busy", 60, 30))
	l, r := temporalFixture(t, assistance.Single, 63, facts)
	out, e := l.Host(planFunc(func(ctx context.Context, in assistance.Input) (assistance.Result, error) {
		for _, item := range in.Context {
			if item.Observer != "alice" {
				t.Fatal("Bob's private update entered Alice's account")
			}
		}
		if len(in.Temporal) != 1 || in.Temporal[0].Observer != "alice" {
			t.Fatal("private temporal view leaked")
		}
		return (assistance.FakePlanner{}).Plan(ctx, in)
	})).Execute(context.Background(), r, nil)
	if e != nil || !out.Delivered || out.Result.Candidates[out.Result.Selected].Reason != "check_current_context" {
		t.Fatal("own evidence control", out, e)
	}
	l, r = temporalFixture(t, assistance.Multi, 63, facts)
	out, e = l.Host(nil).Execute(context.Background(), r, nil)
	if e != nil || out.Delivered || out.Result.Candidates[out.Result.Selected].Reason != "temporal_context" || len(out.Result.Candidates[out.Result.Selected].Evidence) != 0 {
		t.Fatal("private circumstance exposed in restraint", out, e)
	}
}
func TestTemporalHelperExactWaitAndCurrentTimeCannotBeForged(t *testing.T) {
	facts := temporalFacts(5, 3)
	l, r := temporalFixture(t, assistance.Multi, 5, facts)
	out, e := l.Host(nil).Execute(context.Background(), r, nil)
	if e != nil || out.Delivered {
		t.Fatal(e)
	}
	l, r = temporalFixture(t, assistance.Multi, 5, facts)
	out.Result = assistance.ExplicitPreference(r.Goal, r.User)
	out.Result.Version = r.Version
	out.Delivered = true
	if _, e = l.Host(nil).Execute(context.Background(), r, &out); e == nil {
		t.Fatal("recorded proposal bypassed temporal WAIT")
	}
	facts = temporalFacts(63, 3)
	l, r = temporalFixture(t, assistance.Multi, 63, facts)
	out, e = l.Host(assistance.FakePlanner{}).Execute(context.Background(), r, nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = l.Advance(64); e != nil {
		t.Fatal(e)
	}
	if _, e = l.Host(nil).Execute(context.Background(), r, &out); e == nil {
		t.Fatal("historical receipt authorized a current temporal effect")
	}
}
