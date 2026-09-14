"""Opt-in #43 assertion sensitivity. Requires the foreground role disk guard.
Exclusive worktree only. Preserve per-run logs; restore exact source in finally.
Compile errors, timeouts and uncaught changes are failures, never mutation catches.
"""
import argparse
import json
import os
import pathlib
import re
import signal
import subprocess
import uuid


def interrupted(signum, frame):
    raise KeyboardInterrupt("owned mutation interrupted")


signal.signal(signal.SIGTERM, interrupted)
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--review", action="store_true", help="run the additional evaluator/plan controls requested in review")
options = parser.parse_args()
ROOT = pathlib.Path(__file__).resolve().parents[1]
run = os.environ.get("DREAM_TEST_RUN", "")
if not re.fullmatch(r"[A-Za-z0-9_-]{1,100}", run):
    raise RuntimeError("run under scripts/disk-guard.py with an exclusive role worktree")
LOG = ROOT / "bin" / ("alignment-mutations-" + run + "-" + uuid.uuid4().hex[:8])
LOG.mkdir(exist_ok=False)


def command(name, package, tests):
    result = subprocess.run(["go", "test", "-count=1", package, "-run", tests, "-v"], cwd=ROOT,
                            text=True, capture_output=True, timeout=120)
    output = result.stdout + result.stderr
    if len(output.encode()) > 2 << 20:
        (LOG / (name + ".log")).write_text(output[-(2 << 20):])
        raise RuntimeError("bounded mutation output exceeded")
    (LOG / (name + ".log")).write_text(output)
    return result.returncode, output


baseline = "^(TestAlignment.*|TestCompleteImmutableSourceRegistry|TestLegacyEvaluationReportV1RemainsVerifiable)$"
checks = [
    ("registry-wording", "evals/alignment_registry.go", "real human data can be imported and compared.", "a future authorized path exists.", "./evals", "^TestCompleteImmutableSourceRegistry$"),
    ("registry-id", "evals/alignment_registry.go", '"hws19.01", "persistence"', '"hws19.99", "persistence"', "./evals", "^TestCompleteImmutableSourceRegistry$"),
    ("omitted-report-row", "evals/alignment.go", "range r.Registry {", "range r.Registry[:22] {", "./evals", "^TestAlignmentEvidenceAdequacyAndReproduction$"),
    ("canonical-verdict", "evals/alignment.go", "if !reflect.DeepEqual(r, canonical) {", "if false && !reflect.DeepEqual(r, canonical) {", "./evals", "^TestAlignmentRejectsResealedClaimsAndBindings$"),
    ("dataset-evidence", "evals/alignment.go", "dataset.Validate(receipt.EvaluatedAt) != nil || dataset.Hash != expected.DatasetHash", "false", "./evals", "^TestAlignmentRejectsResealedClaimsAndBindings$"),
    ("receipt-required", "evals/alignment.go", "receipt.Verify(r) != nil ||", "", "./evals", "^TestAlignmentRejectsResealedClaimsAndBindings$"),
    ("freeze-before-score", "evals/alignment.go", "r.EvaluatedAt.Before(report.Plan.RegisteredAt)", "false", "./evals", "^TestAlignmentRejectsResealedClaimsAndBindings$"),
    ("human-claim", "evals/alignment.go", "HumanValidity: AlignmentNotTested", "HumanValidity: AlignmentPass", "./evals", "^TestAlignmentEvidenceAdequacyAndReproduction$"),
    ("cross-model-claim", "evals/alignment.go", "CrossModelTransfer: AlignmentNotTested", "CrossModelTransfer: AlignmentPass", "./evals", "^TestAlignmentEvidenceAdequacyAndReproduction$"),
    ("study-claim", "evals/alignment.go", 'RealStudy: "NOT_RUN"', 'RealStudy: "PASS"', "./evals", "^TestAlignmentEvidenceAdequacyAndReproduction$"),
    ("falsifier-polarity", "evals/alignment.go", 'f.FalsifierTriggered = "unknown"', 'f.FalsifierTriggered = "false"', "./evals", "^TestAlignmentEvidenceAdequacyAndReproduction$"),
    ("repeat-comparison", "evals/alignment.go", "if reflect.DeepEqual(r.Evidence.First, r.Evidence.Repeat) {", "if true {", "./evals", "^TestAlignmentVerdictPolarity$"),
    ("request-alias", "evals/alignment.go", "g.ProbeRelationships(ctx, request)", "g.ProbeRelationships(ctx, plan.Probe)", "./evals", "^TestAlignmentLabelBlindFrozenPort$"),
    ("trial-source", "simulator/experiment/relationships.go", "t.InputHash != inputDigest(f, c) ||", "", "./simulator/experiment", "^TestRelationshipProbeUsesControlledInputsAndSeeds$"),
    ("trial-seed", "simulator/experiment/relationships.go", "t.Seed != seed ||", "", "./simulator/experiment", "^TestRelationshipProbeUsesControlledInputsAndSeeds$"),
    ("appraisal-effect", "simulator/experiment/relationships.go", 'f.At, c.Focus)', 'f.At, false)', "./evals", "^TestAlignmentEvidenceAdequacyAndReproduction$"),
]
# These disable the evaluator's comparisons themselves, rather than changing
# generator output. Negative evidence must make each named verdict non-PASS.
review_checks = []
mechanics_test = "^TestAlignmentMechanicsRejectUnsupportedPass$"
review_checks.append(("mechanic-context-threshold", "evals/alignment.go", ")) > 1e-6", ")) >= 0", "./evals", mechanics_test))
for index, name in enumerate(["label", "ablations", "history-reversal", "unknown-permission", "appraisal", "event-state"], start=1):
    review_checks.append(("mechanic-" + name, "evals/alignment.go", f"checks[{index}] = checks[{index}] &&", f"checks[{index}] = checks[{index}] ||", "./evals", mechanics_test))
