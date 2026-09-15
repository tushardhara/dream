package listeningclient

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/tushardhara/dream/adapters/model"
	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func budget(t *testing.T) *Local {
	t.Helper()
	l, err := Budget()
	must(t, err)
	return l
}
func execute(t *testing.T, l *Local, r assistance.ListeningRequest, code string) assistance.ListeningResponse {
	t.Helper()
	must(t, l.Register(r.User, r))
	interpreter, err := RecordedFor(context.Background(), l, r, code)
	must(t, err)
	out, err := l.Host(interpreter).Execute(context.Background(), r)
	must(t, err)
	return out
}
func request(l *Local, actor, id core.ID, at core.LogicalTime, mode string, share bool) assistance.ListeningRequest {
	return l.Request(actor, id, mode, Account(actor, l.other(actor)).Focus, at, share, false)
}

func TestBudgetFightKeepsBothAccountsAndDifferentGoals(t *testing.T) {
	l := budget(t)
	a := execute(t, l, request(l, "alice", "alice-turn", 4, "joint", true), "supported")
	b := execute(t, l, request(l, "bob", "bob-turn", 4, "joint", true), "supported")
	if a.Next != "acknowledge" || b.Next != "offer_plan" || a.Own.DesiredHelp != "listen" || b.Own.DesiredHelp != "coordinate" {
		t.Fatal("different goals collapsed", a, b)
	}
	if len(l.records) != 2 || len(l.records[0].Accounts) != 2 || len(l.records[0].Differences) != 2 || l.records[0].Differences[0].Kind != "different_observation_reports" || l.records[0].Differences[1].Kind != "different_values" {
		t.Fatal("accounts or factual/value differences collapsed", l.records)
	}
	if len(l.records[0].FactualSupport) != 2 {
		t.Fatal("factual support assessment missing")
	}
	for _, fact := range l.records[0].FactualSupport {
		if fact.Status != "participant_report_only" || fact.Key != "budget_limit" {
			t.Fatal("self-report corroborated a fact or adjudicated a value", fact)
		}
	}
	if len(a.Shared) != 2 || a.Shared[1].Words != "I want a practical plan for tomorrow." || a.Shared[1].Speaker != "bob" {
		t.Fatal("summary invented or lost attributed words", a.Shared)
	}
	raw, _ := json.Marshal(a)
	if strings.Contains(string(raw), "150") || strings.Contains(string(raw), "bob-account") || strings.Contains(string(raw), "I value flexibility") || strings.Contains(string(raw), "winner") {
		t.Fatal("private partner account or adjudication leaked", string(raw))
	}
	if a.PartnerState != "independently_joined" || b.PartnerState != "independently_joined" {
		t.Fatal("independent participation lost")
	}
	cli, err := Run(context.Background())
	must(t, err)
	if cli["alice"].Next != a.Next || cli["bob"].Next != b.Next {
		t.Fatal("actual second-host path differs")
	}
}

func privateFixture(t *testing.T, a core.ListeningAccount) *Local {
	t.Helper()
	l := New("private", "helper", "alice", "bob")
	must(t, l.PutAccount(a.Speaker, a, 1))
	must(t, l.AppendBoundary("alice", Boundary("alice", "bob", core.PrivatePreparation, core.Willing, 2)))
	return l
}

