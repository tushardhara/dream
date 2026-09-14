"""Fresh disposable backend demo/replay/export acceptance; fake only, no deployment.
Keeps the larger demo separate from the existing 120-second migration suite.
"""
import signal

def interrupted(signum, frame):
    raise KeyboardInterrupt("owned check interrupted")

signal.signal(signal.SIGTERM, interrupted)

import os
import pathlib
import json
import subprocess
import time
import tempfile
import uuid


def command(*args, **kwargs):
    return subprocess.run(args, check=True, text=True, capture_output=True,
                          timeout=60, **kwargs).stdout.strip()


name = "dream-demo-" + uuid.uuid4().hex[:12]
image = None
scratch = tempfile.TemporaryDirectory(prefix="dream-study-image-")
try:
    image_context = pathlib.Path(scratch.name)
    build_env = dict(os.environ, CGO_ENABLED="0", GOTOOLCHAIN="go1.27.1", DOCKER_CONFIG=scratch.name)
    command("go", "build", "-trimpath", "-o", str(image_context / "hws-generate"), "./cmd/hws-generate", env=build_env)
    (image_context / "Dockerfile").write_text(
        'FROM scratch\nCOPY hws-generate /hws-generate\nUSER 65532:65532\nLABEL dream.study.fixture="' + name + '"\nENTRYPOINT ["/hws-generate"]\n')
    command("docker", "build", "--force-rm", "--label", "dream.test.run=" + os.environ.get("DREAM_TEST_RUN", "standalone"), "--label", "dream.test.role=" + os.environ.get("DREAM_TEST_ROLE", "standalone"), "--network=none", "--iidfile", str(image_context / "id"), scratch.name, env=build_env)
    image = (image_context / "id").read_text().strip()
    command("docker", "run", "--detach", "--rm", "--name", name,
            "--label", "dream.disposable=true", "--label", "dream.test.run=" + os.environ.get("DREAM_TEST_RUN", "standalone"), "--label", "dream.test.role=" + os.environ.get("DREAM_TEST_ROLE", "standalone"), "--cpus", "2", "--memory", "512m",
            "--pids-limit", "128", "--tmpfs", "/var/lib/postgresql",
            "--publish", "127.0.0.1::5432", "--env", "POSTGRES_PASSWORD=disposable_local_only",
            "postgres:18.6")
    for _ in range(60):
        if subprocess.run(["docker", "exec", name, "pg_isready", "-U", "postgres"],
                          capture_output=True).returncode == 0:
            break
        time.sleep(1)
    else:
        raise RuntimeError("owned demo database did not become ready")
    port = command("docker", "port", name, "5432/tcp").rsplit(":", 1)[1]
    artifacts = pathlib.Path("bin") / ("demo-run-" + uuid.uuid4().hex[:12])
    artifacts.mkdir(parents=True, mode=0o700)
    env = dict(os.environ, DREAM_STUDY_TEST_IMAGE=image, DREAM_DEMO_REPORT_DIR=str(artifacts.resolve()), GOTOOLCHAIN="go1.27.1", DREAM_DEMO_FULL="1",
               DREAM_TEST_DSN=f"postgres://postgres:disposable_local_only@127.0.0.1:{port}/postgres?sslmode=disable")
    started = time.monotonic()
    subprocess.run(["go", "test", "-count=1", "-timeout=240s", "-v", "./adapters/postgres", "./adapters/evaluation",
                    "-run", "^(TestDemoCLIIntegration|TestGraphClientIntegration|TestStudyJournalIntegration)$"],
                   env=env, check=True, timeout=270)
    report = {"version": "engineering-demo-acceptance.v1", "source_commit": command("git", "rev-parse", "HEAD"),
              "source_dirty": bool(command("git", "status", "--porcelain")), "demo_and_study_protocol_checks": "pass", "full_program_engineering_gate": "not-tested-by-this-command",
              "measured_total_seconds": time.monotonic()-started, "postgres": {"version": "18.6", "cpus": 2, "memory_mib": 512},
              "behavioral_evidence": "synthetic_only_inconclusive", "real_human_validity": "not-tested",
              "cross_model_validity": "not-tested", "30_real_day_study": "not-run", "live_api_calls": 0,
              "incurred_api_cost_micros": 0, "complete_hws_source_falsifiers": "not-tested",
              "evidence": ["TestDemoCLIIntegration", "TestGraphClientIntegration", "TestStudyJournalIntegration"],
              "skipped": ["live providers", "paid/deployed 30-day run", "real human data", "owner UI entry decision"],
              "falsifiers": [
                  {"id": "recorded_replay", "status": "pass", "evidence": "compiled replay plus derived-state verification"},
                  {"id": "checkpoint_recovery", "status": "pass", "evidence": "partial/resumed vs fresh world hashes"},
                  {"id": "observed_privacy_cases", "status": "pass", "evidence": "own export canaries, unknown actor, study revocation; not a universal privacy guarantee"},
                  {"id": "behavioral_realism", "status": "inconclusive", "evidence": "sparse synthetic reference dynamics only"},
                  {"id": "calibration", "status": "not-tested", "evidence": "separate make evaluation-check; no human sample"},
                  {"id": "cross_model_transfer", "status": "not-tested", "evidence": "two fake ports are not two frontier models"},
                  {"id": "real_human_validity", "status": "not-tested", "evidence": "no real human data"},
                  {"id": "30_real_day_study", "status": "not-tested", "evidence": "protocol tests, not an elapsed 30-day run"},
                  {"id": "complete_hws_source_falsifiers", "status": "not-tested", "evidence": "original source register unavailable; docs/requirements.md"}
              ], "artifacts": str(artifacts)}
    (artifacts / "acceptance.json").write_text(json.dumps(report, indent=2)+"\n")
    print("Retained synthetic artifacts and measured acceptance: " + str(artifacts))
    print(f"PASS: demo, recovery, replay, own-view export and core-only second host; elapsed={time.monotonic()-started:.3f}s")
    print("PostgreSQL 18.6: 2 CPU / 512MiB disposable cluster. Fake/recorded only; live API cost=0; 30-real-day study NOT RUN.")
finally:
    if image:
        subprocess.run(["docker", "image", "rm", image], capture_output=True, timeout=30)
    scratch.cleanup()
    subprocess.run(["docker", "rm", "--force", name], capture_output=True, timeout=30)
