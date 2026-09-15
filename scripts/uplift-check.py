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

    assert r["executed_manifest"], "no executed manifest was emitted"

    disqualified = set(r["metrics_disqualified_as_benefit"])
    assert BANNED <= disqualified, BANNED - disqualified

    assert r["arms"][0] == "none" and len(r["arms"]) == 4, r["arms"]
    # An arm may be reported executed only if the manifest shows it running.
    # This replaces an earlier assertion that multi_perspective is never
    # executed: the assistance consumer implements all four arms, so that arm
    # is now genuinely run rather than absent. The invariant that matters is
    # not which arms run, but that the claim matches the manifest.
    assert r["arms_executed"][0] == "none", r["arms_executed"]
    ran = {u["arm"] for u in r["executed_manifest"]}
    assert set(r["arms_executed"]) == ran, (r["arms_executed"], sorted(ran))
    # Real execution across families and seeds, not an authored record set.
    assert r["comparisons"] >= 12, r["comparisons"]

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

    # Scenario coverage is stated honestly, including what is NOT covered, and
    # every covered family must be backed by the executed manifest rather than
    # by a scenario name. The gate recomputes coverage from the manifest on the
    # compiled binary's own output, so a coverage claim the manifest does not
    # support fails here.
    coverage = r["scenario_coverage"]
    assert len(coverage) == 8, coverage
    manifest = r["executed_manifest"]
    control, candidate = {}, {}
    for u in manifest:
        assert u["people_with_outcomes"] > 0, u
        side = control if u["arm"] == "none" else candidate
        side.setdefault(u["family"], set()).add((u["scenario"], u["seed"]))
    backed = {f for f in control if control[f] & candidate.get(f, set())}
    uncovered = [c["family"] for c in coverage if not c["covered"]]
    for c in coverage:
        if not c["covered"]:
            assert "NOT COVERED" in c["note"], c
            assert c["family"] not in backed, ("family executed but reported uncovered", c)
        else:
            assert c["family"] in backed, ("coverage not backed by the executed manifest", c)
    assert any(c["covered"] for c in coverage), "no family is actually executed"

    # Arms that produced the same policy output did the same thing. Recomputed
    # here from the manifest: no pair of arms that ran identically everywhere
    # they were compared may carry a passing uplift finding, and the report must
    # say so rather than leave the reader to assume the arms differed.
    # Independent versioned RNG streams (#58), recomputed from the report: each
    # arm's streams must be distinct per domain and identical across arms of the
    # same comparison. Independence is per domain, not per arm — see ArmRun.
    by_unit, streams_by_unit = {}, {}
    for u in manifest:
        by_unit.setdefault((u["scenario"], u["seed"]), {})[u["arm"]] = u["policy_hash"]
        streams_by_unit.setdefault((u["scenario"], u["seed"]), {})[u["arm"]] = u["rng_streams"]
        assert u["policy_hash"], ("an executed arm reports no policy receipt", u)
        doms = [st["domain"] for st in u["rng_streams"]]
        seeds = [st["seed"] for st in u["rng_streams"]]
        assert doms and len(set(doms)) == len(doms), ("rng stream domains repeat", u)
        assert len(set(seeds)) == len(seeds), ("two rng stream domains share a seed", u)
    for unit in streams_by_unit.values():
        shared = {tuple(sorted((st["domain"], st["seed"]) for st in v)) for v in unit.values()}
        assert len(shared) == 1, ("arms of one comparison drew different rng streams", unit)
    for f in findings:
        base, cand = f["baseline"], f["candidate"]
        shared = [a for a in by_unit.values() if base in a and cand in a]
        if not shared:
            continue
        same = [a for a in shared if a[base] == a[cand]]
        if same:
            note = " ".join(f.get("uncertainty", [])) + " " + f["evidence"]
            assert "identical policy output" in note, (
                "arms ran identically in some scenario but the report does not say so", base, cand)
        if len(same) == len(shared):
            assert f["status"] != "pass", (
                "uplift credited between arms that ran identically everywhere", base, cand)
    single = sorted({u["family"] for u in manifest if len(u["rng_streams"]) < 2})
    if single:
        print("NOTE: these families draw from a single undifferentiated rng stream; their "
              "consumer does not separate human, exogenous and helper draws: " + ", ".join(single))

    print(f"NOTE: {len(uncovered)} of 8 required scenario families are NOT covered: {', '.join(uncovered)}")

    print("PASS: real-consumer matched-arm uplift over executed families/seeds; no uplift claimed; "
          "per-person burden/unwanted/boundary/delayed retained; engagement metrics disqualified; "
          "executed arms match the manifest; coverage recomputed from the executed "
          "manifest; human validity NOT_TESTED")
    return 0


if __name__ == "__main__":
    sys.exit(main())
