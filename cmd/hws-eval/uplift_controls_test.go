package main

import (
	"context"
	"testing"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/examples/boundaryexperiment"
	"github.com/tushardhara/dream/examples/domainexperiment"
	"github.com/tushardhara/dream/examples/groupexperiment"
	"github.com/tushardhara/dream/examples/listeningclient"
	"github.com/tushardhara/dream/examples/repairclient"
	"github.com/tushardhara/dream/examples/temporalexperiment"
)

// Every consumer's no-assistant arm must be a REAL control: the helper offers
// nothing. Each case also asserts that a candidate arm on the same input does
// offer something, so a control that passes because the whole scenario is inert
// cannot be mistaken for one that withholds.
func TestEveryConsumersNoAssistantArmWithholds(t *testing.T) {
	ctx := context.Background()

	t.Run("boundary", func(t *testing.T) {
		none, e := boundaryexperiment.Run(ctx, "disagreement", assistance.None, nil)
		if e != nil {
			t.Fatal(e)
		}
		for _, h := range none.Helpers {
			if h.Delivered || h.Result.Candidates[h.Result.Selected].Action != assistance.Wait {
				t.Fatalf("the no-assistant arm delivered %v", h.Result.Candidates[h.Result.Selected].Action)
			}
		}
		multi, e := boundaryexperiment.Run(ctx, "disagreement", assistance.Multi, nil)
		if e != nil {
			t.Fatal(e)
		}
		delivered := false
		for _, h := range multi.Helpers {
			delivered = delivered || h.Delivered
		}
		if !delivered {
			t.Fatal("positive control: no arm delivers anything in this scenario")
		}
	})

	t.Run("domain", func(t *testing.T) {
		focus := core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: core.Childcare, RoleContext: "family"}
		none, e := domainexperiment.Run(ctx, focus, assistance.None, nil)
		if e != nil {
			t.Fatal(e)
		}
		if none.Helper.Delivered {
			t.Fatal("the no-assistant arm delivered an intervention")
		}
		multi, e := domainexperiment.Run(ctx, focus, assistance.Multi, nil)
		if e != nil {
			t.Fatal(e)
		}
		if !multi.Helper.Delivered {
			t.Fatal("positive control: no arm delivers anything in this scenario")
		}
	})

	t.Run("temporal", func(t *testing.T) {
		scene := temporalexperiment.Scenes()[1] // daily_60: an arm does deliver here
		none, e := temporalexperiment.Run(ctx, scene, "none", 30, 11, "", nil)
		if e != nil {
			t.Fatal(e)
		}
		if none.Helper.Delivered {
			t.Fatal("the no-assistant arm delivered an intervention")
		}
		simple, e := temporalexperiment.Run(ctx, scene, "simple", 30, 11, "", nil)
		if e != nil {
			t.Fatal(e)
		}
		if !simple.Helper.Delivered {
			t.Fatal("positive control: no arm delivers anything in this scene")
		}
	})

	t.Run("group", func(t *testing.T) {
		c, e := groupexperiment.Fixture(5, 11, false, 4)
		if e != nil {
			t.Fatal(e)
		}
		none, e := groupexperiment.RunArm(ctx, c, assistance.None)
		if e != nil {
			t.Fatal(e)
		}
		if len(none.Helper.Perspectives) != 0 || len(none.Helper.Alternatives) != 0 || none.Helper.Next != "WAIT" {
			t.Fatalf("the no-assistant arm offered something: %d perspective(s), %d alternative(s), next %q",
				len(none.Helper.Perspectives), len(none.Helper.Alternatives), none.Helper.Next)
		}
		// Each candidate arm must differ in WHAT it looks at, not only in that
		// it looks: simple takes no perspective, single takes the asking user's
		// own, multi takes every affected member's. Without this a relabelling
		// would pass as three arms.
		counts := map[assistance.Arm]int{}
		for _, a := range []assistance.Arm{assistance.Simple, assistance.Single, assistance.Multi} {
			rep, e := groupexperiment.RunArm(ctx, c, a)
			if e != nil {
				t.Fatal(e)
			}
			counts[a] = len(rep.Helper.Perspectives)
			if len(rep.Helper.Alternatives) == 0 {
				t.Fatalf("arm %s offered no alternatives", a)
			}
		}
		if counts[assistance.Simple] != 0 {
			t.Fatalf("simple assistance took %d perspective(s)", counts[assistance.Simple])
		}
		if counts[assistance.Single] != 1 {
			t.Fatalf("single perspective took %d perspective(s)", counts[assistance.Single])
		}
		if counts[assistance.Multi] <= counts[assistance.Single] {
			t.Fatalf("multi perspective took %d, no more than single's %d",
				counts[assistance.Multi], counts[assistance.Single])
		}
	})

	t.Run("repair", func(t *testing.T) {
		_, none, e := repairclient.ScenarioArm(ctx, true, assistance.None)
		if e != nil {
			t.Fatal(e)
		}
		for _, p := range none.Periods {
			if len(p.Perspectives) != 0 || p.Next != "WAIT" || p.Expectation != "unresolved" {
				t.Fatalf("the no-assistant arm reported %d perspective(s), next %q, expectation %q",
					len(p.Perspectives), p.Next, p.Expectation)
			}
		}
		counts := map[assistance.Arm]int{}
		for _, a := range []assistance.Arm{assistance.Simple, assistance.Single, assistance.Multi} {
			_, rep, e := repairclient.ScenarioArm(ctx, true, a)
			if e != nil {
				t.Fatal(e)
			}
			last := rep.Periods[len(rep.Periods)-1]
			counts[a] = len(last.Perspectives)
			if last.Expectation == "unresolved" {
				t.Fatalf("arm %s reported nothing about the history", a)
			}
		}
		if counts[assistance.Simple] != 0 {
			t.Fatalf("simple assistance reported %d perspective(s)", counts[assistance.Simple])
		}
		if counts[assistance.Single] != 1 {
			t.Fatalf("single perspective reported %d perspective(s)", counts[assistance.Single])
		}
		if counts[assistance.Multi] <= counts[assistance.Single] {
			t.Fatalf("multi perspective reported %d, no more than single's %d",
				counts[assistance.Multi], counts[assistance.Single])
		}
	})

	t.Run("listening", func(t *testing.T) {
		none, e := listeningclient.RunArm(ctx, assistance.None)
		if e != nil {
			t.Fatal(e)
		}
		for who, r := range none {
			if r.Next != "WAIT" || r.Own != nil || len(r.Shared) != 0 {
				t.Fatalf("the no-assistant arm answered %s with next %q, own=%v, %d shared",
					who, r.Next, r.Own != nil, len(r.Shared))
			}
		}
		multi, e := listeningclient.RunArm(ctx, assistance.Multi)
		if e != nil {
			t.Fatal(e)
		}
		for who, r := range multi {
			if r.Own == nil {
				t.Fatalf("positive control: %s got nothing back in any arm", who)
			}
		}
	})
}

// The listening flow's arm/version agreement, exercised on a request the client
// actually builds. An arm on a v1 request would be ignored by the v1 path and a
// v2 request with no arm has no control to enforce; either way an evaluation
// ends up with several labels for one policy.
func TestListeningArmAndVersionMustAgree(t *testing.T) {
	l, e := listeningclient.Budget()
	if e != nil {
		t.Fatal(e)
	}
	focus := listeningclient.Account("alice", "bob").Focus
	v1 := l.Request("alice", "alice-listen", "private", focus, 4, false, false)
	if e := v1.Validate(); e != nil {
		t.Fatal("positive control: a v1 listening request was rejected:", e)
	}
	v2 := l.ArmedRequest("alice", "alice-listen", "private", focus, 4, false, false, assistance.Single)
	if e := v2.Validate(); e != nil {
		t.Fatal("positive control: a valid v2 listening request was rejected:", e)
	}
	smuggled := v1
	smuggled.Arm = assistance.Multi
	if smuggled.Validate() == nil {
		t.Fatal("a v1 listening request carried an arm that the v1 path would ignore")
	}
	bare := v2
	bare.Arm = ""
	if bare.Validate() == nil {
		t.Fatal("a v2 listening request with no arm was accepted")
	}
}
