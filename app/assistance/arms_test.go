package assistance

import (
	"testing"

	"github.com/tushardhara/dream/core"
)

// The opt-in arm envelopes must not let an arm be carried silently on a v1
// request, and must not let a v2 request omit one. Either way an evaluation
// ends up with four labels for one policy, which is precisely the fabricated
// comparison the arms exist to make impossible.
func TestArmAndVersionMustAgree(t *testing.T) {
	group := GroupRequest{Version: GroupFlowVersion, ID: "r", Session: "s", User: "bob",
		Helper: "helper", Decision: "d", Purpose: "help", At: 1}
	if e := group.Validate(); e != nil {
		t.Fatal("positive control: v1 group request rejected:", e)
	}
	withArm := group
	withArm.Arm = Multi
	if withArm.Validate() == nil {
		t.Fatal("a v1 group request carried an arm that the v1 path would ignore")
	}
	armed := group
	armed.Version = GroupArmVersion
	if armed.Validate() == nil {
		t.Fatal("a v2 group request with no arm was accepted")
	}
	armed.Arm = Multi
	if e := armed.Validate(); e != nil {
		t.Fatal("a valid v2 group request was rejected:", e)
	}

	repair := RepairRequest{Version: RepairFlowVersion, ID: "r", Session: "s", Helper: "helper",
		User: "bob", Actor: "alice", Recipient: "bob", Purpose: "help",
		Focus: core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: core.Finances, RoleContext: "household"}, At: 1}
	if e := repair.Validate(); e != nil {
		t.Fatal("positive control: v1 repair request rejected:", e)
	}
	rArm := repair
	rArm.Arm = Multi
	if rArm.Validate() == nil {
		t.Fatal("a v1 repair request carried an arm that the v1 path would ignore")
	}
	rArmed := repair
	rArmed.Version = RepairArmVersion
	if rArmed.Validate() == nil {
		t.Fatal("a v2 repair request with no arm was accepted")
	}
	rArmed.Arm = None
	if e := rArmed.Validate(); e != nil {
		t.Fatal("a valid v2 repair request was rejected:", e)
	}
}
