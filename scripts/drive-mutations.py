"""Exclusive-worktree synthetic negative controls for alignment #40.
Requires green baselines, actual test assertion failures, and restores every byte.
No live providers, merges, deployments or private data.
"""
import signal

def interrupted(signum, frame):
    raise KeyboardInterrupt("owned check interrupted")

signal.signal(signal.SIGTERM, interrupted)

import pathlib
import subprocess
root = pathlib.Path(__file__).resolve().parents[1]
logs = root / 'bin' / 'drive-mutations'
logs.mkdir(parents=True, exist_ok=True)
probes = [
 ('registry-order', 'simulator/drives/registry.go', '"acquisition"', '"comparison"', '^TestExactRegistryWireContract$'),
 ('legacy-dispatch', 'simulator/drives/codec.go', 'header.Version == dynamics.Version &&', 'false && header.Version == dynamics.Version &&', '^TestNewCodecAndLegacyReplay$'),
 ('context-permission', 'simulator/drives/appraisal.go', 'if e := c.Evidence.Validate(actor, at); e != nil {', 'if e := c.Evidence.Validate(actor, at); false && e != nil {', '^TestDrivePermissionReceiptAndBounds$'),
 ('context-contribution', 'simulator/drives/appraisal.go', 'c[i] = cue.Value * float64(cue.Evidence.Confidence)', 'c[i] = 0 * cue.Value * float64(cue.Evidence.Confidence)', '^TestAllContextDimensionsAndCompetingTendencies$'),
 ('inert-drive', 'simulator/drives/appraisal.go', 'delta := math.Max(-1, math.Min(1, value)) * definitions[i].Gain * gain', 'delta := math.Max(-1, math.Min(1, value)) * definitions[i].Gain * gain; if i == Meaning { delta = 0 }', '^TestPerDriveSensitivity$'),
 ('legacy-policy', 'simulator/dynamics/registry.go', '168 * Hour, .08, .1', '169 * Hour, .08, .1', '^TestSavedLegacyCheckpointAndUpgrade$'),
 ('decay-anchor', 'simulator/drives/state.go', '(v.Values[1]-baseline)', '(v.Values[0]-baseline)', '^TestDriveDecayPartitionAndConfidence$'),
]
for name, file, old, new, test in probes:
    command = ['go', 'test', '-count=1', './simulator/drives', '-run', test]
    subprocess.run(command, cwd=root, check=True, timeout=120)
    path = root / file
    original = path.read_text()
    if original.count(old) != 1:
        raise RuntimeError(name + ': expected one guard')
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
print('All seven controls caught; source restored and baselines green.')
