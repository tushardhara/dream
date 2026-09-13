# Offline evaluation (#16)

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
| No fabricated report evidence | TestReportRejectsInventedEvidence, report Verify, explicit NOT TESTED human/cross-model/source-completeness findings |
| Byte reproduction and actual OS separation | scripts/evaluation-check.py runs compiled host twice and builds a synthetic adversarial permission-probe child |

Falsifier vocabulary is `pass`, `fail`, `inconclusive`, `not-tested`. Invalid input
or a broken generator fails the engineering command rather than emitting a success
report. Frozen split integrity is exercised before every generation. Calibration,
ablations and conditional forecast adequacy remain inconclusive without an
owner-preregistered adequacy decision. Complete HWS source falsifiers, cross-model
transfer and real-human validity are NOT TESTED: no unavailable original falsifier
names or real-human evidence are invented. Existing event/replay/revocation/security
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
