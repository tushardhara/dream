package groupexperiment

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/examples/groupclient"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/demo"
	"reflect"
	"testing"
	"time"
)

func dup[T any](v T) T { b, _ := json.Marshal(v); var out T; _ = json.Unmarshal(b, &out); return out }
func fixture(t *testing.T, n int, omitted bool) Case {
	t.Helper()
	c, e := Fixture(n, 11, omitted, 4)
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func helper(t *testing.T, c Case) (*groupclient.Local, assistance.GroupRequest, assistance.GroupResponse) {
	t.Helper()
	l, e := groupclient.New("session", c.Native.History, c.Budget, c.Reservations, c.Boundaries, c.Native.At)
	if e != nil {
		t.Fatal(e)
	}
	r, e := l.Request(Person(0), "request", c.Native.Decision, c.Native.At)
	if e != nil {
		t.Fatal(e)
	}
	out, e := l.Host().Execute(context.Background(), r, nil)
	if e != nil {
		t.Fatal(e)
	}
	return l, r, out
}
func own(who core.ID) []core.Grant {
	return []core.Grant{{Actor: who, Recipient: who, Purpose: "simulation", Operation: core.Read}, {Actor: who, Recipient: who, Purpose: "simulation", Operation: core.Derive}}
}
func planGrants() []core.Grant {
	return []core.Grant{{Actor: Person(0), Recipient: Person(0), Purpose: "help", Operation: core.Read}, {Actor: Person(0), Recipient: Person(0), Purpose: "help", Operation: core.Derive}}
}
func perspective(t *testing.T, c Case, who core.ID) core.GroupPerspective {
	t.Helper()
	p, e := core.GroupPerspectiveFor(c.Native.History, "friends", Focus(), who, c.Native.At, own(who))
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func selected(tr demo.GroupNativeTrace) behavior.Kind {
	d := tr.Decision.Human
	return d.Candidates[d.Selected].Offer.Kind
}
func TestGroupNativeHistoryChoicesAndReplay(t *testing.T) {
	for _, n := range []int{5, 24} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			changed, reserved := 0, 0
			for seed := uint64(1); seed <= 32; seed++ {
				a, e := Fixture(n, seed, false, 4)
				if e != nil {
					t.Fatal(e)
				}
				b, _ := Fixture(n, seed, true, 4)
				x, e := Run(context.Background(), a)
				if e != nil {
					t.Fatal(e)
				}
				y, e := demo.RunGroups(b.Native, nil)
				if e != nil {
					t.Fatal(e)
				}
				if x.HumanValidity != "NOT_TESTED" || x.GlobalWelfare != "NOT_AGGREGATED" || len(x.Native.Traces) != n || len(x.Helper.Perspectives) != n || len(x.Native.Responses) != n-1 {
					t.Fatal("dishonest/bounded output")
				}
				last, other := x.Native.Traces[n-1], y.Traces[n-1]
				if last.DyadicHash != other.DyadicHash || x.Native.Theme != y.Theme || !reflect.DeepEqual(last.Present, other.Present) {
					t.Fatal("intervention changed control dyads/theme/current attendance")
				}
				if last.Signals.Inclusion != 1 || other.Signals.Inclusion != 1.0/3 || other.Signals.Exclusion != 2.0/3 {
					t.Fatal("ignored attributed group history")
				}
				if last.Decision.Human.Candidates[1].Consequence == other.Decision.Human.Candidates[1].Consequence {
					t.Fatal("group state did not reach native planning utility")
				}
				if selected(last) != selected(other) {
					changed++
				}
				if x.Execution == "reserved_shared_care" {
					reserved++
					if len(x.Reservations) != 1 || len(x.Reservations[0].Tasks) != 2 {
						t.Fatal("native agreed plan did not reserve actual split care")
					}
				}
				if !reflect.DeepEqual(x.Native.Traces[0], y.Traces[0]) {
					t.Fatal("private peer history leaked into unchanged observer")
				}
				if seed == 1 {
					if _, e = demo.RunGroups(a.Native, &x.Native); e != nil {
						t.Fatal("exact native replay", e)
					}
					if _, e = demo.RunGroups(b.Native, &x.Native); e == nil {
						t.Fatal("stale native replay accepted")
					}
				}
			}
			if changed == 0 || reserved == 0 {
				t.Fatalf("missing actual selection changes=%d executions=%d", changed, reserved)
			}
			t.Logf("32 paired seeds: %d changed selections; %d native agreed reservations", changed, reserved)
		})
	}
}
func TestGroupPerPersonEffectsUnknownAndPrivate(t *testing.T) {
	for _, n := range []int{5, 24} {
		a := fixture(t, n, false)
		b := dup(a)
		for i := range b.Native.History.Accounts {
			v := &b.Native.History.Accounts[i]
			if v.Member == Person(n-1) && v.Phase == "experienced" {
				v.Burden = core.ObservedGroupQuantity(9)
				v.Effort = core.ObservedGroupQuantity(9)
			}
		}
		_, _, x := helper(t, a)
		_, _, y := helper(t, b)
		if !reflect.DeepEqual(x.Perspectives[0], y.Perspectives[0]) || *x.Perspectives[0].Benefit.Value != .5 || *x.Perspectives[n-1].Burden.Value == *y.Perspectives[n-1].Burden.Value {
			t.Fatal("third-party consequences collapsed into primary benefit")
		}
		for i := range b.Native.History.Accounts {
			v := &b.Native.History.Accounts[i]
			if v.Member == Person(n-1) {
				v.Meta.Rights.Grants = own(v.Member)
			}
		}
		_, _, hidden := helper(t, b)
		p := hidden.Perspectives[n-1]
		if len(p.Evidence) != 0 || p.Burden.Status != core.Unknown || p.Burden.Value != nil || len(hidden.Alternatives) != 0 {
			t.Fatal("membership became private access/agreement or unknown became zero")
		}
		// Absence, quiet and newcomer status supply neither exclusion nor consent.
		for _, status := range []string{"unknown", "absent", "declined"} {
			c := fixture(t, n, false)
			for i := range c.Native.History.Accounts {
				v := &c.Native.History.Accounts[i]
				if v.Member != Person(n-1) {
					continue
				}
				v.Membership = "newcomer"
				v.Participation = status
				v.CoPresent = nil
				v.Effort = core.UnknownGroupQuantity()
				v.Burden = core.UnknownGroupQuantity()
				v.Benefit = core.UnknownGroupQuantity()
				v.Capacity = core.UnknownGroupQuantity()
				v.NormStance = "unspecified"
				v.ApprovedOptions = nil
				if v.Phase == "planning" {
					v.Participation = "unknown"
				}
			}
			p := perspective(t, c, Person(n-1))
			if p.Exclusion.Status != core.Unknown || p.Burden.Status != core.Unknown || len(p.Patterns) != 0 {
				t.Fatal("quiet/absent newcomer inferred uncaring")
			}
			_, _, out := helper(t, c)
			if len(out.Alternatives) != 0 {
				t.Fatal("silence became agreement")
			}
			native, e := demo.RunGroups(c.Native, nil)
			if e != nil {
				t.Fatal(e)
			}
			if len(native.Traces[n-1].Present) != 1 {
				t.Fatal("group roster became observed attendance")
			}
		}
	}
}
func correction(c Case, index int, id core.ID, at core.LogicalTime) core.GroupAccount {
	a := dup(c.Native.History.Accounts[index])
	a.Supersedes = a.Meta.ID
	a.Meta.ID = id
	a.Meta.Rights.Resource = id
	a.LearnedAt = at
	a.Meta.RecordedAt = time.Unix(int64(at), 0).UTC()
	return a
}
func TestGroupCorrectionsPatternsAndCurrentPermissions(t *testing.T) {
	c := fixture(t, 5, true)
	p := perspective(t, c, Person(4))
	if len(p.Patterns) != 1 || p.Patterns[0].Kind != "repeated_omission" || len(p.Patterns[0].Evidence) != 2 {
		t.Fatal("not evidence derived", p)
	}
	a := correction(c, 4, "corrected-omission", 33)
	a.Participation = "absent"
	a.NormStance = "disputes"
	c.Native.History.Accounts = append(c.Native.History.Accounts, a)
	p = perspective(t, c, Person(4))
	if len(p.Patterns) != 0 || p.Participation != "attended" || len(p.CoPresent) != 5 {
		t.Fatal("correction failed, or old corrected occasion replaced current attendance", p)
	}
	c.Native.History.Accounts[len(c.Native.History.Accounts)-1].Meta.Rights.Revoked = true
	p = perspective(t, c, Person(4))
	if len(p.Patterns) != 0 {
		t.Fatal("revoked correction resurrected superseded omission")
	}
	c = fixture(t, 5, false)
	a = correction(c, 14, "disputed-practice", 33)
	a.NormStance = "disputes"
	c.Native.History.Accounts = append(c.Native.History.Accounts, a)
	p = perspective(t, c, Person(4))
	if len(p.Patterns) != 1 || p.Patterns[0].Stance != "disputes" {
		t.Fatal("member dispute lost")
	}
	// Two renamed decisions at one occasion are not two repeated practices.
	c = fixture(t, 5, false)
	c.Native.History.Decisions[1].Window = c.Native.History.Decisions[0].Window
	c.Native.History.Decisions[1].At = 1
	c.Native.History.Decisions[1].Meta.Valid.Start = 1
	c.Native.History.Accounts = c.Native.History.Accounts[:10]
	p = perspective(t, c, Person(4))
	if len(p.Patterns) != 0 {
		t.Fatal("duplicate occasion created ritual")
	}
}
func TestGroupHistoryDirectContractRejections(t *testing.T) {
	tests := map[string]func(*Case){
		"duplicate decision business identity": func(c *Case) {
			d := dup(c.Native.History.Decisions[3])
			d.Meta.ID = "other-receipt"
			d.Meta.Rights.Resource = d.Meta.ID
			c.Native.History.Accounts = nil // isolate the duplicate decision business-key contract
			c.Native.History.Decisions = append(c.Native.History.Decisions, d)
		},
		"duplicate account business identity": func(c *Case) {
			a := dup(c.Native.History.Accounts[19])
			a.Meta.ID = "other-account"
			a.Meta.Rights.Resource = a.Meta.ID
			c.Native.History.Accounts = append(c.Native.History.Accounts, a)
		},
		"silent care transfer": func(c *Case) { c.Native.History.Decisions[3].Options[1].Tasks[0].Units = 1 },
		"duplicate task": func(c *Case) {
			o := &c.Native.History.Decisions[3].Options[0]
			o.Tasks = []core.GroupTask{{Task: "care", Owner: Person(4), Units: 2}, {Task: "care", Owner: Person(4), Units: 2}}
		},
		"duplicate option":        func(c *Case) { c.Native.History.Decisions[3].Options[1].ID = "solo-care" },
		"unobserved zero":         func(c *Case) { c.Native.History.Accounts[19].Burden.Value = core.ObservedGroupQuantity(0).Value },
		"forged member account":   func(c *Case) { c.Native.History.Accounts[19].Meta.Observer = Person(0) },
		"foreign private lineage": func(c *Case) { c.Native.History.Accounts[19].Meta.Parents = []core.ID{"account:0:0"} },
		"future experience":       func(c *Case) { a := &c.Native.History.Accounts[0]; a.OccurredAt = 2; a.Meta.Valid.Start = 2 },
		"omitted affected member": func(c *Case) { c.Native.History.Decisions[3].Affected = c.Native.History.Decisions[3].Affected[:4] },
		"retroactive consent":     func(c *Case) { c.Native.History.Accounts[0].ApprovedOptions = []core.ID{"solo-care"} },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			c := fixture(t, 5, false)
			mutate(&c)
			if c.Native.History.Validate() == nil {
				t.Fatal("accepted invalid contract")
			}
		})
	}
	c := fixture(t, 5, false)
	raw, e := core.EncodeGroupHistory(c.Native.History)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = core.DecodeGroupHistory(raw); e != nil {
		t.Fatal(e)
	}
	for _, bad := range [][]byte{append(raw, []byte(" {}")...), append([]byte(`{"rogue":true,`), raw[1:]...)} {
		if _, e = core.DecodeGroupHistory(bad); e == nil {
			t.Fatal("non-strict codec")
		}
	}
}
func TestGroupReservationConsentAndReplay(t *testing.T) {
	c := fixture(t, 5, false)
	l, r, out := helper(t, c)
	if len(out.Alternatives) != 3 || len(l.Reservations()) != 0 {
		t.Fatal("helper did not propose or executed without human")
	}
	if _, e := l.Host().Execute(context.Background(), r, &out); e != nil {
		t.Fatal("exact replay", e)
	}
	if e := l.Reserve(Person(1), "reserve", r.Decision, "shared-care", 33); e == nil {
		t.Fatal("foreign planner")
	}
	if e := l.Reserve(Person(0), "reserve", r.Decision, "shared-care", 33); e != nil {
		t.Fatal(e)
	}
	if e := l.Reserve(Person(0), "different-receipt", r.Decision, "solo-care", 33); e == nil {
		t.Fatal("same decision reserved twice")
	}
	rs := l.Reservations()
	other := dup(rs[0])
	other.ID = "different-receipt"
	if core.ValidateGroupReservations(c.Native.History, append(rs, other)) == nil {
		t.Fatal("direct duplicate reservation business key")
	}
	rs[0].Tasks[0].Owner = Person(0)
	if core.ValidateGroupReservations(c.Native.History, rs) == nil {
		t.Fatal("reservation silently changed obligation")
	}
	for _, mutate := range []func(*Case){func(c *Case) { c.Native.History.Accounts[19].ApprovedOptions = nil }, func(c *Case) { c.Native.History.Accounts[19].Capacity = core.UnknownGroupQuantity() }, func(c *Case) { c.Budget.Shared["shared-time"] = 1 }, func(c *Case) { c.Budget.Personal[Person(4)] = 1 }, func(c *Case) { c.Native.History.Accounts[19].Meta.Rights.Revoked = true }} {
		b := dup(c)
		mutate(&b)
		if core.GroupOptionFeasible(b.Native.History, nil, b.Budget, b.Native.Decision, "shared-care", 33, planGrants()) == nil {
			t.Fatal("missing agreement/capacity/rights accepted")
		}
	}
}