review_checks += [
    ("mechanic-seed-variation", "evals/alignment.go", "if len(selected) > 1 {", "if len(selected) > 0 {", "./evals", mechanics_test),
    ("mechanic-repeat", "evals/alignment.go", "if reflect.DeepEqual(e.First, e.Repeat) {", "if e.First != nil {", "./evals", mechanics_test),
]
plan_clauses = [
    ("version", "p.Version != AlignmentPlanVersion"),
    ("revision", "!gitHash.MatchString(p.SourceRevision)"),
    ("tree", "!gitHash.MatchString(p.SourceTree)"),
    ("extract", "p.SourceExtractSHA256 != AlignmentSourceSHA256"),
    ("evaluator", "!hash.MatchString(p.EvaluatorArtifact)"),
    ("generator", "!regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(p.GeneratorArtifact)"),
    ("model", 'p.Model != experiment.RelationshipExperimentVersion+"/"+behavior.ActionPolicy+"/"+drives.ModelVersion'),
    ("dataset", "!hash.MatchString(p.DatasetHash)"),
    ("registry-version", "p.RegistryVersion != AlignmentRegistryVersion"),
    ("registry-hash", "p.RegistryHash != AlignmentRegistryHash()"),
    ("probe", "p.Probe.Validate() != nil"),
    ("hypothesis", "p.Hypothesis != alignmentHypothesis"),
    ("rule", "p.DecisionRule != alignmentRule"),
    ("scope", "p.EvidenceScope != alignmentScope"),
    ("time-missing", "p.RegisteredAt.IsZero()"),
    ("time-zone", "p.RegisteredAt.Location() != time.UTC"),
]
for name, clause in plan_clauses:
    review_checks.append(("plan-" + name, "evals/alignment.go", clause, "false", "./evals", "^TestAlignment(PlanRejectsEveryInvalidField|TamperedRetainedPlanCannotScore)$"))
checks = review_checks if options.review else checks + review_checks

# Every mutation has an independently green relevant baseline and restoration run.
for name, package, tests in [("baseline-evals", "./evals", baseline), ("baseline-probe", "./simulator/experiment", "^TestRelationshipProbeUsesControlledInputsAndSeeds$")]:
    code, output = command(name, package, tests)
    if code:
        raise RuntimeError(name + " failed; see " + str(LOG))
results = []
for name, file, old, new, package, tests in checks:
    path = ROOT / file
    original = path.read_bytes()
    if original.count(old.encode()) != 1:
        raise RuntimeError(name + " requires exactly one source match")
    try:
        path.write_bytes(original.replace(old.encode(), new.encode()))
        code, output = command(name, package, tests)
        if code == 0 or "--- FAIL:" not in output or "[build failed]" in output or "panic: test timed out" in output:
            raise RuntimeError(name + " not caught by a test assertion; see " + str(LOG))
        print("CAUGHT " + name, flush=True)
    finally:
        path.write_bytes(original)
        if path.read_bytes() != original:
            raise RuntimeError(name + " exact source restoration failed")
    code, output = command(name + "-restored", package, tests)
    if code:
        raise RuntimeError(name + " restored source is not green")
    results.append({"mutation": name, "assertion_caught": True, "exact_restore": True, "restored_green": True})
    (LOG / "results.json").write_text(json.dumps(results, indent=2) + "\n")
print("PASS: all " + str(len(results)) + " assertion mutations caught and exact restorations green; " + str(LOG))
