# ADR-0003: atomic journal and revocable payload references

Status: issue #4 implementation; independent integration review required.

## Scope and ownership

app/graph owns AppendCommand/EventWriter and the bounded typed payload contract.
The PostgreSQL adapter implements that port. Core gains only standalone envelope
canonicalization, with the same finite-number/order/negative-zero normalization
as schema v1. Existing core-only imports do not pull in database dependencies.
pgx v5.10.0 is pinned; no provider callback or network model operation is accepted
inside any transaction. The three command binaries remain explicit scaffolds.

The generic journal is scoped by actor, namespace and stream, not a world or
branch. Simulator associations are a separate table with run, branch, learner and
learned_at. They do not force generic hosts to create a world. Jobs/checkpoints
and snapshot/export references exist as storage contracts; lease execution,
snapshot blobs and actual export are deferred to #6/#13/#14.

## Atomicity, ordering and retries

Append validates the event, expected version, payload schema/class and scoped
command identity. One transaction writes the event/envelope, segregated payload,
lineage, stream version, command result and outbox reference. Corrections name an
existing same-subject event; occurrence/valid times can precede ingestion.

A shared PostgreSQL advisory transaction lock serializes journal mutations,
revocation, artifact registration and projection batches. This is deliberately a
single-host foundation. It prevents sequence/commit-order inversion: a projector
cannot acknowledge a higher sequence while a lower one remains uncommitted.
Identity sequence gaps after rollback are normal and do not lose committed rows.
This is not a scalable partitioned scheduler or a substitute for #6's leases.

Idempotency keys are (actor, namespace, operation, key). Equal canonical command
digests return the stored result; a changed payload fails. Caller recorded_at is
excluded from the digest and overwritten by database time. Expected version and
all logical IDs/times/content stay in the digest; set ordering/signed zero are
canonicalized. Retrying after revocation may return an existing event ID/version,
never purged content. New revocation uses the dedicated Revoke transaction; plain
Append refuses the reserved revoke type.

Outbox entries contain only scoped event references. Project reads journal order
and atomically commits derived rows, delivered flags and its checkpoint in a
separate transaction. Repeating a batch is idempotent. Reset/rebuild does not erase
journal history or tombstones. These are at-least-once delivery/idempotent-effect
contracts, not universal exactly-once execution or provider-billing guarantees.

## Temporal queries

Events carry occurred_at, valid_from/valid_to and database-recorded_at separately.
Actor learned_at is in simulator associations. Query requires actor/namespace,
observer-owned subject, valid-at and known-as-of. The initial projection returns
the latest eligible event ID for that subject in journal sequence order; it does
not compute business claims or a global relationship truth. Later application
projections own their own semantics.

known-as-of refers to stored system recorded time, stamped inside the committing
transaction, not a promise of reconstruction of every externally observed
concurrent wall-clock snapshot. Existing historical rows are not rewritten by a
late correction. Revocation overrides historical visibility: old as-of requests
must not resurrect purged content. Logical digests exclude ingestion wall time.

## Role and provenance boundaries

Migration creates unprivileged NOLOGIN group roles with no superuser, role-create,
database-create or BYPASSRLS authority. Production credentials must be provisioned
separately; this ticket does not publish credentials or change external grants.
The harness creates disposable login identities only in its own test cluster.

- dream_writer owns runtime operations but cannot rewrite/delete event envelopes,
  tombstones or lineage, or update schema_versions. It is a trusted internal
  service identity; actor/namespace arguments must come from authenticated scope.
- dream_actor can SELECT observable payloads only, with RLS keyed to session_user.
  It has no metadata, private/research, command-result or artifact access. No
  user-settable role header/GUC provides identity. Login credentials can inherit
  the group while preserving their own session_user. Negative tests use a real
  unprivileged actor login, including attempted SET ROLE escalation.
- dream_private_reader and dream_research_reader can read only their respective
  payload table. They are privileged internal reader purposes, not actor logins.

All source event IDs must exist within actor/namespace before append, and remain
unrevoked. New event IDs are unique; the API cannot introduce lineage cycles
because sources predate the append and existing envelopes/lineage are immutable.
Normal derived appends require an explicit derive context allowed by every stored
source. Owner revocation is a separate operation and does not need derive consent.
Source metadata rights and sensitivity cannot broaden. Payload classes add a
separate restriction: private/research sources cannot move to another reader
class even if metadata says public; observable sources can become more restricted.
The regression for research→observable first failed, then passed after the fix.

This is baseline database isolation and validation, not #12's completed consent,
authenticated principal resolution, full declassification or information-boundary
service. No network-exposed auth placeholder is introduced. Direct administrative
SQL and compromised writer credentials are outside these application contracts;
admin authority can bypass database grants and must not be given to actors.

## Revocation, purge and restoration

Revoke appends its own versioned event and atomically finds transitive descendants,
inserts permanent tombstones, deletes bytes from all payload classes, removes
projection references and invalidates affected snapshot/export references. Its
own reason payload is also purged because it derives from the revoked source;
the audit envelope remains without content text. A racing artifact registration
either commits before revocation and becomes invalid, or denies after revocation.
New registrations/rebuilds/read paths check tombstones; no snapshot payload copies
exist to silently restore. Artifact references are derived bookkeeping initiated
by source events, not a snapshot/export implementation.

Immutable audit envelopes still contain IDs, perspective/provenance, times and
rights; they are not a claim of anonymous metadata or indefinite private-payload
retention. This ticket uses synthetic data only. A real-data deployment needs an
explicit approved metadata/backup/WAL retention policy. Deleting live table rows
does not physically erase old WAL, disk pages, replicas or historical backups.
The tested pg_dump/restore is of the already-purged current database and verifies
that tombstones and missing payloads survive. An older pre-revocation backup must
stay offline and have subsequent tombstones applied before serving; automatic
cross-backup purge is NOT IMPLEMENTED or claimed.

## Schema evolution and evidence

Migration ledger and event/payload schema are version 1. Migrate applies the
initial schema in a transaction, reruns without change and refuses an unknown
ledger. DecodePayload is a strict version dispatcher/identity upcaster for v1;
unknown versions/noncanonical bytes deny. There is no historical schema to
upcast yet. A future version must introduce an explicit pure upcaster and
compatibility fixtures; unsupported data never silently uses current semantics.

make verify includes mandatory disposable PostgreSQL 18.6 tests via
make migration-check, with no existing DSN accepted by the runner. It runs race
checks, tests cancellation/fault-injected rollback of append/projection/revocation,
concurrency, duplicate keys/delivery, late corrections, rebuild, role denial and
artifact/revocation races. Then it dumps/restores the purged schema into another
database. Resource limits and cleanup are in scripts/postgres-check.py. No live
provider, deployment, real-person data or long-running study is involved.
