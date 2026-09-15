package ordinaryclient

import (
	"context"
	"encoding/json"
	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
	"strings"
	"testing"
)

func TestOrdinaryCoreGuards(t *testing.T) {
	end := core.LogicalTime(9)
	pref := core.OrdinaryPreference{Version: core.OrdinaryVersion, Source: "pref", Person: "a", Activity: "game", Kind: core.OrdinaryActivity, Window: core.Interval{End: &end}, Choice: "wanted", Availability: "available", ExpectedBenefit: core.ObservedGroupQuantity(.5), MaxEffort: core.ObservedGroupQuantity(2)}
	story := core.OrdinaryStory{Version: core.OrdinaryVersion, Source: "story", Author: "b", Activity: "game", Kind: core.OrdinaryMemory, Origin: "participant_authored", Words: "Chess & tea — \"again\"."}
	experience := core.OrdinaryExperience{Version: core.OrdinaryVersion, ID: "experience", Opportunity: "opportunity", Participant: "a", Observer: "a", Source: "a", OccurredAt: 2, LearnedAt: 3, Participation: "unknown", Benefit: core.UnknownGroupQuantity(), BurdenReduction: core.UnknownGroupQuantity()}
	if pref.Validate() != nil || story.Validate() != nil || experience.Validate() != nil {
		t.Fatal("invalid positive fixture")
	}
	cases := []struct {
		name, want string
		change     func() error
	}{
		{"preference_identity", "invalid ordinary preference identity/window", func() error { p := pref; p.Source = ""; return p.Validate() }},
		{"preference_window", "invalid ordinary preference identity/window", func() error { p := pref; p.Window.End = nil; return p.Validate() }},
		{"preference_choice", "invalid declared ordinary preference", func() error { p := pref; p.Choice = "engaged"; return p.Validate() }},
		{"availability", "invalid reported availability", func() error { p := pref; p.Availability = "guessed"; return p.Validate() }},
		{"expected_benefit", "invalid ordinary expected benefit/effort", func() error { p := pref; p.ExpectedBenefit = core.ObservedGroupQuantity(2); return p.Validate() }},
		{"effort", "invalid ordinary expected benefit/effort", func() error { p := pref; p.MaxEffort = core.ObservedGroupQuantity(-1); return p.Validate() }},
		{"preference_correction", "invalid ordinary preference correction", func() error { p := pref; p.Corrects = p.Source; return p.Validate() }},
		{"story_identity", "invalid ordinary story identity/kind", func() error { s := story; s.Author = ""; return s.Validate() }},
		{"story_kind", "invalid ordinary story identity/kind", func() error { s := story; s.Kind = core.OrdinaryActivity; return s.Validate() }},
		{"story_origin", "ordinary words require participant authorship", func() error { s := story; s.Origin = "ai_generated"; return s.Validate() }},
		{"story_words", "invalid authored ordinary words", func() error { s := story; s.Words = " "; return s.Validate() }},
		{"story_correction", "invalid ordinary story correction", func() error { s := story; s.Corrects = s.Source; return s.Validate() }},
		{"experience_authorship", "invalid ordinary experience attribution/time", func() error { e := experience; e.Observer = "b"; return e.Validate() }},
		{"experience_source", "invalid ordinary experience attribution/time", func() error { e := experience; e.Source = "b"; return e.Validate() }},
		{"experience_time", "invalid ordinary experience attribution/time", func() error { e := experience; e.LearnedAt = 1; return e.Validate() }},
		{"experience_participation", "invalid observed ordinary participation", func() error { e := experience; e.Participation = "replied"; return e.Validate() }},
		{"experience_benefit", "invalid ordinary experience quantity", func() error { e := experience; e.Benefit = core.ObservedGroupQuantity(2); return e.Validate() }},
		{"experience_burden", "invalid ordinary experience quantity", func() error {
			e := experience
			e.BurdenReduction = core.ObservedGroupQuantity(101)
			return e.Validate()
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := c.change()
			if e == nil || e.Error() != c.want {
				t.Fatal("missing attributable guard", e)
			}
		})
	}
	raw, e := core.EncodeOrdinaryStory(story)
	if e != nil {
		t.Fatal(e)
	}
	got, e := core.DecodeOrdinaryStory(raw)
	if e != nil || assistance.Digest(got) != assistance.Digest(story) {
		t.Fatal("exact story codec", e)
	}
	for _, bad := range [][]byte{append(append([]byte{}, raw...), []byte(" {}")...), []byte(strings.Replace(string(raw), "{", "{\"extra\":1,", 1)), []byte(strings.Repeat(" ", 2049))} {
		if _, e = core.DecodeOrdinaryStory(bad); e == nil {
			t.Fatal("loose story decoding")
		}
	}
}
func TestOrdinaryAuthorityCorrectionAndAncestors(t *testing.T) {
	t.Run("author_binding", func(t *testing.T) {
		l, r := fixture(t, core.OrdinaryMemory)
		s := l.stories[key{"b", r.Activity, r.Kind}]
		s.Corrects = s.Source
		s.Source = "new-story"
		if l.PutStory("a", s, core.Public, []core.ID{"a"}, nil, r.At) == nil {
			t.Fatal("forged author")
		}
		s.Origin = "ai_generated"
		if l.PutStory("b", s, core.Public, []core.ID{"a"}, nil, r.At) == nil {
			t.Fatal("AI attributed")
		}
	})
	t.Run("current_correction", func(t *testing.T) {
		l, r := fixture(t, core.OrdinaryMemory)
		old := execute(t, l, r)
		s := l.stories[key{"b", r.Activity, r.Kind}]
		s.Corrects = s.Source
		s.Source = "corrected-story"
		s.Words = "Actually, we played Go."
		if e := l.PutStory("b", s, core.Public, []core.ID{"a"}, nil, r.At); e != nil {
			t.Fatal(e)
		}
		if _, e := l.Host().Execute(context.Background(), r, &old); e == nil {
			t.Fatal("superseded words replayed")
		}
		r.ID = "fresh"
		r.At = 12
		end := core.LogicalTime(20)
		r.Window = core.Interval{Start: 13, End: &end}
		r, e := l.Request("a", r, "b")
		if e != nil {
			t.Fatal(e)
		}
		o := execute(t, l, r)
		if o.Quote == nil || o.Quote.Words != s.Words {
			t.Fatal("correction missing")
		}
	})
	for _, variant := range []string{"private_parent", "revoked_parent"} {
		t.Run(variant, func(t *testing.T) {
			l, r := fixture(t, core.OrdinaryMemory)
			scope := r.Story.Query.Scope
			entries := l.entries[scope]
			parent := entries[0].Event.Meta.ID
			entries[len(entries)-1].Event.Meta.Parents = []core.ID{parent}
			if variant == "private_parent" {
				entries[0].Event.Meta.Sensitivity = core.Restricted
			} else {
				entries[0].Revoked = true
				entries[0].Content = nil
			}
			l.entries[scope] = entries
			o, e := l.Host().Execute(context.Background(), r, nil)
			if e == nil && o.Action != "WAIT" {
				t.Fatal("ancestor permission leak")
			}
			raw, _ := json.Marshal(o)
			if strings.Contains(string(raw), "chess") {
				t.Fatal("private ancestor words leaked")
			}
		})
	}
	t.Run("unregistered_context", func(t *testing.T) {
		l, r := fixture(t, core.OrdinaryMemory)
		p := r.Preferences[0]
		p.Binding = "forged"
		_, d, e := l.Host().Policy.Approve(context.Background(), p)
		if e == nil && d.Allowed {
			t.Fatal("blanket authority")
		}
		r.Preferences[0].Sources = []core.ID{"story"}
		if _, e = l.Host().Execute(context.Background(), r, nil); e == nil {
			t.Fatal("changed registered request")
		}
	})
	t.Run("stale_time", func(t *testing.T) {
		l, r := fixture(t, core.OrdinaryActivity)
		q := r
		q.ID = "future"
		q.At = 2
		if _, e := l.Request("a", q, ""); e != nil {
			t.Fatal(e)
		}
		if _, e := l.Host().Execute(context.Background(), r, nil); e == nil {
			t.Fatal("stale request clock")
		}
	})
}
func TestOrdinaryPlanCannotUnderstateEffort(t *testing.T) {
	l, r := fixture(t, core.OrdinaryCoordination)
	r.ID = "understated"
	r.Effort["b"] = 0
	r, e := l.Request("a", r, "")
	if e != nil {
		t.Fatal(e)
	}
	if o := execute(t, l, r); o.Action != "WAIT" {
		t.Fatal("plan task load understated", o)
	}
	if l.Reserve(context.Background(), "a", r.ID, "reservation") == nil {
		t.Fatal("understated plan allocated")
	}
}

