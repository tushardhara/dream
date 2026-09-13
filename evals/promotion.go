package evals

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"time"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/experiment"
)

// Promotion is an offline, signed batch protocol. No function in this package
// changes a running policy, prompt, model, or production default. All adequacy
// thresholds are explicit owner decisions, never thresholds invented by a runner.
type Criterion struct {
	Dimension           string           `json:"dimension"`
	Horizon             core.LogicalTime `json:"horizon"`
	MaximumBrierUpper95 float64          `json:"maximum_brier_upper_95"`
	MinimumResolution   float64          `json:"minimum_resolution"`
	MinimumComponents   int              `json:"minimum_components"`
}
type PromotionPlan struct {
	Version     string             `json:"version"`
	Batch       core.ID            `json:"batch"`
	Owner       core.ID            `json:"owner"`
	Scope       string             `json:"scope"`
	Candidate   experiment.Variant `json:"candidate"`
	DatasetHash string             `json:"dataset_hash"`
	ConfigHash  string             `json:"config_hash"`
	Criteria    []Criterion        `json:"criteria"`
}
type SignedPlan struct {
	Plan      PromotionPlan `json:"plan"`
	Signature string        `json:"owner_signature"`
}
type Approval struct {
	Version    string `json:"version"`
	PlanHash   string `json:"plan_hash"`
	ReportHash string `json:"report_hash"`
	Decision   string `json:"decision"`
	Signature  string `json:"owner_signature"`
}
type PromotionEvent struct {
	Version    string             `json:"version"`
	Batch      core.ID            `json:"batch"`
	Owner      core.ID            `json:"owner"`
	PlanHash   string             `json:"plan_hash"`
	ReportHash string             `json:"report_hash"`
	Scope      string             `json:"scope"`
	Candidate  experiment.Variant `json:"candidate"`
	Approval   Approval           `json:"approval"`
	Hash       string             `json:"hash"`
}
type PromotionAuthority struct {
	owner  core.ID
	public ed25519.PublicKey
}

func NewPromotionAuthority(owner core.ID, public ed25519.PublicKey) (PromotionAuthority, error) {
	if owner.Validate() != nil || len(public) != ed25519.PublicKeySize {
		return PromotionAuthority{}, fmt.Errorf("trusted owner key required")
	}
	return PromotionAuthority{owner: owner, public: append(ed25519.PublicKey{}, public...)}, nil
}
func PlanSigningMessage(p PromotionPlan) ([]byte, error) {
	h, e := Digest(p)
	return []byte("dream-promotion-plan.v1/" + h), e
}
func ApprovalSigningMessage(a Approval) ([]byte, error) {
	a.Signature = ""
	h, e := Digest(a)
	return []byte("dream-promotion-approval.v1/" + h), e
}
func verifySignature(public ed25519.PublicKey, message []byte, signature string) bool {
	raw, e := hex.DecodeString(signature)
	return e == nil && ed25519.Verify(public, message, raw)
}
func (a PromotionAuthority) ValidatePlan(p SignedPlan) error {
	x := p.Plan
	hash := regexp.MustCompile(`^[0-9a-f]{64}$`)
	if len(a.public) != ed25519.PublicKeySize || x.Version != "promotion-plan.v1" || x.Owner != a.owner || x.Batch.Validate() != nil || x.Scope != "synthetic_offline_batch" || !x.Candidate.Valid() || x.Candidate == experiment.Baseline || !hash.MatchString(x.DatasetHash) || !hash.MatchString(x.ConfigHash) || len(x.Criteria) < 1 || len(x.Criteria) > 32 {
		return fmt.Errorf("invalid owner promotion plan")
	}
	seen := map[string]bool{}
	for _, c := range x.Criteria {
		key := fmt.Sprintf("%s/%d", c.Dimension, c.Horizon)
		if seen[key] || !actionDimension(c.Dimension) || c.Horizon < 1 || c.Horizon > experiment.MaxHorizon || math.IsNaN(c.MaximumBrierUpper95) || c.MaximumBrierUpper95 < 0 || c.MaximumBrierUpper95 > 1 || math.IsNaN(c.MinimumResolution) || c.MinimumResolution <= 0 || c.MinimumResolution > 1 || c.MinimumComponents < 2 || c.MinimumComponents > 64 {
			return fmt.Errorf("invalid explicit adequacy criterion")
		}
		seen[key] = true
	}
	message, e := PlanSigningMessage(x)
	if e != nil || !verifySignature(a.public, message, p.Signature) {
		return fmt.Errorf("plan lacks trusted owner signature")
	}
	return nil
}

