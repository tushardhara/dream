# Offline evaluation and source alignment (#16, #43)

Run `make evaluation-check`. It builds local static binaries and scratch images,
executes the evaluator twice, compares exact report bytes, and runs the isolation
and scoring tests. The synthetic report is `bin/evaluation-report.json` (ignored
build output). No registry pull, live provider, owner key, human record or policy
activation is needed. Docker and the pinned Go toolchain are required; no running
service or database is required. `make verify` includes this command in addition
to all existing architecture, race, PostgreSQL, transport and operations gates.

For frozen synthetic inputs, build `cmd/hws-eval` and supply
`--dataset <owner-only-json> --config <owner-only-json> --generator-image sha256:<id>`.
The configuration must already pin the dataset hash and exact local generator
image. `--synthetic` is an alternative for the format fixture, never an override
for a supplied dataset/configuration. All five source kinds have typed provenance,
subject/observer, distinct occurred/learned times, expiry, purpose, retain/evaluate
and separate generation consent. Revoked, expired, missing-provenance or real-data
records fail. Real human ingestion is deliberately unavailable without a later
owner-authorized path; schema coverage is not consent or human validation.

The fixture is 12 independent families, 36 distinct synthetic people and 12 groups
across train/calibration/holdout, eight fixed seeds, five variants and two horizons.
It is an engineering format exercise, not the later 200-family/10,000-case research
study. Labels use an arbitrary alternating pattern independent of predictions.
The evaluator accepts at most 64 cases, 512 sources, eight history observations,
32 labels/case, 32 seeds and 8,192 generation requests. Dataset input is bounded
at 8 MiB; the isolated batch at 32 MiB; generator runtime at 60 seconds, one CPU,
256 MiB, 32 PIDs. Larger authorized studies need an explicit subsequent design.

| Acceptance | Evidence |
| --- | --- |
| Separate labels/permissions and no prompt leakage | TestLabelsNeverReachGeneration, TestContainerPermissionProbe, TestPrivateEvaluatorFiles, TestWorkerRejectsLabelsAndOversizedRequests, TestRules |
| Frozen versions and family/person/group/time isolation | TestDatasetLeakageAndConsentNegatives; input/output/config/dataset hashes and TestInvalidGeneratorAndNoData reorder control |
| Real ablations and conditional time advance | TestActualVariantsAndConditionalRollForward; all five candidate distributions differ; history remains unmodified; outage/invalid-input negatives |
| Independent multi-seed uncertainty and calibration | TestFrozenOfflineReportAndCalibration known .5 Brier/.25 base-rate Brier; TestConnectedComponentsNotSeeds; no-data and absent-holdout cases |
| Missingness, latent uncertainty, consent/provenance | TestDatasetLeakageAndConsentNegatives; TestInvalidGeneratorAndNoData; private/public/self/later-action synthetic source kinds |
| Owner-only offline batch protocol | TestPromotionRequiresSeparateOwnerSignaturesAndHoldout; signed preregistration, changed inputs/report, wrong signing key, failed holdout and forbidden production scope |
| No fabricated report evidence | TestReportRejectsInventedEvidence, report Verify, explicit NOT TESTED human/cross-model findings; separate complete source registry |
| Byte reproduction and actual OS separation | scripts/evaluation-check.py runs compiled host twice and builds a synthetic adversarial permission-probe child |

Falsifier vocabulary is `pass`, `fail`, `inconclusive`, `not-tested`. Invalid input
or a broken generator fails the engineering command rather than emitting a success
report. Frozen split integrity is exercised before every generation. Calibration,
ablations and conditional forecast adequacy remain inconclusive without an
owner-preregistered adequacy decision. The legacy `evaluation-report.v1` keeps its seven format-scoped findings. The
complete supplied HWS source registry is reported separately by
`hws-alignment-report.v1` below. Cross-model transfer and real-human validity
remain NOT TESTED; synthetic evidence does not establish either. Existing event/replay/revocation/security
invariants remain exercised by `make verify`, not claimed as scientific passes by
the synthetic report.

The owner-signed protocol is a library boundary for a trusted offline host. No CLI,
API, worker or evaluator result silently activates a production policy. It supports
recording the versioned authorization event in a later explicit host; this ticket
makes no production activation or key provisioning change. Host compromise or an
owner deliberately signing false evidence is outside the generator-isolation
claim. The Docker daemon and evaluator OS identity are trusted. Reports contain
synthetic aggregates and hashes; no real private data has been ingested.

