"""Compiled opt-in native group choices, separate consequences and replay."""
import json
import subprocess


def run(n, *args):
    raw = subprocess.run(["./bin/hws-group", "--people", str(n), *args],
                         capture_output=True, text=True, timeout=30,
                         check=True).stdout
    return raw, json.loads(raw)


for n in (5, 24):
    raw, r = run(n)
    _, omitted = run(n, "--omitted")
    _, burden = run(n, "--care-burden", "9")
    assert r["Version"] == "group-demo.v1"
    assert r["HumanValidity"] == "NOT_TESTED"
    assert r["GlobalWelfare"] == "NOT_AGGREGATED"
    assert len(r["Native"]["Traces"]) == n
    assert len(r["Native"]["Responses"]) == n - 1
    assert len(r["Helper"]["Perspectives"]) == n
    assert len(r["Helper"]["Alternatives"]) == 3
    a, b = r["Native"]["Traces"][-1], omitted["Native"]["Traces"][-1]
    assert a["DyadicHash"] == b["DyadicHash"]
    assert a["Signals"]["inclusion"] == 1
    assert b["Signals"]["exclusion"] == 2 / 3
    assert r["Helper"]["Perspectives"][0] == burden["Helper"]["Perspectives"][0]
    assert burden["Helper"]["Perspectives"][-1]["Burden"]["Value"] == 9
    assert raw == run(n)[0]
    # Native choice is sampled without a success quota. A separate fixed seed
    # sweep in Go proves both real reservations and changed selected actions.
    for receipt in r["Reservations"]:
        assert receipt["Option"] == "shared-care"
        assert sum(t["Units"] for t in receipt["Tasks"]) == 4
print("PASS: native 5/24 group history, separate effects, agreed resource receipts "
      "and exact replay; human validity NOT_TESTED; welfare NOT_AGGREGATED")