func TestAmbiguousFineAndExplicitCorrectionChangeAssistance(t *testing.T) {
	for _, language := range []core.ID{"en", "es"} {
		for _, style := range []string{"literal", "indirect"} {
			t.Run(string(language)+"/"+style, func(t *testing.T) {
				a := Account("alice", "bob")
				a.Original, a.DesiredHelp, a.Confirmation = "Fine", "unknown", "unconfirmed"
				if language == "es" {
					a.Original = "Está bien"
				}
				a.Preferences.Language, a.Preferences.Style, a.Preferences.Channel = language, style, "transcript"
				a.Clauses = []core.ListeningClause{{Key: "acceptance", Kind: "hypothesis", About: "alice", Text: "Possible acceptance", Confidence: .3}, {Key: "fatigue", Kind: "hypothesis", About: "alice", Text: "Possible fatigue or pause", Confidence: .3}}
				if language == "es" {
					a.Clauses[0].Text, a.Clauses[1].Text = "Posible aceptación", "Posible cansancio o pausa"
				}
				l := privateFixture(t, a)
				first := execute(t, l, request(l, "alice", "first", 3, "private", false), "supported")
				if first.Next != "ask_goal" || len(first.Options) != 4 || first.PartnerState != "unknown" || first.Own.DesiredHelp != "unknown" {
					t.Fatal("fine became hidden intent or partner feeling", first)
				}
				if first.Format != "transcript" || language == "es" && !strings.HasPrefix(first.Prompt, "Asistente: ") {
					t.Fatal("declared language/channel ignored", first)
				}
				old := copyValue(l.records[0])
				corrected := a
				corrected.Source, corrected.Corrects, corrected.Confirmation = "corrected", a.Source, "corrected"
				corrected.DesiredHelp, corrected.Original = "coordinate", "I meant I want a practical plan."
				if language == "es" {
					corrected.Original = "Quería decir que quiero un plan práctico."
				}
				must(t, l.PutAccount("alice", corrected, 20))
				second := execute(t, l, request(l, "alice", "second", 21, "private", false), "supported")
				if second.Next != "offer_plan" || second.Own.Source != "corrected" || assistance.Digest(old) != assistance.Digest(l.records[0]) {
					t.Fatal("correction ignored or old history rewritten", second)
				}
			})
		}
	}
}

func TestUnconfirmedMeaningCannotBePromotedByConfidentModel(t *testing.T) {
	a := Account("alice", "bob")
	a.Original, a.Confirmation = "Fine", "unconfirmed"
	l := privateFixture(t, a)
	out := execute(t, l, request(l, "alice", "fine", 3, "private", false), "supported")
	if out.Next != "ask_meaning" || out.Own.Confirmation != "unconfirmed" {
		t.Fatal("model manufactured confirmation", out)
	}
}

func TestMixedFeelingsAndPartnerPrivacy(t *testing.T) {
	var responses, inputs []string
	for _, secret := range []string{"CONCEALED_SADNESS", "CONCEALED_RELIEF"} {
		l := budget(t)
		alice := l.current["alice"]
		alice.Source, alice.Corrects, alice.Confirmation = "mixed", alice.Source, "corrected"
		alice.Feelings = []string{"happy about promotion", "sad about time apart"}
		must(t, l.PutAccount("alice", alice, 4))
		bob := l.current["bob"]
		bob.Source, bob.Corrects, bob.Confirmation = "bob-private", bob.Source, "corrected"
		bob.Feelings = []string{secret}
		must(t, l.PutAccount("bob", bob, 4))
		r := request(l, "alice", "mixed-turn", 5, "private", false)
		must(t, l.Register("alice", r))
		capture := &captureProvider{inner: model.Fake{}}
		out, err := l.Host(model.Listening{Provider: capture}).Execute(context.Background(), r)
		must(t, err)
		if len(out.Own.Feelings) != 2 || out.PartnerState != "unknown" || len(l.records[0].Accounts) != 1 {
			t.Fatal("mixed/absent-partner contract changed", out)
		}
		raw, _ := json.Marshal(out)
		providerRaw, _ := json.Marshal(capture.inputs)
		if strings.Contains(string(raw), "CONCEALED") || strings.Contains(string(providerRaw), "CONCEALED") {
			t.Fatal("private unselected partner state reached helper or response")
		}
		responses = append(responses, string(raw))
		inputs = append(inputs, string(providerRaw))
	}
	if responses[0] != responses[1] || inputs[0] != inputs[1] {
		t.Fatal("private state changed helper input/output")
	}
}

type captureProvider struct {
	inner  hws.ModelProvider
	inputs []hws.ProviderInput
	before func()
}

func (p *captureProvider) Generate(ctx context.Context, input hws.ProviderInput) (hws.ProviderResponse, error) {
	p.inputs = append(p.inputs, copyValue(input))
	if p.before != nil {
		p.before()
		p.before = nil
	}
	return p.inner.Generate(ctx, input)
}

