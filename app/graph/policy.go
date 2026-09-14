package graph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/tushardhara/dream/core"
)

var ErrPolicyEvidence = errors.New("policy evidence unavailable")
var ErrPolicyWriter = errors.New("approved writer failed")
var ErrPolicyAudit = errors.New("policy audit unavailable")

type PolicyAudit struct {
	ContextRevision string           `json:"context_revision,omitempty"`
	ContextHash     string           `json:"context_hash,omitempty"`
	KnownAt         core.LogicalTime `json:"known_at,omitempty"`
	Version         uint32           `json:"version"`
	Binding         core.ID          `json:"binding"`
	Stage           core.ID          `json:"stage"`
	Decision        PolicyDecision   `json:"decision"`
}
type DecisionRecorder interface {
	RecordPolicyDecision(context.Context, PolicyAudit) error
}

type PolicyMode string

const (
	InternalContext             PolicyMode = "internal_context"
	ExternalContext             PolicyMode = "external_context"
	AssistantDisclosure         PolicyMode = "assistant_disclosure"
	SyntheticSelfDisclosure     PolicyMode = "synthetic_self_disclosure"
	CrossPerspectiveAbstraction PolicyMode = "cross_perspective_abstraction"
)

type ContextProposal struct {
	Version           uint32
	AuthorityRevision int64
	Query             MemoryQuery
	Binding           core.ID
	Recipient         core.ID
	Operation         core.Operation
	Mode              PolicyMode
	Sources           []core.ID
}

// ClauseEvidence carries only request-relative ordinals and public policy clause
// IDs. Denials never include source text, hidden identities, grants or labels.
type ClauseEvidence struct {
	SourceOrdinal int
	Clause        core.ID
}
type PolicyDecision struct {
	Allowed  bool
	Action   string
	Evidence []ClauseEvidence
}

func policyDeny(clause core.ID, ordinal int) PolicyDecision {
	return PolicyDecision{Action: "WAIT", Evidence: []ClauseEvidence{{ordinal, clause}}}
}

// ContextAuthority is trusted host evidence, not a client-supplied boolean. HWS
// must bind namespace/world/branch/principal when implementing this port.
type ContextAuthority interface {
	ValidateContext(context.Context, ContextProposal) (bool, error)
	IsFictional(context.Context, core.ID, core.ID) (bool, error)
}
type PolicyService struct {
	journal   MemoryJournal
	authority ContextAuthority
	recorder  DecisionRecorder
}

func NewPolicyService(journal MemoryJournal, authority ContextAuthority, recorder DecisionRecorder) *PolicyService {
	return &PolicyService{journal: journal, authority: authority, recorder: recorder}
}

type SafeContextItem struct {
	Source                             core.ID
	Observer                           core.ID
	Subject                            core.Subject
	Kind                               MemoryKind
	Confidence                         core.Confidence
	OccurredAt, LearnedAt              core.LogicalTime
	Valid                              core.Interval
	Parents, Supporting, Contradicting []core.ID
	Text                               string
}
type SafeContext struct {
	items    []SafeContextItem
	revision [32]byte
	knownAt  core.LogicalTime
}

// Revision and KnownAt bind trusted durable consumers to this validated snapshot.
func (s SafeContext) Revision() string          { return hex.EncodeToString(s.revision[:]) }
func (s SafeContext) KnownAt() core.LogicalTime { return s.knownAt }

func (s SafeContext) Items() []SafeContextItem {
	out := append([]SafeContextItem{}, s.items...)
	for i := range out {
		item := &out[i]
		if item.Subject.Reference != nil {
			owned := *item.Subject.Reference
			item.Subject.Reference = &owned
		}
		if item.Valid.End != nil {
			owned := *item.Valid.End
			item.Valid.End = &owned
		}
		item.Parents = append([]core.ID{}, item.Parents...)
		item.Supporting = append([]core.ID{}, item.Supporting...)
		item.Contradicting = append([]core.ID{}, item.Contradicting...)
	}
	return out
}

// ApprovedContext cannot be minted or rebound by a writer. It is process-local;
// restart reconstructs from a proposal and fresh policy evidence, not serialized
// approval. This is a capability boundary, not a sandbox for arbitrary Go code.
type ApprovedContext struct {
	issuer   *PolicyService
	proposal ContextProposal
	snapshot [32]byte
}

