# ADR-0002: validated observer-owned records and canonical schema v1

Status: issue #3 implementation; integration requires independent review.

Core is an importable package with no world, run, branch, simulation step, latent
state, RNG or learned-at field. `examples/coreclient` compiles using core alone.
`simulator` owns those simulation contracts. `app/graph` owns its RightsReader
port; `app/hws` owns provider-neutral capability discovery. These ports have no
adapters or execution loops yet. They do not imply implemented model access.

## Minimal record semantics

Principal is an explicitly supplied logical ID. A nonparticipant reference is
identified by (observer, local ID). Identical local IDs belonging to different
observers are distinct; there is no implicit identity resolution. Subject is an
exclusive principal/reference union. References cannot be used under a different
observer. IDs are case-sensitive ASCII tokens, 1–128 bytes, limited to letters,
digits, dash, underscore, dot and colon. They are not inferred personal identities.

Evidence, version-1 generic-stream events, claims/hypotheses, relationships,
groups, memories, open loops, intents/proposals/decisions and outcomes carry common
observer/source/sensitivity/lineage/evidence/uncertainty/time/rights metadata.
Source is a principal ID. Text is nonblank valid UTF-8, at most 4096 bytes.
Hypotheses may lack evidence; claims require supporting or contradicting evidence.
The same evidence cannot both support and contradict one record. Separate claims
may contradict each other without being overwritten or collapsed into global truth.

Logical times are nonnegative int64 nanoseconds relative to a caller-defined
consistent epoch; zero is valid. Valid intervals are half-open [start,end), with
nil end meaning open-ended. Event occurred time is distinct. System recorded_at
is a nonzero representable time.Time, normalized to UTC. An actor's learned_at is
only in simulator.KnowledgeFact; knowledge never grants disclosure. No clock or
randomness is generated inside core. Subjective confidence in [0,1] is separate
from optional calibrated probability, which requires a calibration ID. That ID
is provenance, not proof the calibration research passed. Nonfinite numbers deny.

WAIT is a Decision with a reason and no selected proposal; ACT requires one and
must match its intent actor in a closed bundle. Outcomes name a decision, affected
observer and absolute later horizon (at or after valid start). Observed outcomes
require a response and supporting evidence. Unknown/censored outcomes preserve
status and must not assert a response. Multiple outcome records can represent
separate horizons; no causal learning or outcome scoring is implemented here.

## Validation and rights boundaries

Exported DTOs have Validate methods; NewID/NewPrincipal/NewReference validate
creation. Public Go structs remain mutable: raw struct literals/json.Marshal
are not trusted validation boundaries. Canonical and Decode always validate the
whole closed State bundle, including referenced principals/evidence/intents,
duplicate IDs, and cycles across lineage and evidence dependencies. State is an
in-memory validation/codec bundle, not the database, event store, snapshot or
replay system. It is limited to 10,000 total records and 16 MiB encoded input/output.
Partial database reads need a different consumer contract in their owning ticket.

Read, derive, disclose, attribute, aggregate, match, retain, export and
share-on-request are independent exact grants. Actor, recipient, purpose,
operation and resource must all match; missing/malformed contexts and revoked
rights deny. Self-use must name the actor as recipient. There are no wildcards or
implicit owner grants. DeriveRights requires derive permission on every supplied
source and returns only their intersection, with no aliases to source slices.
Closed State validation prevents wider grants or lower sensitivity through parent,
supporting or contradicting provenance, and rejects unrevoked descendants of a
revoked source. Cycles/missing provenance deny rather than being silently truncated.

These are conservative contracts, **not** the production authorization service.
The future consumer must resolve complete current lineage/rights from trusted
storage and authenticated identities. A caller-provided bundle cannot establish
permission, consent, calibration, or revocation freshness. No cross-perspective
abstraction or declassification exists. #4 and #12 must enforce trusted resolution,
immediate revocation and derivative/cache invalidation at their first consumers.
No privacy, retention, consent or evaluation threshold is changed by this ticket.

## Canonical encoding and logical hash

Schema v1 encodes JSON with fixed struct field order. Collection order is not
semantic: entity arrays sort by logical ID; references sort by observer/local ID;
group members sort by their typed identity; provenance ID lists and exact grants
sort lexically. Duplicate set entries are invalid. Nil sets become empty arrays.
Finite float64 numbers use Go encoding/json with signed zero normalized. Logical
integer times remain exact; text is exact UTF-8 with no Unicode normalization.
Golden bytes and a separately computed SHA-256 golden lock these conventions.

Canonical deep-copies before normalization. Decode accepts only the exact canonical
persisted representation, rejecting unknown/duplicate fields, whitespace, trailing
values, alternate ordering and unsupported versions. This strict persisted codec
is not an HTTP request DTO parser. Transport will define its own parsing policy.

LogicalHash starts from canonical bytes, removes recorded_at recursively, sorts
object keys, and hashes the resulting compact JSON with SHA-256. RawMessage keeps
int64 times exact. All logical IDs, times, provenance, rights and statuses remain.
Only system recorded timestamps are excluded. The hash is not authentication or
a persisted-record checksum; future schema/engine compatibility still matters.

## Deferred work

No persistence, event application, authorization adapter, lease runtime, scenario
DSL, human dynamics, model calls, learned policy, scientific validity, snapshots,
export implementation or network API. Database scope/idempotency and trusted
reference resolution arrive in #4; behavior in later owning tickets. This ticket
provides executable contracts and adversarial tests, not those future gates.