func TestDeclinedMediationAndUnequalPowerKeepPrivatePreparation(t *testing.T) {
	for _, condition := range []string{"absent", "declined", "pressure"} {
		t.Run(condition, func(t *testing.T) {
			l := privateFixture(t, Account("alice", "bob"))
			if condition == "declined" {
				must(t, l.AppendBoundary("bob", Boundary("bob", "alice", core.Discussion, core.Declined, 2)))
			}
			if condition == "pressure" {
				for _, actor := range []core.ID{"alice", "bob"} {
					must(t, l.AppendBoundary(actor, Boundary(actor, l.other(actor), core.Discussion, core.Willing, 2)))
				}
				b := Boundary("alice", "bob", core.Discussion, core.PressureSignal, 2)
				b.Basis, b.Signal = "observed_signal", "credible_pressure"
				must(t, l.AppendBoundary("alice", b))
			}
			private := execute(t, l, request(l, "alice", "private", 3, "private", false), "supported")
			if private.Next != "acknowledge" || private.PartnerState != "unknown" {
				t.Fatal("private value made dependent on mediation", private)
			}
			r := request(l, "alice", "joint", 4, "joint", false)
			r.Invite = true
			must(t, l.Register("alice", r))
			provider := &captureProvider{inner: model.Fake{}}
			out, err := l.Host(model.Listening{Provider: provider}).Execute(context.Background(), r)
			must(t, err)
			if out.Next != "WAIT" || out.Own != nil || out.Invitation != "" || len(provider.inputs) != 0 {
				t.Fatal("unwilling/unknown/pressured joint interaction proceeded", out)
			}
		})
	}
}

func TestSharingGrantsRevocationAndRestrictedParaphraseLineage(t *testing.T) {
	for _, condition := range []string{"read_not_share", "revoked_summary", "restricted_parent", "changed_summary_selection"} {
		t.Run(condition, func(t *testing.T) {
			l := budget(t)
			if condition == "read_not_share" {
				must(t, l.PutSummary("bob", "no-share", "I choose these words for myself.", nil, nil, 4))
			}
			if condition == "restricted_parent" {
				must(t, l.PutSummary("bob", "paraphrase", "PRIVATE_SOURCE_IDENTIFYING_PARAPHRASE", []core.ID{"alice"}, []core.ID{"bob-account"}, 4))
			}
			if condition == "read_not_share" || condition == "restricted_parent" {
				b := l.current["bob"]
				b.Source, b.Corrects, b.Confirmation = "bob-corrected", b.Source, "corrected"
				b.Summary = "no-share"
				if condition == "restricted_parent" {
					b.Summary = "paraphrase"
				}
				must(t, l.PutAccount("bob", b, 4))
			}
			r := request(l, "alice", "sharing", 5, "joint", true)
			if condition == "changed_summary_selection" {
				r.Summaries[1].Sources = []core.ID{"bob-account"}
			}
			must(t, l.Register("alice", r))
			if condition == "revoked_summary" {
				must(t, l.Revoke("bob", "bob-summary"))
			}
			out, err := l.Host(model.Listening{Provider: model.Fake{}}).Execute(context.Background(), r)
			if err == nil || out.Own != nil || len(out.Shared) != 0 || len(l.records) != 0 {
				t.Fatal("unpermitted private sharing succeeded", out, err)
			}
		})
	}
}

func TestRevocationDuringModelPreventsCommitAndReplay(t *testing.T) {
	l := budget(t)
	r := request(l, "alice", "race", 4, "joint", true)
	must(t, l.Register("alice", r))
	p := &captureProvider{inner: model.Fake{}, before: func() { must(t, l.Revoke("bob", "bob-summary")) }}
	out, err := l.Host(model.Listening{Provider: p}).Execute(context.Background(), r)
	if err == nil || out.Own != nil || len(l.records) != 0 {
		t.Fatal("revocation during generation delivered or committed", out, err)
	}
	l = budget(t)
	r = request(l, "alice", "replay", 4, "joint", true)
	first := execute(t, l, r, "supported")
	// Idempotency uses committed bytes and current gates, without a model call.
	second, err := l.Host(nil).Execute(context.Background(), r)
	must(t, err)
	if assistance.Digest(first) != assistance.Digest(second) || len(l.records) != 1 {
		t.Fatal("replay changed output or appended twice")
	}
	must(t, l.Revoke("bob", "bob-summary"))
	if out, err = l.Host(nil).Execute(context.Background(), r); err == nil || len(out.Shared) != 0 {
		t.Fatal("replay resurrected revoked words", out)
	}
}

