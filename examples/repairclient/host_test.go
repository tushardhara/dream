package repairclient

import (
	"context"
	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
	"reflect"
	"sync"
	"testing"
)

func mustFixture(t *testing.T) *Local {
	t.Helper()
	l, e := Fixture()
	if e != nil {
		t.Fatal(e)
	}
	return l
}
func mustPeriod(t *testing.T, l *Local, i int, follow bool) {
	t.Helper()
	if e := Period(l, i, follow); e != nil {
		t.Fatal(e)
	}
}
func mustResponse(t *testing.T, l *Local, id core.ID, at core.LogicalTime) assistance.RepairResponse {
	t.Helper()
	r, e := response(context.Background(), l, id, at)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestRepairActualPeriodsSeparateDeescalationAndDurableEvidence(t *testing.T) {
	report, e := Run(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	broken, followed := report.Scenarios[0], report.Scenarios[1]
	if broken.RemainingHours != 8 || followed.RemainingHours != 6 {
		t.Fatal("help did not consume actual resources")
	}
	if broken.Periods[2].Expectation != "repeated_breach_observed" || followed.Periods[1].Expectation != "new_follow_through_observed" || followed.Periods[2].Expectation != "sustained_follow_through_observed" {
		t.Fatal("history did not change current expectation")
	}
	if broken.Periods[2].Next != "consider_selective_distance" || followed.Periods[2].Next != "consider_bounded_plan" {
		t.Fatal("history not consumed by assistance")
	}
	for _, s := range report.Scenarios {
		for _, p := range s.Periods {
			if p.RepairVerdict != "NOT_ASSESSED" || p.ConfirmationRequests != 0 || p.Perspectives[1].Immediate.Assessment != "eased" || p.Perspectives[0].Later.Assessment != "eased" || p.Perspectives[1].Later.Assessment == "eased" {
				t.Fatal("different accounts/phases collapsed or repair invented")
			}
		}
	}
	if len(followed.Periods[2].Perspectives[1].Breached) != 1 || len(followed.Periods[2].Perspectives[1].Fulfilled) != 2 {
		t.Fatal("new support erased old harm")
	}
}
func TestApologyPromiseAndAttemptCannotInventRecipientCompletion(t *testing.T) {
	for _, phase := range []string{"apology", "promise", "help"} {
		t.Run(phase, func(t *testing.T) {
			l := mustFixture(t)
			g := Permissions("alice", "alice", "bob")
			due := core.LogicalTime(15)
			if e := l.Act("alice", ActionCommand{ID: "sorry", Kind: "apologize", At: 1, Grants: g}); e != nil {
				t.Fatal(e)
			}
			if phase != "apology" {
				if e := l.Act("alice", ActionCommand{ID: "promise", Kind: "promise", At: 3, Commitment: "c", Resource: "hours", Units: 1, Due: &due, Grants: g}); e != nil {
					t.Fatal(e)
				}
			}
			if phase == "help" {
				if e := l.Act("alice", ActionCommand{ID: "help", Kind: "help", At: 5, Commitment: "c", Resource: "hours", Units: 1, Grants: g}); e != nil {
					t.Fatal(e)
				}
			}
			out := mustResponse(t, l, "missing", 20)
			if out.Expectation != "unresolved" || out.RepairVerdict != "NOT_ASSESSED" || len(out.Perspectives[1].Fulfilled) != 0 || out.ConfirmationRequests != 0 {
				t.Fatal("action/expiry invented success")
			}
			if phase == "help" && l.Remaining("alice") != 7 {
				t.Fatal("attempt did not execute")
			}
		})
	}
}
func TestCorrectedAndRevokedEvidenceChangesCurrentAdviceAndReplay(t *testing.T) {
	l := mustFixture(t)
	mustPeriod(t, l, 0, false)
	mustPeriod(t, l, 1, true)
	mustPeriod(t, l, 2, true)
	before := mustResponse(t, l, "before", 52)
	saved := l.Evidence()
	if e := l.Correct("bob", "p2-observed", "correction", "unresolved", "", 55); e != nil {
		t.Fatal(e)
	}
	after := mustResponse(t, l, "after", 56)
	if before.Expectation != "sustained_follow_through_observed" || after.Expectation != "new_follow_through_observed" || !reflect.DeepEqual(saved, l.Evidence()[:len(saved)]) {
		t.Fatal("correction not used or history rewritten")
	}
	if e := l.Revoke("bob", "correction"); e != nil {
		t.Fatal(e)
	}
	now := mustResponse(t, l, "revoked", 57)
	if now.Expectation != "new_follow_through_observed" {
		t.Fatal("old success resurrected")
	}
	if e := l.Revoke("bob", "p1-observed"); e != nil {
		t.Fatal(e)
	}
	now = mustResponse(t, l, "revoked-more", 58)
	if now.Expectation != "breach_observed" {
		t.Fatal("revoked success counted")
	}
	r, e := l.Request("bob", "before", 52)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = l.Host().Execute(context.Background(), r, &before); e == nil {
		t.Fatal("replay bypassed current revoked lineage")
	}
}
func TestReadDeriveAndShareAreEachRequiredAcrossFullLineage(t *testing.T) {
	for _, source := range []core.ID{"p0-promise", "p0-practical", "p0-observed"} {
		for _, op := range []core.Operation{core.Read, core.Derive, core.ShareOnRequest} {
			for _, who := range []core.ID{"helper", "bob"} {
				if op == core.ShareOnRequest && who == "bob" {
					continue
				}
				t.Run(string(source)+string(op)+string(who), func(t *testing.T) {
					l := mustFixture(t)
					mustPeriod(t, l, 0, true)
					positive := mustResponse(t, l, "positive", 12)
					if positive.Expectation != "new_follow_through_observed" {
						t.Fatal("positive control")
					}
					for i := range l.log {
						if l.log[i].Meta.ID == source {
							kept := []core.Grant{}
							for _, g := range l.log[i].Meta.Rights.Grants {
								if !(g.Actor == who && g.Operation == op) {
									kept = append(kept, g)
								}
							}
							l.log[i].Meta.Rights.Grants = kept
						}
					}
					out := mustResponse(t, l, "negative", 13)
					if out.Expectation != "unresolved" || len(out.Perspectives[1].Fulfilled) != 0 {
						t.Fatal("missing permission did not suppress derived proof")
					}
				})
			}
		}
	}
}
func TestPrivateEvidenceCannotSteerOrIdentifyItselfInHelperResponse(t *testing.T) {
	run := func(finding string) assistance.RepairResponse {
		l := mustFixture(t)
		mustPeriod(t, l, 0, true)
		for i := range l.log {
			if l.log[i].Meta.ID == "p0-observed" {
				l.log[i].Finding = finding
				l.log[i].Meta.Rights.Grants = Permissions("bob")
			}
		}
		return mustResponse(t, l, "private", 12)
	}
	if assistance.Digest(run("fulfilled")) != assistance.Digest(run("unresolved")) {
		t.Fatal("unshared private assessment changed helper response/hash")
	}
}
func TestPauseEndingRefusalAndIndependentPrivateValue(t *testing.T) {
	l := mustFixture(t)
	mustPeriod(t, l, 0, false)
	g := Permissions("bob", "alice", "bob")
	for i, kind := range []string{"decline", "withdraw", "leave"} {
		if e := l.Act("bob", ActionCommand{ID: core.ID(kind), Kind: kind, At: core.LogicalTime(20 + i*3), Grants: g}); e != nil {
			t.Fatal(e)
		}
		out := mustResponse(t, l, core.ID("step-"+kind), core.LogicalTime(22+i*3))
		want := "respect_pause"
		if kind == "leave" {
			want = "respect_ending"
		}
		if out.Next != want || out.ConfirmationRequests != 0 {
			t.Fatal("boundary outcome not respected")
		}
	}
	r, e := l.Request("alice", "private-support", 29)
	if e != nil {
		t.Fatal(e)
	}
	out, e := l.Host().Execute(context.Background(), r, nil)
	if e != nil || out.Next != "private_support" {
		t.Fatal("other person's ending removed private value", e)
	}
	if e := l.Act("alice", ActionCommand{ID: "pressured", Kind: "help", At: 30, Resource: "hours", Units: 1, Grants: Permissions("alice", "alice", "bob")}); e == nil {
		t.Fatal("help bypassed current refusal")
	}
	if e := l.Act("bob", ActionCommand{ID: "wait", Kind: "wait", At: 30, Grants: g}); e != nil {
		t.Fatal("WAIT unavailable", e)
	}
	if e := l.Boundary("alice", core.Declined, 31); e != nil {
		t.Fatal(e)
	}
	r, e = l.Request("alice", "stop-helper", 32)
	if e != nil {
		t.Fatal(e)
	}
	out, e = l.Host().Execute(context.Background(), r, nil)
	if e != nil || out.Next != "WAIT" || len(out.Evidence) != 0 {
		t.Fatal("own helper refusal ignored")
	}
}
func TestRepairHistoryIsContextSpecificInActualConsumer(t *testing.T) {
	l := mustFixture(t)
	mustPeriod(t, l, 0, false)
	mustPeriod(t, l, 1, false)
	household := mustResponse(t, l, "household", 32)
	if household.Expectation != "repeated_breach_observed" {
		t.Fatal("baseline")
	}
	focus := Focus()
	focus.RoleContext = "work"
	r, e := l.RequestForFocus("bob", "work", 33, focus)
	if e != nil {
		t.Fatal(e)
	}
	out, e := l.Host().Execute(context.Background(), r, nil)
	if e != nil || out.Expectation != "unresolved" || len(out.Evidence) != 0 {
		t.Fatal("household harm dictated work context", e)
	}
}
func TestDuplicateActionReplayAndResourcesCannotManufactureFollowThrough(t *testing.T) {
	l := mustFixture(t)
	mustPeriod(t, l, 0, true)
	if e := l.Submit("bob", "repeat-report", "p0-practical", "observation", "fulfilled", "", "", 11, Permissions("bob", "alice", "bob")); e != nil {
		t.Fatal(e)
	}
	out := mustResponse(t, l, "duplicate-observer", 12)
	if out.Expectation != "new_follow_through_observed" || len(out.Perspectives[1].Fulfilled) != 1 {
		t.Fatal("repeated reporting became sustained behavior")
	}
	before := l.Remaining("alice")
	if e := l.Act("alice", ActionCommand{ID: "p0-practical", Kind: "help", At: 15, Resource: "hours", Units: 1, Grants: Permissions("alice", "alice", "bob")}); e == nil || l.Remaining("alice") != before {
		t.Fatal("duplicate action changed resources")
	}
	if e := l.Act("alice", ActionCommand{ID: "overspend", Kind: "help", At: 15, Resource: "hours", Units: 9, Grants: Permissions("alice", "alice", "bob")}); e == nil || l.Remaining("alice") != before {
		t.Fatal("overspent")
	}
	if e := l.Submit("alice", "self-success", "p0-practical", "observation", "fulfilled", "", "", 15, Permissions("alice", "alice", "bob")); e == nil {
		t.Fatal("sender fabricated recipient fulfilment")
	}
	if e := l.Correct("alice", "p0-observed", "foreign-correction", "unresolved", "", 15); e == nil {
		t.Fatal("corrected foreign account")
	}
}

type wrapJournal struct {
	base   *Local
	before func()
	after  func()
}

func (w wrapJournal) Visit(ctx context.Context, r assistance.RepairRequest, f func(assistance.RepairSnapshot) (assistance.RepairResponse, error)) (assistance.RepairResponse, error) {
	if w.before != nil {
		w.before()
	}
	return w.base.Visit(ctx, r, func(s assistance.RepairSnapshot) (assistance.RepairResponse, error) {
		out, e := f(s)
		if w.after != nil {
			w.after()
		}
		return out, e
	})
}
func TestRepairRevocationAndAtomicDelivery(t *testing.T) {
	l := mustFixture(t)
	mustPeriod(t, l, 0, true)
	r, e := l.Request("bob", "race", 12)
	if e != nil {
		t.Fatal(e)
	}
	ready, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	h := assistance.RepairHost{Journal: wrapJournal{base: l, before: func() { close(ready); <-release }}}
	go func() {
		out, e := h.Execute(context.Background(), r, nil)
		if e == nil && out.Expectation != "unresolved" {
			done <- assistance.ErrInvalid
		} else {
			done <- e
		}
	}()
	<-ready
	if e := l.Revoke("bob", "p0-observed"); e != nil {
		t.Fatal(e)
	}
	close(release)
	if e := <-done; e != nil {
		t.Fatal("revoked before snapshot not enforced", e)
	}
	l = mustFixture(t)
	mustPeriod(t, l, 0, true)
	r, e = l.Request("bob", "atomic", 12)
	if e != nil {
		t.Fatal(e)
	}
	projected, deliver := make(chan struct{}), make(chan struct{})
	revoked := make(chan struct{})
	started := make(chan struct{})
	result := make(chan assistance.RepairResponse, 1)
	h = assistance.RepairHost{Journal: wrapJournal{base: l, after: func() { close(projected); <-deliver }}}
	go func() { out, _ := h.Execute(context.Background(), r, nil); result <- out }()
	<-projected
	go func() { close(started); _ = l.Revoke("bob", "p0-observed"); close(revoked) }()
	<-started
	select {
	case <-revoked:
		t.Fatal("revocation interleaved inside authorization/delivery transaction")
	default:
	}
	close(deliver)
	out := <-result
	<-revoked
	if out.Expectation != "new_follow_through_observed" {
		t.Fatal("ordered pre-revocation delivery")
	}
	if _, e = l.Host().Execute(context.Background(), r, &out); e == nil {
		t.Fatal("post-revocation replay returned stale delivery")
	}
}
func TestRepairConcurrentIdempotencyAndRequestAuthentication(t *testing.T) {
	l := mustFixture(t)
	mustPeriod(t, l, 0, true)
	r, e := l.Request("bob", "same", 12)
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := l.Host().Execute(context.Background(), r, nil); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if len(l.responses) != 1 {
		t.Fatal("duplicate delivery state")
	}
	bad := r
	bad.User = "alice"
	if _, e = l.Host().Execute(context.Background(), bad, nil); e == nil {
		t.Fatal("request actor substitution")
	}
	bad = r
	bad.Focus.RoleContext = "work"
	if _, e = l.Host().Execute(context.Background(), bad, nil); e == nil {
		t.Fatal("request scope substitution")
	}
	if _, e = l.Request("outsider", "foreign", 13); e == nil {
		t.Fatal("foreign authentication")
	}
}

func TestRepairCloselySpacedFollowThroughIsNotSustained(t *testing.T) {
	l := mustFixture(t)
	mustPeriod(t, l, 0, false)
	mustPeriod(t, l, 1, true)
	mustPeriod(t, l, 2, true)
	// Valid alternate authored timing: last period follows immediately after the
	// preceding one. Two distinct commitments alone do not establish duration.
	for i := range l.log {
		v := &l.log[i]
		if len(v.Meta.ID) >= 3 && string(v.Meta.ID)[:3] == "p2-" {
			v.OccurredAt -= 10
			v.EffectAt -= 10
			v.LearnedAt -= 10
			v.Meta.Valid.Start -= 10
			if v.Due != nil {
				*v.Due -= 10
			}
		}
	}
	l.now = 40
	out := mustResponse(t, l, "close", 42)
	if out.Expectation != "new_follow_through_observed" {
		t.Fatal("closely spaced reports manufactured sustained behavior", out.Expectation)
	}
}

func TestRepairNewBreachChangesExpectationWithoutErasingEarlierHelp(t *testing.T) {
	l := mustFixture(t)
	mustPeriod(t, l, 0, true)
	mustPeriod(t, l, 1, true)
	before := mustResponse(t, l, "before-new-breach", 32)
	if before.Expectation != "sustained_follow_through_observed" {
		t.Fatal("positive control")
	}
	mustPeriod(t, l, 2, false)
	out := mustResponse(t, l, "new-breach", 52)
	if out.Expectation != "breach_observed" || len(out.Perspectives[1].Fulfilled) != 2 || len(out.Perspectives[1].Breached) != 1 {
		t.Fatal("new harm ignored or prior support erased")
	}
}
