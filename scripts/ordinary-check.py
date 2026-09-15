"""Bounded compiled ordinary-life comparison; no engagement objective."""
import hashlib
import json
import pathlib
import subprocess


def run(family, arm, seed):
    raw = subprocess.run(["./bin/hws-ordinary", "--family", family,
                          "--arm", arm, "--seed", str(seed)],
                         capture_output=True, text=True, check=True,
                         timeout=30).stdout
    return raw, json.loads(raw)


def selected(trace):
    d = trace["Decision"]["Human"]
    return d["candidates"][d["selected"]]["offer"]["kind"]


rows = []
for family in ("quiet", "daily", "unknown_benefit", "declined_ritual"):
    for seed in range(8):
        generic = None
        for arm in ("none", "generic", "permitted_context"):
            raw, r = run(family, arm, seed)
            assert r["Version"] == "ordinary-life.v1"
            assert r["HumanValidity"] == "NOT_TESTED"
            assert r["GlobalWelfare"] == "NOT_AGGREGATED"
            waits = sum(o["Helper"]["Action"] == "WAIT" for o in r["Opportunities"])
            assert waits > 6
            if arm == "none" or family in ("quiet", "unknown_benefit"):
                assert waits == 12
            for o in r["Opportunities"]:
                if o["Helper"]["Action"] == "WAIT":
                    assert o["Selected"] == "wait"
                if arm == "generic":
                    assert "Quote" not in o["Helper"]
            daily = [t for t in r["Traces"] if t["Stage"] == "daily-life"]
            assert len(daily) == 24
            assert all(t["MemoryBefore"] == t["MemoryAfter"] for t in r["Traces"])
            for receipt in r["Reservations"]:
                assert receipt["Units"] == 1
                assert {(t["Owner"], t["Units"]) for t in receipt["Tasks"]} == {("a", 1), ("b", 1)}
            if arm == "generic":
                generic = r
            if arm == "permitted_context":
                for field in ("Traces", "Experiences", "Reservations"):
                    assert r[field] == generic[field], "undeclared context advantage"
            rows.append({"family": family, "seed": seed, "arm": arm,
                         "helper_waits": waits,
                         "daily_choices": {k: sum(selected(t) == k for t in daily)
                                           for k in ("say", "wait")},
                         "native_declines": sum(selected(t) == "decline" for t in r["Traces"]),
                         "reservations": len(r["Reservations"]),
                         "later_reports": [{"participant": x["Participant"],
                                            "participation": x["Participation"],
                                            "benefit": x["Benefit"],
                                            "burden_reduction": x["BurdenReduction"]}
                                           for x in r["Experiences"]]})
            if family == "daily" and seed == 3 and arm == "permitted_context":
                assert raw == run(family, arm, seed)[0]
report = {"version": "ordinary-life.v1", "human_validity": "NOT_TESTED",
          "global_welfare": "NOT_AGGREGATED", "runs": len(rows),
          "comparison": "NULL: generic and permitted-context native choices and later reports identical",
          "outcome_source": "explicit fictional later self-reports; not inferred from invitations/replies",
          "limitation": "fixed wording is not semantically interpreted by the native policy; no human efficacy claim",
          "rows": rows}
encoded = json.dumps(report, sort_keys=True, separators=(",", ":")).encode()
pathlib.Path("bin/ordinary-report.json").write_bytes(encoded)
print("PASS: 96 bounded ordinary runs, majority WAIT, independent daily life, "
      "exact replay, null context comparison; sha256=" + hashlib.sha256(encoded).hexdigest())
