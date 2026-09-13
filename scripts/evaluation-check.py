"""One offline command builds an isolated generator and reproduces synthetic reports.
No registry pull, mounted labels, network, private data or policy activation.
"""
import json
import os
import pathlib
import socket
import sys
import subprocess
import tempfile

ROOT = pathlib.Path(__file__).resolve().parents[1]


def command(args, env, timeout=180):
    result = subprocess.run(args, cwd=ROOT, env=env, text=True,
                            capture_output=True, timeout=timeout)
    if result.returncode:
        sys.stderr.write((result.stdout + result.stderr)[-8000:])
        raise RuntimeError("offline evaluation command failed: " + args[0])
    return result.stdout


def main():
    with tempfile.TemporaryDirectory(prefix="dream-evaluation-") as scratch:
        path = pathlib.Path(scratch)
        env = dict(os.environ, GOTOOLCHAIN="go1.27.1")
        env.setdefault("DOCKER_CONFIG", scratch)
        context = path / "image"
        context.mkdir()
        build_env = dict(env, CGO_ENABLED="0", GOOS="linux", GOARCH="amd64")
        command(["go", "build", "-trimpath", "-o", str(context / "hws-generate"),
                 "./cmd/hws-generate"], build_env)
        (context / "Dockerfile").write_text("FROM scratch\nCOPY hws-generate /hws-generate\nUSER 65532:65532\nENTRYPOINT [\"/hws-generate\"]\n")
        iid = path / "image-id"
        command(["docker", "build", "--network=none", "--iidfile", str(iid), str(context)], env)
        image = iid.read_text().strip()
        binary = path / "hws-eval"
        command(["go", "build", "-trimpath", "-o", str(binary), "./cmd/hws-eval"], env)
        args = [str(binary), "--synthetic", "--generator-image", image]
        first = command(args, env)
        second = command(args, env)
        if first != second:
            raise RuntimeError("offline report bytes are not reproducible")
        report = json.loads(first)
        if report["real_human_validity"] != "not-tested" or not report["synthetic_only"]:
            raise RuntimeError("invalid scientific claim")
        evidence = ROOT / "bin" / "evaluation-report.json"
        evidence.parent.mkdir(exist_ok=True)
        evidence.write_text(first)
        # A deliberately curious child proves actual OS permissions, beyond the
        # import guard and DTO canary tests. Every marker is synthetic and local.
        secret = path / "evaluator-label.json"
        secret.write_text("SYNTHETIC_EVALUATOR_ONLY_CANARY")
        secret.chmod(0o600)
        listener = socket.socket()
        listener.bind(("127.0.0.1", 0))
        listener.listen(1)
        address = "127.0.0.1:" + str(listener.getsockname()[1])
        probe = path / "probe.go"
        probe.write_text('package main\nimport("os";"net";"time";"fmt";"strings")\nfunc main(){' +
                         'if os.Getuid()!=65532 {os.Exit(11)};' +
                         'if xs,e:=net.Interfaces();e!=nil||len(xs)!=1||xs[0].Name!="lo" {os.Exit(17)};' +
                         'mounts,e:=os.ReadFile("/proc/mounts");if e!=nil {os.Exit(18)};rootRO:=false;for _,line:=range strings.Split(string(mounts),"\\n"){f:=strings.Fields(line);if len(f)>3&&f[1]=="/"{for _,opt:=range strings.Split(f[3],","){rootRO=rootRO||opt=="ro"}}};if !rootRO {os.Exit(19)};' +
                         'if _,e:=os.Stat(' + json.dumps(str(secret)) + ');e==nil {os.Exit(12)};' +
                         'if os.Getenv("DREAM_EVALUATOR_LABEL")!="" {os.Exit(13)};' +
                         'if c,e:=net.DialTimeout("tcp",' + json.dumps(address) + ',time.Second);e==nil {c.Close();os.Exit(14)};' +
                         'if e:=os.WriteFile("/forbidden-write",[]byte("x"),0600);e==nil {os.Exit(15)};' +
                         'if _,e:=os.Stat("/var/run/docker.sock");e==nil {os.Exit(16)};' +
                         'fmt.Println("[]")}\n')
        command(["go", "build", "-trimpath", "-o", str(context / "hws-generate"), str(probe)], build_env)
        probe_iid = path / "probe-image-id"
        command(["docker", "build", "--network=none", "--iidfile", str(probe_iid), str(context)], env)
        test_env = dict(env, DREAM_EVALUATOR_TEST_IMAGE=image,
                        DREAM_EVALUATOR_PROBE_IMAGE=probe_iid.read_text().strip(),
                        DREAM_EVALUATOR_LABEL="SYNTHETIC_ENV_CANARY")
        try:
            print(command(["go", "test", "-count=1", "./adapters/evaluation", "./cmd/hws-eval",
                           "./cmd/hws-generate", "./evals", "./internal/architecture"], test_env))
        finally:
            listener.close()
            subprocess.run(["docker", "image", "rm", probe_iid.read_text().strip()],
                           cwd=ROOT, env=env, capture_output=True, timeout=30)
        print("PASS: byte-identical offline synthetic reports; hash=" + report["hash"])
        print("Report: bin/evaluation-report.json; generator=" + image)
        print("Human validity NOT TESTED; no promotion, live provider or deployment.")


if __name__ == "__main__":
    main()
