package behavior

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/tushardhara/dream/core"
)

const ScopedPolicy = "scoped-human-actions.v1"

type ScopedContact struct {
	With, Topic core.ID
	State       string
	At          core.LogicalTime
	Evidence    core.ID
}

// ScopedActor is a new envelope, not a reinterpretation of ActionActor.Contact.
// The embedded v2 actor stays engaged; this envelope owns relationship contact.
// Legacy left/withdrawn actors require an explicit migration decision, not a guess
// about which relationship they meant. Existing v2 codecs/engines are unchanged.
type ScopedActor struct {
	Version  string
	Human    ActionActor
	Contacts []ScopedContact
}

func NewScopedActor(id core.ID, at core.LogicalTime) (ScopedActor, error) {
	a, e := NewActionActor(id, at)
	return ScopedActor{Version: ScopedPolicy, Human: a, Contacts: []ScopedContact{}}, e
}
func (a ScopedActor) Validate() error {
	if a.Version != ScopedPolicy || a.Human.Validate() != nil || a.Human.Contact != "engaged" || len(a.Contacts) > 16 {
		return fmt.Errorf("invalid scoped actor")
	}
	seen := map[[2]core.ID]bool{}
	for _, c := range a.Contacts {
		key := [2]core.ID{c.With, c.Topic}
		if c.With.Validate() != nil || c.With == a.Human.Drives.Actor || c.Topic.Validate() != nil || c.At.Validate() != nil || c.Evidence.Validate() != nil || c.State != "left" && c.State != "withdrawn" && c.State != "engaged" || seen[key] {
			return fmt.Errorf("invalid scoped contact")
		}
		seen[key] = true
	}
	return nil
}

type ScopedSituation struct {
	Scope      core.InteractionScope
	Situation  ActionSituation
	Boundaries []core.Boundary
}
type ScopedDecision struct {
	Version      string
	Scope        core.InteractionScope
	BoundaryHash string
	Human        ActionDecision
}

func scopedHash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func scopedContact(a ScopedActor, scope core.InteractionScope) string {
	state := "engaged"
	for _, c := range a.Contacts {
		if c.With == scope.Target && (c.Topic == scope.Topic || c.Topic == core.AllTopics) {
			if c.State == "left" {
				return "left"
			}
			if c.State == "withdrawn" {
				state = "withdrawn"
			}
		}
	}
	return state
}

// The trusted intent scope must agree with the executable action. Data disclosure
// and third-party support cannot be relabelled as an ordinary conversation.
func actionScopeClass(o ActionOffer) core.InteractionClass {
	if o.Kind == SeekThirdPartySupport {
		return core.ThirdPartyInvolvement
	}
	if o.Mode != "" && o.Mode != Silence {
		return core.SummarySharing
	}
	if o.Kind == Coordinate || o.Kind == Invite || o.Kind == Promise || o.Kind == Help {
		return core.Coordination
	}
	if o.Kind == Ask {
		return core.Clarification
	}
	return core.Discussion
}
func ChooseScopedAction(a ScopedActor, input ScopedSituation, at core.LogicalTime, draw uint64) (ScopedActor, ScopedDecision, error) {
	if a.Validate() != nil || input.Scope.Validate() != nil || input.Scope.Initiator != a.Human.Drives.Actor {
		return ScopedActor{}, ScopedDecision{}, fmt.Errorf("invalid scoped choice")
	}
	eligibility, e := core.EvaluateBoundaries(input.Boundaries, input.Scope, at)
	if e != nil {
		return ScopedActor{}, ScopedDecision{}, e
	}
	raw, _ := json.Marshal(input.Situation)
	var s ActionSituation
	_ = json.Unmarshal(raw, &s)
	contact := scopedContact(a, input.Scope)
	offers := []ActionOffer{}
	for _, o := range s.Offers {
		if o.Recipient != "" && o.Recipient != input.Scope.Target && o.Recipient != input.Scope.Via {
			return ScopedActor{}, ScopedDecision{}, fmt.Errorf("offer does not match scoped relationship")
		}
		switch o.Kind {
		case Wait:
			offers = append(offers, o)
		case Leave, Withdraw:
			// A person can end/pause without another person's consent. No notification is
			// sent through a refused channel; the target remains in the scoped envelope.
			o.Recipient = ""
			offers = append(offers, o)
		default:
			if eligibility.Allowed && (contact != "withdrawn" || o.Kind == Reconnect) && actionScopeClass(o) == input.Scope.Class {
				offers = append(offers, o)
			}
		}
	}
	s.Offers = offers
	human := a.Human
	human.Contact = contact
	next, decision, e := ChooseAction(human, s, at, draw)
	if e != nil {
		return ScopedActor{}, ScopedDecision{}, e
	}
	out := ScopedActor{Version: ScopedPolicy, Human: next, Contacts: append([]ScopedContact{}, a.Contacts...)}
	selected := decision.Candidates[decision.Selected].Offer
	if selected.Kind == Leave || selected.Kind == Withdraw || selected.Kind == Reconnect {
		state := "engaged"
		if selected.Kind == Leave {
			state = "left"
		}
		if selected.Kind == Withdraw {
			state = "withdrawn"
		}
		found := false
		for i, c := range out.Contacts {
			if c.With == input.Scope.Target && (c.Topic == core.AllTopics || selected.Kind == Reconnect && c.Topic == input.Scope.Topic) {
				out.Contacts[i].State = state
				out.Contacts[i].At = at
				out.Contacts[i].Evidence = decision.Event
				found = true
			}
		}
		if !found {
			out.Contacts = append(out.Contacts, ScopedContact{With: input.Scope.Target, Topic: core.AllTopics, State: state, At: at, Evidence: decision.Event})
		}
	}
	out.Human.Contact = "engaged"
	if out.Validate() != nil {
		return ScopedActor{}, ScopedDecision{}, fmt.Errorf("scoped contact budget")
	}
	return out, ScopedDecision{Version: ScopedPolicy, Scope: input.Scope, BoundaryHash: scopedHash(input.Boundaries), Human: decision}, nil
}
func (a ScopedActor) Encode() ([]byte, error) {
	if a.Validate() != nil {
		return nil, fmt.Errorf("invalid scoped actor")
	}
	b, e := json.Marshal(a)
	if e != nil || len(b) > 131072 {
		return nil, fmt.Errorf("scoped actor byte bound")
	}
	return b, nil
}
func DecodeScopedActor(raw []byte) (ScopedActor, error) {
	var a ScopedActor
	if len(raw) > 131072 {
		return a, fmt.Errorf("scoped actor byte bound")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&a) != nil || d.Decode(new(any)) != io.EOF || a.Validate() != nil {
		return ScopedActor{}, fmt.Errorf("invalid scoped actor encoding")
	}
	return a, nil
}
