package ordinaryclient

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
	"strings"
	"testing"
)

func fixture(t *testing.T, k core.OrdinaryKind) (*Local, assistance.OrdinaryRequest) {
	t.Helper()
	l, r, e := Fixture(k)
	if e != nil {
		t.Fatal(e)
	}
	return l, r
}
func execute(t *testing.T, l *Local, r assistance.OrdinaryRequest) assistance.OrdinaryResponse {
	t.Helper()
	o, e := l.Host().Execute(context.Background(), r, nil)
	if e != nil {
		t.Fatal(e)
	}
	return o
}
func TestOrdinaryPrimitivesAndExactAuthoredWords(t *testing.T) {
	for _, kind := range []core.OrdinaryKind{core.OrdinaryQuiet, core.OrdinaryAppreciation, core.OrdinaryMemory, core.OrdinaryActivity, core.OrdinaryCoordination} {
		t.Run(string(kind), func(t *testing.T) {
			l, r := fixture(t, kind)
			o := execute(t, l, r)
			if kind == core.OrdinaryQuiet {
				if o.Action != "WAIT" {
					t.Fatal("quiet relationship prompted")
				}
			} else if o.Action != string(kind) {
				t.Fatal("supported primitive missing", o)
			}
			if kind == core.OrdinaryAppreciation || kind == core.OrdinaryMemory {
				story := l.stories[key{"b", r.Activity, kind}]
				if o.Quote == nil || o.Quote.Author != "b" || o.Quote.Source != story.Source || o.Quote.Words != story.Words {
					t.Fatal("words fabricated/misattributed")
				}
			}
			if o.GlobalWelfare != "NOT_AGGREGATED" {
				t.Fatal("welfare score")
			}
			if len(l.Reservations()) != 0 {
				t.Fatal("proposal allocated resources")
			}
			if xs, _ := l.Experiences("a"); len(xs) != 0 {
				t.Fatal("invitation became welcome")
			}
			if _, e := l.Host().Execute(context.Background(), r, &o); e != nil {
				t.Fatal("exact replay", e)
			}
		})
	}
}
func TestOrdinaryMissingBenefitTimingAndCapacityWait(t *testing.T) {
	cases := map[string]func(*Local, *assistance.OrdinaryRequest){
		"unknown benefit": func(l *Local, r *assistance.OrdinaryRequest) {
			changePreference(t, l, *r, "b", func(p *core.OrdinaryPreference) { p.ExpectedBenefit = core.UnknownGroupQuantity() })
		},
		"nonpositive benefit": func(l *Local, r *assistance.OrdinaryRequest) {
			changePreference(t, l, *r, "b", func(p *core.OrdinaryPreference) { p.ExpectedBenefit = core.ObservedGroupQuantity(0) })
		},
		"busy": func(l *Local, r *assistance.OrdinaryRequest) {
			changePreference(t, l, *r, "b", func(p *core.OrdinaryPreference) { p.Availability = "busy" })
		},
		"unknown capacity": func(l *Local, r *assistance.OrdinaryRequest) {
			changePreference(t, l, *r, "b", func(p *core.OrdinaryPreference) { p.MaxEffort = core.UnknownGroupQuantity() })
		},
		"effort too high": func(l *Local, r *assistance.OrdinaryRequest) {
			changePreference(t, l, *r, "b", func(p *core.OrdinaryPreference) { p.MaxEffort = core.ObservedGroupQuantity(0) })
		},
		"preference window": func(l *Local, r *assistance.OrdinaryRequest) {
			changePreference(t, l, *r, "b", func(p *core.OrdinaryPreference) { end := core.LogicalTime(3); p.Window.End = &end })
		},
		"declined": func(l *Local, r *assistance.OrdinaryRequest) {
			changePreference(t, l, *r, "b", func(p *core.OrdinaryPreference) { p.Choice = "declined" })
		},
		"missing physical capacity": func(l *Local, r *assistance.OrdinaryRequest) {
			delete(l.budget.Personal, "b")
			l.budget.Personal["c"] = 4
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			l, r := fixture(t, core.OrdinaryActivity)
			change(l, &r)
			r.ID = "fresh"
			var e error
			r, e = l.Request("a", r, "")
			if e != nil {
				t.Fatal(e)
			}
			if o := execute(t, l, r); o.Action != "WAIT" {
				t.Fatal("unsupported benefit/effort proposed", o)
			}
		})
	}
}
func changePreference(t *testing.T, l *Local, r assistance.OrdinaryRequest, person core.ID, change func(*core.OrdinaryPreference)) {
	t.Helper()
	p := copyValue(l.preferences[key{person, r.Activity, r.Kind}])
	p.Corrects = p.Source
	p.Source = core.ID("corrected:" + string(person))
	change(&p)
	if e := l.PutPreference(person, p, []core.ID{"a"}, r.At); e != nil {
		t.Fatal(e)
	}
}
func TestOrdinaryPermissionedStoryAndReplay(t *testing.T) {
	for _, variant := range []string{"private", "revoked", "read", "derive", "share", "foreign-reporter", "ai-origin"} {
		t.Run(variant, func(t *testing.T) {
			l, r := fixture(t, core.OrdinaryMemory)
			base := execute(t, l, r)
			scope := r.Story.Query.Scope
			e := &l.entries[scope][len(l.entries[scope])-1]
			switch variant {
			case "private":
				e.Event.Meta.Sensitivity = core.Restricted
			case "revoked":
				if err := l.Revoke("b", "story"); err != nil {
					t.Fatal(err)
				}
			case "foreign-reporter":
				e.Event.Meta.Source = "a"
			case "ai-origin":
				s := l.stories[key{"b", r.Activity, r.Kind}]
				s.Origin = "ai_generated"
				raw, _ := json.Marshal(s)
				e.Content.Text = string(raw)
			default:
				gs := []core.Grant{}
				for _, g := range e.Event.Meta.Rights.Grants {
					if string(g.Operation) != variant {
						gs = append(gs, g)
					}
				}
				if variant == "share" {
					gs = nil
					for _, g := range e.Event.Meta.Rights.Grants {
						if g.Operation != core.ShareOnRequest {
							gs = append(gs, g)
						}
					}
				}
				e.Event.Meta.Rights.Grants = gs
			}
			if _, err := l.Host().Execute(context.Background(), r, &base); err == nil {
				t.Fatal("stale story replay allowed")
			}
			r.ID = "fresh"
			var err error
			r, err = l.Request("a", r, "b")
			if err != nil {
				t.Fatal(err)
			}
			out, err := l.Host().Execute(context.Background(), r, nil)
			if err == nil && out.Action != "WAIT" {
				t.Fatal("unpermitted story delivered")
			}
			raw, _ := json.Marshal(out)
			if strings.Contains(string(raw), "chess") {
				t.Fatal("private words leaked")
			}
		})
	}
}
func TestOrdinaryCoordinationReducesDefinedBurdenWithoutOffload(t *testing.T) {
	l, r := fixture(t, core.OrdinaryCoordination)
	out := execute(t, l, r)
	if out.PlanOption != "slot" {
		t.Fatal("no slot plan")
	}
	gs := []core.Grant{{Actor: "a", Recipient: "a", Purpose: "help", Operation: core.Read}, {Actor: "a", Recipient: "a", Purpose: "help", Operation: core.Derive}}
	manual, e := core.ReserveGroup(l.groups, nil, l.budget, r.PlanDecision, "manual", "manual-receipt", "a", r.At, gs)
	if e != nil {
		t.Fatal(e)
	}
	if e = l.Reserve(context.Background(), "a", r.ID, "slot-receipt"); e != nil {
		t.Fatal(e)
	}
	actual := l.Reservations()
	if len(actual) != 1 || manual[0].Units-actual[0].Units != 2 || assistance.Digest(manual[0].Tasks) != assistance.Digest(actual[0].Tasks) {
		t.Fatal("no real coordination saving or tasks offloaded")
	}
	if e = l.Reserve(context.Background(), "a", r.ID, "another-receipt"); e == nil {
		t.Fatal("duplicate execution")
	}
	if _, e = l.Host().Execute(context.Background(), r, &out); e == nil {
		t.Fatal("stale pre-reservation proposal replay")
	}
	l, r = fixture(t, core.OrdinaryCoordination)
	r.Requested = false
	r.ID = "not-requested"
	r, e = l.Request("a", r, "")
	if e != nil {
		t.Fatal(e)
	}
	if o := execute(t, l, r); o.Action != "WAIT" {
		t.Fatal("unsolicited plan")
	}
}
func TestOrdinaryBudgetDismissalAndLaterExperience(t *testing.T) {
	l, r := fixture(t, core.OrdinaryActivity)
	for i, at := range []core.LogicalTime{1, 2, 12, 23, 45} {
		q := copyValue(r)
		q.ID = core.ID(fmt.Sprintf("op:%d", i))
		q.At = at
		end := at + 8
		q.Window = core.Interval{Start: at + 1, End: &end}
		var e error
		q, e = l.Request("a", q, "")
		if e != nil {
			t.Fatal(e)
		}
		o := execute(t, l, q)
		want := []string{string(core.OrdinaryActivity), "WAIT", string(core.OrdinaryActivity), "WAIT", string(core.OrdinaryActivity)}[i]
		if o.Action != want {
			t.Fatal("budget/cooldown", at, o)
		}
		if _, e = l.Host().Execute(context.Background(), q, &o); e != nil {
			t.Fatal("idempotent budget", e)
		}
		if o.Action != "WAIT" {
			for _, person := range []core.ID{"a", "b"} {
				x := core.OrdinaryExperience{Version: core.OrdinaryVersion, ID: core.ID(fmt.Sprintf("welcome:%d:%s", i, person)), Opportunity: q.ID, Participant: person, Observer: person, Source: person, OccurredAt: at + 1, LearnedAt: at + 1, Participation: "welcomed", Benefit: core.UnknownGroupQuantity(), BurdenReduction: core.UnknownGroupQuantity()}
				if e = l.Observe(person, x); e != nil {
					t.Fatal(e)
				}
			}
		}

	}
	l, r = fixture(t, core.OrdinaryActivity)
	_ = execute(t, l, r)
	for _, at := range []core.LogicalTime{2, 3} {
		if e := l.AppendBoundary("b", Boundary("b", "a", r.Activity, core.Dismissed, at)); e != nil {
			t.Fatal(e)
		}
	}
	r.ID = "after-dismissal"
	r.At = 20
	end := core.LogicalTime(29)
	r.Window = core.Interval{Start: 21, End: &end}
	r, e := l.Request("a", r, "")
	if e != nil {
		t.Fatal(e)
	}
	if o := execute(t, l, r); o.Action != "WAIT" || o.Reason != "boundary" {
		t.Fatal("dismissal ignored")
	}
	x := core.OrdinaryExperience{Version: core.OrdinaryVersion, ID: "later-report", Opportunity: r.ID, Participant: "b", Observer: "b", Source: "b", OccurredAt: 21, LearnedAt: 21, Participation: "declined", Benefit: core.UnknownGroupQuantity(), BurdenReduction: core.UnknownGroupQuantity()}
	if e = l.Observe("a", x); e == nil {
		t.Fatal("other participant manufactured welcome")
	}
	if e = l.Observe("b", x); e != nil {
		t.Fatal(e)
	}
	xs, e := l.Experiences("b")
	if e != nil || len(xs) != 1 || xs[0].Participation != "declined" || xs[0].Benefit.Value != nil {
		t.Fatal("decline penalty/invented effect")
	}
	if e = l.Observe("b", x); e == nil {
		t.Fatal("duplicate later report")
	}
	x.ID = "duplicate-business-key"
	if l.Observe("b", x) == nil {
		t.Fatal("same opportunity report repeated with new ID")
	}
}

