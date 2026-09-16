# ADR0027: attributed repair evidence and bounded follow-through

Status: implementation for #55; independent exact-SHA integration evidence belongs
in its PR. Supplied #49/#55 requirements are sufficient; no private source HTML.

A promise, apology or helpful action cannot establish a repaired relationship.
`repair-evidence.v1` distinguishes executed action receipts, a later independently
submitted recipient observation, and each participant's immediate/later assessment.
`repair-flow.v1` consumes their current permitted history. No field claims
forgiveness, global trust, a relationship-health score or a universal repair rule.

## Reuse, scope and actual consumer

The reusable contract lives in `core/repair.go`, using existing Metadata, Rights,
logical time and RelationshipFocus. It extends bounded commitment/open-loop
semantics in a new opt-in ledger rather than changing `OpenLoop.Closed`, old
`behavior.Commitment` or recorded cognitive policies. The action rules preserve
ADR0018's distinction: promise creates an intention; help consumes resources;
fulfilment requires explicit later recipient evidence; expiry is not breach proof.
Existing outcome-observation.v1 and listening-account.v1 stay unchanged. This
ledger adds practical fulfilment and recurrence provenance, not a replacement
recipient welfare model or a new gateway, scheduler or durable event store.

`app/assistance.RepairHost` is a real consumer outside worlds. Its trusted journal
supplies authenticated requests/current evidence and performs bounded projection
and delivery in the same critical section as correction/revocation. There is no
model/network call inside or outside that section. The existing exact boundary
engine gates the user's private preparation independently of the other person's
participation. Read AND derive on every source are necessary for both helper and
user; separate helper-to-user ShareOnRequest on the full lineage is also required.
Even counts and source IDs cannot identify unshared private history. The helper
never learns simulator/evaluator truth. Ports are in-process trust boundaries,
not network authentication; record values cannot authenticate their own author.

`examples/repairclient` executes voluntary **authored synthetic commands** for two
fictional people. Help really decrements finite hours atomically; failed/duplicate
commands do not. An observation is a separate authenticated participant submission,
not emitted automatically by help. Refusal/withdrawal/ending prevents practical
commands in that same context; WAIT remains available. These explicit fixture
choices are not stochastic human-policy predictions. The new compiled
`hws-repair` demo is separate from frozen demo.v1/v2/v3 and cognition replay.
`make repair-check` invokes it twice and is required by `make verify`.

Both three-period scenarios start with a breach. Repeated apology/breach yields
an optional selective-distance suggestion. Acknowledgement plus two later acts of
resource-consuming help and recipient observations changes the descriptive
expectation to new, then sustained observed follow-through. Original harm remains
in the evidence. Sender relief and recipient mixed/worse later assessments remain
separate from immediate de-escalation. Both fixtures finish with voluntary refusal,
selective withdrawal, ending and WAIT; these are legitimate outcomes, not failures
to maximize messages. Ordinary private support remains available to the other
participant, without invitations or pressure to resume joint work.

## Contract and history invariants

- At most96 records; each at most4096 encoded bytes, strict unknown-field/trailing
  decoding; two named participants per episode/direction, exact domain/context.
  Episode IDs distinguish loops; same domain/frame can retain recurrence across
  episodes. No cross-context/global aggregation.
- Action receipts are immutable, even through alternate callers. Authored
  observation/interpretation corrections preserve owner, pair, episode, phase,
  reference and occurrence; corrections append with later knowledge time and may
  not fork. A changed or revoked source invalidates current dependent derivations.
  A revoked correction cannot resurrect the old account. Actual resource history
  is not rewritten when someone changes their interpretation.
- Only the recipient can submit an observation of fulfilment or breach. A sender
  may independently report relief, which is not recipient benefit or completion.
  Fulfilment requires matching practical help and occurs strictly after its effect,
  within the promise deadline. Promise/help resources and units must match.
  An explicit break or a later recipient observation after the due time supports
  breach. Silence, expiry, reply counts and an apology do not.
