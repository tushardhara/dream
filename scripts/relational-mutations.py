"""#42 relationship assertion controls. Run exclusively under scripts/disk-guard.py.
Each mutation requires a green baseline, an actual assertion failure, byte-exact
restoration even on interruption, and a green restored baseline. Synthetic only.
"""
import os
import pathlib
import signal
import subprocess


def interrupted(signum, frame):
    raise KeyboardInterrupt("owned mutation interrupted")


signal.signal(signal.SIGTERM, interrupted)
root = pathlib.Path(__file__).resolve().parents[1]
if not os.environ.get("DREAM_TEST_RUN"):
    raise RuntimeError("run under the disk guard")
logs = root / "bin" / "relational-mutations" / os.environ["DREAM_TEST_RUN"]
logs.mkdir(parents=True, exist_ok=False)
probes = [('observer', 'core/relationship.go', 'r.Observer != observer', 'false && r.Observer != observer', 'TestRelationshipSourceFieldsAndUnknown'), ('retrieval', 'simulator/behavior/relationships.go', 'if !sources[id] {', 'if false && !sources[id] {', 'TestRelationshipAccessAndTimeFailClosed'), ('valid-time', 'simulator/behavior/relationships.go', 'at < r.Valid.Start', 'false && at < r.Valid.Start', 'TestRelationshipAccessAndTimeFailClosed'), ('expectation', 'simulator/behavior/relationships.go', 'c.RoleExpectation = measures["expectation"]', 'c.RoleExpectation = ContextValue{}', 'TestRelationalControlledSpecificityAndAblations'), ('role-shortcut', 'simulator/behavior/relationships.go', 'c.Trust = measures["trust"]', 'c.Trust = measures["trust"]; if r.Types[0] == "spouse" { c.Trust.Value = -.99 }', 'TestRelationalControlledSpecificityAndAblations'), ('appraisal', 'simulator/behavior/relationships.go', 'if focus {', 'if false && focus {', 'TestRelationalControlledSpecificityAndAblations'), ('graph-lineage', 'app/graph/relations.go', 'references = append(references, v.Context.Sources()...)', 'references = references', 'TestRelationshipV2CodecPermissionEnvelope'), ('graph-time', 'app/graph/relations.go', '!sameRelationInterval(v.Context.Valid, r.Event.Meta.Valid)', 'false', 'TestRelationshipV2CodecPermissionEnvelope'), ('checkpoint-hash', 'simulator/demo/world.go', 'h != cp.WorldHash', 'false && h != cp.WorldHash', 'TestRelationalFiveAnd24CommonEngineRecovery'), ('five-topology', 'simulator/demo/relational_scenario.go', '{1, 3, "acquaintance"}', '{1, 4, "acquaintance"}', 'TestFivePersonDirectionalTopology'), ('export-observer', 'app/hws/demo.go', 'for _, d := range world.ActionDecisions {\n\t\tif d.Actor == actor {', 'for _, d := range world.ActionDecisions {\n\t\tif true {', 'TestRelationalDemoActorExportAndVersionBinding'), ('scenario-version', 'simulator/scenario/types.go', 'if len(a.Contexts) > 0 {', 'if false && len(a.Contexts) > 0 {', 'TestRelationalScenarioRequiresVersionedCapability')]
probes.extend([
    ("causal-reply", "simulator/demo/relational_world.go", "replyTo = incoming.Decision", 'replyTo = ""', "TestRelationalRepliesAreCausalAndYearBounded"),
    ("actual-help", "simulator/demo/relational_world.go", "w.Resources[selected.Resource] -= selected.Units", "w.Resources[selected.Resource] -= 0 * selected.Units", "TestRelationalRepliesAreCausalAndYearBounded"),
])

for name, file, old, new, test in probes:
    path = root / file
    original = path.read_bytes()
    if original.count(old.encode()) != 1:
        raise RuntimeError(name + ": expected one mutation target")
    command = ["go", "test", "-count=1", "-timeout=90s", ("./simulator/demo" if name == "scenario-version" else "./" + str(path.parent.relative_to(root))), "-run", "^(" + test + ")$"]
    subprocess.run(command, cwd=root, check=True, timeout=120)
    try:
        path.write_bytes(original.replace(old.encode(), new.encode()))
        result = subprocess.run(command, cwd=root, text=True, capture_output=True, timeout=120)
        output = result.stdout + result.stderr
        if len(output.encode()) > 1024 * 1024:
            raise RuntimeError(name + ": mutation output limit")
        (logs / (name + ".log")).write_text(output)
        if result.returncode == 0 or "--- FAIL:" not in output or "[build failed]" in output or "test timed out" in output:
            raise RuntimeError(name + ": no assertion failure: " + output[-800:])
        print("CAUGHT " + name, flush=True)
    finally:
        path.write_bytes(original)
    subprocess.run(command, cwd=root, check=True, timeout=120)
print(f"All {len(probes)} controls caught; bytes restored and baselines green.")
