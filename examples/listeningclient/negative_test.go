package listeningclient

import (
	"context"
	"strings"
	"testing"

	"github.com/tushardhara/dream/adapters/model"
	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
)

func TestCurrentAccountAndCorrectionAuthority(t *testing.T) {
	l := budget(t)
	old := l.current["alice"]
	for _, change := range []string{"foreign actor", "other account", "domain", "frame", "missing correction"} {
		a := old
		a.Source, a.Corrects, a.Confirmation = "new", old.Source, "corrected"
		actor := core.ID("alice")
		switch change {
		case "foreign actor":
			actor = "bob"
		case "other account":
			a.Corrects = "bob-account"
		case "domain":
			a.Focus.Domain = core.Childcare
		case "frame":
			a.Focus.RoleContext = "business"
		case "missing correction":
			a.Corrects = ""
		}
		if l.PutAccount(actor, a, 4) == nil {
			t.Fatal("invalid account replacement accepted", change)
		}
	}
	r := request(l, "alice", "before-correction", 4, "private", false)
	must(t, l.Register("alice", r))
	a := old
	a.Source, a.Corrects, a.Confirmation, a.DesiredHelp = "current", old.Source, "corrected", "pause"
	must(t, l.PutAccount("alice", a, 4))
	if out, err := l.Host(model.Listening{Provider: model.Fake{}}).Execute(context.Background(), r); err == nil || out.Own != nil {
		t.Fatal("old source used after correction", out, err)
	}
	current := request(l, "alice", "current-turn", 5, "private", false)
	must(t, l.Register("alice", current))
	capture := &captureProvider{inner: model.Fake{}}
	out, err := l.Host(model.Listening{Provider: capture}).Execute(context.Background(), current)
	must(t, err)
	if out.Next != "WAIT" || len(capture.inputs) != 0 || out.Own.DesiredHelp != "pause" {
		t.Fatal("explicit pause did not stop model/conversation", out)
	}
}

func TestListeningRequestRejectsPrivateToJointGrantSubstitution(t *testing.T) {
	for _, change := range []string{"version", "private partner", "helper recipient", "read for share", "wrong person", "time", "domain", "frame", "source"} {
		t.Run(change, func(t *testing.T) {
			l := budget(t)
			r := request(l, "alice", "invalid", 4, "joint", true)
			switch change {
			case "version":
				r.Version = "future"
			case "private partner":
				r.Mode = "private"
			case "helper recipient":
				r.Accounts[0].Recipient = "bob"
			case "read for share":
				r.Summaries[1].Operation = core.Read
			case "wrong person":
				r.Summaries[1].Recipient = "outsider"
			case "time":
				r.Accounts[0].Query.KnownAt = 3
			case "domain":
				r.Focus.Domain = core.Childcare
			case "frame":
				r.Focus.RoleContext = "business"
			case "source":
				r.Accounts[0].Sources = []core.ID{"bob-account"}
			}
			if err := l.Register("alice", r); err != nil {
				return
			}
			out, err := l.Host(model.Listening{Provider: model.Fake{}}).Execute(context.Background(), r)
			if err == nil && out.Next != "WAIT" || out.Own != nil || len(out.Shared) != 0 {
				t.Fatal("invalid scope or grant exposed data", out, err)
			}
		})
	}
}

func TestDeclaredCommunicationPreferencesAffectRendering(t *testing.T) {
	results := map[string]assistance.ListeningResponse{}
	for _, style := range []string{"literal", "indirect", "short", "transcript", "unsupported"} {
		a := Account("alice", "bob")
		a.Confirmation, a.Original = "unconfirmed", "Fine"
		a.Preferences.ShortTurns = false
		switch style {
		case "indirect":
			a.Preferences.Style = "indirect"
		case "short":
			a.Preferences.ShortTurns = true
		case "transcript":
			a.Preferences.Channel = "transcript"
		case "unsupported":
			a.Preferences.Language = "fr"
		}
		l := privateFixture(t, a)
		r := request(l, "alice", "preferences", 3, "private", false)
		must(t, l.Register("alice", r))
		capture := &captureProvider{inner: model.Fake{}}
		out, err := l.Host(model.Listening{Provider: capture}).Execute(context.Background(), r)
		must(t, err)
		results[style] = out
		if style == "unsupported" && (out.Next != "WAIT" || out.Prompt != "" || len(capture.inputs) != 0) {
			t.Fatal("unsupported language silently translated", out)
		}
	}
	if results["literal"].Prompt == results["indirect"].Prompt || len(results["short"].Prompt) >= len(results["literal"].Prompt) || !strings.HasPrefix(results["transcript"].Prompt, "Helper: ") {
		t.Fatal("declared wording/length/channel preferences ignored", results)
	}
	for _, plain := range []bool{false, true} {
		a := Account("alice", "bob")
		a.DesiredHelp, a.Preferences.ShortTurns, a.Preferences.PlainLanguage = "understand", false, plain
		l := privateFixture(t, a)
		out := execute(t, l, request(l, "alice", "plain", 3, "private", false), "supported")
		if out.Next != "reflect_accounts" || strings.Contains(out.Prompt, "interpretations") == plain {
			t.Fatal("plain language preference ignored", out)
		}
	}
}

func TestClarificationRespectsCooldownBudgetAndPause(t *testing.T) {
	a := Account("alice", "bob")
	a.DesiredHelp, a.Confirmation = "unknown", "unconfirmed"
	l := privateFixture(t, a)
	for i, tc := range []struct {
		at   core.LogicalTime
		next string
	}{{3, "ask_goal"}, {4, "WAIT"}, {13, "ask_goal"}, {30, "WAIT"}} {
		id := core.ID([]string{"one", "cooldown", "two", "budget"}[i])
		out := execute(t, l, request(l, "alice", id, tc.at, "private", false), "supported")
		if out.Next != tc.next {
			t.Fatal("clarification pacing ignored", tc, out)
		}
	}
}

