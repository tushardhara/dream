# ADR-0008: observer-specific relationships and an independent host

Status: implementation for #9; review/integration evidence belongs in its PR.

## Typed reusable projections

`app/graph.RelationService` ingests and queries typed `RelationState` v1 accounts
of an edge or group. It uses the existing versioned memory journal and its current
source/rights/time checks. These are first-class application projections, not a
new SQL table or a simulator-owned domain. The inner `relation.v1:` discriminator
and strict canonical JSON follow it inside a `RelationshipMemory` text payload.
Old free-text relationship memories remain readable as memory and are skipped by
the typed relation selector. No persisted core/memory v1 fields change.

Each record retains its observer, subject, sources, supporting/contradicting evidence,
confidence, rights, occurrence, learned, valid and recorded times. State carries:

- Edge endpoints (principals or observer-owned references), or a group ID.
- Multiple explicitly named relationship types.
- Named dimensions in [-1,1], each with subjective confidence. There is no canonical
  health scalar or automatic aggregation of dimensions or observers.
- Membership intervals and roles. Intervals are half-open, and overlapping intervals
  for the same member are rejected. Edge members must be endpoints.
- Commitments referencing intent records; open-loop signals referencing open-loop
  records; patterns with their own confidence and explicit evidence IDs.

Signal and pattern references must be included in the envelope's full provenance.
They inherit normal source permissions, knowledge/valid-time filtering and revocation.
Commitment/open-loop source kinds are checked by the service; source kind is immutable,
while the journal rechecks revocation/rights/source knowledge inside the append lock.
This type check does not replace transactional authorization. Source expiry can
conservatively invalidate the dependent relation snapshot; the host must append a
new account without the expired signal. No implicit obligation or completion is
inferred from an intent, participation, silence or expiry.

An edge has no permanent data owner. Its accounts belong to their respective
observers, and every source stays rights-bound. Two observers may use the same edge
ID and disagree completely: the query returns their separate scopes, never their
average. Membership creates **no** grants, including for historical private
contributions. An explicitly authorized audience still needs all normal learned,
valid, recorded and source gates. Current revocation denies historical queries too.

Corrections retain entity ID, kind and edge endpoints, in addition to generic
subject-preserving supersession. Changing an edge's identity requires a new account,
not relabeling an old account with new endpoints. Query returns the active membership
at the requested valid time and retains the underlying account for provenance/history.
The relation ID/kind filter runs before the result budget, so unrelated high-salience
memories cannot crowd a requested edge out of a one-result query.

`CompareRelation` outputs named before/after dimension deltas and their separate
confidences, source record IDs and open-loop signals. It requires the same observer
and relation and checks projection state against its source record. Missing dimensions
remain unknown: they are not filled with zero. This is a descriptive comparison of
supplied snapshots, not a current authorization service or IHG intervention selector.
Callers obtain permitted current/as-of snapshots through RelationService; previously
delivered values cannot be recalled by a later revoke.

## HWS consumer boundary

`app/hws.EdgeContextService` calls the reusable relation service and converts an
actor's **own** permitted edge account into `simulator.EdgeContext`. It preserves
named dimension values/confidences, pattern evidence/confidence, relationship types,
active roles, source/observer/relation IDs and open-loop signals. Another observer's
account cannot be substituted as the actor's own perspective. Multiple permitted
accounts remain multiple context values; there is no averaging or membership-to-care
conversion. The service has no provider calls or behavior/action selection. #11
owns actual behavior tests; #12 still owns the full model information boundary.
No emergent behavior or psychological validity is claimed from these projections.

## Independent consumer and export

`examples/graphclient.Run` imports only core/app/graph from Dream. A host injects
MemoryJournal + EventRevoker ports; it needs no world/run, simulator types, adapters
or model client. `TestGraphClientIntegration` supplies the real PostgreSQL adapter
with a disposable writer role and exercises the example end to end:

1. Validate two fictional principal IDs and ingest their separate statements/claims.
2. Query the opposed views without collapsing them.
3. Correct Alice's source, invalidating the old dependent claim while preserving Bob.
4. Revoke Alice's source and verify source/correction/claim are inaccessible.
5. Deny export to an unauthorized recipient; export Bob's permitted state to the
   explicitly granted recipient. No simulator association is created.

Run the example's real database gate with `make migration-check` (or `make verify`).
It is a callable host example, not a deployed service or a new network CLI. Import
rules explicitly restrict its local dependencies to core/app/graph/itself, including
platform/tagged source. `go list -deps ./examples/graphclient` verifies there is no
transitive simulator, app/hws or adapter dependency.

`MemoryService.Export` reads one fresh scoped snapshot. It composes normal read/
knowledge/time/evidence gates with exact actor, purpose and recipient export grants
on every record and every provenance/supersession source. It filters before the
result budget, preserves rights/provenance in the returned versioned records, and
returns detached values. No durable export cache is added. Exported bytes already
handed to a caller cannot be recalled; every later export rechecks current tombstones.
This is not a #13 persisted snapshot or #12 declassification implementation.

`RevokeMemory` is a reusable orchestration wrapper around the existing atomic
versioned revoke/tombstone/purge operation. Trusted host credentials supply owner
scope; this does not introduce role-header authentication or a new permission model.

## Recovery, limits and evidence

The relation projection is rebuilt from the same canonical memory journal and
current tombstones. Incremental/rebuilt logical hashes agree; a newly constructed
service reads identical typed projections after a connection/service restart.
Revoking a source immediately removes the dependent relation and its HWS modifiers,
without waiting for a projector. Existing journal transaction/idempotency rules and
#8's shared-input hashing regressions remain enabled.

Bounds remain explicit: #8's 512 retained memory events per scope and 50 results,
2048 bytes for the complete typed inner text, 4096 bytes for outer payload. Within
that envelope: at most 24 membership intervals, eight relationship types/dimensions/
patterns/commitments/open loops, eight roles per interval and eight sources per
pattern. The byte ceiling may bind before the count caps. There is no silent
truncation, history eviction or scope-cap increase. Larger #11/#17 runs need reviewed
sizing/storage work; this slice proves small synthetic reusable behavior only.

| Acceptance | Evidence |
| --- | --- |
| Per-observer dimensions/types/roles/patterns/commitments/history/deltas | TestRelationPerspectiveHistoryDeltaAndRebuild; real-PG TestRelationIntegration |
| Temporal membership and no automatic historical rights | TestTemporalMembershipDoesNotGrantHistory; half-open membership and overlap negatives |
| Opposing views never average; no participation=care | TestOpposingRelationshipViewsStaySeparate; membership test asserts no invented dimensions |
| Kind/entity selection before budget; old text compatibility | TestRelationFiltersBeforeLimitAndPreservesLegacyMemory |
| Bounded canonical codec, uncertain signals and complete provenance | TestRelationCodecAndSignals, concurrent immutable encoding checks |
| Permission-bound export and revocation | TestExportRequiresEverySourceAndRecipient; real-PG independent host |
| Genuine non-simulator consumer | TestGraphClientIntegration; import guards and dependency inventory |
| Recovery and immediate source denial in HWS modifiers | TestRelationIntegration, fresh-service equality, source revocation, foreign-perspective rejection |

No provider, paid run, real data, deployment or scientific-validity claim is involved.
