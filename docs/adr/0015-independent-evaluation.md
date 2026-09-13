# ADR-0015: separate offline evaluation and owner-signed batch evidence

Status: implemented for #16; independent engineering review remains required.

The evaluator owns consented synthetic sources, outcome labels, frozen split and
configuration hashes, scores, and promotion evidence. The reference generator
owns only initial self state, already learned observations and typed affordances.
The import guard also denies indirect evaluator imports through adapters or public
API hosts. The offline host runs the generator in a separate nonroot scratch
container with no host mount, environment passthrough, network, writable root,
capabilities or Docker socket. Files imported by the evaluator must be regular,
owned by its effective UID, owner-only, bounded JSON and not symlinks. These are
local evaluator permissions, not a new public evaluator role or HTTP route.

Versioned inputs include dataset/holdout content hash, reference generator version
and local image digest, action policy version, explicit no-prompt version, ordered
seeds, outcome dimensions, logical horizons and bootstrap configuration. Every
prediction binds its exact input/variant/seed request digest. Dataset generation
and scoring use distinct types; JSON detaches input before the generator port.
Approved input sources require generation consent and the actor's own observation;
labels have no generation consent. No future learned source or later-action label
is copied into a prompt. The process probe uses synthetic canaries only.

Chronological windows include initial state time, observed input time, requested
label horizons and later label learned times. Family/person/group IDs cannot cross
splits. Input people and provenance must be declared, preventing a hidden recipient
or memory partner from evading the split. Shared family, person or group joins a
connected component within each split; confidence bounds resample components,
never random seeds as independent people. Baseline probabilities use train labels
only. Unresolvable trust/motive assertions cannot have ground-truth values.

Five fixed reference variants compare a deterministic WAIT baseline, initial
persona with no history/memory, stateful replay, no-memory replay and only the first
approved retrieved perspective. These are declared synthetic ablations, not model
quality claims. Conditional forecasts use no unseen events and fixed affordances
through an explicit validity deadline. Horizons are positive nanoseconds up to
seven days; fixtures use one and two hours. Each choice retains the existing
one-second action-window bound. A forecast outside validity or during an operational
outage abstains, including the baseline; it is not a human WAIT observation.

Calibration is separate per action dimension and horizon. Missing, censored,
not-taken and unresolved observations are counted separately; resolution rate uses
all cases. Brier scores average available seed probabilities per case, with a
train base-rate comparator scored on the same cases. Abstentions and seed counts
are explicit. Selected frequencies are separate from predicted probabilities;
-1 means that seed had no non-abstained predictions. Percentile 95% intervals use
1,000 deterministic connected-component bootstrap replicates in the fixture.
Fewer than two components yields no interval. Score-availability PASS only means
that a score, base-rate comparator and interval exist. It is not a scientific
falsifier or adequacy pass. There is no friendliness/conflict objective or fitted
psychology benchmark.

Promotion is a versioned protocol producing an idempotent authorization event,
not changing a live default. A trusted host installs an owner Ed25519 public key
independently of a candidate. The owner first signs a frozen batch plan containing
input/configuration hashes and explicit outcome/horizon adequacy criteria.
RunPlanned validates this before generating or scoring and binds its plan hash in
the report. The owner separately signs the exact completed report and plan hash.
Authorization checks calibration AND holdout upper Brier confidence bounds,
resolution, independent-component count, and zero abstentions against those
explicit criteria. Missing/inconclusive data, an unsigned or candidate-signed
approval, changed inputs or a failed criterion deny. Supported scope is only
synthetic_offline_batch; no production activation host or default exists. No owner
threshold is selected by this ticket. The test's permissive threshold is synthetic
test data and cannot authorize anything without the trusted test key.

Engineering checks do not supply missing source text. The report lists the known
summary-level checks and explicitly marks complete HWS source falsifiers,
cross-model transfer and real-human validity NOT TESTED. The source register in
docs/requirements.md remains UNVERIFIED, including #7/#11 complete registries.
See docs/evaluation.md for commands and acceptance evidence.