func TestTypedInterpretationActuallyChangesNextAssistance(t *testing.T) {
	for _, code := range []string{"uncertain", "supported", "contradicted"} {
		a := Account("alice", "bob")
		a.DesiredHelp = "understand"
		l := privateFixture(t, a)
		out := execute(t, l, request(l, "alice", "model", 3, "private", false), code)
		want := "ask_meaning"
		if code == "supported" {
			want = "reflect_accounts"
		}
		if out.Next != want {
			t.Fatal("typed proposal ignored or uncertainty became a fact", code, out)
		}
	}
}

func TestPrivateAccountReadRevocationAndMissingInterpreter(t *testing.T) {
	l := privateFixture(t, Account("alice", "bob"))
	r := request(l, "alice", "no-model", 3, "private", false)
	must(t, l.Register("alice", r))
	out, err := l.Host(nil).Execute(context.Background(), r)
	must(t, err)
	if out.Next != "WAIT" {
		t.Fatal("missing interpreter fabricated understanding")
	}
	must(t, l.Revoke("alice", "alice-account"))
	if out, err = l.Host(nil).Execute(context.Background(), r); err == nil || out.Own != nil {
		t.Fatal("private replay bypassed revoked account", out)
	}
}

func TestInvitationAndSummaryNeedTheirOwnCurrentConsent(t *testing.T) {
	for _, mode := range []string{"allowed", "summary consent absent", "invitation declined"} {
		l := budget(t)
		if mode == "summary consent absent" {
			var kept []core.Boundary
			for _, b := range l.boundaries {
				if b.Class != core.SummarySharing {
					kept = append(kept, b)
				}
			}
			l.boundaries = kept
		}
		if mode == "invitation declined" {
			must(t, l.AppendBoundary("bob", Boundary("bob", "alice", core.Discussion, core.Declined, 3)))
		}
		r := request(l, "alice", "consent", 4, "joint", true)
		r.Invite = true
		must(t, l.Register("alice", r))
		if mode == "allowed" {
			interpreter, err := RecordedFor(context.Background(), l, r, "supported")
			must(t, err)
			out, err := l.Host(interpreter).Execute(context.Background(), r)
			must(t, err)
			if !strings.Contains(out.Invitation, "optional") || len(out.Shared) != 2 {
				t.Fatal("consented operation unavailable", out)
			}
		} else {
			capture := &captureProvider{inner: model.Fake{}}
			out, err := l.Host(model.Listening{Provider: capture}).Execute(context.Background(), r)
			must(t, err)
			if out.Next != "WAIT" || out.Own != nil || out.Invitation != "" || len(out.Shared) != 0 || len(capture.inputs) != 0 {
				t.Fatal("data grant bypassed interpersonal consent", mode, out)
			}
		}
	}
}

func TestUnchosenPublicSummaryCannotReplaceCurrentSelection(t *testing.T) {
	l := budget(t)
	must(t, l.PutSummary("bob", "other-public-summary", "These are different public words.", []core.ID{"alice"}, nil, 4))
	r := request(l, "alice", "substitution", 5, "joint", true)
	r.Summaries[1].Sources = []core.ID{"other-public-summary"}
	must(t, l.Register("alice", r))
	out, err := l.Host(model.Listening{Provider: model.Fake{}}).Execute(context.Background(), r)
	if err == nil || len(out.Shared) != 0 || len(l.records) != 0 {
		t.Fatal("generic share grant replaced this person's chosen summary", out, err)
	}
}

func TestForeignAuthorshipCannotBecomeOwnListeningAccount(t *testing.T) {
	l := privateFixture(t, Account("alice", "bob"))
	for scope, entries := range l.entries {
		if scope.Owner == "alice" {
			entries[0].Event.Meta.Source = "bob"
			l.entries[scope] = entries
		}
	}
	r := request(l, "alice", "foreign-authorship", 3, "private", false)
	must(t, l.Register("alice", r))
	out, err := l.Host(model.Listening{Provider: model.Fake{}}).Execute(context.Background(), r)
	if err == nil || out.Own != nil || len(l.records) != 0 {
		t.Fatal("foreign authorship treated as this participant's account", out, err)
	}
}

func TestCurrentParticipantPauseStopsJointUseAndPrivateInvitations(t *testing.T) {
	for _, mode := range []string{"joint", "private"} {
		t.Run(mode, func(t *testing.T) {
			l := budget(t)
			b := l.current["bob"]
			b.Source, b.Corrects, b.Confirmation, b.DesiredHelp = "bob-paused", b.Source, "corrected", "pause"
			must(t, l.PutAccount("bob", b, 4))
			r := request(l, "alice", "peer-paused", 5, mode, mode == "joint")
			r.Invite = true
			must(t, l.Register("alice", r))
			capture := &captureProvider{inner: model.Fake{}}
			out, err := l.Host(model.Listening{Provider: capture}).Execute(context.Background(), r)
			must(t, err)
			if out.Next != "WAIT" || out.Own != nil || len(out.Shared) != 0 || out.Invitation != "" || len(capture.inputs) != 0 || len(l.records) != 0 {
				t.Fatal("current participant pause ignored despite older willingness", out)
			}
			// Another person's pause does not remove private, non-inviting support.
			private := execute(t, l, request(l, "alice", "independent", 6, "private", false), "supported")
			if private.Next != "acknowledge" || private.PartnerState != "unknown" {
				t.Fatal("peer pause removed independent private value", private)
			}
		})
	}
}
