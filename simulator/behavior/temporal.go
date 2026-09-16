package behavior

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/tushardhara/dream/core"
)

const TemporalHumanVersion = "temporal-human-actions.v1"
const TemporalClarificationCooldown core.LogicalTime = 10
const TemporalClarificationBudget = 2

type TemporalClarification struct {
	With, Topic, Decision core.ID
	At                    core.LogicalTime
}
type TemporalActor struct {
	Version        string
	Human          DomainActor
	Clarifications []TemporalClarification
}

func NewTemporalActor(id core.ID, at core.LogicalTime) (TemporalActor, error) {
	human, e := NewDomainActor(id, at)
	return TemporalActor{TemporalHumanVersion, human, []TemporalClarification{}}, e
}
func (a TemporalActor) Validate() error {
	if a.Version != TemporalHumanVersion || a.Human.Validate() != nil || len(a.Clarifications) > 32 {
		return fmt.Errorf("invalid temporal actor")
	}
	seen := map[core.ID]bool{}
	for _, c := range a.Clarifications {
		if c.With.Validate() != nil || c.With == a.Human.Human.Human.Drives.Actor || c.Topic.Validate() != nil || c.Decision.Validate() != nil || c.At < 0 || seen[c.Decision] {
			return fmt.Errorf("invalid clarification receipt")
		}
		seen[c.Decision] = true
	}
	return nil
}

type TemporalSituation struct {
	Reporters map[core.ID]core.ID
	Domain    DomainSituation
	Focus     core.TemporalFocus
	Evidence  []core.TemporalEvidence
}
type TemporalDecision struct {
	Version      string
	Context      core.TemporalContext
	EvidenceHash string
	Human        DomainDecision
}

// ChooseTemporalAction uses the common action engine after temporal eligibility.
// WAIT and unilateral distance remain available. Cooldown is shared across channels
// and role frames for this relationship/topic, so relabelling cannot reset burden.
func ChooseTemporalAction(a TemporalActor, in TemporalSituation, at core.LogicalTime, draw uint64) (TemporalActor, TemporalDecision, error) {
	actor := a.Human.Human.Human.Drives.Actor
	scope := in.Domain.Scoped.Scope
	if len(in.Reporters) > 16 || a.Validate() != nil || in.Focus.Validate() != nil || in.Focus.Observer != actor || in.Focus.Other != scope.Target {
		return TemporalActor{}, TemporalDecision{}, fmt.Errorf("invalid temporal action")
	}
	metadata, e := relationshipEvidence(in.Domain.Scoped.Situation, actor, at)
	if e != nil {
		return TemporalActor{}, TemporalDecision{}, e
	}
	for _, record := range in.Evidence {
		f := record.Fact
		if f.Observer != actor || f.With != scope.Target || f.Channel != in.Focus.Channel || record.Validate(at) != nil {
			return TemporalActor{}, TemporalDecision{}, fmt.Errorf("foreign temporal context")
		}
		account, ok := metadata[f.Account]
		source, sourceOK := metadata[f.Source]
		if in.Reporters[f.Account] != record.Reporter || in.Reporters[f.Source] != record.SourceReporter || !ok || !sourceOK || account.OccurredAt != record.OccurredAt || account.LearnedAt != record.LearnedAt || account.Confidence != record.Confidence || source.Confidence != record.SourceConfidence || source.OccurredAt != record.SourceOccurredAt || source.LearnedAt != record.SourceLearnedAt {
			return TemporalActor{}, TemporalDecision{}, fmt.Errorf("temporal evidence not currently available")
		}
	}
	temporal, e := core.InterpretTemporal(in.Focus, in.Evidence, at)
	if e != nil {
		return TemporalActor{}, TemporalDecision{}, e
	}
	count := 0
	canAsk := temporal.Recommendation == "clarify"
	for _, old := range a.Clarifications {
		if old.With != scope.Target || old.Topic != scope.Topic {
			continue
		}
		count++
		if old.At > at || at-old.At < TemporalClarificationCooldown {
			canAsk = false
		}
	}
	if count >= TemporalClarificationBudget {
		canAsk = false
	}
	input := domainCopy(in.Domain)
	offers := []ActionOffer{}
	for _, offer := range input.Scoped.Situation.Offers {
		if offer.Kind == Wait || offer.Kind == Leave || offer.Kind == Withdraw || offer.Kind == Ask && canAsk || offer.Kind != Ask && temporal.Status == "routine" {
			offers = append(offers, offer)
		}
	}
	input.Scoped.Situation.Offers = offers
	next, decision, e := ChooseDomainAction(a.Human, input, at, draw)
	if e != nil {
		return TemporalActor{}, TemporalDecision{}, e
	}
	out := domainCopy(a)
	out.Human = next
	selected := decision.Human.Human.Candidates[decision.Human.Human.Selected].Offer
	if selected.Kind == Ask {
		out.Clarifications = append(out.Clarifications, TemporalClarification{scope.Target, scope.Topic, decision.Human.Human.ID, at})
	}
	if out.Validate() != nil {
		return TemporalActor{}, TemporalDecision{}, fmt.Errorf("temporal actor budget")
	}
	return out, TemporalDecision{TemporalHumanVersion, temporal, scopedHash(in.Evidence), decision}, nil
}
func (a TemporalActor) Encode() ([]byte, error) {
	if a.Validate() != nil {
		return nil, fmt.Errorf("invalid temporal actor")
	}
	b, e := json.Marshal(a)
	if e != nil || len(b) > 131072 {
		return nil, fmt.Errorf("temporal actor byte bound")
	}
	return b, nil
}
func DecodeTemporalActor(b []byte) (TemporalActor, error) {
	var a TemporalActor
	if len(b) > 131072 {
		return a, fmt.Errorf("temporal actor byte bound")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&a) != nil || a.Validate() != nil {
		return TemporalActor{}, fmt.Errorf("invalid temporal actor codec")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return TemporalActor{}, fmt.Errorf("trailing temporal actor")
	}
	return a, nil
}
