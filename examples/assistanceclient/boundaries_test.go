package assistanceclient

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
)

func scopedFixture(t *testing.T, goal assistance.Goal, class core.InteractionClass) (*Local, assistance.Request) {
	t.Helper()
	l, r := Fixture(assistance.Multi, goal)
	r.Version = assistance.ScopedVersion
	r.Scope = &core.InteractionScope{Version: core.InteractionScopeVersion, Initiator: r.User, Target: "bob", Topic: "money", Class: class}
	if class == core.PrivatePreparation {
		r.Contexts = r.Contexts[:1]
		r.Arm = assistance.Single
	}
	// The fixture's v1 ID is already registered. New version gets a new trusted ID.
	r.ID = "scoped-request"
	for i := range r.Contexts {
		r.Contexts[i].Binding = r.ID
	}
	if e := l.Register(r); e != nil {
		t.Fatal(e)
	}
	return l, r
}
func preference(id, owner, with core.ID, class core.InteractionClass, decision core.Willingness) core.Boundary {
	return core.Boundary{Version: core.BoundaryVersion, Meta: core.Metadata{ID: id, Observer: owner, Source: owner, Sensitivity: core.Restricted, Supporting: []core.ID{"synthetic-source"}, Confidence: .8, Valid: core.Interval{Start: 0}, RecordedAt: time.Unix(1, 0).UTC(), Rights: core.Rights{Resource: id, Grants: []core.Grant{{Actor: owner, Recipient: owner, Purpose: "boundary", Operation: core.Read}}}}, Principal: owner, With: with, Topic: "money", Class: class, Decision: decision, Basis: "self_report", OccurredAt: 1, LearnedAt: 1}
}
func add(t *testing.T, l *Local, b core.Boundary) {
	t.Helper()
	if e := l.AppendBoundary(b.Meta.Observer, b); e != nil {
		t.Fatal(e)
	}
}
func consent(t *testing.T, l *Local, r assistance.Request) {
	t.Helper()
	for _, who := range r.Scope.Participants() {
		with := r.User
		if who == r.User {
			with = r.Scope.Target
		}
		b := preference(core.ID(fmt.Sprintf("%s-%s-%s", who, r.Scope.Topic, r.Scope.Class)), who, with, r.Scope.Class, core.Willing)
		b.Topic = r.Scope.Topic
		add(t, l, b)
	}
}
func nextRequest(t *testing.T, l *Local, r assistance.Request, id core.ID, at core.LogicalTime) assistance.Request {
	t.Helper()
	r = copyValue(r)
	r.ID = id
	r.At = at
	for i := range r.Contexts {
		r.Contexts[i].Binding = id
		r.Contexts[i].Query.ValidAt = at
		r.Contexts[i].Query.KnownAt = at
	}
	if e := l.Register(r); e != nil {
		t.Fatal(e)
	}
	return r
}
func TestScopedConsentBeforePlannerAndDataReads(t *testing.T) {
	for _, condition := range []string{"missing", "explicit", "refused", "unknown", "ended", "hypothesis", "host_refused"} {
		t.Run(condition, func(t *testing.T) {
			l, r := scopedFixture(t, assistance.Coordinate, core.Discussion)
			if condition != "missing" {
				consent(t, l, r)
			}
			if condition == "hypothesis" {
				l.boundaries[1].Basis = "hypothesis"
				l.boundaries[1].Meta.Observer = "alice"
				l.boundaries[1].Meta.Source = "alice"
			}
			decisions := map[string]core.Willingness{"refused": core.Declined, "unknown": core.WillingnessUnknown, "ended": core.Ended}
			if d, ok := decisions[condition]; ok {
				b := preference("negative", "bob", "alice", core.Discussion, d)
				add(t, l, b)
			}
			if condition == "host_refused" {
				l.SetAllowed(false)
			}
			calls := 0
			out, e := l.Host(planFunc(func(ctx context.Context, in assistance.Input) (assistance.Result, error) {
				calls++
				return (assistance.FakePlanner{}).Plan(ctx, in)
			})).Execute(context.Background(), r, nil)
			allowed := condition == "explicit"
			if e != nil || out.Delivered != allowed || calls != map[bool]int{true: 1, false: 0}[allowed] {
				t.Fatal("preplanning gate", out, e, calls)
			}
			if !allowed && (len(l.audits) != 0 || out.Result.Candidates[out.Result.Selected].Reason != "boundary") {
				t.Fatal("denied boundary read private context or lost WAIT")
			}
		})
	}
}
func TestScopedRevocationPlanningCommitAndReplay(t *testing.T) {
	for _, stage := range []string{"planning", "commit", "replay", "stored"} {
		t.Run(stage, func(t *testing.T) {
			l, r := scopedFixture(t, assistance.Coordinate, core.Discussion)
			consent(t, l, r)
			revoke := func() {
				if e := l.RevokeBoundary("bob", "bob-money-discussion", 3); e != nil {
					t.Fatal(e)
				}
			}
			h := l.Host(assistance.FakePlanner{})
			var recorded *assistance.Interaction
			if stage == "replay" || stage == "stored" {
				out, e := h.Execute(context.Background(), r, nil)
				if e != nil {
					t.Fatal(e)
				}
				if stage == "replay" {
					recorded = &out
				}
				revoke()
			}
			if stage == "planning" {
				h.Planner = planFunc(func(ctx context.Context, in assistance.Input) (assistance.Result, error) {
					revoke()
					return (assistance.FakePlanner{}).Plan(ctx, in)
				})
			}
			if stage == "commit" {
				h.Journal = beforeCommit{Local: l, before: revoke}
			}
			before := l.Deliveries()
			history, _ := l.Read(context.Background(), r)
			out, e := h.Execute(context.Background(), r, recorded)
			after, _ := l.Read(context.Background(), r)
			if e == nil || out.Version != "" || l.Deliveries() != before || len(history) != len(after) {
				t.Fatal("revocation bypass", out, e)
			}
		})
	}
}
func TestScopedMoneyPauseRelayAndUnrelatedConversation(t *testing.T) {
	for _, kind := range []string{"direct", "relay", "other_topic", "sibling"} {
		t.Run(kind, func(t *testing.T) {
			l, r := scopedFixture(t, assistance.Coordinate, core.Discussion)
			consent(t, l, r)
			pause := preference("pause", "bob", "alice", core.Discussion, core.TakingBreak)
			end := core.LogicalTime(8)
			pause.Meta.Valid.End = &end
			add(t, l, pause)
			r = copyValue(r)
			r.Contexts = nil
			switch kind {
			case "relay":
				r.Scope.Via = "charlie"
				r.Scope.Class = core.ThirdPartyInvolvement
				r.Participants = append(r.Participants, "charlie")
				consent(t, l, r)
				add(t, l, preference("charlie-discussion", "charlie", "alice", core.Discussion, core.Willing))
			case "other_topic":
				r.Scope.Topic = "weekend"
				consent(t, l, r)
			case "sibling":
				r.Scope.Target = "charlie"
				r.Participants = append(r.Participants, "charlie")
				for _, who := range []core.ID{"alice", "charlie"} {
					with := core.ID("alice")
					if who == "alice" {
						with = "charlie"
					}
					add(t, l, preference(core.ID("sibling-"+string(who)), who, with, core.Discussion, core.Willing))
				}
			}
			r = nextRequest(t, l, r, "next", 3)
			out, e := l.Host(assistance.FakePlanner{}).Execute(context.Background(), r, nil)
			if e != nil || out.Delivered != (kind == "other_topic" || kind == "sibling") {
				t.Fatal(kind, out, e)
			}
		})
	}
}
func TestScopedPressureVersusDisagreementAndPrivatePreparation(t *testing.T) {
	for _, class := range []core.InteractionClass{core.Discussion, core.SummarySharing, core.PrivatePreparation} {
		for _, signal := range []string{"disagreement", "uncertain_pressure", "credible_pressure", "credible_threat"} {
			t.Run(string(class)+"/"+signal, func(t *testing.T) {
				goal := assistance.Coordinate
				if class == core.PrivatePreparation {
					goal = assistance.Listen
				}
				l, r := scopedFixture(t, goal, class)
				consent(t, l, r)
				b := preference("signal", "alice", "bob", core.Discussion, core.PressureSignal)
				b.Basis = "observed_signal"
				b.Signal = signal
				add(t, l, b)
				out, e := l.Host(assistance.FakePlanner{}).Execute(context.Background(), r, nil)
				if e != nil || out.Delivered != (signal == "disagreement" || class == core.PrivatePreparation) {
					t.Fatal(out, e)
				}
			})
		}
	}
}
func TestScopedBreakExpiryNeedsFreshPreferenceAndCorrectionsAreOwned(t *testing.T) {
	l, r := scopedFixture(t, assistance.Coordinate, core.Discussion)
	consent(t, l, r)
	pause := preference("pause", "bob", "alice", core.Discussion, core.TakingBreak)
	end := core.LogicalTime(5)
	pause.Meta.Valid.End = &end
	add(t, l, pause)
	r = nextRequest(t, l, r, "expired", 6)
	out, e := l.Host(assistance.FakePlanner{}).Execute(context.Background(), r, nil)
	if e != nil || out.Delivered {
		t.Fatal("expiry manufactured consent", out, e)
	}
	fresh := preference("fresh", "bob", "alice", core.Discussion, core.Willing)
	fresh.OccurredAt = 7
	fresh.LearnedAt = 7
	fresh.Supersedes = []core.ID{"pause"}
	if l.AppendBoundary("alice", fresh) == nil || l.RevokeBoundary("alice", "pause", 7) == nil {
		t.Fatal("other principal changed preference")
	}
	add(t, l, fresh)
	r = nextRequest(t, l, r, "fresh", 7)
	out, e = l.Host(assistance.FakePlanner{}).Execute(context.Background(), r, nil)
	if e != nil || !out.Delivered {
		t.Fatal("owned correction not effective", out, e)
	}
	// A journal result does not grant the old boundary revision continued authority.
	refusal := preference("refusal", "bob", "alice", core.Discussion, core.Declined)
	refusal.LearnedAt = 7
	add(t, l, refusal)
	if _, e = l.Host(nil).Execute(context.Background(), r, &out); e == nil {
		t.Fatal("replay ignored changed preference")
	}
}
func TestScopedClarificationCooldownBudgetAndBackdating(t *testing.T) {
	l, r := scopedFixture(t, assistance.Unknown, core.Clarification)
	consent(t, l, r)
	for i, at := range []core.LogicalTime{2, 3, 12, 22} {
		if i > 0 {
			r = nextRequest(t, l, r, core.ID(fmt.Sprintf("request-%d", i)), at)
		}
		out, e := l.Host(assistance.FakePlanner{}).Execute(context.Background(), r, nil)
		if e != nil || out.Delivered != (i == 0 || i == 2) {
			t.Fatal("clarification gate", i, out, e)
		}
		if i == 0 {
			again, e := l.Host(nil).Execute(context.Background(), r, &out)
			if e != nil || assistance.Digest(again) != assistance.Digest(out) || l.Deliveries() != 1 {
				t.Fatal("retry consumed cooldown", e)
			}
		}
	}
	earlier := copyValue(r)
	earlier.ID = "backdated"
	earlier.At = 2
	earlier.Contexts = nil
	if l.Register(earlier) == nil {
		t.Fatal("new backdated ID reset cooldown")
	}
}
func TestScopedHistoryHidesBoundaryHashAndOtherScopes(t *testing.T) {
	l, r := scopedFixture(t, assistance.Coordinate, core.Discussion)
	consent(t, l, r)
	if _, e := l.Host(assistance.FakePlanner{}).Execute(context.Background(), r, nil); e != nil {
		t.Fatal(e)
	}
	r = nextRequest(t, l, r, "history", 3)
	check := func(want int) planFunc {
		return func(ctx context.Context, in assistance.Input) (assistance.Result, error) {
			if len(in.History) != want || in.Scope == nil {
				t.Fatal("scoped history", len(in.History), want)
			}
			for _, old := range in.History {
				if old.Scope != nil || old.BoundaryHash != "" || old.EvidenceHash != "" || old.RequestHash != "" {
					t.Fatal("boundary evidence exposed to planner")
				}
			}
			return (assistance.FakePlanner{}).Plan(ctx, in)
		}
	}
	if _, e := l.Host(check(1)).Execute(context.Background(), r, nil); e != nil {
		t.Fatal(e)
	}
	r = copyValue(r)
	r.Scope.Topic = "weekend"
	consent(t, l, r)
	r = nextRequest(t, l, r, "other", 4)
	if _, e := l.Host(check(0)).Execute(context.Background(), r, nil); e != nil {
		t.Fatal(e)
	}
}