func TestOrdinaryRepeatedNonparticipationPauses(t *testing.T) {
	l, r := fixture(t, core.OrdinaryActivity)
	for i, at := range []core.LogicalTime{1, 12, 45} {
		q := copyValue(r)
		q.ID = core.ID(fmt.Sprintf("silent:%d", i))
		q.At = at
		end := at + 8
		q.Window = core.Interval{Start: at + 1, End: &end}
		var e error
		q, e = l.Request("a", q, "")
		if e != nil {
			t.Fatal(e)
		}
		o := execute(t, l, q)
		if i < 2 && o.Action == "WAIT" {
			t.Fatal("positive opportunity missing")
		}
		if i == 2 && (o.Action != "WAIT" || o.Reason != "non_participation_pause") {
			t.Fatal("silence became endless prompts", o)
		}
	}
	// Fresh explicit willingness from BOTH people can reopen an ordinary activity.
	r.At = 46
	for _, person := range []core.ID{"a", "b"} {
		changePreference(t, l, r, person, func(p *core.OrdinaryPreference) { p.Choice = "wanted" })
	}
	r.ID = "fresh-willingness"
	end := core.LogicalTime(55)
	r.Window = core.Interval{Start: 47, End: &end}
	var e error
	r, e = l.Request("a", r, "")
	if e != nil {
		t.Fatal(e)
	}
	if o := execute(t, l, r); o.Action == "WAIT" {
		t.Fatal("explicit fresh preference did not reopen", o)
	}
}
