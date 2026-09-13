// Package model implements provider transport and offline replay, never domain
// policy or canonical transition logic. No adapter is selected from environment.
package model

import (
	"context"
	"encoding/json"

	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
)

type Fake struct{}

func (Fake) Generate(ctx context.Context, r hws.ProviderInput) (hws.ProviderResponse, error) {
	if err := ctx.Err(); err != nil {
		return hws.ProviderResponse{}, err
	}
	if r.Validate() != nil {
		return hws.ProviderResponse{}, hws.ErrModel
	}
	code := map[hws.ModelCapability]string{hws.ModelAppraisal: "care", hws.ModelInterpretation: "uncertain", hws.ModelReconciliation: "retain_both", hws.ModelCandidates: "wait"}[r.Capability]
	o := hws.ModelOutput{Version: 1, Capability: r.Capability, Observer: r.Actor, Findings: []hws.ModelFinding{{Code: code, Value: 0, Confidence: .5, Evidence: []core.ID{r.Context[0].Source}}}}
	raw, _ := json.Marshal(o)
	return hws.ProviderResponse{Generation: "deterministic_fake", Status: hws.ProviderOK, Model: r.Versions.Model, Output: raw, UsageKnown: true, InputTokens: 1, OutputTokens: 1}, nil
}

// Recorded is an immutable, detached mapping from canonical request hashes to
// validated artifact bytes. It has no network client or fallback to generation.
type Recorded struct{ artifacts map[string]hws.ModelArtifact }

func NewRecorded(inputs []hws.ProviderInput, artifacts []hws.ModelArtifact) (*Recorded, error) {
	if len(inputs) != len(artifacts) || len(inputs) > 128 {
		return nil, hws.ErrModel
	}
	r := &Recorded{artifacts: map[string]hws.ModelArtifact{}}
	for i, input := range inputs {
		if artifacts[i].Validate(input) != nil {
			return nil, hws.ErrModel
		}
		digest, _ := input.Digest()
		if _, ok := r.artifacts[digest]; ok {
			return nil, hws.ErrModel
		}
		raw, _ := json.Marshal(artifacts[i])
		var owned hws.ModelArtifact
		if json.Unmarshal(raw, &owned) != nil {
			return nil, hws.ErrModel
		}
		r.artifacts[digest] = owned
	}
	return r, nil
}
func (r *Recorded) Generate(ctx context.Context, input hws.ProviderInput) (hws.ProviderResponse, error) {
	if err := ctx.Err(); err != nil {
		return hws.ProviderResponse{}, err
	}
	if r == nil {
		return hws.ProviderResponse{}, hws.ErrModel
	}
	digest, err := input.Digest()
	if err != nil {
		return hws.ProviderResponse{}, err
	}
	artifact, ok := r.artifacts[digest]
	if !ok || artifact.Validate(input) != nil {
		return hws.ProviderResponse{}, hws.ErrModel
	}
	response := artifact.Response
	response.Output = append([]byte{}, response.Output...)
	return response, nil
}