Run `python3 scripts/evaluation-mutations.py` in an exclusive implementation
worktree for opt-in negative controls. It requires green focused and container
baselines, removes one guard at a time, requires an actual test assertion failure,
and restores the original bytes in `finally`. The eight controls cover cross-split
isolation, Brier arithmetic, owner signature, exact request binding, operational
abstention, owner-only file permissions, network isolation and read-only root.
Logs go to ignored `bin/evaluation-mutations/`; the suite rechecks restored code.
No merge, test merge, live data, external service or permission bypass is involved.

## Complete source registry and frozen evidence

`make evaluation-check` also freezes a new alignment plan before generation,
invokes the isolated relationship probe twice per report, repeats the report,
and verifies exact byte identity against both execution receipts. It retains
`bin/alignment-run-<run>-<id>/` containing the plan, canonical report, actual-time
receipts, a legacy report and negative source-binding logs. On the shared host,
run this heavy command under [the disk guard](test-resource-safety.md).

The immutable registry contains all 13 supplied §19 exits and all 10 §20 primary
falsifiers, with stable IDs, original wording, separate normalized names and
explicit evidence requirements. `PASS`, `FAIL`, `INCONCLUSIVE`, `NOT_TESTED` are
source verdicts; `falsifier_triggered` is `true`, `false` or `unknown`, independent
of whether an engineering check passed. Current synthetic probes leave all ten
falsifiers `unknown`. A non-observation is not a demonstrated false falsifier.

The plan pins source revision/tree, supplied source hash, evaluator binary and
local generator image digests, model/registry/fixture versions, dataset hash,
seeds, hypothesis, decision rule and scope. The CLI freeze checks clean build VCS
metadata and the actual commit tree. Build from a clean committed checkout;
unavailable VCS metadata fails explicitly. Each plan is a new owner-only file
created exclusively, and the evaluator cannot overwrite it. Preserve the plan
independently of reports. Generation receives only the public fixture hash,
version and seeds—never the dataset, labels, hypothesis or scoring rules.

Example after `make evaluation-check` builds the binaries/image (substitute the
actual local image ID and source hashes; choose new file paths):

```sh
bin/hws-eval --synthetic --generator-image sha256:<image-id> \
  --freeze-alignment-plan plan.json \
  --source-revision <commit> --source-tree <tree>
bin/hws-eval --synthetic --generator-image sha256:<image-id> \
  --alignment-plan plan.json --alignment-receipt receipt.json > report.json
chmod 600 report.json
bin/hws-eval --synthetic --generator-image sha256:<image-id> \
  --alignment-plan plan.json --alignment-receipt receipt.json \
  --verify-alignment-report report.json
```

Prefer `umask 077` before creating artifacts; the script uses exclusive 0600
files. Verification also requires the original dataset (or the deterministic
synthetic fixture), so a report cannot replace its own preregistration or source
binding. An actual evaluator timestamp lives in a separate receipt, outside the
logical report hash. Repeating the same frozen plan can therefore preserve exact
report bytes while recording distinct execution times. This is integrity and
provenance checking within a trusted evaluator/OS/Docker host, not signed proof
that an external experiment occurred. Hashes do not authenticate a malicious
host. No evaluation result activates a policy; existing independent owner
signatures, split isolation and holdout requirements remain unchanged.

The evidence reuses #42's exact controlled inputs through a public embedded
fixture. Per seed it executes 16 one-step action/drive-state trajectories: seven
profiles, four label-only swaps, three ablations, no appraisal and unknown
context. Nine separate engineering checks cover sensitivity, label invariance,
ablations, reversed history, unknown permissions, drive changes, selected-action
seed variation and reproduction. These do not establish human plausibility,
longitudinal realism or independent-model agreement. See [ADR0020](adr/0020-source-alignment-evidence.md)
for the full 23-row interpretation. Only bounded reproducibility can earn a
source PASS; unavailable human evidence does not prevent this engineering result.

Calibration readiness retains the source requirement **“real human data can be
imported and compared.”** That operation is unauthorized and unimplemented here,
so it is NOT_TESTED. A future authorized import path is a limitation, not a pass.
Real-human validity and cross-model transfer are NOT_TESTED; the real 30-day study
is NOT_RUN. Registry counts, fixed coefficients and caught mutations establish
engineering coverage, never behavioral validity.

Legacy `evaluation-report.v1` decoding and verification are unchanged; a retained
pre-alignment report is a regression fixture. New legacy reports correct only an
obsolete explanatory sentence about missing source names. That changes their
content hash across evaluator revisions, not the frozen-input byte reproduction
guarantee within one pinned evaluator. Alignment is a separate versioned format,
not a reinterpretation of old verdicts.

For #43 assertion sensitivity, run `python3 scripts/alignment-mutations.py` under
the disk guard in an exclusive worktree. It preserves bounded per-run logs,
requires green baselines, actual assertion failures, exact source restoration and
a green rerun for every mutation. Compile errors and timeouts are not catches.
