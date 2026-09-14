# ADR-0020: complete supplied source registry with bounded evidence

Status: implementation for #43 revision 2; exact review/CI status lives in its PR.

## Decision and source mapping

The supplied HWS §19/§20 extracts are authoritative for this ticket. Private HTML
is unnecessary. `evals/alignment_registry.go` preserves all 13 exits and 10 primary
falsifiers with stable IDs, separate source wording and normalized names, and
explicit evidence requirements. Registry constructors return detached slices;
a golden hash protects the entire wording/requirements mapping. This does not
certify unsupplied sections of the private HWS/IHG documents.

The evaluator freezes `hws-alignment-plan.v1` before any scoring. It binds the
exact source revision/tree/extract, evaluator binary and local image digests,
model/registry/fixture versions, dataset, seeds, hypothesis, rule and scope.
`hws-alignment-report.v1` embeds that protocol, all registry rows, both generation
probes, evidence hashes, findings and separate engineering mechanics. Verification
requires the independently retained plan, actual validated dataset and execution
receipt, then rebuilds the expected report and compares every field. Resealing a
modified verdict or embedded plan does not make it valid.

The CLI checks clean build VCS metadata and commit/tree correspondence when
freezing. Missing metadata is an explicit error. Plans/receipts are exclusive
0600 files; actual evaluator time is in `hws-alignment-receipt.v1`, outside the
logical report hash. Frozen registration time stays fixed across repeated runs.
This preserves the existing exact-byte reproduction gate.

These are integrity checks within a trusted evaluator/OS/Docker boundary. An
independently retained plan is essential; neither a self-hash nor an unsigned
receipt proves an experiment occurred on a hostile host. The library accepts a
trusted generator port; the CLI supplies the restricted container adapter. The
report records output state hashes, not a cryptographic execution attestation.
A structurally valid changed repeat is reported as reproduction FAIL. No human
study, scientific adequacy or signed owner authorization is inferred from hashes.

## Controlled experiment and dependency direction

`simulator/internal/reference` embeds the public synthetic #42 input fixture.
Both the original relationship comparison tests and `simulator/experiment` read
independent copies, without runtime file access or evaluator dependencies. The
probe receives only fixture hash, version and seeds. Its 16 cases per seed use
the same `ApplyRelationship`/`ChooseAction` engine as HWS/demo: seven profiles,
four label swaps, three ablations, no appraisal, unknown context. Full-width
seed-derived draws test actual selected action variation. Input/state hashes and
complete ordered cases/seeds bind each recorded transition.

The external child remains label-blind, networkless, read-only, capability-free,
non-root and resource-bounded. Its only extra command mode is the fixed
`--relationship-probe`. Existing malicious OS probes also exercise that mode.
The reusable core does not import a world, generator or evaluator. Existing split
isolation and independent owner-signed promotion are unchanged. There is no
policy activation, online learning or provider call in this addition.

## Source verdicts for the reference experiment

The table describes a valid two-invocation report with the frozen reference seeds;
it is not a blanket claim about all model behavior. Missing evidence defaults to
NOT_TESTED. One invocation leaves reproduction INCONCLUSIVE; differing valid
repeated evidence makes that scoped criterion FAIL. The nine mechanics checks
have their own statuses and cannot confer behavioral adequacy on source rows.
All falsifier polarities below are `unknown`: non-observation is not false.

| Stable ID | Source criterion / falsifier | Status | Reason / limit |
| --- | --- | --- | --- |
| hws19.01 | Persistence | NOT_TESTED | No longitudinal identity adequacy experiment |
| hws19.02 | Plasticity | INCONCLUSIVE | One-step state changes; adequacy unresolved |
| hws19.03 | Relational specificity | INCONCLUSIVE | Controlled context/label/ablation effects; fixed coefficients do not establish realism |
| hws19.04 | Hidden state | NOT_TESTED | No paired internal/public outcome adequacy experiment |
| hws19.05 | Partial observability | NOT_TESTED | No independent belief-error measurement |
| hws19.06 | Memory | INCONCLUSIVE | History ablation ran; realistic interpretation unresolved |
| hws19.07 | Slow dynamics | NOT_TESTED | No longitudinal accumulation study |
| hws19.08 | Emergence | NOT_TESTED | No group/emergence decision rule or experiment |
| hws19.09 | Stochasticity | INCONCLUSIVE | Actual selected-action variation; independent plausibility absent |
| hws19.10 | Reproducibility | PASS, engineering scope only | Exact repeated one-step action/drive-state evidence under frozen configuration |
| hws19.11 | Counterfactual branching | NOT_TESTED | This probe does not execute authorized runtime forks |
| hws19.12 | Ground-truth isolation | NOT_TESTED | Boundary tests run separately; not embedded as a source adequacy experiment |
| hws19.13 | Calibration readiness | NOT_TESTED | Source requires real human import/compare; unauthorized and unavailable |
| hws20.01 | Prompt/persona dominance | NOT_TESTED | No prompt-versus-state predictive experiment |
| hws20.02 | Hidden state adds little value | NOT_TESTED | No held-out predictive comparison |
| hws20.03 | No relationship differences | INCONCLUSIVE | Context effects observed; behavioral adequacy unresolved |
| hws20.04 | Personality stereotype collapse | NOT_TESTED | No long-run collapse assessment |
| hws20.05 | Unnatural group agreement | NOT_TESTED | No independently assessed group trajectories |
| hws20.06 | Unrealistic memory determinism | INCONCLUSIVE | Memory/seed ablations ran; realism unresolved |
| hws20.07 | No meaningful alternative futures | INCONCLUSIVE | Selected outcomes vary; independent meaningfulness unresolved |
| hws20.08 | Human calibration failure | NOT_TESTED | No authorized held-out human data |
| hws20.09 | Incompatible model laws | NOT_TESTED | No independent second simulator model |
| hws20.10 | No human prediction transfer | NOT_TESTED | No authorized human transfer study |

Calibration source wording remains “real human data can be imported and compared.”
A proposed future import path is a limitation, not satisfaction. Human validity
and cross-model transfer remain NOT_TESTED; real 30-day study remains NOT_RUN.
Unavailable human evidence blocks human-dependent claims, not unrelated engineering
reproduction. Tests, registry counts and mutation catches are engineering evidence.

## Compatibility and verification

The original `evaluation-report.v1` schema and verification remain available.
A retained pre-alignment report/hash is tested without regeneration. New legacy
reports correct an obsolete source-availability explanation; exact bytes are
promised for the same pinned evaluator/configuration, not across source revisions.
Source verdicts use a new version rather than changing the seven legacy findings.

Registry/forgery tests cover missing/renamed/duplicate rows, altered requirements,
source/config/model/seed/dataset changes, absent input/repeat evidence, forged
human/calibration/falsifier/study claims, missing receipts and post hoc rules.
Pure probe tests cover complete input binding, canceled work and selected actions.
`make evaluation-check` retains both formats, freezes before scoring, runs two
reports, verifies exact bytes and both actual-time receipts, checks wrong source
revision/tree rejection, and runs actual container permission probes. Required
full verification includes the original split, signed promotion, race, migration,
replay/export and demo gates. See the exact PR checkpoint for runs, preserved
failures, host enforcement limits and CI coverage; this ADR is not a test log.
