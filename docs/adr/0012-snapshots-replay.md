# ADR-0012: frozen boundaries, exact replay and isolated experiments

Status: proposed for issue #13, subject to independent exact-SHA review.

Snapshots are stored artifacts, not caller-supplied restoration authority.
`SnapshotService` requires a current exact-scope researcher permit: Read for
replay/diff, Retain for capture and Derive for fork, in addition to the research
view's read grant. It accepts a stored scope/key/hash handle, never a submitted
snapshot body. Actor/external views cannot obtain research replay. Raw store
ports remain trusted host composition; #14 supplies network authentication.
Research authorization is current, never restored from an old snapshot. Replay
and diff repeat authorization and re-read the entire requested trajectory before
returning, so later-frame revocation cannot hide behind a still-valid base.
Already-returned copies cannot be retroactively erased.

## Frozen data and version boundaries

`snapshot.v1` captures the committed runtime revision and event, journal offset,
engine/RNG version, full logical state (queue, seen IDs, RNG positions, resources,
knowledge/genesis and policy checkpoint), actor memory realms and exact consumed
model-result/failure references. Configured model limits and already reserved
spending/tokens are captured too. Child snapshots reference their immutable parent
snapshot, retaining access to historical model/configuration provenance without
pretending those model calls occurred in the child. Hashes cover the whole frozen
body; logical replay state hashes remain the runtime's canonical hashes.

Migration 005 stores bodies in a writer-only FORCE-RLS table with an 8 MiB ceiling,
plus immutable snapshot/branch metadata and cross-scope source references. Capture
runs under the existing short journal lock and optimistic revision check. Keys
are immutable and retryable; a revoked key cannot be recreated. Snapshot reads
check stored hash, exact scope, current tombstones and artifact invalidation.
Unknown versions, incomplete actor realms, malformed model references, corruption,
trailing data and byte/count exhaustion fail closed.

The runtime now persists the command, prior logical hash, model-use reference and
any operational-stop input with each transition. Older historical transitions
that lack a recorded command return replay unavailable; no command is guessed.
Existing v4 state/rows are preserved by forward migration. The new optional payload
fields do not rewrite old events or change base-run RNG sequences.

## Replay and comparison

`event_replay` verifies the stored hash chain and applies authoritative committed
states. `recorded_decision_replay` executes the runtime with stored inputs,
outputs, typed resource effects and verified RNG draws, then compares exact hashes
and transition bytes. The replay function has no provider/infrastructure port;
missing or incompatible recordings cannot fall back to fresh generation. A real
separate-process fixture reproduces the stored final hash. Operational deadline
stops are recorded inputs rather than a fresh wall-clock decision during replay.

`fresh_resimulation` is a separate fork operation. Its response and subsequent
loads carry an experiment label with Exact=false, the source handle and coupling
method. Fresh generation is never asserted byte-guaranteed, even with identical
seed/model aliases. This ticket executes only synthetic fake/recorded fixtures;
no live or paid call is made. Exact provider input matching remains #10's rule;
branch names or a shared seed cannot select a parent answer for a different prompt.

Trajectory comparison identifies the first differing logical position, changed
input/evidence IDs and subsequent state-hash differences. Initial memory
comparison ignores journal sequence/recorded wall time while preserving observer,
content, rights and logical learned/valid times. Comparisons are trace differences,
not real-world causal claims or independent evaluation scores.

## Fork isolation, randomness and budgets

Forks use the same authorized operator/namespace/world with distinct branch and
run IDs. The journal records a branch-origin event, child memory materializations,
and the initial child checkpoint atomically. Child events and scopes reference
immutable parent history; parent/sibling writes are never queried as child memory.
Only history learned at the fork cutoff with a closed retained source DAG is
materialized; future knowledge and later parent writes stay out. Every copied
record requires its actor's explicit self Derive right. Scope-local event IDs and
stream versions support new independent corrections/writes. Child grants, leases,
operations, jobs, model requests and caches are not inherited as live authority.

