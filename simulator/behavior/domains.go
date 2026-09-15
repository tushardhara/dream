package behavior

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/dynamics"
)

const DomainPolicy = "domain-human-actions.v1"

type DomainMemory struct {
	Domain      core.RelationshipDomain
	RoleContext core.ID
	Memory      []Memory
}
type DomainOutcome struct {
	Focus   core.RelationshipFocus
	Profile core.RelationshipContext
	Outcome ActionOutcome
}

// DomainActor keeps learned relationship memory outside the unscoped v2 actor.
// The common engine still performs appraisal, choice and observed-outcome updates.
// No automatic transfer rule exists across domains or context frames.
type DomainActor struct {
	Version  string
	Human    ScopedActor
	Memories []DomainMemory
	Outcomes []DomainOutcome
}

func NewDomainActor(id core.ID, at core.LogicalTime) (DomainActor, error) {
	a, e := NewScopedActor(id, at)
	return DomainActor{Version: DomainPolicy, Human: a, Memories: []DomainMemory{}, Outcomes: []DomainOutcome{}}, e
}
func (a DomainActor) Validate() error {
	if a.Version != DomainPolicy || a.Human.Validate() != nil || len(a.Human.Human.Memory) != 0 || len(a.Human.Human.Beliefs) != 0 || len(a.Memories) > 8 || len(a.Outcomes) > 16 {
		return fmt.Errorf("invalid domain actor")
	}
	seen := map[string]bool{}
	for _, m := range a.Memories {
		key := string(m.Domain) + ":" + string(m.RoleContext)
		if !m.Domain.Valid() || m.RoleContext.Validate() != nil || seen[key] || validateActionMemory(m.Memory, a.Human.Human.Drives.Actor) != nil {
			return fmt.Errorf("invalid domain memory")
		}
		seen[key] = true
	}
	ids := map[core.ID]bool{}
	for _, o := range a.Outcomes {
		if o.Focus.Validate() != nil || o.Focus.RoleContext == "" || o.Focus.Account == "" || o.Profile.Version != 2 || o.Profile.Validate(a.Human.Human.Drives.Actor, o.Profile.Other) != nil || o.Profile.Account != o.Focus.Account || o.Profile.Domain != o.Focus.Domain || o.Profile.RoleContext != o.Focus.RoleContext || o.Outcome.Validate() != nil || o.Outcome.Outcome.Observer != a.Human.Human.Drives.Actor || ids[o.Outcome.Outcome.Decision] {
			return fmt.Errorf("invalid domain outcome")
		}
		ids[o.Outcome.Outcome.Decision] = true
	}
	return nil
}
func domainCopy[T any](value T) T {
	raw, _ := json.Marshal(value)
	var out T
	_ = json.Unmarshal(raw, &out)
	return out
}
func domainMemory(a DomainActor, f core.RelationshipFocus) []Memory {
	for _, m := range a.Memories {
		if m.Domain == f.Domain && m.RoleContext == f.RoleContext {
			return domainCopy(m.Memory)
		}
	}
	return []Memory{}
}
func storeDomainMemory(a *DomainActor, f core.RelationshipFocus, memory []Memory) {
	for i, m := range a.Memories {
		if m.Domain == f.Domain && m.RoleContext == f.RoleContext {
			a.Memories[i].Memory = domainCopy(memory)
			return
		}
	}
	a.Memories = append(a.Memories, DomainMemory{f.Domain, f.RoleContext, domainCopy(memory)})
}

type DomainSituation struct {
	Scoped   ScopedSituation
	Focus    core.RelationshipFocus
	Accounts []core.RelationshipContext
}
type DomainDecision struct {
	Version      string
	Focus        core.RelationshipFocus
	Selection    string
	AccountHash  string
	EvidenceHash string
	Human        ScopedDecision
}