// Add an overlapping household role with independent self-authored agreements.
// The same physical person has one capacity portfolio across both memberships.
func overlap(c Case) Case {
	d := dup(c.Native.History.Decisions[3])
	d.ID = "family-decision"
	d.Meta.ID = "family-receipt"
	d.Meta.Rights.Resource = d.Meta.ID
	d.Group = "family"
	d.Focus.RoleContext = "caregiver"
	d.Intention = "elder-care"
	c.Native.History.Decisions = append(c.Native.History.Decisions, d)
	for _, old := range dup(c.Native.History.Accounts) {
		if old.Decision != c.Native.Decision {
			continue
		}
		a := old
		a.Decision = d.ID
		a.Group = d.Group
		a.Focus = d.Focus
		a.Intention = d.Intention
		a.Meta.ID = core.ID("family-" + string(a.Member))
		a.Meta.Rights.Resource = a.Meta.ID
		a.Meta.Supporting = []core.ID{d.Meta.ID}
		c.Native.History.Accounts = append(c.Native.History.Accounts, a)
	}
	return c
}
func TestGroupOverlappingRolesResourcesAndNoRefund(t *testing.T) {
	for _, n := range []int{5, 24} {
		c := overlap(fixture(t, n, false))
		rs, e := core.ReserveGroup(c.Native.History, nil, c.Budget, "family-decision", "solo-care", "family-reservation", Person(0), 33, planGrants())
		if e != nil {
			t.Fatal(e)
		}
		// Even revoking access to that group's agreements cannot refund its care work.
		for i := range c.Native.History.Accounts {
			if c.Native.History.Accounts[i].Group == "family" {
				c.Native.History.Accounts[i].Meta.Rights.Revoked = true
			}
		}
		if core.GroupOptionFeasible(c.Native.History, rs, c.Budget, c.Native.Decision, "shared-care", 33, planGrants()) == nil {
			t.Fatal("overlapping caregiver overbooked")
		}
		if core.GroupOptionFeasible(c.Native.History, rs, c.Budget, c.Native.Decision, "covered-care", 33, planGrants()) != nil {
			t.Fatal("valid alternative with another consenting capable person lost")
		}
		// Different role accounts do not change the friends perspective.
		original := fixture(t, n, false)
		if !reflect.DeepEqual(perspective(t, original, Person(n-1)), perspective(t, c, Person(n-1))) {
			t.Fatal("cross-role private evidence leaked")
		}
		c.Budget.Shared["shared-time"] = 4
		if core.GroupOptionFeasible(c.Native.History, rs, c.Budget, c.Native.Decision, "covered-care", 33, planGrants()) == nil {
			t.Fatal("shared time overbooked")
		}
		c.Budget.Personal[Person(n-1)] = 3
		if core.ValidateGroupPortfolio(c.Native.History, rs, c.Budget) == nil {
			t.Fatal("imported invalid portfolio accepted")
		}
	}
}
func TestGroupCurrentBoundaryCorrectionAndRevocationReplay(t *testing.T) {
	c := fixture(t, 5, false)
	for _, change := range []string{"boundary", "correction", "revoke", "request-forgery", "cancel"} {
		t.Run(change, func(t *testing.T) {
			l, r, out := helper(t, c)
			switch change {
			case "boundary":
				if e := l.AppendBoundary(Person(4), Boundary(Person(4), Person(0), core.Coordination, core.Declined, 34)); e != nil {
					t.Fatal(e)
				}
			case "correction":
				a := correction(c, 19, "withdraw-option", 34)
				a.ApprovedOptions = nil
				if e := l.Correct(Person(4), a); e != nil {
					t.Fatal(e)
				}
			case "revoke":
				if e := l.Revoke(Person(4), "account:3:4"); e != nil {
					t.Fatal(e)
				}
			case "request-forgery":
				r.User = Person(1)
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				if _, e := l.Host().Execute(ctx, r, &out); e == nil {
					t.Fatal("canceled request delivered")
				}
				return
			}
			if _, e := l.Host().Execute(context.Background(), r, &out); e == nil {
				t.Fatal("stale/forged replay delivered")
			}
			if change != "request-forgery" {
				if e := l.Reserve(Person(0), "reserve", c.Native.Decision, "shared-care", 34); e == nil {
					t.Fatal("current boundary/correction/revocation bypass")
				}
			}
		})
	}
}
func TestGroupNativeAgreementAndPresence(t *testing.T) {
	a := fixture(t, 5, false)
	b := dup(a)
	b.Native.History.Accounts[19].ApprovedOptions = nil
	x, e := demo.RunGroups(a.Native, nil)
	if e != nil {
		t.Fatal(e)
	}
	y, e := demo.RunGroups(b.Native, nil)
	if e != nil {
		t.Fatal(e)
	}
	if !x.Traces[4].OwnAgreement || y.Traces[4].OwnAgreement {
		t.Fatal("own agreement ignored")
	}
	for _, candidate := range y.Traces[4].Decision.Human.Candidates {
		if candidate.Offer.Kind == behavior.Coordinate {
			t.Fatal("no-agreement plan executable")
		}
	}
	// Current self-observed co-presence changes feasible recipient actions while
	// all dyadic state and the invitation roster remain identical.
	b = dup(a)
	b.Native.History.Accounts[14].CoPresent = []core.ID{Person(4)}
	y, e = demo.RunGroups(b.Native, nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(y.Traces[4].Present) != 1 || x.Traces[4].DyadicHash != y.Traces[4].DyadicHash || len(y.Traces[4].Decision.Human.Candidates) >= len(x.Traces[4].Decision.Human.Candidates) {
		t.Fatal("attendance ignored in native feasibility")
	}
}

func TestGroupNoImplicitGrantsOrFutureEvidence(t *testing.T) {
	c := fixture(t, 5, false)
	for _, gs := range [][]core.Grant{{}, own(Person(4))[:1], {{Actor: Person(4), Recipient: Person(4), Purpose: "simulation", Operation: core.Export}}} {
		if _, e := core.GroupPerspectiveFor(c.Native.History, "friends", Focus(), Person(4), 33, gs); e == nil {
			t.Fatal("incomplete grants accepted")
		}
	}
	c.Native.At = 5
	p := perspective(t, c, Person(4))
	if len(p.Evidence) != 0 || p.Inclusion.Status != core.Unknown {
		t.Fatal("future observations escaped")
	}
	c = fixture(t, 5, false)
	c.Native.History.Decisions[3].Meta.Rights.Revoked = true
	if _, e := demo.RunGroups(c.Native, nil); e == nil {
		t.Fatal("native consumed revoked decision")
	}
	// Unshared peer-only state cannot change a different observer's choice.
	c = fixture(t, 5, false)
	x, e := demo.RunGroups(c.Native, nil)
	if e != nil {
		t.Fatal(e)
	}
	c.Native.History.Accounts[14].Meta.Rights.Revoked = true
	y, e := demo.RunGroups(c.Native, nil)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(x.Traces[0], y.Traces[0]) {
		t.Fatal("peer-private mutation changed observer")
	}
	if reflect.DeepEqual(x.Traces[4], y.Traces[4]) {
		t.Fatal("own revocation ignored")
	}
	if _, e = demo.RunGroups(c.Native, &x); e == nil {
		t.Fatal("native replay bypassed revocation")
	}
}
func TestGroupDeliverySerializesRevocationAndOwnsOutputs(t *testing.T) {
	c := fixture(t, 5, false)
	l, r, out := helper(t, c)
	entered, release, delivered, revoked := make(chan struct{}), make(chan struct{}), make(chan error, 1), make(chan error, 1)
	go func() {
		_, e := l.VisitGroup(context.Background(), r, func(s assistance.GroupSnapshot) (assistance.GroupResponse, error) {
			close(entered)
			<-release
			return out, nil
		})
		delivered <- e
	}()
	<-entered
	go func() { revoked <- l.Revoke(Person(4), "account:3:4") }()
	select {
	case e := <-revoked:
		t.Fatal("revocation interleaved before held delivery", e)
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	if e := <-delivered; e != nil {
		t.Fatal(e)
	}
	if e := <-revoked; e != nil {
		t.Fatal(e)
	}
	if _, e := l.Host().Execute(context.Background(), r, &out); e == nil {
		t.Fatal("post-revocation replay")
	}
	l, r, out = helper(t, c)
	out.Alternatives[0].Tasks[0].Units = 99
	fresh, e := l.Host().Execute(context.Background(), r, nil)
	if e != nil || fresh.Alternatives[0].Tasks[0].Units == 99 {
		t.Fatal("caller mutated stored answer")
	}
	ctx, cancel := context.WithCancel(context.Background())
	if _, e = l.VisitGroup(ctx, r, func(s assistance.GroupSnapshot) (assistance.GroupResponse, error) { cancel(); return fresh, nil }); e == nil {
		t.Fatal("cancellation during delivery ignored")
	}
}
func TestGroupQuantityUnknownIsNotZero(t *testing.T) {
	for _, q := range []core.GroupQuantity{core.UnknownGroupQuantity(), {Status: core.Censored}, core.ObservedGroupQuantity(0), core.ObservedGroupQuantity(100)} {
		if q.Validate(0, 100) != nil {
			t.Fatal("valid observed/unknown distinction lost")
		}
	}
	for _, q := range []core.GroupQuantity{{Status: core.Observed}, {Status: core.Unknown, Value: core.ObservedGroupQuantity(0).Value}, core.ObservedGroupQuantity(-1), core.ObservedGroupQuantity(101)} {
		if q.Validate(0, 100) == nil {
			t.Fatal("invalid quantity accepted")
		}
	}
}

func TestGroupCorrectedPracticeChangesNativeAppraisal(t *testing.T) {
	c := fixture(t, 5, false)
	before, e := demo.RunGroups(c.Native, nil)
	if e != nil {
		t.Fatal(e)
	}
	a := correction(c, 14, "dispute-current-practice", 33)
	a.NormStance = "disputes"
	c.Native.History.Accounts = append(c.Native.History.Accounts, a)
	after, e := demo.RunGroups(c.Native, nil)
	if e != nil {
		t.Fatal(e)
	}
	if before.Traces[4].DyadicHash != after.Traces[4].DyadicHash || before.Traces[4].Decision.Human.Candidates[1].Consequence == after.Traces[4].Decision.Human.Candidates[1].Consequence {
		t.Fatal("corrected disputed practice did not change native appraisal")
	}
	// Renaming the intention cannot change consequences: repetition, observed
	// participation and self-declared stance carry meaning, not a label lookup.
	renamed := dup(c)
	for i := range renamed.Native.History.Decisions {
		renamed.Native.History.Decisions[i].Intention = "uninterpreted-label"
	}
	for i := range renamed.Native.History.Accounts {
		renamed.Native.History.Accounts[i].Intention = "uninterpreted-label"
	}
	neutral, e := demo.RunGroups(renamed.Native, nil)
	if e != nil {
		t.Fatal(e)
	}
	for i, tr := range after.Traces {
		for j, candidate := range tr.Decision.Human.Candidates {
			if candidate.Consequence != neutral.Traces[i].Decision.Human.Candidates[j].Consequence {
				t.Fatal("intention label scripted outcome")
			}
		}
	}
}

func TestGroupHistorySignalsAttributable(t *testing.T) {
	c := fixture(t, 5, false)
	a, e := demo.RunGroups(c.Native, nil)
	if e != nil {
		t.Fatal(e)
	}
	c = fixture(t, 5, true)
	b, e := demo.RunGroups(c.Native, nil)
	if e != nil {
		t.Fatal(e)
	}
	if a.Traces[4].Signals.Inclusion != 1 || b.Traces[4].Signals.Inclusion != 1.0/3 || b.Traces[4].Signals.Exclusion != 2.0/3 || a.Traces[4].Decision.Human.Candidates[1].Consequence == b.Traces[4].Decision.Human.Candidates[1].Consequence {
		t.Fatal("group history ignored in native signals/choice")
	}
}
func TestGroupPermissionDimensionsAndRoleIsolation(t *testing.T) {
	for _, op := range []core.Operation{core.Read, core.Derive, core.ShareOnRequest} {
		c := fixture(t, 5, false)
		for i := range c.Native.History.Accounts {
			a := &c.Native.History.Accounts[i]
			if a.Member != Person(4) {
				continue
			}
			gs := []core.Grant{}
			for _, g := range a.Meta.Rights.Grants {
				if g.Operation != op {
					gs = append(gs, g)
				}
			}
			a.Meta.Rights.Grants = gs
		}
		_, _, out := helper(t, c)
		if len(out.Perspectives[4].Evidence) != 0 || len(out.Alternatives) != 0 {
			t.Fatal("permission dimension bypass", op)
		}
	}
	c := overlap(fixture(t, 5, false))
	for i := range c.Native.History.Decisions {
		if c.Native.History.Decisions[i].Group == "family" {
			c.Native.History.Decisions[i].Group = "friends"
		}
	}
	for i := range c.Native.History.Accounts {
		if c.Native.History.Accounts[i].Group == "family" {
			c.Native.History.Accounts[i].Group = "friends"
		}
	}
	if !reflect.DeepEqual(perspective(t, fixture(t, 5, false), Person(4)), perspective(t, c, Person(4))) {
		t.Fatal("same-group different role was collapsed")
	}
}
func TestGroupDecisionBusinessIdentityIndependentOfEvidence(t *testing.T) {
	c := fixture(t, 5, false)
	h := c.Native.History
	h.Accounts = nil
	d := dup(h.Decisions[3])
	d.Meta.ID = "distinct-receipt"
	d.Meta.Rights.Resource = d.Meta.ID
	d.ID = "distinct-decision"
	h.Decisions = append(h.Decisions, d)
	if h.Validate() != nil {
		t.Fatal("positive distinct business identity failed")
	}
	h.Decisions[len(h.Decisions)-1].ID = c.Native.Decision
	if h.Validate() == nil {
		t.Fatal("duplicate business identity accepted without downstream accounts")
	}
}
