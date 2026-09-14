package domainexperiment

import (
	"context"
	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
	"reflect"
	"testing"
)

func TestActualDomainConsumersAndReplay(t *testing.T) {
	family := core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: core.Childcare, RoleContext: "family"}
	a, e := Run(context.Background(), family, nil)
	if e != nil || !a.Helper.Delivered || a.Human.Selection != "selected" {
		t.Fatal(a, e)
	}
	bfocus := family
	bfocus.RoleContext = "business"
	b, e := Run(context.Background(), bfocus, nil)
	if e != nil || b.Helper.Delivered || reflect.DeepEqual(a.Human.Human.Human.Candidates, b.Human.Human.Human.Candidates) {
		t.Fatal("context ignored by consumers", e)
	}
	for _, focus := range []core.RelationshipFocus{{Version: core.RelationshipFocusVersion, Domain: core.Childcare}, {Version: core.RelationshipFocusVersion, Domain: core.Finances, RoleContext: "family"}} {
		unknown, e := Run(context.Background(), focus, nil)
		if e != nil || unknown.Helper.Delivered || len(unknown.Human.Human.Human.Candidates) != 1 {
			t.Fatal("unknown domain/frame facilitated", e)
		}
	}
	replay, e := Run(context.Background(), family, &a)
	if e != nil || assistance.Digest(a) != assistance.Digest(replay) {
		t.Fatal("mechanical replay", e)
	}
	if _, e = Run(context.Background(), bfocus, &a); e == nil {
		t.Fatal("context relabelled replay")
	}
	a.Human.AccountHash = "forged"
	if _, e = Run(context.Background(), family, &a); e == nil {
		t.Fatal("human evidence replay forged")
	}
}