// ApplyDomainRelationship accepts a selected, currently permitted domain account.
// Context-frame uncertainty discounts its measures, in addition to the original
// measure-source discount. This never changes data rights or interpersonal consent.
func ApplyDomainRelationship(s ActionSituation, r core.RelationshipContext, actor core.ID, at core.LogicalTime) (ActionSituation, error) {
	if r.Version != 2 || r.Validate(actor, r.Other) != nil {
		return ActionSituation{}, fmt.Errorf("domain account required")
	}
	metadata, e := relationshipEvidence(s, actor, at)
	if e != nil {
		return ActionSituation{}, e
	}
	for _, id := range append(r.Sources(), r.Account) {
		if _, ok := metadata[id]; !ok {
			return ActionSituation{}, fmt.Errorf("domain evidence not currently retrieved")
		}
	}
	frame := metadata[r.ContextSource]
	account := metadata[r.Account]
	owned := domainCopy(r)
	for i := range owned.Measures {
		owned.Measures[i].Confidence *= frame.Confidence * account.Confidence
	}
	owned.Version = 1
	owned.Account = ""
	owned.Domain = ""
	owned.RoleContext = ""
	owned.ContextSource = ""
	return ApplyRelationship(s, owned, actor, at, true)
}
func ChooseDomainAction(a DomainActor, in DomainSituation, at core.LogicalTime, draw uint64) (DomainActor, DomainDecision, error) {
	actor := a.Human.Human.Drives.Actor
	if a.Validate() != nil || in.Focus.Validate() != nil || in.Scoped.Scope.Topic != core.ID(in.Focus.Domain) || in.Scoped.Scope.Initiator != actor || in.Scoped.Scope.Class == core.SummarySharing && in.Focus.Domain != core.Confidentiality {
		return DomainActor{}, DomainDecision{}, fmt.Errorf("invalid domain choice")
	}
	s := domainCopy(in.Scoped)
	if len(s.Situation.Contexts) != 0 || len(s.Situation.Relationships) != 0 || len(s.Situation.Beliefs) != 0 {
		return DomainActor{}, DomainDecision{}, fmt.Errorf("unscoped relationship input")
	}
	for _, r := range in.Accounts {
		if r.Observer != actor {
			return DomainActor{}, DomainDecision{}, fmt.Errorf("foreign private account in human inputs")
		}
	}
	selected, e := core.SelectRelationship(in.Accounts, in.Focus, actor, s.Scope.Target, at)
	if e != nil {
		return DomainActor{}, DomainDecision{}, e
	}
	out := domainCopy(a)
	focus := in.Focus
	out.Human.Human.Memory = []Memory{}
	if selected.Status == "selected" {
		focus.RoleContext = selected.Account.RoleContext
		focus.Account = selected.Account.Account
		s.Situation, e = ApplyDomainRelationship(s.Situation, *selected.Account, actor, at)
		if e != nil {
			return DomainActor{}, DomainDecision{}, e
		}
		memory := domainMemory(out, focus)
		metadata, e := relationshipEvidence(s.Situation, actor, at)
		if e != nil {
			return DomainActor{}, DomainDecision{}, e
		}
		for _, m := range memory {
			for _, id := range m.Evidence {
				if _, ok := metadata[id]; !ok {
					return DomainActor{}, DomainDecision{}, fmt.Errorf("retained domain learning lacks current evidence")
				}
			}
		}
		out.Human.Human.Memory = memory
	} else {
		offers := []ActionOffer{}
		for _, o := range s.Situation.Offers {
			if o.Kind == Wait || o.Kind == Leave || o.Kind == Withdraw {
				offers = append(offers, o)
			}
		}
		s.Situation.Offers = offers
	}
	// Childcare/financial trust does not create confidentiality eligibility, even
	// with a separately valid data-disclosure grant supplied by a caller.
	if in.Focus.Domain != core.Confidentiality {
		offers := []ActionOffer{}
		for _, o := range s.Situation.Offers {
			if o.Mode == "" || o.Mode == Silence {
				offers = append(offers, o)
			}
		}
		s.Situation.Offers = offers
	}
	next, d, e := ChooseScopedAction(out.Human, s, at, draw)
	if e != nil {
		return DomainActor{}, DomainDecision{}, e
	}
	next.Human.Memory = []Memory{}
	next.Human.Beliefs = []Belief{}
	out.Human = next
	chosen := d.Human.Candidates[d.Human.Selected].Offer
	if selected.Status == "selected" && chosen.Recipient != "" && chosen.Kind != Wait {
		outcome, e := TrackAction(d.Human, s.Situation.Horizon)
		if e != nil {
			return DomainActor{}, DomainDecision{}, e
		}
		out.Outcomes = append(out.Outcomes, DomainOutcome{focus, domainCopy(*selected.Account), outcome})
	}
	if out.Validate() != nil {
		return DomainActor{}, DomainDecision{}, fmt.Errorf("domain actor budget")
	}
	return out, DomainDecision{Version: DomainPolicy, Focus: focus, Selection: selected.Status, AccountHash: scopedHash(in.Accounts), EvidenceHash: scopedHash(in.Scoped.Situation.RelationshipEvidence), Human: d}, nil
}

