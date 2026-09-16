package assistanceclient

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/app/graph"
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

func TestTemporalHelperBindsStillPermittedFactsOnBothReplayPaths(t *testing.T) {
	for _, stored := range []bool{false, true} {
		facts := append(temporalFacts(63, 3), LifeFact("bob", "alice", "available", 60, 30))
		l, r := temporalFixture(t, assistance.Multi, 63, facts)
		old, e := l.Host(assistance.FakePlanner{}).Execute(context.Background(), r, nil)
		if e != nil || !old.Delivered {
			t.Fatal("positive replay control", e)
		}
		var prior *assistance.Interaction
		if !stored {
			prior = &old
			l, r = temporalFixture(t, assistance.Multi, 63, facts)
		}
		// Same permissions and same eligibility: only the supported life claim changes.
		for scope, entries := range l.entries {
			for i := range entries {
				if entries[i].Event.Meta.ID == "bob-circumstance" {
					f, e := graph.DecodeTemporal(graph.MemoryRecord{Event: entries[i].Event, Content: *entries[i].Content})
					if e != nil {
						t.Fatal(e)
					}
					f.Category = "experience"
					entries[i].Content.Text, e = graph.EncodeTemporal(f)
					if e != nil {
						t.Fatal(e)
					}
				}
			}
			l.SetMemory(scope, entries)
		}
		before := l.Deliveries()
		if _, e = l.Host(nil).Execute(context.Background(), r, prior); e == nil || l.Deliveries() != before {
			t.Fatal("changed permitted fact reused old authority", stored)
		}
	}
}

func TestTemporalHelperRejectsForgedSourceAuthorshipAndFreshness(t *testing.T) {
	for _, change := range []string{"author", "chronology"} {
		l, r := temporalFixture(t, assistance.Multi, 63, temporalFacts(63, 3))
		for scope, entries := range l.entries {
			for i := range entries {
				if entries[i].Event.Meta.ID == "bob-temporal-source" {
					if change == "author" {
						entries[i].Event.Meta.Source = "alice"
					} else {
						entries[i].Event.OccurredAt = 3
						for j := range entries[i].Content.Learned {
							entries[i].Content.Learned[j].At = 3
						}
					}
				}
			}
			l.SetMemory(scope, entries)
		}
		calls := 0
		_, e := l.Host(planFunc(func(context.Context, assistance.Input) (assistance.Result, error) {
			calls++
			return assistance.Result{}, nil
		})).Execute(context.Background(), r, nil)
		if e == nil || calls != 0 || l.Deliveries() != 0 {
			t.Fatal("forged temporal provenance consumed", change, e, calls)
		}
	}
}

// newTemporalSnapshot supplies a newly authored diary and source, with a new
// request identity. It preserves helper history/burden in the same host.
func newTemporalSnapshot(t *testing.T, l *Local, at core.LogicalTime, channel core.ID) assistance.Request {
	t.Helper()
	facts := temporalFacts(at, 3)
	for i := range facts {
		facts[i].Channel = channel
		facts[i].Account = core.ID(fmt.Sprintf("%s-%d", facts[i].Account, at))
		facts[i].Source = core.ID(fmt.Sprintf("%s-%d", facts[i].Source, at))
	}
	fresh, r, e := TemporalFixture(assistance.Multi, at, channel, facts)
	if e != nil {
		t.Fatal(e)
	}
	for scope, entries := range fresh.entries {
		l.SetMemory(scope, entries)
	}
	r.ID = core.ID(fmt.Sprintf("snapshot-%d-%s", at, channel))
	for i := range r.Contexts {
		r.Contexts[i].Binding = r.ID
	}
	if e = l.Register(r); e != nil {
		t.Fatal(e)
	}
	return r
}
func TestTemporalHelperFreshSnapshotsKeepClarificationBudget(t *testing.T) {
	l, r := temporalFixture(t, assistance.Multi, 63, temporalFacts(63, 3))
	for i, at := range []core.LogicalTime{63, 64, 73, 83} {
		if i > 0 {
			r = newTemporalSnapshot(t, l, at, "chat")
		}
		calls := 0
		out, e := l.Host(planFunc(func(ctx context.Context, in assistance.Input) (assistance.Result, error) {
			calls++
			return (assistance.FakePlanner{}).Plan(ctx, in)
		})).Execute(context.Background(), r, nil)
		want := i == 0 || i == 2
		if e != nil || out.Delivered != want || calls != map[bool]int{true: 1, false: 0}[want] {
			t.Fatal("fresh temporal snapshot reset clarification budget", at, e, out, calls)
		}
	}
	if l.Deliveries() != 2 {
		t.Fatal("wrong bounded burden", l.Deliveries())
	}
}
func TestTemporalHelperHistoryFiltersChannelAndScrubsProvenance(t *testing.T) {
	for _, channel := range []core.ID{"chat", "phone"} {
		l, r := temporalFixture(t, assistance.Multi, 63, temporalFacts(63, 3))
		if out, e := l.Host(assistance.FakePlanner{}).Execute(context.Background(), r, nil); e != nil || !out.Delivered {
			t.Fatal(e)
		}
		r = newTemporalSnapshot(t, l, 73, channel)
		calls := 0
		out, e := l.Host(planFunc(func(ctx context.Context, in assistance.Input) (assistance.Result, error) {
			calls++
			want := 0
			if channel == "chat" {
				want = 1
			}
			if len(in.History) != want {
				t.Fatal("cross-channel temporal history", channel, len(in.History))
			}
			for _, old := range in.History {
				if old.Temporal != nil || old.Focus != nil || old.Scope != nil || old.EvidenceHash != "" || old.BoundaryHash != "" || old.RequestHash != "" {
					t.Fatal("historical provenance leaked to planner")
				}
				for _, candidate := range old.Result.Candidates {
					if len(candidate.Evidence) != 0 {
						t.Fatal("historical evidence leaked")
					}
				}
			}
			return (assistance.FakePlanner{}).Plan(ctx, in)
		})).Execute(context.Background(), r, nil)
		if e != nil || !out.Delivered || calls != 1 {
			t.Fatal("history control did not reach planner", channel, e, out)
		}
	}
}

