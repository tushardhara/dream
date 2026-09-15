"""Exercise the compiled offline listening consumer, without model/network calls."""
import json
import subprocess

result = subprocess.run(["./bin/hws-listening"], capture_output=True, text=True,
                        timeout=20, check=True)
report = json.loads(result.stdout)
assert report["Version"] == "listening-flow.v1"
assert report["Evidence"] == "synthetic_recorded_fixture"
assert report["SemanticQuality"] == "NOT_TESTED"
alice, bob = report["Responses"]["alice"], report["Responses"]["bob"]
assert alice["Next"] == "acknowledge" and bob["Next"] == "offer_plan"
assert alice["Own"]["DesiredHelp"] == "listen"
assert bob["Own"]["DesiredHelp"] == "coordinate"
assert alice["Shared"][1] == {
    "Speaker": "bob", "Words": "I want a practical plan for tomorrow."}
assert "150" not in json.dumps(alice) and "bob-account" not in json.dumps(alice)
assert "100" not in json.dumps(bob) and "alice-account" not in json.dumps(bob)
replay = subprocess.run(["./bin/hws-listening"], capture_output=True, text=True,
                        timeout=20, check=True)
assert result.stdout == replay.stdout
print("PASS: compiled synthetic listening, distinct goals, exact chosen words, "
      "private account isolation and byte-identical recorded fixture; "
      "semantic/human quality NOT_TESTED")
