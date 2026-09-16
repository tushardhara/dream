"""Regenerate the committed evaluation reports and require them to be unchanged.

docs/evaluation/temporal-v1.json and docs/evaluation/recipient-response-v1.json
are cited as acceptance evidence by #48 and #53, but nothing regenerated or
diffed them: a consumer change that altered either would have merged silently
while the documents kept quoting numbers the code no longer produced (#78).

Both generators are offline and bounded: no database, model, network or
credential. If a consumer change is intended to change a report, regenerate the
JSON in the same commit and this gate passes by construction, leaving the diff
reviewable.

Each run writes into a fresh temporary directory that cannot hold a previous
run's output, and the produced file must exist afterwards. The first version of
this gate wrote into bin/, which persists between runs, so a generator that
exited 0 without writing anything left the earlier artifact in place and the
comparison passed against it — a staleness check that could itself be fooled by
a stale artifact.
"""
import pathlib
import subprocess
import tempfile

REPORTS = [
    ("bin/temporal-report", "docs/evaluation/temporal-v1.json"),
    ("bin/response-report", "docs/evaluation/recipient-response-v1.json"),
]

with tempfile.TemporaryDirectory(prefix="dream-report-check-") as scratch:
    for binary, committed in REPORTS:
        name = pathlib.PurePath(binary).name
        regenerated = pathlib.Path(scratch) / (name + ".json")
        subprocess.run(["./" + binary, "-out", str(regenerated)], check=True, timeout=300)
        assert regenerated.is_file(), (
            "./" + binary + " exited 0 without writing " + str(regenerated) + ". The gate "
            "cannot compare what the generator did not produce; it must never fall back "
            "on an earlier run's output.")
        produced = regenerated.read_bytes()
        expected = pathlib.Path(committed).read_bytes()
        assert produced, "./" + binary + " wrote an empty report"
        assert produced == expected, (
            committed + " is stale: the generator now produces " + str(len(produced)) +
            " bytes against the committed " + str(len(expected)) + ". Regenerate it in "
            "the same commit as the consumer change and explain the diff, or fix the "
            "change; never edit the committed report by hand.")
        print("PASS: " + committed + " reproduces byte-for-byte (" + str(len(produced)) + " bytes)")

print("PASS: committed evaluation reports regenerate byte-identically from the "
      "compiled generators, each into a fresh directory; offline synthetic fixtures "
      "only, no provider, database or network; human validity NOT_TESTED")
