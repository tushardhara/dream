package evals

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"testing"

	"github.com/tushardhara/dream/simulator/experiment"
)

func TestPromotionRequiresSeparateOwnerSignaturesAndHoldout(t *testing.T) {
	d, c := fixture(t)
	r, e := Run(context.Background(), d, c, experiment.Generator{}, fixtureNow)
	if e != nil {
		t.Fatal(e)
	}
	public, private, e := ed25519.GenerateKey(nil)
	if e != nil {
		t.Fatal(e)
	}
	a, e := NewPromotionAuthority("owner", public)
	if e != nil {
		t.Fatal(e)
	}
	p := SignedPlan{Plan: PromotionPlan{Version: "promotion-plan.v1", Batch: "synthetic-batch-1", Owner: "owner", Scope: "synthetic_offline_batch", Candidate: experiment.Stateful, DatasetHash: d.Hash, ConfigHash: r.ConfigHash, Criteria: []Criterion{{Dimension: "wait", Horizon: c.Horizons[0], MaximumBrierUpper95: 1, MinimumResolution: 1, MinimumComponents: 4}}}}
	signPlan := func() {
		message, _ := PlanSigningMessage(p.Plan)
		p.Signature = hex.EncodeToString(ed25519.Sign(private, message))
	}
	signPlan()
	if e = a.CheckInputs(p, d, c); e != nil {
		t.Fatal(e)
	}
	r, e = a.RunPlanned(context.Background(), p, d, c, experiment.Generator{}, fixtureNow)
	if e != nil {
		t.Fatal(e)
	}
	planHash, _ := Digest(p.Plan)
	approval := Approval{Version: "promotion-approval.v1", PlanHash: planHash, ReportHash: r.Hash, Decision: "approve_synthetic_batch"}
	if _, e = a.Authorize(p, r, approval); e == nil {
		t.Fatal("self approval without owner signature")
	}
	signApproval := func() {
		message, _ := ApprovalSigningMessage(approval)
		approval.Signature = hex.EncodeToString(ed25519.Sign(private, message))
	}
	signApproval()
	event, e := a.Authorize(p, r, approval)
	if e != nil || event.Scope != "synthetic_offline_batch" {
		t.Fatal("valid explicit synthetic approval failed", e)
	}
	again, e := a.Authorize(p, r, approval)
	if e != nil || event.Hash != again.Hash {
		t.Fatal("non-idempotent batch event", e)
	}
	_, attacker, _ := ed25519.GenerateKey(nil)
	message, _ := ApprovalSigningMessage(approval)
	approval.Signature = hex.EncodeToString(ed25519.Sign(attacker, message))
	if _, e = a.Authorize(p, r, approval); e == nil {
		t.Fatal("candidate key approved itself")
	}
	signApproval()
	r.Hash = "changed"
	if _, e = a.Authorize(p, r, approval); e == nil {
		t.Fatal("changed report accepted")
	}
	r, _ = SealReport(r)
	c.Seeds = append([]uint64{}, c.Seeds...)
	c.Seeds[0]++
	if a.CheckInputs(p, d, c) == nil {
		t.Fatal("changed preregistration accepted")
	}
	p.Plan.Criteria[0].MaximumBrierUpper95 = 0
	signPlan()
	r, e = a.RunPlanned(context.Background(), p, d, r.Config, experiment.Generator{}, fixtureNow)
	if e != nil {
		t.Fatal(e)
	}
	approval.ReportHash = r.Hash
	approval.PlanHash, _ = Digest(p.Plan)
	signApproval()
	if _, e = a.Authorize(p, r, approval); e == nil {
		t.Fatal("failed holdout silently approved")
	}
	p.Plan.Scope = "production"
	signPlan()
	if a.ValidatePlan(p) == nil {
		t.Fatal("production activation exposed")
	}
}
