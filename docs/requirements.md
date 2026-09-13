# Requirements matrix and source gaps

Authority is the executable summaries in epic #1 and tickets #2–#17. Original
PRD attachments are unavailable and are not published here. The source reference
register below enumerates every cited section/ID but does **not** invent per-section
meaning from missing text. Full source traceability remains UNVERIFIED. The String
Between Us supplies intent, not additional implementation scope.

## Executable invariant mapping

These labels E1–E12 refer to epic correctness rules, not invented IHG source IDs.

| Requirement | Engineering owner | #2 evidence / deferred gate |
| --- | --- | --- |
| Reusable core; no compulsory worlds; package ownership | #2, #3, #9 | TestRepositoryBoundaries, TestRules; second-host proof deferred #9 |
| E1 versioned durable events, scoped idempotency | #3, #4 | DEFERRED; no persistence implemented |
| E2 distinct occurred/valid/learned/recorded times and corrections | #3, #4, #8 | DEFERRED; temporal/as-of tests required |
| E3 injected time/RNG, stable order, canonical hashes | #5, #6 | DEFERRED; deterministic fixtures/runtime tests |
| E4 recorded/fake exact replay; fresh generation stochastic | #10, #13 | DEFERRED; no replay claim |
| E5 fenced writer, crash-safe checkpoints, no provider calls in transactions | #4, #6, #10 | DEFERRED; recovery/duplicate-effect tests |
| E6 auth/manifests/audit/budget negatives with first consumers; #12 before models | #4, #6, #12, #15 | workflow fixes dependency order; no consumers yet |
| E7 fictional own-experience disclosure differs from IHG restricted-context policy | #12, #11 | DEFERRED; explicit policy tests |
| E8 abstraction disabled by default; unknown evidence denies/WAITs | #12 | DEFERRED; no privacy guarantees claimed |
| E9 immediate revocation and derivative invalidation; replay cannot resurrect purge | #4, #12, #13 | DEFERRED; retention/backups must be documented |
| E10 trusted credentials; no exposed placeholder auth | #14 | command scaffolds open no network sockets; actual auth DEFERRED |
| E11 bounded daily state; offline heldout candidate promotion | #7, #11, #16 | DEFERRED; no online training/promotion |
| E12 explicit paid/live budget/infrastructure configuration | #10, #15, #17 | default CI has no live calls; optional targets fail closed |
| Observer/evidence/uncertainty/time preserved; no global relationship truth or engagement objective | #3 onward | DEFERRED domain/behavior tests |
| Independent evaluation; generation must not import evals | #2, #16 | TestRules forbids simulator/app/evals imports; research harness deferred |
| Backend-only, PostgreSQL-only local compose | #2 | compose.yaml and explicit CLI scaffolds |
| Negative architecture invariant including tagged/platform source | #2 | TestIllegalTaggedImport and TestPlatformFilesAndMalformedSource; testdata/illegal/core/bad.go |
| Clean checkout, race/build/help, PR and integration CI | #2 | make verify, .github/workflows/verify.yml; remote results in PR |
| Durable SHA-bound review/integration; owner-only main | #2 | AGENTS.md, agent-workflow.md, PR template; independent review pending |
| 24 humans / 8 groups / 30+ edges after 2–4 actor fixtures | #5, #17 | DEFERRED; no simulation implemented |
| 12-month virtual horizon distinct from real-time 30-day study | #17 | DEFERRED; live study NOT RUN |
| 200 scenario families / 10,000+ scale; preregistered ablations/holdouts/calibration | #16, #17 / later research | DEFERRED research targets; not bootstrap acceptance |
| Real-human transfer/validity; UI entry by owner engineering approval | #16, #17 / owner | NOT TESTED without real-human dataset; UI implementation excluded |

## Source reference register

Every row is a missing-source gap, not a verified section-level interpretation.
Ticket summaries constrain implementation in the interim; obtain authorized source
access to complete traceability without publishing originals or private data.

| Document | Section / invariant ID | Status |
| --- | --- | --- |
| HWS PRD v0.1 | §2 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §3 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §4 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §5 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §6 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §7 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §8 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §9 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §10 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §11 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §12 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §13 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §14 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §15 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §16 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §17 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §18 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §19 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §20 | UNVERIFIED: original text unavailable; summarized scope above |
| IHG canonical PRD v1.1 | 5 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 14 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 15 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 16 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 17 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 18 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 77 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 78 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 79 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 80 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 81 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 82 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 83 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 93 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 94 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 95 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 96 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 97 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 98 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 99 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 100 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 101 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 102 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 103 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 104 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 105 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 106 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 102A | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 102B | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 102C | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 102D | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 102E | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 102F | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 102G | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 102H | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 102I | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 193 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 201 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 202A | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 203 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 204 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 205 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 206 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 207 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 208 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 209 | UNVERIFIED: original text unavailable; summarized invariants above |
| IHG canonical PRD v1.1 | 210 | UNVERIFIED: original text unavailable; summarized invariants above |