type barrierJournal struct {
	l       *Local
	entered chan struct{}
	release chan struct{}
	cancel  context.CancelFunc
}

func (j barrierJournal) VisitOrdinary(ctx context.Context, r assistance.OrdinaryRequest, fn func(context.Context, assistance.OrdinarySnapshot) (assistance.OrdinaryResponse, error)) (assistance.OrdinaryResponse, error) {
	return j.l.VisitOrdinary(ctx, r, func(tx context.Context, s assistance.OrdinarySnapshot) (assistance.OrdinaryResponse, error) {
		close(j.entered)
		<-j.release
		if j.cancel != nil {
			j.cancel()
		}
		return fn(tx, s)
	})
}
func TestOrdinaryAtomicRevocationAndCancellation(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "revocation", true: "cancellation"}[cancelled], func(t *testing.T) {
			l, r := fixture(t, core.OrdinaryMemory)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			j := barrierJournal{l: l, entered: make(chan struct{}), release: make(chan struct{})}
			if cancelled {
				j.cancel = cancel
			}
			h := l.Host()
			h.Journal = j
			done := make(chan error, 1)
			go func() { _, e := h.Execute(ctx, r, nil); done <- e }()
			<-j.entered
			revoked := make(chan error, 1)
			started := make(chan struct{})
			go func() { close(started); revoked <- l.Revoke("b", "story") }()
			<-started
			close(j.release)
			e := <-done
			if cancelled && e == nil || !cancelled && e != nil {
				t.Fatal("delivery ordering", e)
			}
			if e = <-revoked; e != nil {
				t.Fatal(e)
			}
			if cancelled && len(l.responses) != 0 {
				t.Fatal("cancelled response committed")
			}
			if !cancelled {
				if _, e = l.Host().Execute(context.Background(), r, nil); e == nil {
					t.Fatal("post-revocation replay")
				}
			}

		})
	}
}