// CheckInputs validates the owner's preregistration before accessing labels or
// generating. It does not authorize promotion; a separate report-bound signature
// is required after independent holdout assessment.
func (a PromotionAuthority) CheckInputs(p SignedPlan, d Dataset, c Config) error {
	if e := a.ValidatePlan(p); e != nil {
		return e
	}
	h, e := Digest(c)
	if e != nil || h != p.Plan.ConfigHash || d.Hash != p.Plan.DatasetHash || c.DatasetHash != d.Hash {
		return fmt.Errorf("plan differs from frozen inputs")
	}
	return nil
}
func (a PromotionAuthority) Authorize(p SignedPlan, r Report, approval Approval) (PromotionEvent, error) {
	if e := a.ValidatePlan(p); e != nil {
		return PromotionEvent{}, e
	}
	if r.Verify() != nil || p.Plan.ConfigHash != r.ConfigHash || p.Plan.DatasetHash != r.DatasetHash {
		return PromotionEvent{}, fmt.Errorf("report differs from preregistration")
	}
	planHash, e := Digest(p.Plan)
	if r.PreregistrationHash != planHash {
		return PromotionEvent{}, fmt.Errorf("report was not generated under the signed preregistration")
	}
	if e != nil {
		return PromotionEvent{}, e
	}
	if approval.Version != "promotion-approval.v1" || approval.PlanHash != planHash || approval.ReportHash != r.Hash || approval.Decision != "approve_synthetic_batch" {
		return PromotionEvent{}, fmt.Errorf("missing exact report approval")
	}
	message, e := ApprovalSigningMessage(approval)
	if e != nil || !verifySignature(a.public, message, approval.Signature) {
		return PromotionEvent{}, fmt.Errorf("approval lacks trusted owner signature")
	}
	for _, criterion := range p.Plan.Criteria {
		for _, split := range []Split{Calibration, Holdout} {
			found := 0
			for _, m := range r.Metrics {
				if m.Variant != p.Plan.Candidate || m.Split != split || m.Dimension != criterion.Dimension || m.Horizon != criterion.Horizon {
					continue
				}
				found++
				if m.Status != Pass || m.BrierCI == nil || m.BrierCI.Units < criterion.MinimumComponents || m.BrierCI.High > criterion.MaximumBrierUpper95 || m.ResolutionRate == nil || *m.ResolutionRate < criterion.MinimumResolution || m.Abstentions != 0 {
					return PromotionEvent{}, fmt.Errorf("holdout/calibration criterion failed or inconclusive")
				}
			}
			if found != 1 {
				return PromotionEvent{}, fmt.Errorf("missing/duplicate holdout criterion")
			}
		}
	}
	event := PromotionEvent{Version: "promotion-authorized.v1", Batch: p.Plan.Batch, Owner: a.owner, PlanHash: planHash, ReportHash: r.Hash, Scope: p.Plan.Scope, Candidate: p.Plan.Candidate, Approval: approval}
	event.Hash, e = Digest(event)
	return event, e
}

func (a PromotionAuthority) RunPlanned(ctx context.Context, p SignedPlan, d Dataset, c Config, g BatchGenerator, now time.Time) (Report, error) {
	if e := a.CheckInputs(p, d, c); e != nil {
		return Report{}, e
	}
	r, e := Run(ctx, d, c, g, now)
	if e != nil {
		return Report{}, e
	}
	r.PreregistrationHash, e = Digest(p.Plan)
	if e != nil {
		return Report{}, e
	}
	return SealReport(r)
}
