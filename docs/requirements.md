# Requirements matrix and source gaps

Authority is the executable summaries in epic #1 and tickets #2–#17, plus owner-supplied alignment epic #39 and its child tickets. Original
PRD attachments are unavailable and are not published here. The source reference
register below enumerates every cited section/ID but does **not** invent per-section
meaning from missing text. Full source traceability remains UNVERIFIED. The String
Between Us supplies intent, not additional implementation scope.

## Executable invariant mapping

These labels E1–E12 refer to epic correctness rules, not invented IHG source IDs.

| Requirement | Implementation / evidence | Remaining scope |
| --- | --- | --- |
| Reusable core; no compulsory worlds | `core`, `app/graph`, `examples/graphclient`; architecture and independent second-host tests | Full IHG product not claimed |
| E1 durable versioned events and scoped idempotency | PostgreSQL event store, migrations and integration tests (#3/#4) | Source-wide private PRD audit unavailable |
| E2 occurred/valid/learned/recorded time and correction | Temporal graph/memory queries, replay and correction tests | No global relationship truth |
| E3 injected time/RNG, stable ordering and hashes | Runtime, dynamics, drives and action tests | Fresh model outputs remain stochastic |
| E4 compatible recorded/fake replay | Recorded model operations and replay/branch tests | No exact reproduction claim for fresh calls |
| E5 fencing, crash checkpoints, provider transaction boundary | Runtime leases, recovery and blocked-provider integration tests | Bounded single-host deployment design |
| E6 authorization, manifests, audit and budgets | Application admission and PostgreSQL RLS/auth/budget negatives | No paid/live calls authorized |
| E7 disclosure policies; E8 deny unknown, abstraction off by default | Separate fictional/strict policies and source-propagation tests | No covert persuasion or automated relationship advice |
| E9 immediate revocation and derivative invalidation | Revocation, cache/export/replay races and purge-aware restore | Already delivered offline artifacts cannot be recalled |
| E10 trusted credentials | Authenticated gRPC/HTTP management host, tenant/grant negatives | Execution needs configured embedding handler; no deployment |
| E11 bounded state and offline owner promotion | Drive/action budgets; isolated evaluator with signed preregistration/holdout | No online weight updates or automatic activation |
| E12 explicit paid/live configuration | Fake/recorded defaults; live/soak targets fail closed | Live research and infrastructure need later owner scope |
| Observer/evidence/uncertainty/time | Claim, memory, relationship codecs and approved-context appraisal | Synthetic behavior is not human validity |
| Independent evaluation | Import boundaries, isolated label-blind child, split/consent tests | Complete supplied source report is ADR0020; adequacy mostly unresolved |
| Clean verification and operations | `make verify`, pinned generation, race/Postgres/restore/container/demo checks | Exact SHA-bound results and failures are in PR checkpoints |
| Durable review and owner-only main | AGENTS.md, workflow, PR #38 | Engineering integration is not a released product |
| 24 people / eight groups / relationships | Bounded demo: 104 directional reports; five-person contrast fixture | Operational smoke, not production capacity |
| 12 model months versus 30 real days | 360 virtual days, 24 sparse observation periods; study clock gates | Real 30-day study NOT_RUN |
| 200 families / 10,000 cases | Small bounded synthetic harness and split/holdout mechanisms | Research scale target NOT_RUN |
| Real-human transfer and UI | Explicit NOT_TESTED scientific verdicts | No real data, UI or interventions authorized |

## Source reference register

Tickets #40–#43 supply HWS §6, §7, §9, §19 and §20 extracts. Those extracts
are mapped below and require no private HTML. All other missing original text
remains a source gap; do not infer section-level completeness from ticket closure.

| Document | Section / invariant ID | Status |
| --- | --- | --- |
| HWS PRD v0.1 | §2 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §3 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §4 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §5 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §6 drive registry | Owner-supplied exact22-name registry in #40; simulator/drives and ADR0017. Other original section text is not certified |
| HWS PRD v0.1 | §7 | SUPPLIED SOURCE: #42 revision 2 provides relationship fields/principles; ADR-0019 maps bounded implementation and tests. Full private document remains unverified. |
| HWS PRD v0.1 | §8 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §9 | SUPPLIED SOURCE: #41 revision 2 gives all27 actions and14-stage lifecycle; ADR-0018 maps implementation/tests. Full private document remains unverified. |
| HWS PRD v0.1 | §10 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §11 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §12 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §13 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §14 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §15 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §16 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §17 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §18 | UNVERIFIED: original text unavailable; summarized scope above |
| HWS PRD v0.1 | §19 | SUPPLIED SOURCE: #43 revision 2, all13 exit criteria; immutable registry and evidence report, ADR0020. |
| HWS PRD v0.1 | §20 | SUPPLIED SOURCE: #43 revision 2, all10 primary falsifiers; explicit unknown polarity and adequacy limits, ADR0020. |
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
| Complete HWS §6 drive-name registry | Exact22 names supplied by owner in #40; new versioned simulator/drives implementation, ADR0017 and registry tests. Independent integration gate still required |
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
codec and ticket-named v1 registry predate the supplied #41 source. ADR-0018
now maps the complete supplied §9 grammar. They do not claim full private HWS
source coverage, large studies or validated human realism. Pure internal planner/handler
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

## #15 operations and audit

ADR-0014 and docs/operations.md map append-only known-usage facts, retained
reservations, cross-scope provider limits, recovery fencing, redacted metrics/OTel,
graceful drain/readiness, authorized audit/reproducibility verification and an
actual pre-revocation backup restore drill. Schema 6 adds a quarantine gate and
retained revoked IDs; no administrator-proof erasure or automatic journal freshness
is claimed. The product maintenance worker is not an agent supervisor. Engineering
acceptance still requires full checks and independent exact-SHA review/integration.

## #16 independent evaluation

ADR-0015 and docs/evaluation.md map the independent evaluator process, frozen
family/person/group/time splits, label-blind generation, consented synthetic imports,
proper scoring/missingness, clustered uncertainty, conditional forecasts and
separately owner-signed batch authorization protocol. All reports keep real-human
validity and cross-model transfer NOT TESTED; complete original HWS falsifier and
registry traceability remain UNVERIFIED. No production defaults or owner adequacy
thresholds are changed. Engineering integration still requires exact-SHA review
and passing complete checks.

## #17 demo and separate study protocol

ADR0016, docs/backend-demo.md and docs/study-protocol.md map the 24-person,
eight-overlapping-group, 48-directed-edge sparse reference demo; actual fake
multi-seed run/replay/export/recovery measurements; and the evaluator-owned fixed
30-real-day reservation/report protocol. Existing runtime/cognitive/receipt caps
are retained. Engineering acceptance, synthetic behavioral evidence and human
validity are separate fields. A real 30-day run, live independent frontier-model
transfer, real-human validity and unavailable complete HWS falsifiers are NOT
TESTED/NOT RUN. The backend console contract and UI backlog are owner-review
material only; no UI entry, deployment, model-weight update or main merge occurs.

## Alignment #40: full drive registry and explicit old-state compatibility

ADR0017 maps the owner-supplied22-drive registry, bounded hypothesis parameters,
permitted context/history/resource/relationship/belief/uncertainty appraisal,
version-aware decoder and isolated new runtime host. Legacy six-variable state
and recorded policies are retained with their old wire semantics. Only this
registry source gap is resolved; #41–#44 remain separate sequential work.
