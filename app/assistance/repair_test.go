package assistance

import (
	"github.com/tushardhara/dream/core"
	"testing"
)

func TestRepairRequestDirectContract(t *testing.T) {
	good := RepairRequest{Version: RepairFlowVersion, ID: "request", Session: "session", Helper: "helper", User: "bob", Actor: "alice", Recipient: "bob", Purpose: "help", Focus: core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: core.Finances, RoleContext: "household"}, At: 1}
	if e := good.Validate(); e != nil {
		t.Fatal("valid requesting recipient", e)
	}
	sender := good
	sender.User = "alice"
	if e := sender.Validate(); e != nil {
		t.Fatal("valid requesting sender", e)
	}
	for name, change := range map[string]func(*RepairRequest){
		"version": func(r *RepairRequest) { r.Version = "old" }, "request": func(r *RepairRequest) { r.ID = "" }, "session": func(r *RepairRequest) { r.Session = "" }, "helper": func(r *RepairRequest) { r.Helper = "" }, "purpose": func(r *RepairRequest) { r.Purpose = "" }, "foreign user": func(r *RepairRequest) { r.User = "outsider" }, "self pair": func(r *RepairRequest) { r.Actor = r.Recipient }, "helper is actor": func(r *RepairRequest) { r.Helper = r.Actor }, "helper is recipient": func(r *RepairRequest) { r.Helper = r.Recipient }, "negative time": func(r *RepairRequest) { r.At = -1 }, "missing frame": func(r *RepairRequest) { r.Focus.RoleContext = "" }, "foreign account": func(r *RepairRequest) { r.Focus.Account = "private" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := good
			change(&bad)
			if bad.Validate() == nil {
				t.Fatal("invalid direct request accepted")
			}
		})
	}
}
