package model

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
)

type listeningProviderFunc func(context.Context, hws.ProviderInput) (hws.ProviderResponse, error)

func (f listeningProviderFunc) Generate(ctx context.Context, in hws.ProviderInput) (hws.ProviderResponse, error) {
	return f(ctx, in)
}
func listeningFixture() assistance.ListeningInput {
	a := core.ListeningAccount{Version: core.ListeningVersion, Source: "account", Speaker: "alice", Other: "bob", Focus: core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: core.Finances, RoleContext: "household"}, Original: "Fine", DesiredHelp: "unknown", Confirmation: "unconfirmed", Preferences: core.CommunicationPreferences{Language: "en", Style: "unspecified", Channel: "text"}}
	raw, _ := core.EncodeListeningAccount(a)
	return assistance.ListeningInput{Binding: "turn", Helper: "helper", Account: a, Evidence: graph.SafeContextItem{Source: a.Source, Observer: a.Speaker, Reporter: a.Speaker, Subject: core.Subject{Principal: a.Speaker}, Confidence: .5, Text: string(raw)}}
}
func TestListeningUsesTypedProviderAndRejectsUnsupportedClaims(t *testing.T) {
	in := listeningFixture()
	got, err := (Listening{Provider: Fake{}}).Interpret(context.Background(), in)
	if err != nil || got.Code != "uncertain" || got.Speaker != "alice" || got.Source != "account" {
		t.Fatal(got, err)
	}
	for _, bad := range []string{"winner", "foreign observer", "foreign evidence", "extra intent", "multiple findings", "wrong model", "refusal", "unknown usage", "live generation"} {
		t.Run(bad, func(t *testing.T) {
			provider := listeningProviderFunc(func(ctx context.Context, request hws.ProviderInput) (hws.ProviderResponse, error) {
				response, err := (Fake{}).Generate(ctx, request)
				if err != nil {
					return response, err
				}
				var output hws.ModelOutput
				_ = json.Unmarshal(response.Output, &output)
				switch bad {
				case "winner":
					output.Findings[0].Code = "alice_wins"
				case "foreign observer":
					output.Observer = "bob"
				case "foreign evidence":
					output.Findings[0].Evidence = []core.ID{"concealed-sadness"}
				case "extra intent":
					response.Output = []byte(`{"version":1,"capability":"interpretation","observer":"helper","hidden_intent":"acceptance","findings":[]}`)
					return response, nil
				case "multiple findings":
					output.Findings = append(output.Findings, hws.ModelFinding{Code: "supported", Confidence: .4, Evidence: []core.ID{"account"}})
				case "wrong model":
					response.Model = "other"
				case "refusal":
					response.Status = hws.ProviderRefused
				case "unknown usage":
					response.UsageKnown = false
				case "live generation":
					response.Generation = "fresh_stochastic"
				}
				response.Output, _ = json.Marshal(output)
				return response, nil
			})
			if _, err := (Listening{Provider: provider}).Interpret(context.Background(), in); err == nil {
				t.Fatal("invalid typed proposal accepted", bad)
			}
		})
	}
}

func TestListeningInputBindsExactAuthorizedAccount(t *testing.T) {
	for _, bad := range []string{"speaker", "source", "reporter", "altered account"} {
		in := listeningFixture()
		switch bad {
		case "speaker":
			in.Evidence.Observer = "bob"
		case "source":
			in.Evidence.Source = "another"
		case "reporter":
			in.Evidence.Reporter = "bob"
		case "altered account":
			in.Account.DesiredHelp = "coordinate"
		}
		if _, err := ListeningInput(in); err == nil {
			t.Fatal("provider input rebound authorized evidence", bad)
		}
	}
}