func TestScopedConcurrentClarificationsCommitOneEffect(t *testing.T) {
	l, r := scopedFixture(t, assistance.Unknown, core.Clarification)
	consent(t, l, r)
	other := nextRequest(t, l, r, "concurrent", r.At)
	var prepared, sent sync.WaitGroup
	prepared.Add(2)
	sent.Add(2)
	h := l.Host(assistance.FakePlanner{})
	h.Journal = beforeCommit{Local: l, before: func() { prepared.Done(); prepared.Wait() }}
	results := make(chan error, 2)
	for _, request := range []assistance.Request{r, other} {
		go func(request assistance.Request) {
			defer sent.Done()
			_, e := h.Execute(context.Background(), request, nil)
			results <- e
		}(request)
	}
	sent.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		}
	}
	if success != 1 || l.Deliveries() != 1 {
		t.Fatal("concurrent requests bypassed atomic cooldown", success, l.Deliveries())
	}
}
func TestScopedVersionsPrivateAccountsAndForgedWaitReplay(t *testing.T) {
	l, r := scopedFixture(t, assistance.Coordinate, core.Discussion)
	out, e := l.Host(nil).Execute(context.Background(), r, nil)
	if e != nil {
		t.Fatal(e)
	}
	// A valid-looking proposal cannot override the exact boundary WAIT on replay.
	out.Result = assistance.ExplicitPreference(r.Goal, r.User)
	out.Result.Version = r.Version
	out.Delivered = true
	if _, e := l.Host(nil).Execute(context.Background(), r, &out); e == nil {
		t.Fatal("forged replay overrode unknown willingness")
	}
	for _, version := range []string{assistance.Version, "future"} {
		bad := copyValue(r)
		bad.Version = version
		if bad.Validate() == nil {
			t.Fatal("scope reinterpreted version", version)
		}
	}
	_, private := scopedFixture(t, assistance.Listen, core.PrivatePreparation)
	private.Arm = assistance.Multi
	private.Contexts = copyValue(r.Contexts)
	for i := range private.Contexts {
		private.Contexts[i].Binding = private.ID
	}
	if private.Validate() == nil {
		t.Fatal("private preparation imported another person's account")
	}
}

