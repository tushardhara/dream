# ADR-0007: bounded observer-owned temporal memory

Status: implementation for issue #8; exact-SHA review evidence belongs in its PR.

## Scope and ownership

`app/graph.MemoryService` works without a world, model, vector index or simulator.
It stores `memory.v1` events through the existing generic journal. The purgeable
`MemoryContent` v1 supports episodic, semantic, relationship, narrative, claim,
hypothesis, open-loop and intent records. Relationship memories are observer
recollections; #9 owns relationship/group state. A narrative is a material summary
with explicit parent sources. A claim requires supporting or contradicting evidence.
Hypotheses retain their observer and subject and never assert another person's truth.

The existing core v1 bundle/codec remains compatible. Its simple Memory/OpenLoop/
Intent values are not silently changed into a new persisted format. This application
record composes the core event's provenance, subjective confidence, optional
calibration, valid interval, rights and observer-owned subject. Learned-at audiences,
text, salience, decay and expiry are purgeable content, not immutable metadata.
There is no inferred identity matching, cross-owner search or placeholder pooling.

Synthetic emotional residue remains `simulator/dynamics` (ADR-0006), not reusable
memory state. `simulator/belief.Compare` is an offline synthetic research helper:
it compares a supplied synthetic actual measurement with an observer-attributed
hypothesis estimate. Missing estimate/evidence remains unknown, invalid/nonfinite
values and mismatched/foreign subjects fail. Its difference is not a calibrated
accuracy measure, a real-person truth, or an actor/model-view field. #12 must keep
research comparisons outside actor/model context.

## Time and selection

Every query supplies one owner/namespace, requester, exact self-read purpose,
subject, valid time, actor known-at, system recorded-as-of, and result limit.

- Occurred-at is when the observation/belief was formed, not when it was learned.
- Each audience member's learned-at must be at or after occurrence. The observer
  must be listed. A derivative cannot become known to an audience before its source.
- Valid intervals are half-open and apply to both selected records and evidence.
- The database stamps recorded-at. It is excluded from logical command payloads
  and replay hashes. A later database insertion cannot appear in an earlier as-of.
- Known-at filters learned/occurred times and expires open loops/intents at their
  exclusive expiry. Expiry is mandatory for those two kinds. No hidden wall clock
  is consulted by domain retrieval. Quiet-time decay needs no event rewrite.
- Evidence resolution composes all of these conditions recursively, including
  current rights and revocation; unknown, future and restricted sources deny.

For age = (known-at − requester learned-at) / half-life, decayed salience is
`salience * 2^(-age)` and recency is `1/(1+age)`. The score is
`round((0.75*decayedSalience + 0.25*recency)*1e12)/1e12`. Both terms are bounded.
Ties use ascending case-sensitive record ID. This is a documented deterministic
retrieval heuristic, not a probability, relevance calibration or learned model.
The rationale contains six enum strings and three numeric terms, no generated text.
Confidence is preserved, not silently decayed into a different probability.

Contradicting evidence does not delete or adjudicate either proposition. Explicit
supersession names a prior record with the same observer-owned subject. A correction
suppresses that prior record only at the correction's valid interval, after the
requester learns it and after it was recorded. Records depending on that source
then deny too. Supersession is revocation lineage, but not a requirement that the
replacement continue believing the old proposition. A revoked replacement cannot
resurrect the old proposition: with learned-at purged, suppression conservatively
uses its retained occurrence/valid/recorded metadata. A restricted replacement may
suppress an old source without revealing the replacement. This conservative omission
is intentional; this slice makes no noninterference guarantee about result absence.

Material narrative summaries are stricter: any source correction invalidates them
for *all* as-of queries, recursively, even when the original historical source is
still queryable. They must be rebuilt as new events with current sources. Current
revocation denies every kind, including historical reads. There is no asynchronous
window that permits stale cached text while waiting for recomputation.

## Storage, rights and concurrency

All memory content uses the private payload role boundary even if its metadata is
public. There is no new table or migration. `AppendMemory` holds the existing short
journal lock while reading the bounded current scope and validating source knowledge;
then generic append enforces source existence, derivation authorization, sensitivity,
grant intersection, subject-correct supersession, scoped idempotency and optimistic
stream version. Event, payload, lineage, stream, command receipt and outbox commit
atomically. No callbacks/model calls occur inside this transaction.

