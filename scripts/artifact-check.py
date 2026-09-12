"""Fail closed when future artifacts need a real generation/migration gate."""
from pathlib import Path
import sys

mode = sys.argv[1]
if mode == "generated":
    artifacts = list(Path("api").rglob("*.proto")) + list(Path(".").rglob("*.pb.go"))
    for p in Path(".").rglob("*.go"):
        if any(line.startswith("//go:generate ") for line in p.read_text().splitlines()):
            artifacts.append(p)
elif mode == "migrations":
    artifacts = [p for p in Path("migrations").rglob("*") if p.is_file() and p.name != "README.md"]
else:
    raise SystemExit("unknown artifact check")
if artifacts:
    raise SystemExit(f"BLOCKED: implement real {mode} validation before adding: {artifacts}")
print(f"N/A: no {mode} inputs exist in bootstrap; no validation claimed.")