func (p ContextProposal) validate() bool {
	if p.Version != 1 || p.Query.Validate() != nil || p.Binding.Validate() != nil || p.Recipient.Validate() != nil || len(p.Sources) == 0 || !relationIDs(p.Sources, 16) {
		return false
	}
	if (core.Grant{Actor: p.Query.Actor, Recipient: p.Recipient, Purpose: p.Query.Purpose, Operation: p.Operation}).Validate() != nil {
		return false
	}
	switch p.Mode {
	case InternalContext:
		return p.Operation == core.Read || p.Operation == core.Derive || p.Operation == core.Retain || p.Operation == core.Attribute || p.Operation == core.Aggregate || p.Operation == core.Match
	case ExternalContext:
		return p.Operation == core.Read
	case AssistantDisclosure:
		return p.Operation == core.Disclose || p.Operation == core.Export || p.Operation == core.ShareOnRequest
	case SyntheticSelfDisclosure:
		return p.Operation == core.Disclose
	case CrossPerspectiveAbstraction:
		return true
	}
	return false
}
func (p *PolicyService) evaluate(ctx context.Context, proposal ContextProposal) (SafeContext, [32]byte, PolicyDecision, error) {
	if !proposal.validate() {
		return SafeContext{}, [32]byte{}, policyDeny("invalid_request", -1), nil
	}
	if proposal.Mode == CrossPerspectiveAbstraction {
		return SafeContext{}, [32]byte{}, policyDeny("abstraction_unproven", -1), nil
	}
	if p == nil || p.journal == nil {
		return SafeContext{}, [32]byte{}, policyDeny("authority_unavailable", -1), nil
	}
	if p.authority == nil {
		return SafeContext{}, [32]byte{}, policyDeny("context_binding_unknown", -1), nil
	}
	bound, err := p.authority.ValidateContext(ctx, proposal)
	if err != nil {
		return SafeContext{}, [32]byte{}, policyDeny("authority_unavailable", -1), ErrPolicyEvidence
	}
	if !bound {
		return SafeContext{}, [32]byte{}, policyDeny("context_binding_unknown", -1), nil
	}
	if proposal.Mode == SyntheticSelfDisclosure {
		if p.authority == nil || proposal.Query.Actor != proposal.Query.Scope.Owner {
			return SafeContext{}, [32]byte{}, policyDeny("fictional_identity_unverified", -1), nil
		}
		allowed, err := p.authority.IsFictional(ctx, proposal.Binding, proposal.Query.Actor)
		if err != nil {
			return SafeContext{}, [32]byte{}, policyDeny("authority_unavailable", -1), ErrPolicyEvidence
		}
		if !allowed {
			return SafeContext{}, [32]byte{}, policyDeny("fictional_identity_unverified", -1), nil
		}
	}
	entries, err := p.journal.ReadMemory(ctx, proposal.Query.Scope)
	if err != nil {
		return SafeContext{}, [32]byte{}, policyDeny("authority_unavailable", -1), ErrPolicyEvidence
	}
	idx, err := RebuildMemory(proposal.Query.Scope, entries)
	if err != nil {
		return SafeContext{}, [32]byte{}, policyDeny("authority_unavailable", -1), ErrPolicyEvidence
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		return SafeContext{}, [32]byte{}, policyDeny("authority_unavailable", -1), ErrPolicyEvidence
	}
	snapshot := sha256.Sum256(raw)
	safe := SafeContext{items: []SafeContextItem{}, revision: snapshot, knownAt: proposal.Query.KnownAt}
	total := 0
	for ordinal, id := range proposal.Sources {
		e, exists := idx.byID[id]
		if !exists || e.Revoked || e.Content == nil {
			return SafeContext{}, [32]byte{}, policyDeny("source_unavailable", ordinal), nil
		}
		q := proposal.Query
		q.Subject = e.Event.Subject
		q.Limit = 1
		picks := idx.selectMatchingMemory(q, func(r MemoryRecord) bool { return r.Event.Meta.ID == id })
		if len(picks) != 1 {
			return SafeContext{}, [32]byte{}, policyDeny("source_unavailable", ordinal), nil
		}
		seen := map[core.ID]bool{}
		var permitted func(core.ID) bool
		permitted = func(id core.ID) bool {
			if seen[id] {
				return true
			}
			seen[id] = true
			e, ok := idx.byID[id]
			if !ok || e.Revoked || e.Content == nil {
				return false
			}
			if !e.Event.Meta.Rights.Allows(core.PermissionRequest{Resource: id, Context: core.Grant{Actor: q.Actor, Recipient: proposal.Recipient, Purpose: q.Purpose, Operation: proposal.Operation}}) {
				return false
			}
			own := e.Event.Meta.Observer == q.Actor && e.Event.Subject.Principal == q.Actor && e.Event.Subject.Reference == nil
			if proposal.Mode == SyntheticSelfDisclosure && !own {
				return false
			}
			if proposal.Mode == AssistantDisclosure && e.Event.Meta.Sensitivity == core.Restricted && !own {
				return false
			}
			sources := memorySources(e.record())
			if e.Supersedes != "" {
				sources = append(sources, e.Supersedes)
			}
			for _, source := range sources {
				if !permitted(source) {
					return false
				}
			}
			return true
		}
		if !permitted(id) {
			return SafeContext{}, [32]byte{}, policyDeny("right_or_attribution_unavailable", ordinal), nil
		}
		total += len(e.Content.Text)
		if total > 4096 {
			return SafeContext{}, [32]byte{}, policyDeny("context_budget", ordinal), nil
		}
		learned, _ := learnedAt(*e.Content, q.Actor)
		safe.items = append(safe.items, SafeContextItem{Source: id, Observer: e.Event.Meta.Observer, Subject: e.Event.Subject, Kind: e.Content.Kind, Confidence: e.Event.Meta.Confidence, OccurredAt: e.Event.OccurredAt, LearnedAt: learned, Valid: e.Event.Meta.Valid, Parents: e.Event.Meta.Parents, Supporting: e.Event.Meta.Supporting, Contradicting: e.Event.Meta.Contradicting, Text: e.Content.Text})
	}
	encoded, err := json.Marshal(safe.Items())
	if err != nil || len(encoded) > 32768 {
		return SafeContext{}, [32]byte{}, policyDeny("context_budget", -1), nil
	}
	return safe, snapshot, PolicyDecision{Allowed: true, Action: "ALLOW", Evidence: []ClauseEvidence{{-1, "explicit_rights_current_lineage"}}}, nil
}
func (p *PolicyService) record(ctx context.Context, binding, stage core.ID, decision PolicyDecision, proof ...SafeContext) error {
	if p == nil || p.recorder == nil {
		return ErrPolicyAudit
	}
	if binding.Validate() != nil {
		binding = "unverified"
	}
	audit := PolicyAudit{Version: 1, Binding: binding, Stage: stage, Decision: decision}
	if decision.Allowed && len(proof) == 1 {
		raw, _ := json.Marshal(proof[0].Items())
		hash := sha256.Sum256(raw)
		audit.ContextHash = hex.EncodeToString(hash[:])
		audit.ContextRevision = proof[0].Revision()
		audit.KnownAt = proof[0].KnownAt()
	}
	if p.recorder.RecordPolicyDecision(ctx, audit) != nil {
		return ErrPolicyAudit
	}
	return nil
}

