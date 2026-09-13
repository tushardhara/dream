# ADR-0017: complete versioned HWS section 6 drive registry

Authority: owner-supplied extracted registry in issue #40, alignment epic #39.
Raw PRDs remain private. The 22 names and order are now available; this closes
only the drive-name registry gap, not the action or falsifier source gaps.

`simulator/drives` implements state codec2, registry `hws-section6.v1` and model
`appraisal.drives.v1`. Registry() returns an immutable value copy. RegistryHash
pins the entire ordered definition array, including baseline/gain/half-life.
Names have exactly the order in issue40; they are research categories, not
psychological facts, diagnoses, sin labels or fixed personality assignments.

Each drive has an explicit baseline and gain in [0,1] and half-life between
8 and168 hours, listed in Registry(). Stable substrate reactivity/plasticity are
bounded [0,1]. Responses use clipped signed signal combinations times per-drive
gain, (.1+.9*reactivity)*plasticity*event-confidence, attenuated by perceived
uncertainty. Values and confidences are quantized to1e-9. Parameters and equations
are uncalibrated hypotheses and require a new version if changed.

The stage record is exactly EVENT, PERCEPTION, APPRAISAL, STATE CHANGE, represented
by fixed enums and 22 numeric deltas. No raw prose or unrestricted chain-of-thought
is retained. Six explicit context slots represent the observer's setting,
supportive history, resource availability, relationship support, safety belief
and uncertainty. Every slot has individually validated self Read+Derive rights,
source, learned/occurred time and confidence. Unknown confidence contributes zero;
no count of sources establishes psychological certainty or permission. These are
trusted-host inputs, not network authentication or inferred global relationship
truth. Context permission is checked even on a duplicate before its receipt is
accepted; the receipt digest binds all context values and metadata. State also
modulates appraisal (e.g. effort avoidance competes with care/approach).

Decay evaluates immutable anchors at requested virtual time, never feeding
rounded intermediate values back into the exponent. Confidence decays using the
same half-life. The 32-receipt/32-cause and64KiB state bounds are unchanged;
resource exhaustion denies without eviction. Context sources consume causal
capacity too. Reference response exposes competing tendencies, including WAIT;
it neither selects an intervention nor maximizes attachment/engagement.

Compatibility uses explicit version-aware decoding and a separately invoked
upgrade; replay never invokes an upgrade. `DecodeRecorded` accepts canonical old codec1/ticket7-subset.v1 with
its frozen dynamics implementation, or new codec2/hws-section6.v1. It returns a
tagged result, never copying old array ordinals to new drives. Old fatigue is
NOT acquisition and old slow residue is NOT loss avoidance. Old bytes/hash and
old replay remain unchanged. Cross-version checkpoints deny with supported-version
errors. To adopt the new model, create an explicitly selected new run; do not
rewrite recorded events, baselines, receipts or resources.

`app/hws.DriveAppraisalHandler` is the explicit new-model runtime host, with scoped
perception and version2 checkpoints. It retains the4096-byte runtime data cap and
rejects any actor/state size that exceeds it. Existing AppraisalHandler/behavior
v1 remain frozen compatibility consumers. #41 owns the new27-action/14-stage loop;
#42 owns relationship-specific behavior and new24-person demo adoption. This
registry ticket does not silently upgrade those recorded policies.

| Acceptance | Evidence |
| --- | --- |
| Exact22 names/order/parameters/version | TestExactRegistryWireContract; pinned RegistryHash |
| Legacy/current dispatch, wrong version/array/canonical denial | TestNewCodecAndLegacyReplay; old dynamics goldens retained |
| Context and competing drives | TestAllContextDimensionsAndCompetingTendencies |
| Partition invariant decay/confidence | TestDriveDecayPartitionAndConfidence |
| Permission, uncertainty, receipts, finite bounds | TestDrivePermissionReceiptAndBounds |
| Runtime version isolation, replay, no prose | TestDriveRuntimeReplayAndVersionBoundary |
| Durable persistence/retry/version boundary | TestRuntimeIntegration/DriveRegistryPersistenceAndVersionIsolation |

No schema migration is needed: both explicit payload versions use the existing
journal. Full fresh PostgreSQL migration/restore and old replay tests remain
mandatory. No paid model, real data, UI, main write or live30-day study occurs.
Human validity and cross-model transfer remain NOT TESTED.

Revision 2 retains three additional, separately versioned factors:
`legacy-factors.v1` fatigue, scarcity/opportunity and slow residue. Their old
baselines, gains, equations, confidence and half-lives are retained exactly.
The new appraisal uses an explicit effort burden hypothesis (0.7 fatigue +
0.3 effort avoidance); neither variable overwrites or aliases the other.
Rest also depends on fatigue/residue; Wait also depends on scarcity/opportunity.

`UpgradeLegacy` is a pure bounded new-state constructor requiring a new branch ID
and retaining the old canonical source hash. Only care/status/belonging transfer
by exact semantic name; the three other old variables transfer into factors.
The nineteen new drives start at declared baselines with zero confidence.
Receipts and causes are retained, never reset to recover quota; an old receipt
cannot be replayed as a fresh new appraisal. The caller must establish the new
branch through the existing authorized host fork/new-run path and preserve its
budgets; this function cannot authorize or write a branch. The original recorded
state and policy remain untouched. Tests pin an actual pre-alignment saved demo
checkpoint, its projected actor state and a continuation hash calculated using
the original baseline checkout; fixture provenance is in testdata/README.md.

TestPerDriveSensitivity tests each named drive independently against nine signal
inputs and six permitted context cues, then intervenes only that drive to verify
its tendency effect. Tendency mapping: acquisition/comparison/status protection/
threat response/identity protection/loss avoidance/status → Defend; approach
desire/curiosity/novelty/reward seeking/competence/autonomy/meaning → Approach;
care/reciprocity/fairness/belonging/attachment → Support; effort avoidance → Rest
and Support/Wait; safety/certainty → Wait. Competing effects remain simultaneous.
TestRetainedFactorsRemainDistinct compares all three old factor equations with
the legacy implementation and rejects fatigue/effort-avoidance aliasing.
TestSavedLegacyCheckpointAndUpgrade pins saved bytes/hashes/provenance/recovery.
TestSavedLegacyDemoReplay checks the original full replay and world hash.

Source extract provenance: HWS PRD SHA-256
f7ddaf8ff06587abd8f845a4b70e87894cb7442dd97dbafedb64df09c88e7037,
owner revision-2 issues. This is not a full original-document audit.
