# ADR-0011: bounded synthetic cognitive actions and outcomes

Status: proposed for issue #11, subject to independent exact-SHA review.

`CognitiveService.Step` composes approved retrieval, durable typed interpretation,
then `Apply` and the pure `CognitiveHandler`. A trusted `CognitivePlanner` maps
permitted perceived evidence into structured signals and possible affordances;
it cannot select the action, invent model beliefs, authorize disclosure, or mark a
successful model call as an outage. `dynamics.Appraise` remains the sole appraisal
owner. Its actor/event receipt is required, consumed once, and never evicted.
The resulting drives, observer-attributed interpretation hypotheses, retrieved
relationship evidence and bounded outcome memory feed `human-actions.v1`.
Interpretations are not canonical truth. Conflicting duplicate source proposals
fail instead of silently reconciling to certainty. No hidden labels or GodState
enter the planner/provider port. This is synthetic HWS behavior, not an IHG
production decision engine, real-user advice, persuasion, matching or notification.

## Selection and execution

The executable registry is wait, observe, ask, self_disclose, help, decline,
invite, break_promise and third_party_support. These include the actions named in
#11 and the earlier typed model candidates. Complete original HWS source-list
coverage is **UNVERIFIED**; no unseen action/mode is implemented by guessing.
Unknown modes and malformed proposals fail even when other proposals are safe.
Unsafe but structurally valid affordances are filtered by current recipient,
actor availability, resource capacity, action duration, disclosure and commitment
constraints. WAIT is always present. The initial venue has all scenario humans;
there is no travel/location mechanism or implicit teleportation.

The policy computes explicit bounded subjective consequences, normalizes positive
weights, and mixes 90% weighted sampling with 10% uniform exploration. The exact
64-bit named RNG draw and its top-53-bit selection fraction, all normalized
candidate probabilities, selected index, policy version and `explored` flag are
recorded. `explored` means the sampled action differs from the highest-weight
candidate, not an opaque model ranking or a claim of scientific calibration.
Counterfactual tests compare distributions rather than forcing particular choices.

Only a selected action can emit a typed delivery, at its later completion time,
to its selected synthetic recipient. Ordinary utterances are enum messages and
cannot spend resources or fulfill commitments. Help requires an explicit bounded
`runtime.Consumption`; the runtime atomically checks and subtracts capacity with
the checkpoint/draw/delivery. Invalid delivery rolls back the whole transition.
Commitments may be initialized from explicitly provenance-bound trusted synthetic
inputs; break-promise changes only an existing actor-owned pending commitment.
Help does not assert fulfillment merely because it was selected or spoke words:
fulfillment remains pending without later specific completion evidence. No new
promise-creation/fulfillment mode is silently inferred from free prose.
Third-party support delivers a request to the selected supporter; it does not
quote another principal's restricted experience.

