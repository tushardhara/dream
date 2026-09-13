package hws

import (
	"context"
	"encoding/json"
	"time"

	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
	rt "github.com/tushardhara/dream/simulator/runtime"
)

type ModelProvider interface {
	Generate(context.Context, ProviderInput) (ProviderResponse, error)
}
type ModelContextReader interface {
	ModelContext(context.Context, ViewPermit, graph.ApprovedContext, Scope, core.ID) (graph.SafeContext, error)
}

type ModelRequest struct {
	Scope      Scope
	Principal  core.ID
	Key        core.ID
	Capability ModelCapability
	Permit     ViewPermit
	Approved   graph.ApprovedContext
}
type ModelIntent struct {
	MemoryRevision string            `json:"memory_revision"`
	At             core.LogicalTime  `json:"at"`
	Version        uint32            `json:"version"`
	Scope          Scope             `json:"scope"`
	Principal      core.ID           `json:"principal"`
	MemoryScope    graph.MemoryScope `json:"memory_scope"`
	Key            core.ID           `json:"key"`
	Input          ProviderInput     `json:"input"`
}

func (i ModelIntent) Validate() error {
	expected, e := (ViewRealm{Scope: i.Scope, Principal: i.Principal}).MemoryScope()
	if e != nil || i.Version != 1 || len(i.MemoryRevision) != 64 || i.At < 0 || i.Key.Validate() != nil || i.MemoryScope != expected || i.Input.Actor != i.Principal || i.Input.Key != i.Key || i.Input.Validate() != nil {
		return ErrModel
	}
	return nil
}

type ModelAttempt struct {
	Deadline time.Time
	Intent   ModelIntent
	Number   int
	Fence    int64
	Complete *ModelArtifact
}
type ModelArtifact struct {
	Version       uint32           `json:"version"`
	RequestDigest string           `json:"request_digest"`
	Response      ProviderResponse `json:"response"`
	Output        ModelOutput      `json:"output"`
	Hash          string           `json:"hash"`
	// Recorded means these bytes are durable and reused; fresh calls remain stochastic.
	Mode string `json:"mode"`
}

func (a ModelArtifact) Validate(input ProviderInput) error {
	digest, e := input.Digest()
	if e != nil || a.Version != 1 || a.RequestDigest != digest || (a.Response.Generation != "deterministic_fake" && a.Response.Generation != "fresh_stochastic") || a.Response.Status != ProviderOK || a.Response.Model != input.Versions.Model || !a.Response.UsageKnown || a.Response.InputTokens < 0 || a.Response.OutputTokens < 0 || a.Response.OutputTokens > input.MaxOutputTokens || a.Mode != "recorded" {
		return ErrModel
	}
	out, e := DecodeModelOutput(a.Response.Output, input)
	if e != nil {
		return ErrModel
	}
	b, _ := json.Marshal(out)
	c, _ := json.Marshal(a.Output)
	if string(b) != string(c) {
		return ErrModel
	}
	copy := a
	copy.Hash = ""
	hash, e := ModelDigest(copy)
	if e != nil || a.Hash != hash {
		return ErrModel
	}
	return nil
}

type ModelStore interface {
	ConfigureModels(context.Context, Scope, ModelLimits) error
	BeginModel(context.Context, ModelIntent, ModelLimits) (ModelAttempt, error)
	FinishModel(context.Context, ModelAttempt, *ModelArtifact, ProviderStatus) error
}

// ModelRoute is trusted configuration, fixed for a gateway instance. Rates are
// conservative integer microcurrency/token ceilings, never inferred from aliases.
type ModelRoute struct {
	Versions             ModelVersions
	Capabilities         []ModelCapability
	InputMicrosPerToken  int64
	OutputMicrosPerToken int64
	MaxOutputTokens      int64
	Limits               ModelLimits
}