Every source in this first memory slice must be another memory.v1 record in the
same owner/namespace. Hosts can ingest an observation as an episodic record. This
explicit restriction avoids pretending an arbitrary generic event has learned-at
metadata. All sources must precede the new record, making cycles and forward links
impossible. Generic journal APIs are trusted low-level infrastructure and must not
be exposed as a replacement for the memory ingestion service. #14 owns credentials;
#12 owns richer policy. Owner scope and requester IDs are supplied by a trusted host,
not accepted as authentication from a network client.

Ownership alone is not a wildcard permission: exact read grants are required for
the requester on every selected record/source; explicit derive grants are required
when appending derivatives. Grant broadening through a correction remains denied
by the existing store. A known future disclosure time can be represented in an
explicit audience list; dynamic audience/consent changes are not introduced here.
A host that needs a later change must use a reviewed policy/consent event design,
not mutate an old payload or bypass source rights.

Reads use one PostgreSQL statement for the whole scoped memory journal, payloads
and current tombstones. A call started after a completed revocation sees denial;
a read already linearized on an earlier snapshot cannot be retroactively recalled.
Existing recursive purge covers memory sources, summaries and supersessions.
Immutable envelope/lineage metadata is retained under ADR-0003; backups/WAL are not
claimed instantly erased. Previously delivered caller data cannot be recalled.

## Cache, rebuild and budgets

`MemoryCache` is a bounded value holding only selection IDs and numeric rationale.
A cache lookup **always** reads the current journal. The key includes the complete
query and exact current entries (including recorded metadata and tombstones). A
change forces reselection; failure to read/validate the store fails closed. A hit
reconstructs fresh records from that snapshot. Caller mutations cannot modify its
rationale. This avoids retained cached text/rights; it saves selection computation,
not database work. Do not replace it with an offline cache without equivalent
current-policy validation.

The disposable `MemoryIndex` accepts ordered events and idempotent duplicate delivery.
Current tombstones replace live content irreversibly; replaying an older live event
cannot restore it. Rebuild uses current canonical journal entries/tombstones, not
payloads recovered from an old snapshot. Its logical hash excludes system record
times and physical sequence numbers; IDs, provenance, logical times and tombstones
remain significant. The index is not an authorization cache and is never queried
by the service without refreshing current storage.

Explicit first-slice limits: 512 total memory events per owner/namespace (including
retained tombstones), 50 results, 32 learned audience members, 32 provenance links,
128 grants, 2048 text bytes and the unchanged 4096-byte encoded payload ceiling.
One scope scan reads at most 513 rows and fails closed on overflow. There is no
eviction that silently discards history/rights. These bounds support the current
small offline fixtures, not the final long-horizon #17 run; #11/#15 must introduce
reviewed pagination/index/retention design when composing larger runs. The generic
41001 write lock still serializes short transactions (ADR-0005).

## Acceptance mapping

| Requirement | Evidence |
| --- | --- |
| Contradictions, late disclosures, valid/known/recorded time | TestTemporalContradictionDisclosureAndCorrection; real-PG TemporalAudienceAndCachedRead |
| Future/restricted evidence absent; scope and audience | TestEvidencePermissionsCacheAndImmediateRevocation, TestMemoryAppendKnowledgeAndSourceBoundary; real-PG AppendNegativesAreAtomic |
| Immediate revocation/cache denial and summary invalidation | TestMaterialInvalidationAndExpiry; real-PG CorrectionInvalidatesMaterialCache and ImmediatePurgeCacheAndRebuild |
| No cyclic evidence or cross-observer placeholders | TestMemoryReplayNeverResurrectsAndLogicalHash, TestMemoryValidationBudgetsAndCanonical |
| Deterministic bounded decay/ranking/rationale | TestMemoryDecayAndStableRanking analytic score golden; budget/canonical negatives |
| Incremental/rebuilt logical state and no resurrection | TestMemoryReplayNeverResurrectsAndLogicalHash; real-PG replay after purge and generic projection rebuild |
| Atomic journal, retries and trusted recorded time | real-PG ScopedIdempotencyAndTrustedRecordedAt (eight duplicate writers), injected outbox fault rollback |
| Synthetic beliefs remain observer-attributed | TestObserverAttributedComparison; dynamics tests from #7 retain residue behavior |

All fixtures are synthetic and offline; no LLM or provider credentials are required.
Complete original PRD traceability remains UNVERIFIED as recorded in requirements.md.