The child keeps the source step/event/horizon budgets, consumed resources,
appraisal receipts and reserved model-token/spending floor. Its deadline is the
minimum of the requested duration and the existing parent's deadline; retries do
not extend it. Limits cannot silently reset during a fork. Model budgets remain
per-run ceilings, not an aggregate billing authorization for a whole experiment
tree; live branch runs still require explicit owner funding/configuration, and
#15 owns additional operational admission controls.

`runtime.branch.v1` is an explicit engine profile, rejected by older engines.
Endogenous draws use a child-scope domain. With `common-exogenous-counter.v1`, only
handler-designated `exogenous:` streams share the parent's compatible counter
identity/positions; private human choices remain independent. Otherwise all
post-fork streams are independent. The method is persisted in the branch record.
Coupling assumes matching exogenous semantics; it does not claim that different
prompts or diverging event schedules yield comparable provider randomness.

An optional new observation is recorded as an alternative configuration, with
ordinary actor/time/ID/queue constraints. The full fork specification and its hash
are retained in the initial child event payload. The implemented human-actions.v1
policy may be selected explicitly; unknown policy versions fail. There is no
implicit policy promotion, retroactive event edit, permission expansion, reset of
shared history or destructive snapshot import.

## Revocation and limits

Cross-scope source edges must point to earlier journal events. Revocation computes
a closure across ordinary lineage, these edges and model applications before
purging payloads. Thus a parent source can invalidate a snapshot and all dependent
children, while revoking a child copy invalidates that child's dependent canonical
state without changing its parent or sibling. Snapshot creation/fork versus
revocation races share the journal lock. Tombstones and metadata survive; bodies
do not reappear through replay, fork, old handles or purge-aware restore.

A new snapshot taken after an unrelated memory purge may retain redacted metadata
and support replay. Forking such a snapshot is currently unavailable: dropping
those tombstones would permit reuse of revoked IDs in the child. This conservative
restriction remains until metadata-only tombstone inheritance is implemented;
SnapshotAfterPurgeCannotDropTombstones pins both redacted reads and atomic denial.

Retention limitations remain: delivered bytes, old MVCC/WAL and offline backups
need the prior documented handling; this does not claim retroactive erasure.
Snapshot bodies are at most 8 MiB, trajectories at most 128 frames/32 MiB, and all
existing runtime/memory/RNG/cognitive caps remain. New child RNG domains can exhaust
the existing stream-position cap; that fails without eviction. This is bounded
engineering evidence, not a long-horizon or 24-human study claim. Full unavailable
HWS registry/PRD traceability and human-validity gates remain UNVERIFIED/NOT TESTED.

## Acceptance mapping

| Requirement | Evidence |
| --- | --- |
| Hash/version/corruption and exact recorded state | TestRecordedAndEventReplayExactAndTampering; TestSnapshotIntegration/ExactReplayScopeAndCorruption; separate TestReplayProcessHelper |
| Real parent/sibling isolation and knowledge cutoff | ConcurrentForksKnowledgeCutoffAndWrites; scoped wrong-branch/run negatives |
| Snapshot before revocation cannot resurrect | RevocationAcrossSnapshotChildAndSibling; ConcurrentRevocationCannotResurrectFork; cognitive snapshots retain exact model/failure references and purge |
| Distinct fresh mode and coupled/exclusive streams | TestForkRandomnessAlternativesAndParentImmutability; BranchBudgetAndDeadlineDoNotReset checks persisted Exact=false label |
| Over-time budgets and finite capacity | BranchBudgetAndDeadlineDoNotReset; TestReservedCapacityAffordanceContinuesAndReplays; existing future-reservation positive/negative tests |
| Research rights and replay revalidation | TestSnapshotAuthorityAndCurrentReplayRevalidation |
| Alternatives and noncausal difference report | TestTrajectoryDiffNamesAlternativeAndFollowOn; retroactive/foreign-world/unknown-policy negatives |
| Migration/RLS/restore | V4UpgradeAndRestrictedSnapshotTable; five-version/seven-policy purged-state pg_dump/restore gate |

`env -u DREAM_TEST_DSN make verify` runs all required static, race, architecture,
real disposable PostgreSQL, migration/restore and build/help gates. Docker cleanup
remains scoped to the exact uniquely named container created by that check.
