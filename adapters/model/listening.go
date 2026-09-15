package model

import (
	"context"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
)

// Listening uses the existing typed model provider port and output validator.
// Composition in examples/listeningclient supplies Fake or Recorded, never a
// live provider. This is not another durable/world gateway or a language model.
type Listening struct{ Provider hws.ModelProvider }

func ListeningInput(in assistance.ListeningInput) (hws.ProviderInput, error) {
	if in.Binding.Validate() != nil || in.Helper.Validate() != nil || in.Account.Validate() != nil || in.Evidence.Source != in.Account.Source || in.Evidence.Observer != in.Account.Speaker || in.Evidence.Reporter != in.Account.Speaker {
		return hws.ProviderInput{}, hws.ErrModel
	}
	encoded, err := core.DecodeListeningAccount([]byte(in.Evidence.Text))
	if err != nil || assistance.Digest(encoded) != assistance.Digest(in.Account) {
		return hws.ProviderInput{}, hws.ErrModel
	}
	key := core.ID("listen:" + assistance.Digest(struct{ Binding, Source core.ID }{in.Binding, in.Account.Source})[:32])
	input := hws.ProviderInput{Version: 1, Key: key, Capability: hws.ModelInterpretation, Versions: hws.ModelVersions{Schema: "model.v1", Capability: "cognition.v1", Model: "synthetic-listening.v1", Prompt: "cognition.v1", Policy: "policy.v1"}, Actor: in.Helper, Context: []graph.SafeContextItem{in.Evidence}, MaxOutputTokens: 128}
	return input, input.Validate()
}

func (m Listening) Interpret(ctx context.Context, in assistance.ListeningInput) (assistance.ListeningInterpretation, error) {
	input, err := ListeningInput(in)
	if err != nil || m.Provider == nil {
		return assistance.ListeningInterpretation{}, hws.ErrModel
	}
	response, err := m.Provider.Generate(ctx, input)
	if err != nil || response.Status != hws.ProviderOK || response.Generation != "deterministic_fake" || response.Model != input.Versions.Model || !response.UsageKnown || response.InputTokens < 0 || response.OutputTokens < 0 || response.OutputTokens > input.MaxOutputTokens {
		return assistance.ListeningInterpretation{}, hws.ErrModel
	}
	output, err := hws.DecodeModelOutput(response.Output, input)
	if err != nil || len(output.Findings) != 1 || len(output.Findings[0].Evidence) != 1 || output.Findings[0].Evidence[0] != in.Account.Source {
		return assistance.ListeningInterpretation{}, hws.ErrModel
	}
	f := output.Findings[0]
	return assistance.ListeningInterpretation{Source: in.Account.Source, Speaker: in.Account.Speaker, Code: f.Code, Confidence: f.Confidence}, nil
}