func (p *PolicyService) Approve(ctx context.Context, proposal ContextProposal) (approved ApprovedContext, decision PolicyDecision, resultErr error) {
	defer func() {
		if err := p.record(ctx, proposal.Binding, "approve", decision); err != nil {
			approved = ApprovedContext{}
			decision = policyDeny("audit_unavailable", -1)
			resultErr = err
		}
	}()

	// Detach caller-owned slices before retaining the proposal in a capability.
	raw, err := json.Marshal(proposal)
	if err != nil {
		return ApprovedContext{}, policyDeny("invalid_request", -1), err
	}
	var owned ContextProposal
	if err = json.Unmarshal(raw, &owned); err != nil {
		return ApprovedContext{}, policyDeny("invalid_request", -1), err
	}
	_, hash, decision, err := p.evaluate(ctx, owned)
	if err != nil || !decision.Allowed {
		return ApprovedContext{}, decision, err
	}
	return ApprovedContext{issuer: p, proposal: owned, snapshot: hash}, decision, nil
}
func (p *PolicyService) Revalidate(ctx context.Context, approved ApprovedContext, binding core.ID) (safeOut SafeContext, decisionOut PolicyDecision, resultErr error) {
	defer func() {
		if err := p.record(ctx, binding, "revalidate", decisionOut, safeOut); err != nil {
			safeOut = SafeContext{}
			decisionOut = policyDeny("audit_unavailable", -1)
			resultErr = err
		}
	}()

	if p == nil || approved.issuer != p || binding != approved.proposal.Binding {
		return SafeContext{}, policyDeny("invalid_capability", -1), nil
	}
	safe, hash, decision, err := p.evaluate(ctx, approved.proposal)
	if err != nil || !decision.Allowed {
		return SafeContext{}, decision, err
	}
	if hash != approved.snapshot {
		return SafeContext{}, policyDeny("stale_capability", -1), nil
	}
	return safe, decision, nil
}

type WriterSpan struct {
	Source     core.ID
	Start, End int
}
type WriterDraft struct {
	Spans   []WriterSpan
	Fiction *FictionDraft
}
type ApprovedWriter interface {
	Write(context.Context, SafeContext) (WriterDraft, error)
}
type ValidatedOutput struct {
	FictionMode string `json:",omitempty"`
	Text        string
	Sources     []core.ID
	Evidence    []SafeContextItem
}

