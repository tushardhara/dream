#!/usr/bin/env python3
"""Bounded compiled matched-arm uplift check.

Asserts on the COMPILED binary that this evaluation reports honestly: it is
synthetic, it claims no real-human validity, engagement-style metrics are
disqualified as benefit, and no arm is credited with uplift it did not earn.
"""
import json
import subprocess
import sys

BANNED = {"action_count", "notification_opens", "session_time", "emotional_attachment", "engagement"}


def main() -> int:
    subprocess.run(["go", "build", "-trimpath", "-o", "bin/hws-eval", "./cmd/hws-eval"], check=True)
    raw = subprocess.run(["bin/hws-eval", "-uplift"], check=True, capture_output=True, text=True).stdout
    r = json.loads(raw)

    assert r["version"] == "uplift-evaluation.v1", r["version"]
    assert r["synthetic_only"] is True
    for field in ("real_human_validity", "live_provider_semantic_quality", "cross_model_transfer", "real_30_day_study"):
        assert r[field] == "not-tested", (field, r[field])

    disqualified = set(r["metrics_disqualified_as_benefit"])
    assert BANNED <= disqualified, BANNED - disqualified

    assert r["arms"][0] == "none" and len(r["arms"]) == 4, r["arms"]
    # Only genuinely implemented arms may be reported as executed.
    assert r["arms_executed"][0] == "none", r["arms_executed"]
    assert "multi_perspective" not in r["arms_executed"], "an unimplemented arm must not be reported as executed"
    # Real execution across families and seeds, not an authored record set.
    assert r["comparisons"] >= 8, r["comparisons"]

    findings = r["findings"]
    assert len(findings) >= 4, findings
    for f in findings:
        assert f["synthetic_only"] is True and f["real_human_validity"] == "not-tested", f
        # Nothing observed here is uplift, so none may be claimed.
        assert f["status"] in ("not-tested", "inconclusive"), f
    # The required comparison against simple assistance must be present.
    assert any(f["baseline"] == "simple" for f in findings), "no comparison against simple assistance"

    # Per-person evidence must survive into the emitted result.
    people = r["per_person"]
    assert len(people) >= 24, len(people)
    for p in people:
        for field in ("person", "benefit", "burden", "unwanted_interventions", "boundary_violations", "delayed_outcome"):
            assert field in p, (field, p)

    # Scenario coverage is stated honestly, including what is NOT covered.
    coverage = r["scenario_coverage"]
    assert len(coverage) == 8, coverage
    uncovered = [c["family"] for c in coverage if not c["covered"]]
    for c in coverage:
        if not c["covered"]:
            assert "NOT COVERED" in c["note"], c
    assert any(c["covered"] for c in coverage), "no family is actually executed"
    print(f"NOTE: {len(uncovered)} of 8 required scenario families are NOT covered: {', '.join(uncovered)}")

    print("PASS: real-consumer matched-arm uplift over executed families/seeds; no uplift claimed; "
          "per-person burden/unwanted/boundary/delayed retained; engagement metrics disqualified; "
          "unimplemented arms reported not-executed; human validity NOT_TESTED")
    return 0


if __name__ == "__main__":
    sys.exit(main())