func TestOrdinaryIndependentObservedBurden(t *testing.T) {
	l, r := fixture(t, core.OrdinaryCoordination)
	execute(t, l, r)
	x := core.OrdinaryExperience{Version: core.OrdinaryVersion, ID: "self-burden", Opportunity: r.ID, Participant: "b", Observer: "b", Source: "b", OccurredAt: 2, LearnedAt: 2, Participation: "unknown", Benefit: core.UnknownGroupQuantity(), BurdenReduction: core.ObservedGroupQuantity(-.5)}
	if e := l.Observe("b", x); e != nil {
		t.Fatal(e)
	}
	xs, e := l.Experiences("b")
	if e != nil || len(xs) != 1 || xs[0].Participation != "unknown" || xs[0].Benefit.Value != nil || *xs[0].BurdenReduction.Value != -.5 {
		t.Fatal("burden conflated with welcome", xs, e)
	}
	as, _ := l.Experiences("a")
	if len(as) != 0 {
		t.Fatal("another participant's report leaked")
	}
}

func TestOrdinaryOverlappingGroupDuties(t *testing.T) {
	for _, reserved := range []bool{false, true} {
		l, r := fixture(t, core.OrdinaryActivity)
		r.ID = "effort-four"
		r.Effort["a"] = 4
		r.Effort["b"] = 4
		if reserved {
			gs := []core.Grant{{Actor: "a", Recipient: "a", Purpose: "help", Operation: core.Read}, {Actor: "a", Recipient: "a", Purpose: "help", Operation: core.Derive}}
			rs, e := core.ReserveGroup(l.groups, nil, l.budget, "ordinary-plan", "manual", "existing-care", "a", r.At, gs)
			if e != nil {
				t.Fatal(e)
			}
			l.reservations = rs
		}
		q, e := l.Request("a", r, "")
		if e != nil {
			t.Fatal(e)
		}
		o := execute(t, l, q)
		if (o.Action == "WAIT") != reserved {
			t.Fatal("overlapping care duties ignored", reserved, o)
		}
	}
}

func TestOrdinaryExplicitUnwelcomePauses(t *testing.T) {
	l, r := fixture(t, core.OrdinaryActivity)
	execute(t, l, r)
	x := core.OrdinaryExperience{Version: core.OrdinaryVersion, ID: "unwelcome", Opportunity: r.ID, Participant: "b", Observer: "b", Source: "b", OccurredAt: 2, LearnedAt: 2, Participation: "unwelcome", Benefit: core.ObservedGroupQuantity(-.2), BurdenReduction: core.UnknownGroupQuantity()}
	if e := l.Observe("b", x); e != nil {
		t.Fatal(e)
	}
	r.ID = "after-unwelcome"
	r.At = 20
	end := core.LogicalTime(28)
	r.Window = core.Interval{Start: 21, End: &end}
	q, e := l.Request("a", r, "")
	if e != nil {
		t.Fatal(e)
	}
	if o := execute(t, l, q); o.Action != "WAIT" || o.Reason != "non_participation_pause" {
		t.Fatal("explicit negative ignored", o)
	}
}