## Issue #3 contract evidence

| Ticket criterion | Current evidence | Remaining owning-ticket gate |
| --- | --- | --- |
| Validated reusable records, generic events | core value/records/state; TestRecordsRoundTrip, TestInvalidRecords | #4 durable event transactions/projections |
| Observer-owned identity and contradictory perspectives | TestContradictoryPerspectivesAndIdentityIsolation | #9 second host use cases; no implicit identity resolution |
| Evidence, lineage, sensitivity and uncertainty | TestInvalidRecords cycles/missing evidence/NaN/calibration/time cases | #8 retrieval and #12 trusted information boundary |
| Separate exact permissions and no implicit broadening | TestPermissionsDenyUnknownAndSeparateOperations, TestDerivationIntersection, TestStateCannotBroadenSourceRights, FuzzRightsNeverWiden | #4/#12 trusted authorization and revocation consumers |
| Simulator-only IDs/knowledge/RNG/latent contracts | simulator/types.go, TestSimulatorLocalContracts, architecture guard | #5/#6/#7 scenarios/runtime/dynamics |
| Consumer-defined ports outside pure entities | app/graph RightsReader, app/hws CapabilityProvider; examples/coreclient | concrete adapters and model gateway deferred |
| Canonical encoding, logical hashing | TestCanonicalGolden, TestProvenanceGolden, TestLogicalHashGolden, TestLogicalHashAndOrdering, FuzzDecode | replay/snapshot guarantees deferred #13 |
| Constructor invalid-input handling | TestIDConstructorsRejectInvalid, FuzzConstructors, TestNaNEncoding | domain evolution requires new regressions |
| WAIT and unknown/censored/observed outcomes | TestOutcomeStatuses, TestInvalidRecords | #11 outcome learning and #16 independent evaluation |

See ADR-0002 for exact codec and validation boundaries. Source-section traceability
above remains UNVERIFIED; contract tests do not substitute for unavailable originals.

## Issue #4 storage evidence

| Criterion | Real-Postgres evidence |
| --- | --- |
| Generic events plus separate simulator/job/artifact references | migration 001; simulator learned-at/learner rows tested separately from events |
| Atomic expected-version/idempotency/outbox effects | AtomicIdempotencyAndConcurrency; eight duplicate callers and two competing versions |
| Actor/namespace/operation key scope; no wall-clock logical hash | ScopedKeysAndWallTimeIndependentDigest; graph TestDigestCanonicalSetsAndZero |
| Interrupted transaction rollback | InterruptedAppendRollsBack, TemporalCorrectionProjectionRecovery, RevocationRollback |
| Valid-at/known-as-of late correction and rebuild | TemporalCorrectionProjectionRecovery; old row remains in prior system-time view |
| Restricted table grants and actor isolation | DatabaseGrantsAndActorIsolation; real actor login and reader-role negatives |
| Source permission/class restrictions | PayloadClassCannotBroaden (red/green regression); append validates trusted stored provenance |
| Purge, tombstones and snapshot/export/rebuild invalidation | RevocationPurgeAndArtifacts, ArtifactRevocationRace; already-purged pg_dump/restore |
| Schema forward/rerun/version dispatch | Migrate initial/rerun, TestPayloadVersionDispatch, mandatory migration-check |

See ADR-0003 for backup/WAL limits, internal trusted-scope requirements and deferred
upcasters for future schemas. No complete #12 privacy gate or #13 snapshot engine
is claimed. Earlier N/A migration statements describe bootstrap history only;
current make verify runs the real disposable Postgres gate.

## Issue #5 scenario evidence

| Criterion | Evidence |
| --- | --- |
| Versioned world, adults, groups/relationships, resources, knowledge/memory, latent/future DSL | simulator/scenario types, strict adapters/scenario parser; quiet-overlap fixture |
| Validated genesis rather than mutable initialization | TestGenesisAndCapabilities; tampered envelope/payload/hash and incompatible engine negatives |
| Actor/research/future boundary | TestActorViewBoundary, TestFixtureGoldenAndBoundary; private/latent/label/future canaries, foreign knowledge denial, deep-copy mutation |
| References, IDs, intervals, grants, capacity/overflow | TestInvalidScenarios, TestResourceIntervalsAndOverflow |
| Unknown/duplicate fields, size/depth/nodes/aliases/typing | TestYAMLRejects, TestReaderBound, FuzzYAML |
| Stable encoding/hash and deterministic schedule ties | byte/hash fixture goldens, TestCanonicalProperties, FuzzCanonical |
| Offline machine-readable CLI | TestValidateCLI; parser/CLI use no database or model credentials |
| Migration and future capability semantics | ADR-0004; declared plus inferred capabilities, old engine denies execution |

