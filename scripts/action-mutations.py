"""#41 assertion controls. Run exclusively under scripts/disk-guard.py.
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
logs = root / "bin" / "action-mutations" / os.environ["DREAM_TEST_RUN"]
logs.mkdir(parents=True, exist_ok=False)
probes = [
    ("late-fulfillment", "app/hws/action_cognitive.go", "s.Observation.Event.OccurredAt > p.Due && frame.Response.CommitmentStatus == \"fulfilled\"", "false && s.Observation.Event.OccurredAt > p.Due && frame.Response.CommitmentStatus == \"fulfilled\"", "TestActionPromiseActualFulfillmentAndBreach"),
    ("legacy-competing-score", "simulator/behavior/behavior.go", "score = .2 + t.Approach + .4*trust + .3*disclosure", "score = .2 + t.Approach + .9*trust + .3*disclosure", "TestFrozenLegacyCompetingActionsV1"),
    ("legacy-trust", "simulator/behavior/behavior.go", "score = .2 + t.Support + .3*trust", "score = .2 + t.Support + .4*trust", "TestFrozenLegacyContextActionsV1"),
    ("legacy-belief", "simulator/behavior/behavior.go", "score = .2 + t.Approach + .2*belief", "score = .2 + t.Approach + .3*belief", "TestFrozenLegacyContextActionsV1"),
    ("legacy-disclosure", "simulator/behavior/behavior.go", "score = .2 + t.Approach + .4*trust + .3*disclosure", "score = .2 + t.Approach + .4*trust + .4*disclosure", "TestFrozenLegacyContextActionsV1"),
    ("wait-no-effect", "simulator/behavior/action_choice.go", 'if selected.Kind != Wait {\n\t\tnext.AvailableAt', 'if true {\n\t\tnext.AvailableAt', "TestWaitHasNoAvailabilityOrContactEffect"),
    ("softened-semantics", "app/graph/fiction.go", '\"I feel a little \" + feeling + \"' + ", though I'm not ready to say more." + '\"', '\"Let\'s change the topic.\"', "TestTypedFictionModesAndAssistantBoundary"),
    ("registry-order", "simulator/behavior/actions.go", '"say"', '"statement"', "TestExact27ActionRegistry"),
    ("context-role", "simulator/behavior/action_choice.go", ".1*c.RoleExpectation.weighted()", "0*c.RoleExpectation.weighted()", "TestActionContextSensitivityUnknownAndForeignEvidence"),
    ("delivery-time", "simulator/behavior/action_choice.go", "e.OccurredAt < o.EffectAt", "false", "TestActionCannotLearnBeforeDelivery"),
    ("left-eligibility", "simulator/behavior/action_choice.go", 'a.Contact == "left" && o.Kind != Reconnect', 'false && a.Contact == "left" && o.Kind != Reconnect', "TestActionResourcesCommitmentsAndContactConstraints"),
    ("stage-validation", "simulator/behavior/action_choice.go", 'v != want', 'false && v != want', "TestActionLifecycleDeterminismLearningAndTampering"),
    ("promise-fulfillment", "app/hws/action_cognitive.go", 'c.Commitments[j].Status = frame.Response.CommitmentStatus', 'c.Commitments[j].Status = "pending"', "TestActionPromiseActualFulfillmentAndBreach"),
    ("help-allocation", "app/hws/action_cognitive.go", 'out.Consume = []rt.Consumption{{Resource: selected.Resource, Units: selected.Units}}', 'out.Consume = []rt.Consumption{{Resource: selected.Resource, Units: selected.Units + 1}}', "TestAll27ActionsExecuteBoundedEffects"),
    ("codec-policy", "app/hws/action_cognitive.go", 'c.Version != 2', 'false && c.Version != 2', "TestActionRuntimeReplaySizeAndVersionDispatch"),
    ("fiction-assistant", "app/graph/fiction.go", 'mode != SyntheticSelfDisclosure', 'false && mode != SyntheticSelfDisclosure', "Test.*Fiction.*"),
]
for name, file, old, new, test in probes:
    path = root / file
    original = path.read_bytes()
    if original.count(old.encode()) != 1:
        raise RuntimeError(name + ": expected one mutation target")
    command = ["go", "test", "-count=1", "-timeout=90s", "./" + str(path.parent.relative_to(root)), "-run", "^(" + test + ")$"]
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
