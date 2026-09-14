package behavior

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/tushardhara/dream/core"
)

// ActionPolicy is opt-in for new cognition runs. Policy and Registry remain the
// frozen human-actions.v1 contract; no old choice acquires these semantics.
const ActionPolicy = "human-actions.v2"
const (
	Say                   Kind = "say"
	Answer                Kind = "answer"
	Reveal                Kind = "reveal"
	PartiallyReveal       Kind = "partially_reveal"
	Hide                  Kind = "hide"
	Lie                   Kind = "lie"
	Joke                  Kind = "joke"
	Challenge             Kind = "challenge"
	Apologize             Kind = "apologize"
	Support               Kind = "support"
	Complain              Kind = "complain"
	Argue                 Kind = "argue"
	Withdraw              Kind = "withdraw"
	Coordinate            Kind = "coordinate"
	Promise               Kind = "promise"
	Ignore                Kind = "ignore"
	Delay                 Kind = "delay"
	ChangeTopic           Kind = "change_topic"
	SeekThirdPartySupport Kind = "seek_third_party_support"
	Reconnect             Kind = "reconnect"
	Leave                 Kind = "leave"
)

type DisclosureMode string

const (
	Full        DisclosureMode = "full"
	Partial     DisclosureMode = "partial"
	Softened    DisclosureMode = "softened"
	Humorous    DisclosureMode = "joke"
	Deflection  DisclosureMode = "deflection"
	FalseClaim  DisclosureMode = "lie"
	Omission    DisclosureMode = "omission"
	TopicChange DisclosureMode = "topic_change"
	Silence     DisclosureMode = "silence"
)

func (m DisclosureMode) Valid() bool {
	switch m {
	case Full, Partial, Softened, Humorous, Deflection, FalseClaim, Omission, TopicChange, Silence:
		return true
	}
	return false
}

// ActionDefinition is the complete source-ordered semantics table. An utterance
// emits only a typed speech act unless separately authorized fictional content is
// present. It does not imply agreement, assistance, consent or a later response.
type ActionDefinition struct {
	Kind       Kind           `json:"kind"`
	Recipient  string         `json:"recipient"`
	Evidence   bool           `json:"evidence"`
	Effect     string         `json:"effect"`
	Disclosure DisclosureMode `json:"disclosure,omitempty"`
	Resolution string         `json:"resolution"`
}

var actionDefinitions = [...]ActionDefinition{
	{Say, "required", true, "statement", "", "observed_response"},
	{Ask, "required", true, "question", "", "observed_response"},
	{Answer, "required", true, "answer", "", "observed_response"},
	{Reveal, "required", true, "own_disclosure", Full, "observed_response"},
	{PartiallyReveal, "required", true, "own_disclosure", Partial, "observed_response"},
	{Hide, "none", true, "suppress", Silence, "unknown_or_censored"},
	{Lie, "required", true, "fictional_false_claim", FalseClaim, "observed_response"},
	{Joke, "required", true, "humor", Humorous, "observed_response"},
	{Challenge, "required", true, "contest_claim", "", "observed_response"},
	{Apologize, "required", true, "acknowledge_harm", "", "observed_response"},
	{Support, "required", true, "express_support", "", "observed_response"},
	{Complain, "required", true, "express_dissatisfaction", "", "observed_response"},
	{Argue, "required", true, "disagree", "", "observed_response"},
	{Withdraw, "optional", false, "withdraw", "", "unknown_or_observed"},
	{Coordinate, "required", true, "propose_coordination", "", "observed_response"},
	{Invite, "required", true, "offer_invitation", "", "observed_response"},
	{Decline, "required", true, "refuse_offer", "", "observed_response"},
	{Promise, "required", true, "create_commitment", "", "explicit_fulfillment_or_breach"},
	{BreakPromise, "required", true, "break_commitment", "", "explicit_breach"},
	{Help, "required", true, "consume_resource", "", "explicit_fulfillment_or_response"},
	{Ignore, "optional", true, "no_delivery", "", "unknown_or_censored"},
	{Delay, "optional", false, "delay", "", "unknown_or_observed"},
	{ChangeTopic, "required", true, "change_topic", TopicChange, "observed_response"},
	{SeekThirdPartySupport, "required", true, "request_support_without_third_party_content", "", "observed_response"},
	{Reconnect, "required", true, "reconnect", "", "observed_response"},
	{Leave, "optional", false, "leave", "", "unknown_or_observed"},
	{Wait, "none", false, "no_effect", "", "unknown_or_censored"},
}

