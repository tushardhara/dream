package hws

import (
	"context"
	"errors"
	"fmt"

	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/dynamics"
	rt "github.com/tushardhara/dream/simulator/runtime"
)

// CognitivePlanner is a trusted synthetic perception/action-affordance port.
// It receives only approved retrieved evidence, never the research view. It may
// propose affordances but cannot select actions, grant disclosure or allocate.
type CognitivePlanner interface {
	Plan(context.Context, rt.Input, graph.SafeContext) (CognitiveFrame, error)
}
type CognitiveService struct {
	Policy  string
	Gateway *ModelGateway
	Views   *ViewService
	Runtime Runtime
	Planner CognitivePlanner
}
type SelfDisclosure struct {
	Recipient core.ID
	Sources   []core.ID
	Writer    graph.ApprovedWriter
}
type fixedCognitiveFrame struct {
	input rt.Input
	frame CognitiveFrame
}

func (f fixedCognitiveFrame) Frame(i rt.Input) (CognitiveFrame, error) {
	if i != f.input {
		return CognitiveFrame{}, fmt.Errorf("prepared cognition input changed")
	}
	return f.frame, nil
}

// Apply consumes an already durable interpretation artifact from Gateway.Execute.
// Prepare/generate, policy output validation, and canonical application are
// separate stages; there is no provider callback inside the transition/SQL tx.
func (s CognitiveService) Apply(ctx context.Context, request ModelRequest, artifact ModelArtifact, lease Lease, key core.ID, disclosure *SelfDisclosure) (Receipt, error) {
	if s.Gateway == nil || s.Views == nil || s.Runtime.Store == nil || s.Planner == nil || request.Capability != ModelInterpretation || s.Policy != "" && s.Policy != behavior.Policy && s.Policy != behavior.ActionPolicy {
		return Receipt{}, ErrModel
	}
	safe, err := s.Views.ModelContext(ctx, request.Permit, request.Approved, request.Scope, request.Principal)
	if err != nil {
		return Receipt{}, err
	}
	input := ProviderInput{Version: 1, Key: request.Key, Capability: request.Capability, Versions: s.Gateway.route.Versions, Actor: request.Principal, Context: safe.Items(), MaxOutputTokens: s.Gateway.route.MaxOutputTokens}
	if artifact.Validate(input) != nil {
		return Receipt{}, ErrModel
	}
	snap, err := s.Runtime.Store.LoadRun(ctx, request.Scope)
	if err != nil {
		return Receipt{}, err
	}
	if len(snap.State.Queue) == 0 || snap.State.Queue[0].Actor != request.Principal {
		return Receipt{}, ErrModel
	}
	event := snap.State.Queue[0]
	frame, err := s.Planner.Plan(ctx, event, safe)
	if err != nil {
		return Receipt{}, err
	}
	// Planner cannot relabel policy failure as operational WAIT, forge model
	// beliefs, or enable self disclosure by setting a boolean/string field.
	if frame.Situation.Outage || frame.Situation.DisclosureRecipient != "" || frame.Disclosure != "" || frame.ModelHash != "" || len(frame.Situation.Beliefs) != 0 || len(frame.Situation.Relationships) != 0 {
		return Receipt{}, ErrModel
	}
	if s.Policy == behavior.ActionPolicy {
		if !validActionPlan(frame) {
			return Receipt{}, ErrModel
		}
		for _, item := range safe.Items() {
			frame.Actions.Sources = append(frame.Actions.Sources, item.Source)
		}
	} else if frame.Actions != nil {
		return Receipt{}, ErrModel
	}
	approvedEvent := false
	for _, item := range safe.Items() {
		approvedEvent = approvedEvent || item.Source == event.ID
	}
	if !approvedEvent || frame.Situation.Perceived.Event != event.ID || frame.Situation.Perceived.Actor != request.Principal {
		return Receipt{}, ErrViewDenied
	}
	for _, commitment := range frame.Situation.Commitments {
		found := false
		for _, item := range safe.Items() {
			found = found || item.Source == commitment.ID
		}
		if !found {
			return Receipt{}, ErrViewDenied
		}
	}
	relations, err := safe.Relations()
	if err != nil {
		return Receipt{}, err
	}
	for _, relation := range relations {
		if relation.Observer != request.Principal {
			return Receipt{}, ErrViewDenied
		}
		v := relation.State
		if v.Kind != "edge" || v.From.Principal != request.Principal || v.To.Principal == "" {
			continue
		}
		if v.Context != nil && frame.Actions == nil {
			return Receipt{}, ErrModel
		}
		if frame.Actions != nil && v.Context != nil {
			frame.Actions.Relationships = append(frame.Actions.Relationships, *v.Context)
			continue
		}
		m := behavior.Memory{Other: v.To.Principal, Evidence: []core.ID{relation.Source}}
		for _, dimension := range v.Dimensions {
			switch dimension.Name {
			case "trust":
				m.Trust = dimension.Value * float64(dimension.Confidence)
			case "disclosure":
				m.Disclosure = dimension.Value * float64(dimension.Confidence)
			}
		}
		frame.Situation.Relationships = append(frame.Situation.Relationships, m)
	}
	if frame.Actions != nil && len(frame.Actions.Relationships) > 0 {
		for _, item := range safe.Items() {
			// Internal markers are backed by this approved context and revalidated
			// before commit; they cannot authorize a user-facing read/disclosure.
			p := dynamics.Perceived{Actor: request.Principal, Event: item.Source, OccurredAt: item.OccurredAt, LearnedAt: item.LearnedAt, Confidence: item.Confidence, Rights: core.Rights{Resource: item.Source, Grants: []core.Grant{{Actor: request.Principal, Recipient: request.Principal, Purpose: "simulation", Operation: core.Read}, {Actor: request.Principal, Recipient: request.Principal, Purpose: "simulation", Operation: core.Derive}}}}
			frame.Actions.RelationshipEvidence = append(frame.Actions.RelationshipEvidence, p)
		}
	}
	frame.ModelHash = artifact.Hash
	for _, finding := range artifact.Output.Findings {
		// Every evidence ID remains observer-attributed. Conflicting duplicate
		// source interpretations fail the bounded belief validator, never collapse.
		for _, source := range finding.Evidence {
			frame.Situation.Beliefs = append(frame.Situation.Beliefs, behavior.Belief{Source: source, Observer: request.Principal, Code: finding.Code, Value: finding.Value, Confidence: finding.Confidence})
		}
	}
	runtime := s.Runtime
	if disclosure != nil {
		approvedSources := map[core.ID]bool{}
		for _, item := range safe.Items() {
			approvedSources[item.Source] = true
		}
		for _, source := range disclosure.Sources {
			if !approvedSources[source] {
				return Receipt{}, ErrViewDenied
			}
		}
		cap, decision, e := s.Views.Propose(ctx, request.Permit, disclosure.Sources, disclosure.Recipient, core.Disclose, graph.SyntheticSelfDisclosure)
		if e != nil {
			return Receipt{}, e
		}
		if !decision.Allowed {
			return Receipt{}, ErrViewDenied
		}
		output, decision, e := s.Views.Write(ctx, request.Permit, cap, disclosure.Writer)
		if e != nil {
			return Receipt{}, e
		}
		if !decision.Allowed {
			return Receipt{}, ErrViewDenied
		}
		if s.Policy == behavior.ActionPolicy {
			if !behavior.DisclosureMode(output.FictionMode).Valid() {
				return Receipt{}, ErrViewDenied
			}
			frame.Actions.Disclosure = &behavior.DisclosureGrant{Recipient: disclosure.Recipient, Mode: behavior.DisclosureMode(output.FictionMode), Sources: append([]core.ID{}, output.Sources...)}
		}
		frame.Disclosure = output.Text
		frame.Situation.DisclosureRecipient = disclosure.Recipient
		prior := runtime.BeforeCommit
		runtime.BeforeCommit = func(c context.Context) error {
			if prior != nil {
				if e := prior(c); e != nil {
					return e
				}
			}
			_, d, e := s.Views.policy.Revalidate(c, cap, request.Permit.binding)
			if e != nil {
				return e
			}
			if !d.Allowed {
				return ErrViewDenied
			}
			return nil
		}
	}
	runtime.Handler = CognitiveHandler{Source: fixedCognitiveFrame{event, frame}, Policy: s.Policy}
	return s.Gateway.Apply(ctx, request, artifact, runtime, lease, key)
}