func TestScopedReplayPinsStillPermissiveBoundaryEvidence(t *testing.T) {
	for _, stored := range []bool{false, true} {
		t.Run(fmt.Sprint(stored), func(t *testing.T) {
			l, r := scopedFixture(t, assistance.Coordinate, core.Discussion)
			consent(t, l, r)
			out, e := l.Host(assistance.FakePlanner{}).Execute(context.Background(), r, nil)
			if e != nil {
				t.Fatal(e)
			}
			if !stored {
				l, r = scopedFixture(t, assistance.Coordinate, core.Discussion)
				consent(t, l, r)
			}
			fresh := preference("additional-willingness", "bob", "alice", core.Discussion, core.Willing)
			fresh.LearnedAt = 2
			add(t, l, fresh)
			snapshot, e := l.ReadBoundaries(context.Background(), r)
			if e != nil {
				t.Fatal(e)
			}
			decision, e := core.EvaluateBoundaries(snapshot.Records, *r.Scope, snapshot.Now)
			if e != nil || !decision.Allowed {
				t.Fatal("control must remain eligible", e)
			}
			before := l.Deliveries()
			var record *assistance.Interaction
			if !stored {
				record = &out
			}
			if _, e := l.Host(nil).Execute(context.Background(), r, record); e == nil || l.Deliveries() != before {
				t.Fatal("replay ignored changed but still-permissive policy evidence")
			}
		})
	}
}

func TestScopedExtraParticipantCannotSupplyPrivateAccount(t *testing.T) {
	_, r := scopedFixture(t, assistance.Coordinate, core.Discussion)
	r.Scope.Target = "charlie"
	r.Participants = append(r.Participants, "charlie")
	if r.Validate() == nil {
		t.Fatal("out-of-scope participant supplied private context")
	}
	r.Contexts = r.Contexts[:1]
	if r.Validate() != nil {
		t.Fatal("in-scope own context positive control")
	}
}