func ActionRegistry() []ActionDefinition {
	return append([]ActionDefinition{}, actionDefinitions[:]...)
}
func ActionRegistryHash() string {
	b, _ := json.Marshal(actionDefinitions)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func Definition(k Kind) (ActionDefinition, bool) {
	for _, d := range actionDefinitions {
		if d.Kind == k {
			return d, true
		}
	}
	return ActionDefinition{}, false
}

type ActionOffer struct {
	Kind       Kind             `json:"kind"`
	Recipient  core.ID          `json:"recipient,omitempty"`
	Evidence   []core.ID        `json:"evidence,omitempty"`
	Duration   core.LogicalTime `json:"duration"`
	Resource   core.ID          `json:"resource,omitempty"`
	Units      int64            `json:"units,omitempty"`
	Commitment core.ID          `json:"commitment,omitempty"`
	Due        core.LogicalTime `json:"due,omitempty"`
	Mode       DisclosureMode   `json:"mode,omitempty"`
}

func (o ActionOffer) Validate() error {
	d, ok := Definition(o.Kind)
	if !ok || o.Duration < 1 || o.Duration > 1000000 || len(o.Evidence) > 8 {
		return fmt.Errorf("unknown action or action bounds")
	}
	if d.Recipient == "required" && o.Recipient.Validate() != nil || d.Recipient == "none" && o.Recipient != "" || o.Recipient != "" && o.Recipient.Validate() != nil {
		return fmt.Errorf("invalid action recipient")
	}
	if d.Evidence && len(o.Evidence) == 0 {
		return fmt.Errorf("action requires evidence")
	}
	seen := map[core.ID]bool{}
	for _, id := range o.Evidence {
		if id.Validate() != nil || seen[id] {
			return fmt.Errorf("invalid action evidence")
		}
		seen[id] = true
	}
	if o.Kind == Help || o.Kind == Promise {
		if o.Resource.Validate() != nil || o.Units < 1 || o.Units > 1000000 {
			return fmt.Errorf("resource action needs bounded units")
		}
	} else if o.Resource != "" || o.Units != 0 {
		return fmt.Errorf("speech cannot allocate resources")
	}
	if o.Kind == Promise {
		if o.Commitment.Validate() != nil || o.Due < 1 {
			return fmt.Errorf("promise needs id and deadline")
		}
	} else if o.Due != 0 {
		return fmt.Errorf("unexpected promise deadline")
	}
	if o.Commitment != "" && (o.Commitment.Validate() != nil || o.Kind != Promise && o.Kind != Help && o.Kind != BreakPromise) || o.Kind == BreakPromise && o.Commitment == "" {
		return fmt.Errorf("invalid commitment reference")
	}
	if d.Disclosure != "" {
		if o.Mode != d.Disclosure {
			return fmt.Errorf("action/disclosure mode mismatch")
		}
	} else if o.Mode != "" {
		if o.Kind != Say && o.Kind != Answer && o.Kind != Support || o.Mode != Full && o.Mode != Partial && o.Mode != Softened && o.Mode != Deflection && o.Mode != Omission {
			return fmt.Errorf("unsupported disclosure mode")
		}
	}
	return nil
}
func (o ActionOffer) Delivered() bool {
	return o.Recipient != "" && o.Kind != Ignore && o.Kind != Hide && o.Kind != Wait
}
