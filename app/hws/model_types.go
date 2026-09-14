package hws

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"time"

	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
)

// MaxModelWireBytes is also the conservative input-token reservation ceiling.
const MaxModelWireBytes = 48000

var ErrModel = errors.New("model request or result invalid")
var ErrModelBudget = errors.New("model execution budget unavailable")
var ErrModelBusy = errors.New("model execution already in flight")
var ErrModelUncertain = errors.New("model attempt uncertain; reservation retained")

type ModelCapability string

const (
	ModelAppraisal      ModelCapability = "appraisal"
	ModelInterpretation ModelCapability = "interpretation"
	ModelReconciliation ModelCapability = "reconciliation"
	ModelCandidates     ModelCapability = "candidates"
)

func (c ModelCapability) Valid() bool {
	return c == ModelAppraisal || c == ModelInterpretation || c == ModelReconciliation || c == ModelCandidates
}

type ModelVersions struct {
	Schema     core.ID `json:"schema"`
	Capability core.ID `json:"capability"`
	Model      core.ID `json:"model"`
	Prompt     core.ID `json:"prompt"`
	Policy     core.ID `json:"policy"`
}

func (v ModelVersions) Validate() error {
	if v.Schema != "model.v1" || v.Capability != "cognition.v1" || v.Prompt != "cognition.v1" || v.Policy != "policy.v1" || v.Model.Validate() != nil {
		return ErrModel
	}
	return nil
}

// ProviderInput contains only policy-approved data, never a world snapshot,
// credential, mutable prompt template, tool specification or client role header.
type ProviderInput struct {
	Version         uint32                  `json:"version"`
	Key             core.ID                 `json:"key"`
	Capability      ModelCapability         `json:"capability"`
	Versions        ModelVersions           `json:"versions"`
	Actor           core.ID                 `json:"actor"`
	Context         []graph.SafeContextItem `json:"context"`
	MaxOutputTokens int64                   `json:"max_output_tokens"`
}

func (r ProviderInput) Validate() error {
	if r.Version != 1 || r.Key.Validate() != nil || !r.Capability.Valid() || r.Versions.Validate() != nil || r.Actor.Validate() != nil || r.MaxOutputTokens < 1 || r.MaxOutputTokens > 8192 || len(r.Context) < 1 || len(r.Context) > 16 {
		return ErrModel
	}
	seen := map[core.ID]bool{}
	text := 0
	for _, item := range r.Context {
		if item.Source.Validate() != nil || seen[item.Source] || item.Observer.Validate() != nil || item.Subject.ValidateFor(item.Observer) != nil || item.Confidence.Validate() != nil {
			return ErrModel
		}
		seen[item.Source] = true
		text += len(item.Text)
	}
	b, e := json.Marshal(r)
	if e != nil || len(b) > 40000 || text > 4096 {
		return ErrModel
	}
	return nil
}
func ModelDigest(value any) (string, error) {
	b, e := json.Marshal(value)
	if e != nil {
		return "", ErrModel
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
func (r ProviderInput) Digest() (string, error) {
	if r.Validate() != nil {
		return "", ErrModel
	}
	return ModelDigest(r)
}

// Cognitive outputs are proposals with subjective uncertainty and source refs,
// not instructions, actions or calibrated scientific labels. #11 decides effects.
type ModelFinding struct {
	Code       string          `json:"code"`
	Value      float64         `json:"value"`
	Confidence core.Confidence `json:"confidence"`
	Evidence   []core.ID       `json:"evidence"`
}
type ModelOutput struct {
	Version    uint32          `json:"version"`
	Capability ModelCapability `json:"capability"`
	Observer   core.ID         `json:"observer"`
	Findings   []ModelFinding  `json:"findings"`
}

func (o ModelOutput) Validate(r ProviderInput) error {
	if r.Validate() != nil || o.Version != 1 || o.Capability != r.Capability || o.Observer != r.Actor || len(o.Findings) < 1 || len(o.Findings) > 8 {
		return ErrModel
	}
	codes := map[ModelCapability]map[string]bool{
		ModelAppraisal:      {"threat": true, "opportunity": true, "care": true, "status": true},
		ModelInterpretation: {"supported": true, "contradicted": true, "uncertain": true},
		ModelReconciliation: {"retain_both": true, "prefer_supported": true, "wait": true},
		ModelCandidates:     {"wait": true, "observe": true, "ask": true, "self_disclose": true},
	}
	sources := map[core.ID]bool{}
	for _, s := range r.Context {
		sources[s.Source] = true
	}
	seen := map[string]bool{}
	for _, f := range o.Findings {
		if !codes[o.Capability][f.Code] || seen[f.Code] || math.IsNaN(f.Value) || math.IsInf(f.Value, 0) || f.Value < -1 || f.Value > 1 || f.Confidence.Validate() != nil || len(f.Evidence) < 1 || len(f.Evidence) > 16 {
			return ErrModel
		}
		seen[f.Code] = true
		refs := map[core.ID]bool{}
		for _, id := range f.Evidence {
			if !sources[id] || refs[id] {
				return ErrModel
			}
			refs[id] = true
		}
	}
	b, e := json.Marshal(o)
	if e != nil || len(b) > 16384 {
		return ErrModel
	}
	return nil
}
func DecodeModelOutput(raw []byte, r ProviderInput) (ModelOutput, error) {
	var o ModelOutput
	if len(raw) > 16384 {
		return o, ErrModel
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&o) != nil || d.Decode(new(any)) != io.EOF || o.Validate(r) != nil {
		return ModelOutput{}, ErrModel
	}
	return o, nil
}

type ProviderStatus string

const (
	ProviderOK          ProviderStatus = "ok"
	ProviderRefused     ProviderStatus = "refused"
	ProviderRateLimited ProviderStatus = "rate_limited"
	ProviderUnavailable ProviderStatus = "unavailable"
	ProviderMalformed   ProviderStatus = "malformed"
)

type ProviderResponse struct {
	Generation   string         `json:"generation"`
	Status       ProviderStatus `json:"status"`
	Model        core.ID        `json:"model"`
	Output       []byte         `json:"output"`
	InputTokens  int64          `json:"input_tokens"`
	OutputTokens int64          `json:"output_tokens"`
	UsageKnown   bool           `json:"usage_known"`
}

// ModelLimits reserves the conservative full per-attempt ceiling before each
// external call. Unknown/uncertain usage never refunds the reservation.
type ModelLimits struct {
	Version            uint32        `json:"version"`
	Tokens             int64         `json:"tokens"`
	SpendMicros        int64         `json:"spend_micros"`
	MaxInFlight        int           `json:"max_in_flight"`
	MaxAttempts        int           `json:"max_attempts"`
	AttemptTokens      int64         `json:"attempt_tokens"`
	AttemptSpendMicros int64         `json:"attempt_spend_micros"`
	Timeout            time.Duration `json:"timeout_ns"`
}

func (l ModelLimits) Validate() error {
	if l.Version != 1 || l.Tokens < 1 || l.Tokens > 1000000000 || l.SpendMicros < 1 || l.SpendMicros > 1000000000000 || l.MaxInFlight < 1 || l.MaxInFlight > 8 || l.MaxAttempts < 1 || l.MaxAttempts > 3 || l.AttemptTokens < 1 || l.AttemptTokens > 1000000 || l.AttemptSpendMicros < 1 || l.AttemptSpendMicros > l.SpendMicros || l.AttemptTokens > l.Tokens || l.Timeout < time.Millisecond || l.Timeout > time.Minute {
		return ErrModelBudget
	}
	return nil
}