- Every provenance/support/contrary/reference link resolves to earlier evidence in
  the same scope. Current projections check the full closure, including commitment
  roots and correction ancestry, under current rights even for historical replay.
  The bounded DAG traversal is memoized. Unshared private fields cannot steer a
  user response or its digest.
- Distinct observed commitments, not repeated reports, contribute to recurrence.
  Only follow-through after the latest currently supported breach counts toward
  a changed expectation. Two distinct commitments separated by at least20 logical
  units are needed for the descriptive `sustained_follow_through_observed` label.
  Twenty is an explicit fixture duration, **not a human-validity threshold**.
  Close reports or duplicate messages cannot satisfy it. A new breach changes the
  next expectation while retaining prior help. No history forces a human action.
- Perspectives retain separate immediate and later assessments. Unknown evidence
  stays unresolved, and `RepairVerdict=NOT_ASSESSED` is asserted by the compiled
  gate. The helper offers optional steps; it does not execute them or equate
  participant interpretation with independently evidenced behavioral change.
- No success-confirmation prompt exists: `ConfirmationRequests=0` is a tested
  output invariant. Requests/responses are capped32, authenticated and idempotent.
  Old response replay recomputes the projection at its original logical time using
  today's rights, and denies a changed result rather than returning cached disclosure.
  Current-time requests apply corrections learned by that time; historical views
  do not retroactively rewrite what had then been learned. Refusal to use the helper
  returns empty WAIT; a participant's voluntary ending is respected.

## Criterion-to-code/test evidence

| #55 criterion | Executable evidence |
| --- | --- |
| Bounded commitment stages, independent observation and interpretation | RepairRecord/ValidateRepairLog; direct contract controls for sender-manufactured fulfilment, missing evidence, promise/apology substituted for practical help, time/resource/reference mismatches. |
| Topic/time recurrence with provenance | CurrentRepair and RepairHost; scope isolation in actual consumer; full read/derive/share closure controls; distinct commitments and20-unit duration controls. |
| Actual multi-period apology/refusal/promise/breach/help/WAIT/distance | repairclient.Period/Scenario and compiled repair-check; real resource decrement, duplicate/overspend controls, same initial breach across both fixtures. |
| De-escalation versus durable behavior, contrary assessments and legitimate endings | TestRepairActualPeriodsSeparateDeescalationAndDurableEvidence; sender eased versus recipient mixed/worse; pauses/endings and independent private-value tests. |
| Later proof without repeated success questioning | Strictly later recipient observation; immediate/attempt-only/expiry controls; ConfirmationRequests always zero. |
| Apology cannot mark forgiveness/repair | Direct record validation rejects forgiven/repaired; actual apology/promise/help-without-observation remains unresolved; compiled NOT_ASSESSED marker. |
| Repeated breach differs from follow-through | Actual periods change helper expectation/optional next step and resources; compiled gate asserts both. |
| Old harm not deterministic; new evidence changes expectations without erasure | Same initial breach, followed by new/sustained support; reverse new-breach control preserves earlier help; exact context isolation. |
| Correction/revocation/current-access replay | Immutable receipt and correction identity/fork negatives; current closure; correction+revocation no resurrection; pre-snapshot revocation and atomic delivery race tests; post-revocation replay denies. |
| Pause/ending valid; no trust/message proof | Contact enforcement and private-value tests; repeated-observation dedup; helper contains no trust update or interaction reward; zero confirmations and NOT_ASSESSED. |

## Limits and claims

This is a bounded in-memory reference host with voluntarily authored participant
commands and reports, not empirical independent corroboration, spontaneous human
behavior, semantic listening competence, validated forgiveness, calibrated trust,
real-world repair efficacy, or measured helpful-AI uplift. `HumanValidity=NOT_TESTED`
and the explicit authored-evidence marker are compiled-gate assertions. Reports
may disagree; the system does not resolve who is objectively right. The helper
has no automatic contact/resumption path; a production authenticated host/durable
journal, real study and richer repair semantics need separate scope. Its trusted
fixture audit accessor is not a participant-facing export API. No UI, live/paid
providers, deployment, real-person data, outreach or main merge is included.
