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
 ('fatigue-response', 'simulator/drives/appraisal.go', '.4*s.Factors.Variables[Fatigue].Values[0]', '.4*v(EffortAvoidance)', '^TestRetainedFactorResponseSensitivity$'),
 ('residue-response', 'simulator/drives/appraisal.go', '.2*s.Factors.Variables[SlowResidue].Values[0]', '.2*v(EffortAvoidance)', '^TestRetainedFactorResponseSensitivity$'),
 ('scarcity-response', 'simulator/drives/appraisal.go', '.1*s.Factors.Variables[ScarcityOpportunity].Values[0]', '.1*v(Safety)', '^TestRetainedFactorResponseSensitivity$'),
 ('legacy-appraisal-gain', 'simulator/dynamics/registry.go', '8 * Hour, .25, .2', '8 * Hour, .26, .2', '^TestSavedLegacyAppraisalContinuation$'),
 ('factor-order', 'simulator/drives/appraisal.go', 'effortBurden :=', 'out.Factors = out.Factors.appraise(o.Event, out.Substrate, at); effortBurden :=', '^TestRetainedFactorAppraisalOrdering$'),
 ('wire-version', 'simulator/drives/state.go', 's.Version != Version', 'false && s.Version != Version', '^TestRetainedFactorWireVersion$'),
 ('upgrade-authority', 'app/hws/snapshots.go', 'if e := s.authorize(ctx, p, spec.Source.Scope, core.Derive); e != nil {', 'if e := s.authorize(ctx, p, spec.Source.Scope, core.Derive); false && e != nil {', '^TestUpgradeLegacyIsNotBranchAuthority$'),
 ('inert-drive', 'simulator/drives/appraisal.go', 'delta := math.Max(-1, math.Min(1, value)) * definitions[i].Gain * gain', 'delta := math.Max(-1, math.Min(1, value)) * definitions[i].Gain * gain; if i == Meaning { delta = 0 }', '^TestPerDriveSensitivity$'),
 ('legacy-policy', 'simulator/dynamics/registry.go', '168 * Hour, .08, .1', '169 * Hour, .08, .1', '^TestSavedLegacyCheckpointAndUpgrade$'),
 ('decay-anchor', 'simulator/drives/state.go', '(v.Values[1]-baseline)', '(v.Values[0]-baseline)', '^TestDriveDecayPartitionAndConfidence$'),
]
for name, file, old, new, test in probes:
    package = './app/hws' if file.startswith('app/hws/') else './simulator/drives'
    command = ['go', 'test', '-count=1', package, '-run', test]
    subprocess.run(command, cwd=root, check=True, timeout=120)
    path = root / file
    original = path.read_text()
    if original.count(old) != 1:
        raise RuntimeError(name + ': expected one guard')
    try:
        mutated = original.replace(old, new)
        if name == 'factor-order':
            # Move the update, do not duplicate it and accidentally test doubling.
            late = '\n\tout.Factors = out.Factors.appraise(o.Event, out.Substrate, at)\n'
            if mutated.count(late) != 1:
                raise RuntimeError('expected one late factor update')
            mutated = mutated.replace(late, '\n')
        path.write_text(mutated)
        result = subprocess.run(command, cwd=root, text=True, capture_output=True, timeout=120)
        output = result.stdout + result.stderr
        (logs / (name + '.log')).write_text(output)
        if result.returncode == 0 or '--- FAIL:' not in output:
            raise RuntimeError(name + ': no assertion failure: ' + output[-800:])
        print('CAUGHT ' + name, flush=True)
    finally:
        path.write_text(original)
    subprocess.run(command, cwd=root, check=True, timeout=120)
print(f'All {len(probes)} controls caught; source restored and baselines green.')