func (r ModelRoute) Validate() error {
	if r.Versions.Validate() != nil || r.Limits.Validate() != nil || r.InputMicrosPerToken < 1 || r.OutputMicrosPerToken < 1 || r.InputMicrosPerToken > 1000000 || r.OutputMicrosPerToken > 1000000 || r.MaxOutputTokens < 1 || r.MaxOutputTokens > 8192 || len(r.Capabilities) < 1 || len(r.Capabilities) > 4 {
		return ErrModelBudget
	}
	seen := map[ModelCapability]bool{}
	for _, c := range r.Capabilities {
		if !c.Valid() || seen[c] {
			return ErrModel
		}
		seen[c] = true
	}
	return nil
}

type ModelGateway struct {
	store    ModelStore
	views    ModelContextReader
	provider ModelProvider
	route    ModelRoute
}

func NewModelGateway(store ModelStore, views ModelContextReader, provider ModelProvider, route ModelRoute) (*ModelGateway, error) {
	if store == nil || views == nil || provider == nil || route.Validate() != nil {
		return nil, ErrModel
	}
	route.Capabilities = append([]ModelCapability{}, route.Capabilities...)
	return &ModelGateway{store, views, provider, route}, nil
}
func (g *ModelGateway) Execute(ctx context.Context, r ModelRequest) (ModelArtifact, error) {
	if g == nil || r.Scope.Validate() != nil || r.Principal.Validate() != nil || r.Key.Validate() != nil {
		return ModelArtifact{}, ErrModel
	}
	supported := false
	for _, c := range g.route.Capabilities {
		supported = supported || c == r.Capability
	}
	if !supported {
		return ModelArtifact{}, ErrModel
	}
	// Policy precedes request construction, durable retrieval and every provider call.
	safe, err := g.views.ModelContext(ctx, r.Permit, r.Approved, r.Scope, r.Principal)
	if err != nil {
		return ModelArtifact{}, ErrViewDenied
	}
	input := ProviderInput{Version: 1, Key: r.Key, Capability: r.Capability, Versions: g.route.Versions, Actor: r.Principal, Context: safe.Items(), MaxOutputTokens: g.route.MaxOutputTokens}
	// Reserve the entire supported wire byte ceiling as input tokens. This
	// includes JSON escaping, schema and protocol overhead without a tokenizer guess.
	inputBound := int64(MaxModelWireBytes)
	if input.Validate() != nil || inputBound+input.MaxOutputTokens > g.route.Limits.AttemptTokens || inputBound*g.route.InputMicrosPerToken+input.MaxOutputTokens*g.route.OutputMicrosPerToken > g.route.Limits.AttemptSpendMicros {
		return ModelArtifact{}, ErrModelBudget
	}
	scope, _ := (ViewRealm{Scope: r.Scope, Principal: r.Principal}).MemoryScope()
	intent := ModelIntent{Version: 1, MemoryRevision: safe.Revision(), At: safe.KnownAt(), Scope: r.Scope, Principal: r.Principal, MemoryScope: scope, Key: r.Key, Input: input}
	if err = g.store.ConfigureModels(ctx, r.Scope, g.route.Limits); err != nil {
		return ModelArtifact{}, err
	}
	for n := 0; n < g.route.Limits.MaxAttempts; n++ {
		if err = ctx.Err(); err != nil {
			return ModelArtifact{}, err
		}
		if _, err = g.views.ModelContext(ctx, r.Permit, r.Approved, r.Scope, r.Principal); err != nil {
			return ModelArtifact{}, ErrViewDenied
		}
		attempt, err := g.store.BeginModel(ctx, intent, g.route.Limits)
		if err != nil {
			return ModelArtifact{}, err
		}
		if attempt.Complete != nil {
			if attempt.Complete.Validate(input) != nil {
				return ModelArtifact{}, ErrModel
			}
			return *attempt.Complete, nil
		}
		callCtx, cancel := context.WithDeadline(ctx, attempt.Deadline)
		// Detach all provider-owned slices; adapters cannot rewrite the durable intent.
		encoded, _ := json.Marshal(input)
		var providerInput ProviderInput
		if json.Unmarshal(encoded, &providerInput) != nil {
			cancel()
			return ModelArtifact{}, ErrModel
		}
		response, callErr := g.provider.Generate(callCtx, providerInput)
		contextErr := callCtx.Err()
		cancel()
		status := response.Status
		if callErr != nil || contextErr != nil {
			status = ProviderUnavailable
		}
		var artifact *ModelArtifact
		if status == ProviderOK {
			output, decodeErr := DecodeModelOutput(response.Output, input)
			if decodeErr != nil || (response.Generation != "deterministic_fake" && response.Generation != "fresh_stochastic") || response.Model != input.Versions.Model || !response.UsageKnown || response.InputTokens < 0 || response.OutputTokens < 0 || response.InputTokens+response.OutputTokens > g.route.Limits.AttemptTokens || response.OutputTokens > input.MaxOutputTokens || response.InputTokens > inputBound || response.InputTokens*g.route.InputMicrosPerToken+response.OutputTokens*g.route.OutputMicrosPerToken > g.route.Limits.AttemptSpendMicros {
				status = ProviderMalformed
			} else {
				digest, _ := input.Digest()
				a := ModelArtifact{Version: 1, RequestDigest: digest, Response: response, Output: output, Mode: "recorded"}
				a.Hash, _ = ModelDigest(a)
				artifact = &a
			}
		}
		if status != ProviderOK && status != ProviderRefused && status != ProviderRateLimited && status != ProviderUnavailable && status != ProviderMalformed {
			status = ProviderMalformed
		}
		// Never persist an output whose context changed during generation.
		if _, err = g.views.ModelContext(ctx, r.Permit, r.Approved, r.Scope, r.Principal); err != nil {
			artifact = nil
			status = ProviderMalformed
		}
		if saveErr := g.store.FinishModel(ctx, attempt, artifact, status); saveErr != nil {
			return ModelArtifact{}, saveErr
		}
		if err != nil {
			return ModelArtifact{}, ErrViewDenied
		}
		if artifact != nil {
			return *artifact, nil
		}
		if ctx.Err() != nil {
			return ModelArtifact{}, ctx.Err()
		}
		if status != ProviderRateLimited && status != ProviderUnavailable {
			return ModelArtifact{}, ErrModel
		}
		if n+1 < g.route.Limits.MaxAttempts {
			timer := time.NewTimer(time.Duration(n+1) * 25 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ModelArtifact{}, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return ModelArtifact{}, ErrModelUncertain
}

// Apply binds one already-durable model artifact to a single runtime step. The
// injected handler supplies a deterministic transition, not another model call.
// #11 owns action semantics. Current context is checked before the handler and
// again after it, outside the database transaction; storage atomically verifies
// the artifact hash, source snapshot and single consumption during commit.
func (g *ModelGateway) Apply(ctx context.Context, request ModelRequest, artifact ModelArtifact, runtime Runtime, lease Lease, key core.ID) (Receipt, error) {
	if g == nil {
		return Receipt{}, ErrModel
	}
	supported := false
	for _, capability := range g.route.Capabilities {
		supported = supported || capability == request.Capability
	}
	if !supported {
		return Receipt{}, ErrModel
	}
	check := func(c context.Context) error {
		safe, e := g.views.ModelContext(c, request.Permit, request.Approved, request.Scope, request.Principal)
		if e != nil {
			return ErrViewDenied
		}
		input := ProviderInput{Version: 1, Key: request.Key, Capability: request.Capability, Versions: g.route.Versions, Actor: request.Principal, Context: safe.Items(), MaxOutputTokens: g.route.MaxOutputTokens}
		if artifact.Validate(input) != nil {
			return ErrModel
		}
		return nil
	}
	if e := check(ctx); e != nil {
		return Receipt{}, e
	}
	previous := runtime.BeforeCommit
	runtime.BeforeCommit = func(c context.Context) error {
		if previous != nil {
			if e := previous(c); e != nil {
				return e
			}
		}
		return check(c)
	}
	runtime.Model = &ModelUse{Key: request.Key, Hash: artifact.Hash}
	return runtime.Execute(ctx, request.Scope, lease, key, rt.Command{Kind: "step"})
}
