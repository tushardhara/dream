package core

import (
	"encoding/json"
	"fmt"
	"sort"
)

// The registry bounds the first domain-aware contract. No arbitrary string is
// silently promoted to a new domain, and no domain supplies trust coefficients.
type RelationshipDomain string

const (
	Childcare             RelationshipDomain = "childcare"
	Finances              RelationshipDomain = "finances"
	Confidentiality       RelationshipDomain = "confidentiality"
	PracticalCoordination RelationshipDomain = "coordination"
	EmotionalSupport      RelationshipDomain = "emotional_support"
)

func (d RelationshipDomain) Valid() bool {
	return d == Childcare || d == Finances || d == Confidentiality || d == PracticalCoordination || d == EmotionalSupport
}

const RelationshipFocusVersion = "relationship-focus.v1"

// RoleContext names an explicit frame supported by each account's ContextSource;
// it supplies no numerical role defaults. Account chooses among conflicting accounts. Neither field
// grants access; both must resolve against currently permitted evidence.
type RelationshipFocus struct {
	Version     string             `json:"version"`
	Domain      RelationshipDomain `json:"domain"`
	RoleContext ID                 `json:"role_context,omitempty"`
	Account     ID                 `json:"account,omitempty"`
}

func (f RelationshipFocus) Validate() error {
	if f.Version != RelationshipFocusVersion || !f.Domain.Valid() || f.RoleContext != "" && f.RoleContext.Validate() != nil || f.Account != "" && f.Account.Validate() != nil {
		return fmt.Errorf("invalid relationship focus")
	}
	return nil
}

type RelationshipSelection struct {
	Status       string
	Account      *RelationshipContext
	Alternatives []ID
}

// SelectRelationship never folds conflicting accounts into a single vector.
// Only the requested observer/pair/domain participates; unscoped legacy vectors
// are not inferred to apply to a domain. An explicit account may choose a view
// without deleting other views from the underlying evidence log.
func SelectRelationship(records []RelationshipContext, focus RelationshipFocus, observer, other ID, at LogicalTime) (RelationshipSelection, error) {
	if focus.Validate() != nil || ids(observer, other) != nil || observer == other || at.Validate() != nil || len(records) > 16 {
		return RelationshipSelection{}, fmt.Errorf("invalid domain selection")
	}
	out := RelationshipSelection{Status: "unknown", Alternatives: []ID{}}
	seen := map[[2]ID]bool{}
	candidates := []RelationshipContext{}
	explicitFound := false
	for _, r := range records {
		if r.Validate(r.Observer, r.Other) != nil {
			return RelationshipSelection{}, fmt.Errorf("invalid relationship account")
		}
		if r.Version != 2 {
			continue
		}
		key := [2]ID{r.Observer, r.Account}
		if seen[key] {
			return RelationshipSelection{}, fmt.Errorf("duplicate relationship account")
		}
		seen[key] = true
		if r.Observer != observer || r.Other != other {
			continue
		}
		matches := r.Domain == focus.Domain && (focus.RoleContext == "" || r.RoleContext == focus.RoleContext) && r.Valid.Start <= at && (r.Valid.End == nil || at < *r.Valid.End)
		if focus.Account == r.Account {
			explicitFound = true
			if !matches {
				return RelationshipSelection{}, fmt.Errorf("explicit account outside requested context")
			}
		}
		if !matches {
			continue
		}
		out.Alternatives = append(out.Alternatives, r.Account)
		if focus.Account == "" || focus.Account == r.Account {
			candidates = append(candidates, r)
		}
	}
	sort.Slice(out.Alternatives, func(i, j int) bool { return out.Alternatives[i] < out.Alternatives[j] })
	if focus.Account != "" && !explicitFound {
		return out, nil
	}
	if len(candidates) > 1 {
		out.Status = "ambiguous"
		return out, nil
	}
	if len(candidates) == 1 {
		raw, _ := json.Marshal(candidates[0])
		var owned RelationshipContext
		_ = json.Unmarshal(raw, &owned)
		out.Status = "selected"
		out.Account = &owned
	}
	return out, nil
}

// BindRelationshipDomain is an explicit migration. Legacy numeric measures are
// deliberately not copied: the caller must author domain-specific evidence.
// Passing no measures preserves unknown trust/expectations in the new account.
func BindRelationshipDomain(legacy RelationshipContext, account ID, domain RelationshipDomain, roleContext, contextSource ID, measures []RelationshipMeasure) (RelationshipContext, error) {
	if legacy.Version != 1 || legacy.Validate(legacy.Observer, legacy.Other) != nil {
		return RelationshipContext{}, fmt.Errorf("legacy relationship required")
	}
	raw, _ := json.Marshal(legacy)
	var out RelationshipContext
	_ = json.Unmarshal(raw, &out)
	out.Version = 2
	out.Account = account
	out.Domain = domain
	out.RoleContext = roleContext
	out.ContextSource = contextSource
	out.Measures = append([]RelationshipMeasure{}, measures...)
	if out.Validate(out.Observer, out.Other) != nil {
		return RelationshipContext{}, fmt.Errorf("invalid explicit domain migration")
	}
	return out, nil
}
