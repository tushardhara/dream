"""Regenerate the committed evaluation reports and require them to be unchanged.

docs/evaluation/temporal-v1.json and docs/evaluation/recipient-response-v1.json
are cited as acceptance evidence by #48 and #53, but nothing regenerated or
diffed them: a consumer change that altered either would have merged silently
while the documents kept quoting numbers the code no longer produced (#78).

Both generators are offline and bounded: no database, model, network or
credential. If a consumer change is intended to change a report, regenerate the
JSON in the same commit and this gate passes by construction, leaving the diff
reviewable.
"""
import pathlib
import subprocess

REPORTS = [
    ("bin/temporal-report", "docs/evaluation/temporal-v1.json"),
    ("bin/response-report", "docs/evaluation/recipient-response-v1.json"),
]

pathlib.Path("bin").mkdir(exist_ok=True)
for binary, committed in REPORTS:
    regenerated = binary + ".json"
    subprocess.run(["./" + binary, "-out", regenerated], check=True, timeout=300)
    produced = pathlib.Path(regenerated).read_bytes()
    expected = pathlib.Path(committed).read_bytes()
    assert produced == expected, (
        committed + " is stale: the generator now produces " + str(len(produced)) +
        " bytes against the committed " + str(len(expected)) + ". Regenerate it in "
        "the same commit as the consumer change and explain the diff, or fix the "
        "change; never edit the committed report by hand.")
    print("PASS: " + committed + " reproduces byte-for-byte (" + str(len(produced)) + " bytes)")

print("PASS: committed evaluation reports regenerate byte-identically from the "
      "compiled generators; offline synthetic fixtures only, no provider, database "
      "or network; human validity NOT_TESTED")
