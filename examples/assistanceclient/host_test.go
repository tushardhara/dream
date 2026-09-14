package assistanceclient

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
)

type planFunc func(context.Context, assistance.Input) (assistance.Result, error)

func (f planFunc) Plan(ctx context.Context, in assistance.Input) (assistance.Result, error) {
	return f(ctx, in)
}
func TestSecondHostAndArms(t *testing.T) {
	if out, e := Run(context.Background()); e != nil || !out.Delivered {
		t.Fatal(out, e)
	}
	for _, arm := range []assistance.Arm{assistance.None, assistance.Simple, assistance.Single, assistance.Multi} {
		t.Run(string(arm), func(t *testing.T) {
			l, r := Fixture(arm, assistance.Coordinate)
			calls := 0
			planner := planFunc(func(ctx context.Context, in assistance.Input) (assistance.Result, error) {
				calls++
				want := 1
				if arm == assistance.Multi {
					want = 2
				}
				if len(in.Context) != want || in.Helper == in.User {
					t.Fatal("wrong identity/perspective", in)
				}
				return (assistance.FakePlanner{}).Plan(ctx, in)
			})
			out, e := l.Host(planner).Execute(context.Background(), r, nil)
			if e != nil {
				t.Fatal(e)
			}
			if arm == assistance.None {
				if out.Delivered || l.Deliveries() != 0 {
					t.Fatal("WAIT delivered")
				}
			} else if !out.Delivered {
				t.Fatal("positive control missing")
			}
			if (arm == assistance.None || arm == assistance.Simple) && calls != 0 {
				t.Fatal("disabled/simple consulted planner")
			}
			before := l.Deliveries()
			again, e := l.Host(planner).Execute(context.Background(), r, &out)
			if e != nil || assistance.Digest(out) != assistance.Digest(again) || l.Deliveries() != before {
				t.Fatal("replay repeated delivery", e)
			}
		})
	}
}
func TestCurrentAccessNormalErrorReplay(t *testing.T) {
	for _, path := range []string{"normal", "error", "replay"} {
		t.Run(path, func(t *testing.T) {
			l, r := Fixture(assistance.Multi, assistance.Coordinate)
			ctx := context.Background()
			original, e := l.Host(assistance.FakePlanner{}).Execute(ctx, r, nil)
			if e != nil {
				t.Fatal(e)
			}
			// New host replays a prior record under its current access state.
			l, r = Fixture(assistance.Multi, assistance.Coordinate)
			scope := r.Contexts[1].Query.Scope
			revoke := func() {
				entries, _ := l.ReadMemory(ctx, scope)
				entries[0].Revoked = true
				entries[0].Content = nil
				l.SetMemory(scope, entries)
			}
			planner := planFunc(func(ctx context.Context, in assistance.Input) (assistance.Result, error) {
				revoke()
				if path == "error" {
					return assistance.Result{}, errors.New("PRIVATE_CANARY")
				}
				return (assistance.FakePlanner{}).Plan(ctx, in)
			})
			var record *assistance.Interaction
			if path == "replay" {
				revoke()
				record = &original
			}
			out, e := l.Host(planner).Execute(ctx, r, record)
			if e == nil || l.Deliveries() != 0 || out.Version != "" || strings.Contains(e.Error(), "PRIVATE_CANARY") {
				t.Fatal("revoked normal/error/replay leaked", out, e)
			}
		})
	}
}
func TestForbiddenSourcesAndModeNeverReachPlanner(t *testing.T) {
	for _, change := range []string{"latent", "future", "labels", "foreign", "mode", "helper", "time"} {
		t.Run(change, func(t *testing.T) {
			l, r := Fixture(assistance.Single, assistance.Coordinate)
			switch change {
			case "latent", "labels", "foreign":
				r.Contexts[0].Sources = []core.ID{core.ID(change)}
			case "future":
				entries, _ := l.ReadMemory(context.Background(), r.Contexts[0].Query.Scope)
				entries[0].Content.Learned[1].At = 100
				l.SetMemory(r.Contexts[0].Query.Scope, entries)
			case "mode":
				r.Contexts[0].Mode = graph.SyntheticSelfDisclosure
			case "helper":
				r.Helper = "alice"
			case "time":
				r.Contexts[0].Query.KnownAt++
			}
			// Registering trusted fixture requests cannot mint nonexistent source rights.
			l.requests = map[core.ID]assistance.Request{}
			_ = l.Register(r)
			called := false
			_, e := l.Host(planFunc(func(context.Context, assistance.Input) (assistance.Result, error) {
				called = true
				return assistance.Result{}, nil
			})).Execute(context.Background(), r, nil)
			if e == nil || called || l.Deliveries() != 0 {
				t.Fatal("forbidden context reached planner", change, e)
			}
		})
	}
}
func TestEligibilityOutputAndReplayBindings(t *testing.T) {
	for _, change := range []string{"refused", "output_source", "output_recipient", "result_version", "unknown_goal", "no_wait", "seed", "arm", "evidence"} {
		t.Run(change, func(t *testing.T) {
			l, r := Fixture(assistance.Single, assistance.Coordinate)
			ctx := context.Background()
			out, e := l.Host(assistance.FakePlanner{}).Execute(ctx, r, nil)
			if e != nil {
				t.Fatal(e)
			}
			l, r = Fixture(assistance.Single, assistance.Coordinate)
			switch change {
			case "refused":
				l.SetAllowed(false)
			case "output_source":
				out.Result.Candidates[1].Evidence = []assistance.EvidenceRef{{Owner: "bob", Source: "private"}}
			case "output_recipient":
				out.Result.Candidates[1].Recipient = "bob"
			case "result_version":
				out.Result.Version = "future"
			case "unknown_goal":
				r.Goal = assistance.Unknown
			case "no_wait":
				out.Result.Candidates = out.Result.Candidates[1:]
				out.Result.Selected = 0
			case "seed":
				r.Seed++
			case "arm":
				r.Arm = assistance.Multi
			case "evidence":
				r.Contexts[0].Sources = []core.ID{"different"}
			}
			_, e = l.Host(assistance.FakePlanner{}).Execute(ctx, r, &out)
			if e == nil || l.Deliveries() != 0 {
				t.Fatal("invalid replay delivered", change)
			}
		})
	}
}
func TestOutageUnknownPauseAndInjection(t *testing.T) {
	ctx := context.Background()
	l, r := Fixture(assistance.Single, assistance.Coordinate)
	out, e := l.Host(planFunc(func(context.Context, assistance.Input) (assistance.Result, error) {
		return assistance.Result{}, errors.New("SECRET_PROVIDER_ERROR")
	})).Execute(ctx, r, nil)
	if e != nil || out.Delivered || out.Result.Candidates[0].Reason != "unavailable" {
		t.Fatal(out, e)
	}
	for _, goal := range []assistance.Goal{assistance.Unknown, assistance.Pause, assistance.Understand} {
		l, r := Fixture(assistance.Simple, goal)
		out, e := l.Host(nil).Execute(ctx, r, nil)
		if e != nil {
			t.Fatal(e)
		}
		selected := out.Result.Candidates[out.Result.Selected]
		if goal == assistance.Unknown && selected.Action != assistance.Clarify || goal != assistance.Unknown && selected.Action != assistance.Wait {
			t.Fatal("assumed reconciliation", out)
		}
	}
	for _, raw := range []string{`{"Version":"assistance.v1","GodState":"CANARY"}`, `{} {}`, strings.Repeat("x", 8193)} {
		if _, e := assistance.DecodeResult([]byte(raw)); e == nil {
			t.Fatal("unknown provider wire accepted")
		}
	}
}
func TestConcurrentRetryOneDeliveryAndOwnedHistory(t *testing.T) {
	l, r := Fixture(assistance.Multi, assistance.Coordinate)
	h := l.Host(assistance.FakePlanner{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := h.Execute(context.Background(), r, nil); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if l.Deliveries() != 1 {
		t.Fatal("duplicate effect", l.Deliveries())
	}
	history, _ := l.Read(context.Background(), r)
	history[0].Result.Candidates[1].Evidence[0].Source = "tampered"
	again, _ := l.Read(context.Background(), r)
	if again[0].Result.Candidates[1].Evidence[0].Source == "tampered" {
		t.Fatal("history alias")
	}
}
func TestOutcomeDoesNotInventBenefit(t *testing.T) {
	base := assistance.Outcome{Version: assistance.Version, Interaction: "help", Participant: "bob", Observer: "bob", Source: "self-report", OccurredAt: 1, LearnedAt: 2, Confidence: .7, Status: core.Unknown, Kind: "observed"}
	if base.Validate() != nil {
		t.Fatal("unknown record invalid")
	}
	value := .5
	base.Benefit = &value
	if base.Validate() == nil {
		t.Fatal("unknown became positive")
	}
	base.Status = core.Observed
	if base.Validate() != nil {
		t.Fatal("observed evidence rejected")
	}
	sender := base
	sender.Observer = "alice"
	sender.Participant = "alice"
	sender.Benefit = &value
	adverse := -.5
	base.Benefit = &adverse
	raw, _ := json.Marshal([]assistance.Outcome{sender, base})
	if !strings.Contains(string(raw), "-0.5") {
		t.Fatal("discordance lost")
	}
	base.LearnedAt = 0
	if base.Validate() == nil {
		t.Fatal("learned before occurred")
	}
}

type beforeCommit struct {
	*Local
	before func()
}

func (j beforeCommit) Commit(ctx context.Context, r assistance.Request, out assistance.Interaction, validate func(context.Context) error) error {
	j.before()
	return j.Local.Commit(ctx, r, out, validate)
}
func TestCommitRechecksEligibilityAndEvidence(t *testing.T) {
	for _, kind := range []string{"eligibility", "evidence", "audit"} {
		t.Run(kind, func(t *testing.T) {
			l, r := Fixture(assistance.Multi, assistance.Coordinate)
			h := l.Host(assistance.FakePlanner{})
			h.Journal = beforeCommit{Local: l, before: func() {
				switch kind {
				case "eligibility":
					l.SetAllowed(false)
				case "evidence":
					s := r.Contexts[1].Query.Scope
					entries, _ := l.ReadMemory(context.Background(), s)
					entries[0].Event.Meta.Rights.Grants = nil
					l.SetMemory(s, entries)
				case "audit":
					l.mu.Lock()
					l.audits = make([]graph.PolicyAudit, 1024)
					l.mu.Unlock()
				}
			}}
			if out, e := h.Execute(context.Background(), r, nil); e == nil || out.Version != "" || l.Deliveries() != 0 {
				t.Fatal("commit bypass", out, e)
			}
			history, _ := l.Read(context.Background(), r)
			if len(history) != 0 {
				t.Fatal("failed commit appended history")
			}
		})
	}
}
