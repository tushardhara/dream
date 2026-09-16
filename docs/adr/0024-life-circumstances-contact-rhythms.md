# ADR 0024: Life circumstances, observed contact and current applicability

Status: implemented for #48, pending independent review. Based on verified #52
integration `2f248610e1f7164910c3dbe6ddc00a03c094cb8b`. Backend/offline synthetic only.

## Existing foundation and decision

The merged scenario contract required numeric adult age. Drive confidence changes
from explicit anchors; graph retrieval ranks permitted, valid records by salience
and recency. Neither establishes a contact expectation or whether a historical
fact still describes current circumstances. Relationship v2, domain consumers and
assistance v3 already preserve observer perspectives and scoped willingness. Their
completed review evidence is reused; this is not another aggregate audit.

Reusable temporal contracts now live in `core/temporal.go`, graph projection in
`app/graph/temporal.go`, helper integration in `app/assistance`, and native human
integration in `simulator/behavior/temporal.go`. Synthetic scenes and labels remain
in `examples/temporalexperiment`; no simulated latent state enters the helper.

## Contracts and bounds

`temporal-fact.v1` records account, observer, person, relationship partner, channel,
source, basis, category and logical confirmation time. Circumstances distinguish
responsibility, transition, preference and experience. Their bounded signals are
busy, routine_changed and available. Self-report requires the person, observer,
record author and source author to agree. Another observer's hypothesis is retained
as such, never promoted to an established circumstance or willingness.

`TemporalEvidence` retains record/source occurrence and learned times, confidence
and historical validity. A new wrapper around an old source cannot assert fresher
confirmation or observation coverage. Both typed account and source must be present
in the actual approved/retrieved set. Human projection checks matching Read/Derive
rights, original metadata and authors supplied by the trusted graph adapter.

`temporal-focus.v1` binds observer, partner and channel. `temporal-context.v1` uses
explicit logical days (maximum clock 1e9); adapters must opt into that calibration.
No wall-clock duration or age coefficient is silently inferred. Freshness and gap
bounds are at most 3650 days. Interpretation accepts at most 16 records and eight
strictly ordered observations per diary. Duplicate accounts fail. Competing current
preferences/diaries return unknown rather than being averaged.

An explicit own preference is separate from an estimate. Estimation needs at least
four permitted contacts, complete channel coverage, positive intervals and a
largest/smallest interval ratio at most four. Its upper gap is twice the largest
observed interval and its confidence is half the record/source confidence product.
Insufficient, irregular, old or incomplete coverage means unknown/WAIT. An observed
gap names time since a permitted observation in that channel only; it never means
no contact anywhere, rejection, deterioration or an obligation to reconnect.

Confidence products below 0.5 and hypotheses cannot establish these facts. A fresh
busy report restrains action. A supported change invalidates an older preference or
pre-change estimated window; later reconfirmation can restore current applicability.
Freshness decays by an explicit half-life from `ConfirmedAt`; stale means age exceeds
`FreshFor`. This changes current applicability/confidence without ending historical
fact validity or changing graph salience/retrieval priority. Unknown circumstances
are allowed. No hidden event is invented to explain an unobserved interval.

## Graph, correction and replay

Canonical `temporal.v1:` episodic memory binds the outer account ID, observer,
subject, reporter and source lineage. `TemporalService.Put` delegates persistence,
permissions and optimistic concurrency to the existing memory service. Corrections
cannot change observer/person/partner/channel/kind/basis identity. Existing graph
selection resolves supersession at valid/learned/recorded query times; revocation
cannot resurrect a superseded assumption. Current data permissions still govern
historical reads.

Context proposal **v2** adds `SafeContextItem.Reporter` from original source metadata.
The field is omitted for v1, preserving legacy bytes. Assistance v4 requires v2;
earlier assistance formats continue to require v1. This is explicit versioning,
not an in-place reinterpretation of recorded provider inputs.

The helper's raw approved-evidence hash transitively pins derived temporal views;
request hashes pin focus, goal and policy version. Fresh, external-recorded and
stored execution all enforce the same exact temporal WAIT. Still-permitted content
changes invalidate old receipts. Historical planner summaries filter temporal
channel/focus and scrub focus, source refs and request/evidence/boundary hashes.