// ResolveDomainAction selects the domain from the retained decision receipt, not
// a caller's domain label. Only observed, permissioned, time-valid evidence can
// update that exact domain/context bucket through the existing learning engine.
func ResolveDomainAction(a DomainActor, decision core.ID, evidence dynamics.Perceived, current []dynamics.Perceived, other core.ID, response string) (DomainActor, error) {
	if a.Validate() != nil {
		return DomainActor{}, fmt.Errorf("invalid domain actor")
	}
	out := domainCopy(a)
	for i, pending := range out.Outcomes {
		if pending.Outcome.Outcome.Decision != decision {
			continue
		}
		human := out.Human.Human
		human.Memory = domainMemory(out, pending.Focus)
		if len(current) > 16 {
			return DomainActor{}, fmt.Errorf("current domain evidence bound")
		}
		present := map[core.ID]bool{}
		for _, p := range current {
			if present[p.Event] || p.Validate(human.Drives.Actor, evidence.LearnedAt) != nil {
				return DomainActor{}, fmt.Errorf("invalid current outcome context")
			}
			present[p.Event] = true
		}
		required := append(pending.Profile.Sources(), pending.Profile.Account)
		for _, m := range human.Memory {
			required = append(required, m.Evidence...)
		}
		for _, id := range required {
			if !present[id] {
				return DomainActor{}, fmt.Errorf("domain outcome context no longer permitted")
			}
		}
		learned, outcome, e := ResolveAction(human, pending.Outcome, evidence, other, response)
		if e != nil {
			return DomainActor{}, e
		}
		for i, m := range learned.Memory {
			if m.Other == other {
				for _, id := range append(pending.Profile.Sources(), pending.Profile.Account) {
					found := false
					for _, old := range learned.Memory[i].Evidence {
						found = found || old == id
					}
					if !found {
						learned.Memory[i].Evidence = append(learned.Memory[i].Evidence, id)
					}
				}
			}
		}
		storeDomainMemory(&out, pending.Focus, learned.Memory)
		out.Outcomes[i].Outcome = outcome
		if out.Validate() != nil {
			return DomainActor{}, fmt.Errorf("domain memory budget")
		}
		return out, nil
	}
	return DomainActor{}, fmt.Errorf("domain decision not retained")
}
func (a DomainActor) Encode() ([]byte, error) {
	if a.Validate() != nil {
		return nil, fmt.Errorf("invalid domain actor")
	}
	raw, e := json.Marshal(a)
	if e != nil || len(raw) > 131072 {
		return nil, fmt.Errorf("domain actor byte bound")
	}
	return raw, nil
}
func DecodeDomainActor(raw []byte) (DomainActor, error) {
	var out DomainActor
	if len(raw) > 131072 {
		return out, fmt.Errorf("domain actor byte bound")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&out) != nil || d.Decode(new(any)) != io.EOF || out.Validate() != nil {
		return DomainActor{}, fmt.Errorf("invalid domain actor encoding")
	}
	return out, nil
}
