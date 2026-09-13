// Package drives implements the owner-supplied HWS section 6 research registry.
// Values are bounded hypotheses, not psychological facts or personality labels.
package drives

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/core"
)

const Version = 2
const RegistryVersion = "hws-section6.v1"
const ModelVersion = "appraisal.drives.v1"
const Count = 22
const Hour core.LogicalTime = 3600 * 1e9
const (
	Acquisition = iota
	Comparison
	StatusProtection
	ThreatResponse
	ApproachDesire
	EffortAvoidance
	RewardSeeking
	Safety
	Belonging
	Autonomy
	Competence
	Care
	Fairness
	Reciprocity
	Status
	IdentityProtection
	Curiosity
	Meaning
	Certainty
	Novelty
	Attachment
	LossAvoidance
)

type Definition struct {
	ID       core.ID          `json:"id"`
	Baseline float64          `json:"baseline"`
	Gain     float64          `json:"gain"`
	HalfLife core.LogicalTime `json:"half_life"`
}

// Ordinals and hypothesis parameters are immutable wire/model contracts. Changes
// require a new registry/model version, not editing a deployed definition.
var definitions = [Count]Definition{
	{"acquisition", .3, .25, 12 * Hour}, {"comparison", .2, .2, 12 * Hour},
	{"status_protection", .3, .25, 24 * Hour}, {"threat_response", .2, .3, 8 * Hour},
	{"approach_desire", .4, .25, 12 * Hour}, {"effort_avoidance", .2, .25, 8 * Hour},
	{"reward_seeking", .3, .25, 12 * Hour}, {"safety", .5, .25, 24 * Hour},
	{"belonging", .5, .25, 24 * Hour}, {"autonomy", .5, .2, 24 * Hour},
	{"competence", .4, .2, 48 * Hour}, {"care", .5, .25, 24 * Hour},
	{"fairness", .4, .2, 48 * Hour}, {"reciprocity", .4, .2, 48 * Hour},
	{"status", .3, .25, 12 * Hour}, {"identity_protection", .4, .2, 168 * Hour},
	{"curiosity", .4, .2, 24 * Hour}, {"meaning", .4, .15, 168 * Hour},
	{"certainty", .4, .2, 24 * Hour}, {"novelty", .3, .2, 12 * Hour},
	{"attachment", .5, .15, 168 * Hour}, {"loss_avoidance", .3, .25, 48 * Hour},
}

const RegistryHash = "ed52b4d061de4ce17271f3949d173e830afe09053b00060461d2960d00191485"

func Registry() [Count]Definition { return definitions }
func ValidateRegistry() error {
	raw, _ := json.Marshal(definitions)
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != RegistryHash {
		return fmt.Errorf("unsupported drive registry order or hypothesis version")
	}
	return nil
}
