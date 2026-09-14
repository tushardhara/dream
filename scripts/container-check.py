"""Build/smoke only: nonroot, networkless, read-only disposable containers.
No publication, deployment, provider, daemon configuration or unrelated cleanup.
"""
import signal

def interrupted(signum, frame):
    raise KeyboardInterrupt("owned check interrupted")

signal.signal(signal.SIGTERM, interrupted)

import json
import os
import pathlib
import subprocess
import tempfile
import uuid


def command(args, **kwargs):
    return subprocess.run(args, check=True, text=True, capture_output=True, timeout=120, **kwargs).stdout.strip()


with tempfile.TemporaryDirectory(prefix="dream-image-check-") as scratch:
    env = dict(os.environ, GOTOOLCHAIN="go1.27.1", CGO_ENABLED="0", GOOS="linux", GOARCH="amd64")
    # Scratch-based build needs no registry authentication; do not copy secrets
    # or require write access to a user's Docker configuration directory.
    env.setdefault("DOCKER_CONFIG", scratch)
    pathlib.Path("bin/container").mkdir(parents=True, exist_ok=True)
    for name in ("hws", "hws-api", "hws-worker", "hws-admin"):
        command(["go", "build", "-trimpath", "-o", "bin/container/" + name, "./cmd/" + name], env=env)
    revision = command(["git", "rev-parse", "HEAD"])
    iid = pathlib.Path(scratch) / "image-id"
    command(["docker", "build", "--force-rm", "--label", "dream.test.run=" + os.environ.get("DREAM_TEST_RUN", "standalone"), "--label", "dream.test.role=" + os.environ.get("DREAM_TEST_ROLE", "standalone"), "--network=none", "--iidfile", str(iid), "--build-arg", "REVISION=" + revision, "."], env=env)
    image = iid.read_text().strip()
    details = json.loads(command(["docker", "image", "inspect", image], env=env))[0]
    if details["Config"]["User"] != "65532:65532":
        raise RuntimeError("runtime image is not nonroot")
    for entry in ("hws", "hws-api", "hws-worker", "hws-admin"):
        name = "dream-image-check-" + uuid.uuid4().hex[:12]
        try:
            command(["docker", "run", "--rm", "--name", name, "--label", "dream.disposable=true", "--label", "dream.test.run=" + os.environ.get("DREAM_TEST_RUN", "standalone"), "--label", "dream.test.role=" + os.environ.get("DREAM_TEST_ROLE", "standalone"),
                     "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges",
                     "--memory=128m", "--cpus=1", "--pids-limit=32", "--entrypoint", "/" + entry,
                     image, "--help"], env=env)
        finally:
            subprocess.run(["docker", "rm", "--force", name], env=env, capture_output=True, timeout=30)
    print("PASS: nonroot static linux/amd64 image, four networkless/read-only help checks; image=" + image)
