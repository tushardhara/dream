package core

import (
	"reflect"
	"testing"
)

func domainProfile(account, observer, other ID, domain RelationshipDomain, frame ID, trust float64) RelationshipContext {
	return RelationshipContext{Version: 2, Account: account, Domain: domain, RoleContext: frame, ContextSource: ID(string(observer) + "-frame"), Observer: observer, Other: other, Types: []ID{"sibling", "business_partner"}, Valid: Interval{}, Measures: []RelationshipMeasure{{Kind: "trust", Value: trust, Confidence: .8, Source: ID(string(account) + "-outcome")}, {Kind: "expectation", Value: trust, Confidence: .7, Source: ID(string(account) + "-outcome")}}}
}
func domainFocus(domain RelationshipDomain, frame ID) RelationshipFocus {
	return RelationshipFocus{Version: RelationshipFocusVersion, Domain: domain, RoleContext: frame}
}
func TestDomainSelectionKeepsOverlappingContradictoryAccounts(t *testing.T) {
	care := domainProfile("care", "a", "b", Childcare, "family", .9)
	business := domainProfile("business", "a", "b", Childcare, "business", -.8)
	finance := domainProfile("finance", "a", "b", Finances, "family", -.6)
	opposite := domainProfile("care", "b", "a", Childcare, "family", -.5)
	records := []RelationshipContext{care, business, finance, opposite}
	for _, want := range []RelationshipContext{care, business, finance, opposite} {
		out, e := SelectRelationship(records, domainFocus(want.Domain, want.RoleContext), want.Observer, want.Other, 1)
		if e != nil || out.Status != "selected" || !reflect.DeepEqual(*out.Account, want) {
			t.Fatal("domain/context/observer selection", out, e)
		}
	}
	out, e := SelectRelationship(records, domainFocus(Childcare, ""), "a", "b", 1)
	if e != nil || out.Status != "ambiguous" || out.Account != nil || len(out.Alternatives) != 2 {
		t.Fatal("ambiguous roles collapsed", out, e)
	}
	conflict := domainProfile("conflict", "a", "b", Childcare, "family", -.9)
	records = append(records, conflict)
	out, e = SelectRelationship(records, domainFocus(Childcare, "family"), "a", "b", 1)
	if e != nil || out.Status != "ambiguous" || len(out.Alternatives) != 2 {
		t.Fatal("conflict overwritten", out, e)
	}
	choice := domainFocus(Childcare, "family")
	choice.Account = "care"
	out, e = SelectRelationship(records, choice, "a", "b", 1)
	if e != nil || out.Status != "selected" || out.Account.Account != "care" || len(out.Alternatives) != 2 {
		t.Fatal("explicit choice lost contrary account", out, e)
	}
	out.Account.Measures[0].Value = 0
	if records[0].Measures[0].Value != .9 {
		t.Fatal("selection aliases source")
	}
	choice.Domain = Finances
	if _, e := SelectRelationship(records, choice, "a", "b", 1); e == nil {
		t.Fatal("explicit record escaped domain")
	}
}
func TestDomainsAreBoundedAndLegacyTrustDoesNotTransfer(t *testing.T) {
	legacy := RelationshipContext{Version: 1, Observer: "a", Other: "b", Types: []ID{"spouse"}, Valid: Interval{}, Measures: []RelationshipMeasure{{Kind: "trust", Value: 1, Confidence: 1, Source: "general"}}}
	migrated, e := BindRelationshipDomain(legacy, "migrated", Childcare, "family", "frame", nil)
	if e != nil || len(migrated.Measures) != 0 || len(legacy.Measures) != 1 {
		t.Fatal("migration manufactured scoped trust", migrated, e)
	}
	if out, e := SelectRelationship([]RelationshipContext{legacy, migrated}, domainFocus(Finances, "family"), "a", "b", 1); e != nil || out.Status != "unknown" || out.Account != nil {
		t.Fatal("legacy/childcare trust transferred", out, e)
	}
	bad := migrated
	bad.Domain = "new-arbitrary-domain"
	if bad.Validate("a", "b") == nil {
		t.Fatal("unbounded domain proliferation")
	}
	bad = migrated
	bad.Version = 1
	if bad.Validate("a", "b") == nil {
		t.Fatal("legacy version accepted domain fields")
	}
	bad = migrated
	bad.ContextSource = bad.Account
	if bad.Validate("a", "b") == nil {
		t.Fatal("account authorized its own frame")
	}
	if _, e := SelectRelationship([]RelationshipContext{migrated, migrated}, domainFocus(Childcare, "family"), "a", "b", 1); e == nil {
		t.Fatal("duplicate account")
	}
	migrated.Valid.Start = 2
	if out, e := SelectRelationship([]RelationshipContext{migrated}, domainFocus(Childcare, "family"), "a", "b", 1); e != nil || out.Status != "unknown" {
		t.Fatal("future account used", out, e)
	}
}
