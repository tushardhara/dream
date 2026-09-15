"""Exercise compiled multi-period repair, resource effects and honest verdicts."""
import json
import subprocess

def run():
    return subprocess.run(["./bin/hws-repair"], capture_output=True, text=True,
                          timeout=20, check=True).stdout
raw = run()
r = json.loads(raw)
assert r["Version"] == "repair-flow.v1"
assert r["Evidence"] == "authored_synthetic_commands_and_reports"
assert r["HumanValidity"] == "NOT_TESTED"
broken, followed = r["Scenarios"]
assert broken["Periods"][-1]["Expectation"] == "repeated_breach_observed"
assert followed["Periods"][-1]["Expectation"] == "sustained_follow_through_observed"
assert followed["Periods"][1]["Expectation"] == "new_follow_through_observed"
assert broken["RemainingHours"] == 8 and followed["RemainingHours"] == 6
assert broken["Periods"][-1]["Next"] == "consider_selective_distance"
assert followed["Periods"][-1]["Next"] == "consider_bounded_plan"
for s in r["Scenarios"]:
    assert s["Pause"]["Next"] == "respect_pause"
    assert s["Ending"]["Next"] == "respect_ending"
    assert s["Ending"]["Options"] == ["WAIT"]
    assert len(s["EvidenceHash"]) == 64
    assert {"promise", "decline", "withdraw", "leave", "wait"} <= set(s["Actions"])
    for p in s["Periods"] + [s["Pause"], s["Ending"]]:
        assert p["RepairVerdict"] == "NOT_ASSESSED" and p["ConfirmationRequests"] == 0
    alice, bob = s["Periods"][-1]["Perspectives"]
    assert alice["Later"]["Assessment"] == "eased"
    assert bob["Immediate"]["Assessment"] == "eased"
    assert bob["Later"]["Assessment"] in ("mixed", "worse")
assert raw == run()
print("PASS: compiled repair command effects, later independent evidence, contrary "
      "accounts, selective pause/ending and replay; human validity NOT_TESTED")