func TestConcurrentReplayCommitsOnce(t *testing.T) {
	l := budget(t)
	r := request(l, "alice", "same", 4, "joint", true)
	must(t, l.Register("alice", r))
	interpreter, err := RecordedFor(context.Background(), l, r, "supported")
	must(t, err)
	host := l.Host(interpreter)
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := host.Execute(context.Background(), r)
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		must(t, err)
	}
	if len(l.records) != 1 {
		t.Fatal("duplicate commit")
	}
}

type commitChangeJournal struct {
	*Local
	change func()
}

func (j commitChangeJournal) Commit(ctx context.Context, r assistance.ListeningRequest, record assistance.ListeningRecord, validate func(context.Context) error) error {
	j.change()
	return j.Local.Commit(ctx, r, record, validate)
}

func TestRevocationAtAtomicCommitBlocksPreviouslyValidatedSummary(t *testing.T) {
	l := budget(t)
	r := request(l, "alice", "commit-race", 4, "joint", true)
	must(t, l.Register("alice", r))
	interpreter, err := RecordedFor(context.Background(), l, r, "supported")
	must(t, err)
	host := l.Host(interpreter)
	host.Journal = commitChangeJournal{Local: l, change: func() { must(t, l.Revoke("bob", "bob-summary")) }}
	out, err := host.Execute(context.Background(), r)
	if err == nil || len(out.Shared) != 0 || len(l.records) != 0 {
		t.Fatal("commit did not revalidate atomically", out, err)
	}
}

type ownerCodeProvider struct{ otherCode string }

func (p ownerCodeProvider) Generate(ctx context.Context, in hws.ProviderInput) (hws.ProviderResponse, error) {
	r, err := (model.Fake{}).Generate(ctx, in)
	if err != nil {
		return r, err
	}
	var out hws.ModelOutput
	_ = json.Unmarshal(r.Output, &out)
	out.Findings[0].Code = "supported"
	if in.Context[0].Observer == "bob" {
		out.Findings[0].Code = p.otherCode
	}
	r.Output, _ = json.Marshal(out)
	return r, nil
}

func TestJointPartnerPrivateInterpretationCannotIdentifySourceInQuestion(t *testing.T) {
	var responses []string
	for _, code := range []string{"supported", "uncertain"} {
		l := budget(t)
		b := l.current["bob"]
		b.Source, b.Corrects, b.Confirmation = "bob-private", b.Source, "corrected"
		b.Feelings = []string{"PRIVATE_SADNESS_" + code}
		must(t, l.PutAccount("bob", b, 4))
		r := request(l, "alice", "joint-private", 5, "joint", true)
		must(t, l.Register("alice", r))
		out, err := l.Host(model.Listening{Provider: ownerCodeProvider{otherCode: code}}).Execute(context.Background(), r)
		must(t, err)
		if out.Next != "acknowledge" || len(l.records[0].Accounts) != 2 || l.records[0].Accounts[1].Feelings[0] != b.Feelings[0] {
			t.Fatal("own goal or independent private account lost", out)
		}
		raw, _ := json.Marshal(out)
		if strings.Contains(string(raw), "PRIVATE_SADNESS") || strings.Contains(string(raw), "bob-private") {
			t.Fatal("source-identifying private question or account leaked")
		}
		responses = append(responses, string(raw))
	}
	if responses[0] != responses[1] {
		t.Fatal("private partner hypotheses changed another participant's prompt")
	}
}

func TestBoundaryWithdrawalDuringModelPreventsDelivery(t *testing.T) {
	l := budget(t)
	r := request(l, "alice", "withdraw", 4, "joint", true)
	must(t, l.Register("alice", r))
	p := &captureProvider{inner: model.Fake{}, before: func() {
		must(t, l.AppendBoundary("bob", Boundary("bob", "alice", core.Discussion, core.Declined, 4)))
	}}
	out, err := l.Host(model.Listening{Provider: p}).Execute(context.Background(), r)
	if err == nil || out.Own != nil || len(l.records) != 0 {
		t.Fatal("withdrawal during model call ignored", out, err)
	}
}