func TestTemporalCorrectionChangesCurrentConsumerPreservesAuthorizedHistory(t *testing.T) {
	ctx := context.Background()
	oldFacts := append(temporalFacts(63, 3), LifeFact("alice", "bob", "busy", 60, 30), LifeFact("bob", "alice", "busy", 60, 30))
	l, oldRequest := temporalFixture(t, assistance.Multi, 63, oldFacts)
	old, e := l.Host(nil).Execute(ctx, oldRequest, nil)
	if e != nil || old.Delivered {
		t.Fatal("busy historical control", e)
	}
	nextFacts := append(temporalFacts(73, 3), LifeFact("alice", "bob", "available", 72, 30), LifeFact("bob", "alice", "available", 72, 30))
	for i := range nextFacts {
		nextFacts[i].Account += "-new"
		nextFacts[i].Source += "-new"
	}
	fresh, next, e := TemporalFixture(assistance.Multi, 73, "chat", nextFacts)
	if e != nil {
		t.Fatal(e)
	}
	for scope, entries := range fresh.entries {
		combined := copyValue(l.entries[scope])
		for _, entry := range entries {
			if !strings.HasSuffix(string(entry.Event.Meta.ID), "-new") {
				continue
			}
			if strings.HasPrefix(entry.Content.Text, "temporal.v1:") {
				entry.Supersedes = core.ID(strings.TrimSuffix(string(entry.Event.Meta.ID), "-new"))
			}
			entry.Sequence = int64(len(combined) + 1)
			combined = append(combined, entry)
		}
		l.SetMemory(scope, combined)
	}
	// Future journal rows do not rewrite the authorized old view at its old clock.
	historical, r := temporalFixture(t, assistance.Multi, 63, oldFacts)
	for scope, entries := range l.entries {
		historical.SetMemory(scope, entries)
	}
	replay, e := historical.Host(nil).Execute(ctx, r, &old)
	if e != nil || assistance.Digest(replay) != assistance.Digest(old) {
		t.Fatal("authorized historical replay changed", e)
	}
	next.ID = "corrected-temporal"
	for i := range next.Contexts {
		next.Contexts[i].Binding = next.ID
	}
	if e = l.Register(next); e != nil {
		t.Fatal(e)
	}
	current, e := l.Host(assistance.FakePlanner{}).Execute(ctx, next, nil)
	if e != nil || !current.Delivered {
		t.Fatal("corrected circumstance ignored by current helper", e, current)
	}
	q := oldRequest.Contexts[0].Query
	q.Actor = "alice"
	q.Purpose = "simulation"
	view, e := (graph.TemporalService{Memory: graph.MemoryService{Journal: l}}).Query(ctx, q)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, v := range view {
		if v.Fact.Kind == "circumstance" {
			found = true
			if v.Fact.Signal != "busy" {
				t.Fatal("old history silently rewritten")
			}
		}
	}
	if !found {
		t.Fatal("historically valid circumstance disappeared")
	}
	if _, e = l.Host(nil).Execute(ctx, oldRequest, &old); e == nil {
		t.Fatal("historical view authorized a new current effect")
	}
}

func TestTemporalHelperEstimatedRhythmAndHypothesisControls(t *testing.T) {
	for _, name := range []string{"estimated", "sparse", "irregular", "hypothesis"} {
		facts := []core.TemporalFact{}
		for _, f := range temporalFacts(63, 3) {
			if name != "hypothesis" && f.Kind == "expectation" {
				continue
			}
			if name == "sparse" && f.Kind == "contacts" {
				f.Observations = []core.LogicalTime{1, 3}
			}
			if name == "irregular" && f.Kind == "contacts" {
				f.Observations = []core.LogicalTime{0, 1, 20, 21}
			}
			facts = append(facts, f)
		}
		if name == "hypothesis" {
			life := LifeFact("alice", "bob", "busy", 60, 30)
			life.Person = "bob"
			life.Basis = "hypothesis"
			facts = append(facts, life)
		}
		l, r := temporalFixture(t, assistance.Multi, 63, facts)
		calls := 0
		out, e := l.Host(planFunc(func(ctx context.Context, in assistance.Input) (assistance.Result, error) {
			calls++
			if name == "estimated" {
				for _, view := range in.Temporal {
					if view.Rhythm.Basis != "estimated" || view.Rhythm.Samples != 4 || view.Rhythm.Confidence >= 1 {
						t.Fatal("estimate promoted to explicit certainty")
					}
				}
			}
			return (assistance.FakePlanner{}).Plan(ctx, in)
		})).Execute(context.Background(), r, nil)
		want := name == "estimated" || name == "hypothesis"
		if e != nil || out.Delivered != want || calls != map[bool]int{true: 1, false: 0}[want] {
			t.Fatal("estimator or attribution ignored", name, e, out)
		}
	}
}