// ModelFailureReader is a consumer port for durable exhausted provider failures.
// It does not accept a caller-supplied PASS/error label as canonical evidence.
type ModelFailureReader interface {
	FailedModel(context.Context, Scope, core.ID) (ModelIntent, ModelUse, error)
}

// Step composes retrieval/recorded generation and application. Only a verified,
// settled retryable failure can take the operational WAIT branch. Policy errors,
// malformed results, capacity exhaustion and uncertain in-flight work fail closed.
func (s CognitiveService) Step(ctx context.Context, request ModelRequest, lease Lease, key core.ID, disclosure *SelfDisclosure) (Receipt, error) {
	if s.Gateway == nil || s.Views == nil || s.Planner == nil || s.Runtime.Store == nil || request.Capability != ModelInterpretation || s.Policy != "" && s.Policy != behavior.Policy && s.Policy != behavior.ActionPolicy {
		return Receipt{}, ErrModel
	}
	artifact, err := s.Gateway.Execute(ctx, request)
	if err == nil {
		return s.Apply(ctx, request, artifact, lease, key, disclosure)
	}
	if !errors.Is(err, ErrModelUncertain) {
		return Receipt{}, err
	}
	failures, ok := s.Gateway.store.(ModelFailureReader)
	if !ok {
		return Receipt{}, ErrModel
	}
	intent, use, err := failures.FailedModel(ctx, request.Scope, request.Key)
	if err != nil {
		return Receipt{}, err
	}
	safe, err := s.Views.ModelContext(ctx, request.Permit, request.Approved, request.Scope, request.Principal)
	if err != nil {
		return Receipt{}, err
	}
	expected := ProviderInput{Version: 1, Key: request.Key, Capability: request.Capability, Versions: s.Gateway.route.Versions, Actor: request.Principal, Context: safe.Items(), MaxOutputTokens: s.Gateway.route.MaxOutputTokens}
	digest, err := expected.Digest()
	actual, e := intent.Input.Digest()
	if err != nil || e != nil || digest != actual || !use.Failed {
		return Receipt{}, ErrModel
	}
	snap, err := s.Runtime.Store.LoadRun(ctx, request.Scope)
	if err != nil {
		return Receipt{}, err
	}
	if len(snap.State.Queue) == 0 || snap.State.Queue[0].Actor != request.Principal {
		return Receipt{}, ErrModel
	}
	input := snap.State.Queue[0]
	found := false
	for _, item := range safe.Items() {
		found = found || item.Source == input.ID
	}
	if !found {
		return Receipt{}, ErrViewDenied
	}
	frame, err := s.Planner.Plan(ctx, input, safe)
	if err != nil {
		return Receipt{}, err
	}
	if s.Policy == behavior.ActionPolicy {
		if !validActionPlan(frame) {
			return Receipt{}, ErrModel
		}
		for _, item := range safe.Items() {
			frame.Actions.Sources = append(frame.Actions.Sources, item.Source)
		}
	} else if frame.Actions != nil {
		return Receipt{}, ErrModel
	}
	// An outage does not infer beliefs, deliver disclosure or learn a response.
	frame.Situation.Outage = true
	frame.Situation.Beliefs = nil
	frame.Situation.Relationships = nil
	frame.Situation.DisclosureRecipient = ""
	frame.Disclosure = ""
	frame.Response = nil
	frame.ModelHash = use.Hash
	runtime := s.Runtime
	runtime.Model = &use
	runtime.Handler = CognitiveHandler{Source: fixedCognitiveFrame{input, frame}, Policy: s.Policy}
	prior := runtime.BeforeCommit
	runtime.BeforeCommit = func(c context.Context) error {
		if prior != nil {
			if e := prior(c); e != nil {
				return e
			}
		}
		_, e := s.Views.ModelContext(c, request.Permit, request.Approved, request.Scope, request.Principal)
		return e
	}
	return runtime.Execute(ctx, request.Scope, lease, key, rt.Command{Kind: "step"})
}
