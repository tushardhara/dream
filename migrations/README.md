# Database schema v6

001_initial.sql through 006_operations.sql are embedded and applied transactionally by
adapters/postgres.Migrate. Use dedicated migration authority in an isolated Dream
database/cluster. Runtime writer credentials cannot create schema/roles or update
the version ledger. Existing unsafe group roles are rejected. Empty databases install v1–v6 atomically; existing v1/v2/v3/v4/v5 databases upgrade forward to v6. Rerunning v6 is idempotent; unknown or noncontiguous ledger versions fail. The upgrade test preserves an existing journal event. No historical production schema or
destructive down migration is invented.

`make migration-check` exercises forward migration and rerun on a new PostgreSQL
18.6 container, real DB regressions, and pg_dump/restore into another database.
Tests create only synthetic data. This is mandatory in make verify/CI, not N/A.
The earlier artifact-check.py migration sentinel remains fail-closed if invoked;
it is no longer the Make validation path and cannot substitute for these tests.

Backups taken before a revocation can still contain private bytes. Such a backup
must remain offline until all later tombstones are re-applied from the retained
revocation record. This ticket does not implement a production backup/WAL erasure
policy or claim old backups are automatically sanitized. See ADR-0003.

Version 2 adds restricted runtime heads, checkpoint/event payloads, durable operation receipts and lease audit. Runtime payloads and pending command input are purged with revocation; the restore gate checks those tables too.

Version 3 adds exact session-login actor/namespace/class reader mappings and FORCE
RLS. Version 4 adds immutable per-run model budgets, fenced attempt metadata,
restricted/purgeable request and response artifacts, and canonical application
references. Model payloads are writer-only with FORCE RLS. Source revocation also
invalidates dependent runtime checkpoints; run revocation purges all attempts.
The restore gate now checks all six ledger entries and all seven forced payload/mapping policies.

Version 5 adds restricted, hash-bound snapshot bodies, immutable snapshot/branch
metadata and cross-scope source edges. Revocation traverses these edges with model
applications and same-scope lineage before purging snapshot and runtime payloads.
Child memory materializations keep immutable source references and a knowledge
cutoff. Snapshot payloads are writer-only with FORCE RLS; migration authority is
still separate from runtime credentials. Forward-v4 preservation and purge-aware
restore are mandatory gates. There is no destructive down migration.

Version 6 adds append-only model usage settlement facts, restore admission and
retained revocation references including IDs absent from an old backup. It does
not refund prior budgets or invent actual usage for older attempts. The mandatory
old-backup drill restores pre-revocation bytes, blocks adapter access during
quarantine, rejects an incomplete independent journal, rolls back a failed purge,
and proves snapshot/export/projection revocation before service admission. See
[operations](../docs/operations.md) and ADR-0014 for credential configuration,
operator freshness responsibility and backup/WAL retention limitations.
