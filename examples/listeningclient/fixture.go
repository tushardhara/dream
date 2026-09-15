package listeningclient

import (
	"context"
	"encoding/json"
	"time"

	"github.com/tushardhara/dream/adapters/model"
	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
)

func Account(speaker, other core.ID) core.ListeningAccount {
	return core.ListeningAccount{Version: core.ListeningVersion, Source: core.ID(string(speaker) + "-account"), Speaker: speaker, Other: other, Focus: core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: core.Finances, RoleContext: "household"}, Original: "I want to discuss our budget.", DesiredHelp: "listen", Confirmation: "confirmed", Preferences: core.CommunicationPreferences{Language: "en", Style: "literal", Channel: "text", ShortTurns: true, PlainLanguage: true}}
}

func Boundary(actor, other core.ID, class core.InteractionClass, decision core.Willingness, at core.LogicalTime) core.Boundary {
	id := core.ID(string(actor) + "-" + string(class) + "-" + string(decision))
	return core.Boundary{Version: core.BoundaryVersion, Meta: core.Metadata{ID: id, Observer: actor, Source: actor, Sensitivity: core.Restricted, Confidence: 1, Supporting: []core.ID{"explicit-participant-choice"}, Valid: core.Interval{Start: at}, RecordedAt: time.Unix(int64(at), 0).UTC(), Rights: core.Rights{Resource: id}}, Principal: actor, With: other, Topic: core.ID(core.Finances), Class: class, Decision: decision, Basis: "self_report", OccurredAt: at, LearnedAt: at}
}

func Budget() (*Local, error) {
	l := New("budget", "helper", "alice", "bob")
	for _, actor := range []core.ID{"alice", "bob"} {
		other := core.ID("alice")
		if actor == "alice" {
			other = "bob"
		}
		words := "I want to feel heard about keeping a reserve."
		if actor == "bob" {
			words = "I want a practical plan for tomorrow."
		}
		if err := l.PutSummary(actor, core.ID(string(actor)+"-summary"), words, []core.ID{actor, other}, nil, 1); err != nil {
			return nil, err
		}
	}
	for _, actor := range []core.ID{"alice", "bob"} {
		other := core.ID("alice")
		if actor == "alice" {
			other = "bob"
		}
		a := Account(actor, other)
		a.Summary = core.ID(string(actor) + "-summary")
		a.Clauses = []core.ListeningClause{{Key: "budget_limit", Kind: "observation", About: actor, Text: "I remember agreeing to 100.", Confidence: .8}, {Key: "priority", Kind: "value", About: actor, Text: "I value a reserve.", Confidence: 1}}
		if actor == "bob" {
			a.DesiredHelp = "coordinate"
			a.Clauses[0].Text = "I remember agreeing to 150."
			a.Clauses[1].Text = "I value flexibility."
		}
		if err := l.PutAccount(actor, a, 2); err != nil {
			return nil, err
		}
	}
	for _, actor := range []core.ID{"alice", "bob"} {
		for _, class := range []core.InteractionClass{core.PrivatePreparation, core.Discussion, core.SummarySharing} {
			if err := l.AppendBoundary(actor, Boundary(actor, l.other(actor), class, core.Willing, 3)); err != nil {
				return nil, err
			}
		}
	}
	return l, nil
}

// RecordedFor constructs explicitly authored synthetic model fixtures through
// the existing artifact codec and immutable Recorded adapter. The selected code
// is fixture ground truth about orchestration, NOT independent semantic evidence.
func RecordedFor(ctx context.Context, l *Local, r assistance.ListeningRequest, code string) (model.Listening, error) {
	policy := graph.NewPolicyService(l, l, l)
	var inputs []hws.ProviderInput
	var artifacts []hws.ModelArtifact
	for _, p := range r.Accounts {
		cap, d, err := policy.Approve(ctx, p)
		if err != nil || !d.Allowed {
			return model.Listening{}, assistance.ErrDenied
		}
		safe, d, err := policy.Revalidate(ctx, cap, r.ID)
		if err != nil || !d.Allowed || len(safe.Items()) != 1 {
			return model.Listening{}, assistance.ErrDenied
		}
		item := safe.Items()[0]
		a, err := core.DecodeListeningAccount([]byte(item.Text))
		if err != nil {
			return model.Listening{}, err
		}
		input, err := model.ListeningInput(assistance.ListeningInput{Binding: r.ID, Helper: r.Helper, Account: a, Evidence: item})
		if err != nil {
			return model.Listening{}, err
		}
		response, err := (model.Fake{}).Generate(ctx, input)
		if err != nil {
			return model.Listening{}, err
		}
		output := hws.ModelOutput{Version: 1, Capability: hws.ModelInterpretation, Observer: r.Helper, Findings: []hws.ModelFinding{{Code: code, Confidence: .4, Evidence: []core.ID{a.Source}}}}
		response.Output, _ = json.Marshal(output)
		digest, _ := input.Digest()
		artifact := hws.ModelArtifact{Version: 1, RequestDigest: digest, Response: response, Output: output, Mode: "recorded"}
		artifact.Hash, _ = hws.ModelDigest(artifact)
		if artifact.Validate(input) != nil {
			return model.Listening{}, hws.ErrModel
		}
		inputs = append(inputs, input)
		artifacts = append(artifacts, artifact)
	}
	provider, err := model.NewRecorded(inputs, artifacts)
	return model.Listening{Provider: provider}, err
}

func Run(ctx context.Context) (map[core.ID]assistance.ListeningResponse, error) {
	l, err := Budget()
	if err != nil {
		return nil, err
	}
	out := map[core.ID]assistance.ListeningResponse{}
	for _, actor := range []core.ID{"alice", "bob"} {
		r := l.Request(actor, core.ID(string(actor)+"-listen"), "joint", Account(actor, l.other(actor)).Focus, 4, true, false)
		if err := l.Register(actor, r); err != nil {
			return nil, err
		}
		interpreter, err := RecordedFor(ctx, l, r, "supported")
		if err != nil {
			return nil, err
		}
		response, err := l.Host(interpreter).Execute(ctx, r)
		if err != nil {
			return nil, err
		}
		out[actor] = response
	}
	return out, nil
}