Runtime persistence/application is #6, dynamics #7, runtime retrieval/policy #8/#12;
fixture labels make no independent evaluation or human realism claim.

## Issue #6 runtime evidence

| Criterion | Evidence |
| --- | --- |
| Pure virtual time, deterministic equal-time order and named/versioned RNG | TestDeterministicRecovery, TestInjectionAndNamedRNG, TestRNGGoldenAndBudgets, TestInclusiveRunUntilTies, TestReservationReleaseBeforeAllocation, FuzzTrajectory |
| Durable step/run-until/inject/pause/resume/cancel and idempotency | real-PG CreateIdempotencyAndScope, RunUntilRecoveryAndControlBoundary, SimultaneousDuplicateAndVersion |
| Expiry/reclaim, fencing, renewal and optimistic versions | LeaseReclaimFencingRenewAndOverlap, LeaseExpiryDuringCommitRollsBack, SimultaneousDuplicateAndVersion |
| Atomic draws/outputs/events/checkpoint/receipt with crash recovery | RollbackOutputsRNGAndOperation, KillRestartMatchesCleanTrajectory (actual killed child process) |
| Run manifest, minimum audit and step/event/horizon/duration bounds | schema v2, TestControlsAndBudgets, OperationalBudgetDiscardsComputedTransition, DeadlineCrossingDuringTransaction |
| Restricted durable state, revocation/no resurrection, forward upgrade | RestrictedRuntimeGrants, RevocationPurgesAndPreventsResume, ForwardUpgradePreservesJournal; extended purge-aware restore gate |
| #5 review carry-forward assertions | TestYAMLRejects pins alias/limit codes; TestForeignKnowledgeOwnershipGuard pins exact ownership path/message |

See ADR-0005 for host operation semantics, serialized journal write limitation,
trusted handler/port boundary and deferred CLI/API, dynamics/replay/scientific gates.

## Issue #7 synthetic appraisal evidence and source gap

| Criterion | Evidence / status |
| --- | --- |
| Complete HWS §6 drive-name registry | **UNVERIFIED / NOT VERIFIABLE**: complete source list unavailable. No all-names coverage claim. Owner source request remains open |
| Versioned documented initial active subset | ticket7-subset.v1 contains only the six concepts explicitly named in #7; ADR-0006 lists defaults, active and unsupported parameter coverage |
| Stable substrate, bounded uncertain latent state, time and causes | TestDecaySubdivisionAndResidue, TestPlasticityBoundsAndCompoundInteraction, TestCodecAndCausalLedger |
| Permitted perceived event → appraisal → competing deltas | TestPermissionAndStageIdempotency, TestReferenceTransitionGolden, AppraisalHandler boundary tests |
| One stage owner/key; no double appraisal | appraisal.v1 actor/event key; duplicate/conflict/revocation negatives; real-PG AppraisalPersistenceAndRetry |
| Analytic decay and subdivision contract | 997-way subdivision, analytic half-life check, FuzzDecayAndAppraisal; explicit 1e-9 quantum/tolerance |
| Structured short rationale, no private trace | enum/delta-only Rationale; runtime raw-text canary and unknown rationale-code rejection |
| Intervention changes reference tendency; no action selection | TestInterventionChangesCompetingTendency; #11 owns actual actions/outcomes |
| Pinned encoding/restart/limits | canonical codec/hash tests, runtime restart hash comparison and existing 4096-byte cap retained |

Proceeding with the explicitly scoped subset follows the existing epic missing-source
policy and Claude's clarification on issue #7 (comment 5651532761). This does not
satisfy or erase the complete-registry source gap. Registry/model additions require
new versions and explicit migration; unknown initialization parameters fail closed.

## Issue #8 temporal memory evidence