A current v4 operation requires the trusted boundary snapshot clock to equal the
request clock. An old receipt cannot authorize a new effect after time advances.
Authorized historical graph inspection remains available. Mechanical replay at the
original clock reproduces the permitted historical view even with later journal
rows present. This distinction is tested through the actual helper, not only a
codec or stale in-memory derived value.

## Actual helper and human use

`assistance.v4` adds an owned temporal focus and separate derived observer views.
Single-helper mode uses its user's account; multi-helper mode requires every
participating view to support the requested action. An unknown goal permits a
clarification only when all required views recommend it. Coordination requires
routine context; listening/understanding excludes busy/unknown context; pause waits.
The deterministic temporal planner emits only the fixed, non-identifying
`check_current_context` reason. It does not quote a private life update in an
outgoing explanation. Simple/disabled arms remain explicit controls.

Data grants and willingness are separate. The independent #51 gate runs before
context acquisition, after planning and inside commit. Break/end/no-contact always
win; a gap grants no permission. The existing helper limit of two clarifications
and ten-day cooldown persists across new temporal snapshots.

`temporal-human-actions.v1` wraps the existing domain actor and invokes the actual
native action engine. Temporal eligibility filters clarification and ordinary
interaction offers; WAIT and unilateral leave/withdraw remain possible. Clarification
receipts share a two-question budget and ten-day cooldown across channels/role
frames for the same partner/topic. Other partners/topics do not spend that budget.
There are at most 32 receipts and a 128 KiB strict actor codec. Lookup and receipt
writes have separate controls. Human snapshots are trusted authorized inputs; this
pure simulator function does not claim to be an external delivery transaction.

## Scenario migration and engineering evidence

`WithTemporalContexts` makes a detached explicit genesis migration. It adds owned
initial facts and the inferred `temporal-context.v1` capability. Public
`unknown_ages` explicitly allows age zero for an unknown numerical age in the
synthetic adult cohort; ordinary legacy age zero still fails. Actor views contain
only their own private temporal facts. Numeric age is metadata and is never used
for maturity, obligations, trust or action coefficients. Legacy canonical bytes,
capability rejection and replay remain tested.

| Requirement | Actual evidence |
| --- | --- |
| Optional age, no maturity/obligation coefficient | scenario migration/unknown-age tests; `TestAgeAndHiddenChannelAreNotConsumerCoefficients` compares actual choices/probabilities at 18/35/75/120/unknown |
| Life, rhythm, 2/60-day gaps and staleness affect decisions | core tests; actual helper/human temporal tests; twelve composed scenarios and separate life/rhythm/stale input mutations |
| Explicit preference vs uncertain estimate, sparse/irregular channels | core minimum-sample/contradiction tests and `TestTemporalHelperEstimatedRhythmAndHypothesisControls` |
| Break, busy, no-contact, long-gap WAIT | actual composed helper and both native humans; zero-draw WAIT positive controls; inherited independent boundary gate |
| Observer disagreement and private updates | private Single/Multi helper test, owned graph retrieval, canary checks, separate human perspectives |
| Correction/revocation/current vs historical view | graph supersession/revocation tests; actual helper correction with later journal rows; fresh/planning/commit/external/stored permission controls |
| Authorship/freshness laundering rejected | graph envelope tests, actual helper source-author/chronology controls, human metadata checks |
| Clarification burden | fresh helper snapshots at 63/64/73/83; human repeated choices/codec; independent receipt read-scope and write controls |
| Evidence-bound replay | exact WAIT forgery, still-permitted content change on external/stored paths, current-clock and history filter/scrub tests |
| Matched comparisons and limitations | `docs/evaluation/temporal-v1.md` and actual 16-seed JSON; evaluator adverse/null-outcome test |

The committed report distinguishes policy-labelled appropriateness, unnecessary and
missed clarification, uncertainty-category agreement, and privacy/boundary probes.
It reports the human's missed opportunities and the identical helper baselines.
Fixed helper seed replication is not independent empirical evidence. Human inputs
stay fixed across helper arms, and no helper-induced human benefit is claimed;
that response path belongs to #53. Synthetic labels and thresholds are hypotheses,
not real-human validation. No UI, live provider, outreach, deployment, private source
document, real-person data, new supervisor or scheduler is introduced.
