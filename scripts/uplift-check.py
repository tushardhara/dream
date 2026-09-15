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
    assert r["comparisons"] >= 2, r["comparisons"]

    findings = r["findings"]
    assert len(findings) == 3, findings
    for f in findings:
        assert f["baseline"] == "none", f
        assert f["synthetic_only"] is True and f["real_human_validity"] == "not-tested", f
        # The fixture carries no evidence of uplift, so none may be claimed.
        assert f["status"] in ("not-tested", "inconclusive"), f
    assert any(f["status"] == "inconclusive" for f in findings), "an arm with later evidence must be scored"
    assert any(f["status"] == "not-tested" for f in findings), "an arm without later evidence must stay not-tested"

    print("PASS: matched 4-arm synthetic uplift; no uplift claimed; engagement metrics disqualified; "
          "human validity NOT_TESTED; live-provider/cross-model/30-day study NOT_TESTED")
    return 0


if __name__ == "__main__":
    sys.exit(main())