// Write supports bounded quotations and explicit typed fictional transforms.
// Arbitrary paraphrase or
// free-form generation is not claimed safe by string filters. Untrusted source
// text cannot mint capabilities or choose new source rights. #10 supplies adapters.
func (p *PolicyService) Write(ctx context.Context, approved ApprovedContext, binding core.ID, writer ApprovedWriter) (output ValidatedOutput, decisionOut PolicyDecision, resultErr error) {
	defer func() {
		if err := p.record(ctx, binding, "writer_output", decisionOut); err != nil {
			output = ValidatedOutput{}
			decisionOut = policyDeny("audit_unavailable", -1)
			resultErr = err
		}
	}()

	safe, decision, err := p.Revalidate(ctx, approved, binding)
	if err != nil || !decision.Allowed {
		return ValidatedOutput{}, decision, err
	}
	if writer == nil || (approved.proposal.Mode != AssistantDisclosure && approved.proposal.Mode != SyntheticSelfDisclosure) {
		return ValidatedOutput{}, policyDeny("writer_not_authorized", -1), nil
	}
	draft, err := writer.Write(ctx, SafeContext{items: safe.Items()})
	if err != nil {
		return ValidatedOutput{}, policyDeny("writer_failed", -1), ErrPolicyWriter
	}
	// Providers run outside transactions. Revocation or evidence changes during a
	// writer call invalidate the output before it can be returned to its recipient.
	_, decision, err = p.Revalidate(ctx, approved, binding)
	if err != nil || !decision.Allowed {
		return ValidatedOutput{}, decision, err
	}
	if draft.Fiction != nil {
		if len(draft.Spans) != 0 {
			return ValidatedOutput{}, policyDeny("mixed_writer_protocol", -1), nil
		}
		return renderFiction(*draft.Fiction, safe, approved.proposal.Mode)
	}
	if len(draft.Spans) == 0 || len(draft.Spans) > 16 {
		return ValidatedOutput{}, policyDeny("invalid_writer_output", -1), nil
	}
	items := map[core.ID]SafeContextItem{}
	for _, item := range safe.items {
		items[item.Source] = item
	}
	out := ValidatedOutput{Sources: []core.ID{}}
	for i, span := range draft.Spans {
		item, ok := items[span.Source]
		text := item.Text
		if !ok || span.Start < 0 || span.End <= span.Start || span.End > len(text) || !utf8.ValidString(text[span.Start:span.End]) {
			return ValidatedOutput{}, policyDeny("invalid_writer_output", i), nil
		}
		if i > 0 {
			out.Text += " "
		}
		out.Text += text[span.Start:span.End]
		out.Sources = append(out.Sources, span.Source)
		item.Text = text[span.Start:span.End]
		out.Evidence = append(out.Evidence, item)
		if len(out.Text) > 4096 {
			return ValidatedOutput{}, policyDeny("output_budget", i), nil
		}
	}
	encoded, err := json.Marshal(out)
	if err != nil || len(encoded) > 32768 {
		return ValidatedOutput{}, policyDeny("output_budget", -1), nil
	}
	return out, decision, nil
}

// AbstractionAssessment is a versioned research record, never an authorization
// token. No guessed count/posterior/coalition threshold can enable production use.
type AbstractionAssessment struct {
	Version             uint32
	AttributionEvidence []core.ID
	NotesCode           core.ID
}

func (a AbstractionAssessment) Decision() (PolicyDecision, error) {
	if a.Version != 1 || a.NotesCode.Validate() != nil || !relationIDs(a.AttributionEvidence, 16) {
		return policyDeny("invalid_assessment", -1), fmt.Errorf("invalid research assessment")
	}
	if len(a.AttributionEvidence) == 0 {
		return policyDeny("attribution_unknown", -1), nil
	}
	return policyDeny("research_only_not_privacy_proof", -1), nil
}

// RevalidateInternal pins a cognition consumer to own-recipient derive authority;
// a disclosure/export capability cannot be silently repurposed as model context.
func (p *PolicyService) RevalidateInternal(ctx context.Context, approved ApprovedContext, binding, actor core.ID) (SafeContext, PolicyDecision, error) {
	if approved.proposal.Mode != InternalContext || approved.proposal.Operation != core.Derive || approved.proposal.Query.Actor != actor || approved.proposal.Recipient != actor {
		d := policyDeny("internal_use_mismatch", -1)
		if err := p.record(ctx, binding, "revalidate", d); err != nil {
			return SafeContext{}, policyDeny("audit_unavailable", -1), err
		}
		return SafeContext{}, d, nil
	}
	return p.Revalidate(ctx, approved, binding)
}
