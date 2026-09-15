package responseexperiment

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
)

func TestActualHelperRecipientOutcomes(t *testing.T) {
	scene := Scene{Expectation: -1, Observation: "observed", ShareReport: true, Followup: true}
	run, e := Run(context.Background(), scene, assistance.Multi, 11, nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(run.Humans) != 16 || len(run.Helper) != 16 || run.FirstIntervention < 0 || len(run.Reports) == 0 {
		t.Fatal("consumer did not execute", len(run.Humans), len(run.Reports), run.FirstIntervention)
	}
	counts := map[string]int{}
	later := 0
	conflicting := 0
	for _, o := range run.Outcomes {
		if o.Position == "recipient" {
			counts[o.Appraisal]++
			if o.Phase == "later" {
				later++
			}
		}
		if o.Position == "sender" && o.Appraisal == "supportive" {
			for _, recipient := range run.Outcomes {
				if recipient.Interaction == o.Interaction && recipient.Position == "recipient" && recipient.Appraisal == "dismissive" {
					conflicting++
				}
			}
		}
	}
	if counts["dismissive"] == 0 || counts["neutral"]+counts["unresolved"] == 0 || later == 0 || conflicting == 0 {
		t.Fatal("no real adverse/null/delayed/separate account path", counts, later, conflicting)
	}
	raw, _ := json.Marshal(run.Reports)
	for _, private := range []string{"Private", "AppraisalKey", "Rationale", "alice-private-response", "bob-private-response"} {
		if strings.Contains(string(raw), private) {
			t.Fatal("private source/model state in helper report", private)
		}
	}
	if _, e = Run(context.Background(), scene, assistance.Multi, 11, &run); e != nil {
		t.Fatal("recorded replay", e)
	}
	run.Responses[0].Draw++
	if _, e = Run(context.Background(), scene, assistance.Multi, 11, &run); e == nil {
		t.Fatal("tampered recipient replay accepted")
	}
	t.Log(counts, later, conflicting)
}
func TestHelperCannotSeePrivateRecipientState(t *testing.T) {
	a := Scene{Expectation: 1, Observation: "observed", Followup: true}
	first, e := Run(context.Background(), a, assistance.Multi, 11, nil)
	if e != nil {
		t.Fatal(e)
	}
	a.Expectation = -1
	a.PrivateFear = .9
	second, e := Run(context.Background(), a, assistance.Multi, 11, nil)
	if e != nil {
		t.Fatal(e)
	}
	if assistance.Digest(first.Helper) != assistance.Digest(second.Helper) {
		t.Fatal("helper decisions depend on private recipient state")
	}
	if assistance.Digest(first.Responses) == assistance.Digest(second.Responses) {
		t.Fatal("actual recipient ignores private context")
	}
	if len(first.Reports) != 0 || len(second.Reports) != 0 {
		t.Fatal("unshared private outcome reported")
	}
}
func TestHelperMissingObservationsAndSameHumanEngineAcrossArms(t *testing.T) {
	for _, reason := range []string{"silence", "lost_observation", "declined_participation", "missing_followup"} {
		scene := Scene{Expectation: -1, Observation: reason, ShareReport: true, Followup: true}
		if reason == "missing_followup" {
			scene.Observation = "observed"
			scene.Followup = false
		}
		run, e := Run(context.Background(), scene, assistance.Multi, 11, nil)
		if e != nil {
			t.Fatal(reason, e)
		}
		found := 0
		for _, o := range run.Outcomes {
			if o.Position != "recipient" || reason == "missing_followup" && o.Phase != "later" {
				continue
			}
			found++
			if o.Status == core.Observed || o.Missing != reason || o.Benefit != nil || o.Burden != nil {
				t.Fatal("missing observation became welfare", reason)
			}
		}
		if found == 0 {
			t.Fatal("unexercised missing outcome", reason)
		}
	}
	for _, arm := range []assistance.Arm{assistance.None, assistance.Simple, assistance.Single, assistance.Multi} {
		run, e := Run(context.Background(), Scene{Expectation: -1, Observation: "observed", Followup: true}, arm, 11, nil)
		if e != nil {
			t.Fatal(arm, e)
		}
		if len(run.Humans) != 16 {
			t.Fatal("human stopped with helper")
		}
		for _, d := range run.Humans {
			if d.Version != "domain-human-actions.v1" {
				t.Fatal("different human model across arms")
			}
		}
		if arm == assistance.None {
			if run.FirstIntervention != -1 {
				t.Fatal("no-helper intervention")
			}
			for _, h := range run.Helper {
				if h.Delivered {
					t.Fatal("no-helper delivered")
				}
			}
		}
	}
}

func TestHelperObservedLearningChangesNativeChoice(t *testing.T) {
	scene := Scene{Expectation: -1, Observation: "observed", Followup: true}
	observed, e := Run(context.Background(), scene, assistance.Multi, 11, nil)
	if e != nil {
		t.Fatal(e)
	}
	scene.Observation = "silence"
	missing, e := Run(context.Background(), scene, assistance.Multi, 11, nil)
	if e != nil {
		t.Fatal(e)
	}
	if assistance.Digest(observed.Helper) != assistance.Digest(missing.Helper) {
		t.Fatal("private outcomes reached helper planner")
	}
	different := false
	for i, d := range observed.Humans {
		if d.Human.Human.Draw != missing.Humans[i].Human.Human.Draw {
			t.Fatal("human randomness changed")
		}
		if assistance.Digest(d.Human.Human.Candidates) != assistance.Digest(missing.Humans[i].Human.Human.Candidates) {
			if i < 2 {
				t.Fatal("future learning affected early choice")
			}
			different = true
			break
		}
	}
	if !different {
		t.Fatal("helper experiment ignores observed learning")
	}
}

func TestHelperReportsCurrentRightsCorrectionsAndActionBinding(t *testing.T) {
	run, e := Run(context.Background(), Scene{Expectation: -1, Observation: "observed", ShareReport: true, Followup: true}, assistance.Multi, 11, nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(run.Actions) == 0 {
		t.Fatal("no human chose an advice affordance")
	}
	action := run.Actions[0]
	var interaction assistance.Interaction
	for _, h := range run.Helper {
		if h.ID == action.Interaction {
			interaction = h
		}
	}
	if interaction.Result.Candidates[interaction.Result.Selected].Recipient != interaction.User {
		t.Fatal("helper delivered to someone other than requesting person")
	}
	humanFound := false
	for _, h := range run.Humans {
		d := h.Human.Human
		if d.ID == action.Action && d.Actor == action.Sender && d.Candidates[d.Selected].Offer.Recipient == action.Recipient {
			humanFound = true
		}
	}
	if !humanFound {
		t.Fatal("outcome action has no actual human choice")
	}
	original, e := assistance.OutcomeReports(interaction, action, run.Outcomes, interaction.Helper, 100)
	if e != nil || len(original) == 0 {
		t.Fatal("reports", e)
	}
	for _, which := range []string{"revoked", "wrong_recipient", "wrong_action", "future", "before_effect", "forged_reply"} {
		raw, _ := json.Marshal(run.Outcomes)
		var log []core.OutcomeObservation
		_ = json.Unmarshal(raw, &log)
		receipt := action
		switch which {
		case "revoked":
			for i := range log {
				log[i].Meta.Rights.Revoked = true
			}
		case "wrong_recipient":
			receipt.Recipient = "outsider"
		case "wrong_action":
			receipt.Action = "unrelated"
		case "future":
			receipt.At = 101
		case "before_effect":
			for i := range log {
				if log[i].Action == action.Action && log[i].Position == "recipient" {
					log[i].OccurredAt = action.At
					log[i].LearnedAt = action.At
				}
			}
		case "forged_reply":
			for i := range log {
				if log[i].Action == action.Action && log[i].Position == "recipient" {
					log[i].Reply = "nonexistent-reply"
				}
			}

		}
		got, e := assistance.OutcomeReports(interaction, receipt, log, interaction.Helper, 100)
		if e == nil && len(got) != 0 {
			t.Fatal("unavailable/wrongly linked account exposed", which)
		}
	}
	var prior core.OutcomeObservation
	for _, o := range run.Outcomes {
		if o.Interaction == action.Interaction && o.Position == "recipient" && o.Phase == "later" {
			prior = o
			break
		}
	}
	if prior.Meta.ID == "" {
		t.Fatal("no later recipient report")
	}
	raw, _ := json.Marshal(run.Outcomes)
	before := string(raw)
	correction := prior
	correction.Meta.ID = "explicit-later-correction"
	correction.Meta.Rights.Resource = correction.Meta.ID
	correction.Supersedes = prior.Meta.ID
	correction.OccurredAt = 65
	correction.LearnedAt = 65
	correction.Appraisal = "dismissive"
	b, c := -.6, .8
	correction.Benefit = &b
	correction.Burden = &c
	log, e := core.AppendOutcome(run.Outcomes, correction)
	if e != nil {
		t.Fatal(e)
	}
	now, e := assistance.OutcomeReports(interaction, action, log, interaction.Helper, 65)
	if e != nil {
		t.Fatal(e)
	}
	seen := false
	for _, o := range now {
		if o.ID == prior.Meta.ID {
			t.Fatal("superseded account remained current")
		}
		if o.ID == correction.Meta.ID {
			seen = true
			if o.Appraisal != "dismissive" {
				t.Fatal("correction ignored")
			}
		}
	}
	if !seen {
		t.Fatal("current correction missing")
	}
	past, e := assistance.OutcomeReports(interaction, action, log, interaction.Helper, 64)
	if e != nil || assistance.Digest(past) != assistance.Digest(original) {
		t.Fatal("correction rewrote known history", e)
	}
	raw, _ = json.Marshal(run.Outcomes)
	if string(raw) != before {
		t.Fatal("append aliased historical outcomes")
	}
	log[len(log)-1].Meta.Rights.Revoked = true
	revoked, e := assistance.OutcomeReports(interaction, action, log, interaction.Helper, 65)
	if e != nil {
		t.Fatal(e)
	}
	for _, o := range revoked {
		if o.ID == prior.Meta.ID || o.ID == correction.Meta.ID {
			t.Fatal("revoked correction resurrected old conclusion")
		}
	}
}