Self-disclosure uses `ViewService.Propose` with SyntheticSelfDisclosure and exact
Disclose rights followed by the bounded quotation writer. Disclosure sources must
also belong to the model's approved context so runtime model lineage includes all
potentially delivered text. The capability is checked again before commit. Only
a selected disclosure delivers its quotation. Third-party assistant disclosure
restrictions remain unchanged. The pure handler/planner are trusted internal
composition boundaries; they are not authenticated network request types (#14).

## Outcomes, outages and durability

Every choice, including WAIT, starts an observer-attributed unknown outcome with
an explicit horizon. A later trusted response supplies decision ID, actual
observer/other, occurred/learned time, response enum and permitted evidence.
Evidence may not resolve an outcome twice or update another actor. Supportive or
dismissive evidence changes bounded own-perspective memory/trust/disclosure;
neutral evidence records a response without a signed change. No weights/prompts
train online. Unresolved outcomes censor when a subsequent transition reaches
their horizon. No transition means no invented observation or closure.

Provider outages are not behavioral conclusions. Only a settled retryable failure
at the immutable maximum attempt count is eligible for operational WAIT. Refusal,
malformed output, permission failure, budget exhaustion and an uncertain running
attempt cannot be relabeled as outage. PostgreSQL returns an exact failure digest
and rechecks it, current source revision/logical time and single consumption in
the canonical commit. `ModelUse.failed` extends the versioned operation digest;
its omitted false field preserves prior successful-model digests. Existing model
application lineage makes source revocation purge operational WAIT state too.
No migration or external/live service is needed. A runtime operation receipt is
the recovery boundary after a lost reply; restarting provider generation with a
stale context capability is not a receipt lookup.

`cognitive.v1:` is a new bounded zlib/base64 checkpoint codec. It retains at most
four actors, sixteen outcomes, sixteen explicit commitments and the latest
complete choice; each actor retains the unchanged #7 32-receipt limit, eight
memories and eight current beliefs. Expanded JSON is at most 64 KiB; encoded
runtime Data remains at most **4096 bytes**. Decode rejects unknown fields,
trailing bytes, alternate wire forms, invalid source/choice envelopes and
expansion bombs. Encoding is pinned to the supported toolchain. Exceeding any
limit fails without eviction; this is a 2–4-actor engineering slice, not 24-human
or long-horizon scale evidence. Existing appraisal.v1 checkpoints remain usable
with their original handler; switching their running policy is not an implicit
migration. #13/#15/#17 must address larger replay/study needs explicitly.
The public actor view decodes only own current drives from this format; it never
returns other actors' beliefs, outcomes, candidate lists or model artifacts.

All canonical state continues through the existing fenced runtime event/operation
transaction. Provider calls and planner/output-policy work occur outside SQL
transactions. Sources revoked before application deny commit; after application,
model-to-runtime lineage purges checkpoints and queued deliveries. Already-read
bytes, old MVCC snapshots/WAL and old external backups retain the prior documented
limitations. This change does not claim retroactive erasure of delivered data.

## Acceptance evidence

| Requirement | Evidence |
| --- | --- |
| One appraisal owner, restart and recorded logical outputs | TestChoiceDeterminismSensitivityAndWait; TestCognitiveFourActorsReplayAndPrivatePublicBoundary; TestCognitiveCodecAndDuplicateAppraisal; TestCognitiveIntegration/recorded |
| Named finite actions, WAIT, normalized sampling and constraints | TestRegistryConstraintsAndTampering; FuzzChoiceDraw; TestActionConsumptionAtomicAndBounded |
| Own-fiction positive disclosure and private/public separation | TestCognitiveIntegration/disclosure; four-actor private-canary test; TestCognitiveActorViewDoesNotExposePrivatePipeline |
| Observer-specific beliefs, relationship/state/memory sensitivity | TestChoiceDeterminismSensitivityAndWait; TestApprovedRelationProjectionKeepsObserverAndProvenance; approved relation projection in CognitiveService |
| Delayed evidence, unknown/censored outcomes, bounded learning/repair | TestDelayedLearningEvidenceBoundsAndRepair; TestCognitiveDelayedResponseUpdatesOwnState |
| Operational WAIT and exact failure evidence | TestCognitiveIntegration/outage, /refused, /forged_operational; TestCognitiveFailClosedAndOperationalWait |
| Revocation before/after apply, durable receipt recovery | TestCognitiveIntegration/revoked_during_plan plus source purge and restarted operation checks |
| Explicit sizing, no ledger eviction | TestReceiptExhaustionDoesNotEvict; TestCognitiveFailClosedAndOperationalWait/outcome_budget; bounded codec tests |

`env -u DREAM_TEST_DSN make verify` runs formatter/vet, Python sentinels, normal
and race tests, architecture guard, fresh PostgreSQL migrations/integration,
purged pg_dump/restore, builds and CLI help checks. New tests use only synthetic
fixtures and deterministic fake/recorded data. No live API, actual provider price,
paid run, deployment, 30-day study or real-human validity check has been run.
Scientific thresholds and independent evaluation remain #16; source traceability
limitations remain visible and are not converted into an engineering PASS.

Permanent resource consumption also preserves the minimum capacity needed by
already queued reservation/release intervals. `SpendableResources` supplies the
executor's affordability ceiling without exposing future event text to cognition;
the runtime independently rechecks it. The red/green regression
TestActionCannotConsumeCommittedFutureReservation pins this temporal constraint.
