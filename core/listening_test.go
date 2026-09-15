package core

import (
	"bytes"
	"testing"
)

func listeningAccount() ListeningAccount {
	return ListeningAccount{Version: ListeningVersion, Source: "account", Speaker: "alice", Other: "bob", Focus: RelationshipFocus{Version: RelationshipFocusVersion, Domain: Finances, RoleContext: "household"}, Original: "Fine", DesiredHelp: "unknown", Confirmation: "unconfirmed", Preferences: CommunicationPreferences{Language: "en", Style: "unspecified", Channel: "text"}, Feelings: []string{"happy", "sad"}, Clauses: []ListeningClause{{Key: "meaning", Kind: "hypothesis", About: "alice", Text: "I may want a pause", Confidence: .3}}}
}

func TestListeningAccountPreservesMixedFeelingsAndUncertainty(t *testing.T) {
	a := listeningAccount()
	raw, err := EncodeListeningAccount(a)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeListeningAccount(raw)
	if err != nil || got.Original != "Fine" || len(got.Feelings) != 2 || got.DesiredHelp != "unknown" || got.Clauses[0].Kind != "hypothesis" {
		t.Fatal("account collapsed or inferred intent", got, err)
	}
	roundtrip, _ := EncodeListeningAccount(got)
	if !bytes.Equal(raw, roundtrip) {
		t.Fatal("account replay changed bytes")
	}
}

func TestListeningSummaryRequiresASeparateSource(t *testing.T) {
	for _, tc := range []struct {
		name    string
		summary ID
		valid   bool
	}{
		{name: "sharing is optional", valid: true},
		{name: "separately authored words", summary: "chosen-summary", valid: true},
		{name: "raw original account is not a summary", summary: "account"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := listeningAccount()
			a.Confirmation = "confirmed"
			a.Summary = tc.summary
			if err := a.Validate(); (err == nil) != tc.valid {
				t.Fatalf("summary source %q with original %q: Validate() = %v, want valid=%t", a.Summary, a.Source, err, tc.valid)
			}
		})
	}
}

func TestListeningContractRejectsInventedCertaintyAndUnconfirmedSharing(t *testing.T) {
	for _, mutation := range []string{"version", "goal", "certain hypothesis", "winner", "unconfirmed summary", "correction missing", "self correction", "invalid preference", "duplicate feelings", "duplicate clause"} {
		t.Run(mutation, func(t *testing.T) {
			a := listeningAccount()
			switch mutation {
			case "version":
				a.Version = "future"
			case "goal":
				a.DesiredHelp = "make_partner_agree"
			case "certain hypothesis":
				a.Clauses[0].Confidence = 1
			case "winner":
				a.Clauses[0].Kind = "winner"
			case "unconfirmed summary":
				a.Summary = "summary"
			case "correction missing":
				a.Confirmation = "corrected"
			case "self correction":
				a.Confirmation, a.Corrects = "corrected", a.Source
			case "invalid preference":
				a.Preferences.Style = "inferred_from_age"
			case "duplicate feelings":
				a.Feelings = append(a.Feelings, "happy")
			case "duplicate clause":
				a.Clauses = append(a.Clauses, a.Clauses[0])
			}
			if a.Validate() == nil {
				t.Fatal("invalid contract accepted", mutation)
			}
		})
	}
	raw, _ := EncodeListeningAccount(listeningAccount())
	for _, bad := range [][]byte{append(append([]byte{}, raw...), raw...), bytes.Replace(raw, []byte(`"Version":`), []byte(`"winner":"alice","Version":`), 1)} {
		if _, err := DecodeListeningAccount(bad); err == nil {
			t.Fatal("unknown field or trailing input accepted")
		}
	}
}