Reusable memory/claim/open-loop/intent services live in `app/graph`, backed by the
existing generic PostgreSQL journal. ADR-0007 maps temporal contradiction/disclosure,
evidence/permission, supersession/expiry, cache/revocation, bounded ranking and
incremental/rebuild acceptance to focused tests and real-PG integration subtests.
Synthetic actual-vs-believed comparison remains in `simulator/belief`; emotional
residue remains the #7 dynamics implementation. No LLM, world or vector service is
required by retrieval. The unchanged payload cap and explicit per-scope record
budget are small-fixture limits, not long-horizon scale claims; #11/#15 carry that
integration work. Full source traceability and later #12 policy/#14 credentials
remain separate gates, not fabricated passes.

## Issue #9 relationship and second-host evidence

ADR-0008 maps first-class observer-specific edge/group projections, temporal roles,
multiple types, uncertain dimensions/patterns, commitment/open-loop references,
history and deltas to focused tests and TestRelationIntegration. Membership grants
no private historical access; opposing perspectives remain separate. The independent
`examples/graphclient.Run` consumer executes statement ingestion, differing claims,
correction, revocation and permitted export against real disposable PostgreSQL in
TestGraphClientIntegration, with no world/run or simulator imports. Import guards
pin that boundary. app/hws only converts permitted own-perspective projections to
simulator edge context; actual behavior is #11 and model information policy is #12.
Small typed-record byte/count limits remain explicit, not final scale evidence.


## Issue #12 information boundary evidence

ADR-0009 maps exact per-right lineage checks, opaque context capabilities, current
runtime revision/time, cache revocation, bounded quotation output and mandatory
sanitized audits to unit/race and real PostgreSQL tests. Fictional own-disclosure
has a positive control; restricted third-party assistant disclosure fails directly
and through derivatives. Actor/self/research/external views are distinct. External
GodState, labels/future state, unknown attribution and injected writer sources deny.
Cross-perspective abstraction remains disabled, including research assessments;
there is no inferred privacy guarantee. Migration 003 adds authenticated-login,
actor/namespace/class mappings and FORCE RLS; runtime checkpoints stay writer-only.
Trusted host composition is not network authentication (#14), and models/actions
remain #10/#11. See ADR-0009 for byte/count limits, attribution assumptions, old
snapshots/delivered-data limitations and complete requirement-to-test mapping.

## Issue #10 model gateway evidence

ADR-0010 maps typed cognition proposals, version/observer/provenance validation,
policy-approved context, offline fake/recorded adapters and mocked Responses HTTP
conformance to executable tests. Migration 004 persists per-run reservations,
fenced attempts and purgeable request/result artifacts before canonical application.
Crash/retry, concurrency, cancellation, stale-source and permit-revocation negatives
cover the first provider consumer; no network callback holds a transaction. Recorded
replay preserves bytes with no fallback; fresh generation is explicitly stochastic.
Live provider choice, credentials/prices and paid runs remain owner-configured and
NOT RUN. #11 owns actions and sizing; #14 owns network authentication. Existing
scientific/source gaps and privacy limitations are not waived by this engineering gate.

## Issue #11 cognitive/action evidence

ADR-0011 maps the typed single-owner cognitive pipeline, versioned synthetic action
registry, normalized recorded selection, safe delivery/resource effects, own-fiction
disclosure and delayed observer-specific outcomes to unit/race and real PostgreSQL
evidence. Operational WAIT requires durable exhausted retryable failure evidence;
refusals and permission errors cannot masquerade as human behavior. Appraisal
receipts and the runtime 4096-byte cap remain hard limits. The bounded four-actor
codec and ticket-named action registry do not claim complete unavailable HWS source
coverage, large studies or validated human realism. Pure internal planner/handler
ports are not network authentication. See ADR-0011 for recovery, privacy and sizing
limits and later-ticket responsibilities.

## Issue #13 snapshot, replay and branch evidence

ADR-0012 maps frozen scope/key/hash handles, research authorization, current
revocation checks, exact recorded replay and isolated counterfactual experiments
to corruption, concurrent branch/revocation, knowledge-cutoff, budget and process
restart tests. Migration 005 adds restricted snapshot bodies and immutable
cross-scope provenance; all old source and scientific limitations remain.
Fresh experiments are explicitly not byte-guaranteed. Original actor/cognitive,
model and runtime caps remain; no permission, budget, receipt or time reset is
hidden inside a fork. Large/live studies and network authentication remain later
gates, not implied by snapshot/replay engineering evidence.

## #14 authenticated transport

Versioned gRPC/HTTP/OpenAPI, exact configured scopes/roles, non-owner runtime
startup, durable admission, runtime/view/snapshot adapters and revalidated paged
exports are implemented. ADR0013 maps verification and states the management-only
standalone executable versus embedded execution-host distinction. Engineering
integration still requires independent exact-SHA review, green CI and an
INTEGRATED record; implementation is not a main release or deployment.
