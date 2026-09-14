"""Opt-in #17 guard-removal controls. Run only in an exclusive idle worktree.
Every probe needs a green baseline and an actual assertion failure; restore bytes
in finally. Uses synthetic tests only, with no provider, merge or deployment.
"""
import signal

def interrupted(signum, frame):
    raise KeyboardInterrupt("owned check interrupted")

signal.signal(signal.SIGTERM, interrupted)

import pathlib
import subprocess

root = pathlib.Path(__file__).resolve().parents[1]
logs = root / 'bin' / 'demo-mutations'
logs.mkdir(parents=True, exist_ok=True)
probes = [
    ('projection-hash', 'simulator/demo/world.go', 'state.Data != "" && h != cp.WorldHash', 'false && state.Data != "" && h != cp.WorldHash', './simulator/demo', '^TestTwentyFourPersonYearReconstructionAndRecovery$'),
    ('unseen-group', 'simulator/demo/world.go', 'if visible {', 'if visible || true {', './simulator/demo', '^TestDemoInitialDerivationAndUnseenGroup$'),
    ('real-study-overclaim', 'app/hws/demo.go', 'a.RealStudy != "not-run" || ', '', './app/hws', '^TestDemoCannotClaimRealStudyOrAcceptCorruption/study$'),
    ('study-quota', 'evals/study.go', 's.Reserved+event.Reserved > s.Plan.TotalPredictions || ', '', './evals', '^TestStudyQuotaUncertainRecoveryAndRepeatedFailure$'),
    ('study-launch-deadline', 'evals/study.go', 'ctx.Err() != nil || !c.Clock.Now().Before(s.Deadline) || !c.Clock.Now().Before(reserved.At.Add(5*time.Minute))', 'ctx.Err() != nil', './evals', '^TestStudyStopsNewProviderAtFixedDeadline$'),
]
for name, file, old, new, package, test in probes:
    command = ['go', 'test', '-count=1', package, '-run', test]
    subprocess.run(command, cwd=root, check=True, timeout=120)
    path = root / file
    original = path.read_text()
    if original.count(old) != 1:
        raise RuntimeError(name + ': expected exactly one guard')
    try:
        path.write_text(original.replace(old, new))
        result = subprocess.run(command, cwd=root, text=True, capture_output=True, timeout=120)
        output = result.stdout + result.stderr
        (logs / (name + '.log')).write_text(output)
        if result.returncode == 0 or '--- FAIL:' not in output:
            raise RuntimeError(name + ': no assertion failure: ' + output[-800:])
        print('CAUGHT ' + name, flush=True)
    finally:
        path.write_text(original)
    subprocess.run(command, cwd=root, check=True, timeout=120)
print('All five guards restored; all focused baselines green.')
