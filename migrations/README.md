# Database schema v2

001_initial.sql and 002_runtime.sql are embedded and applied transactionally by
adapters/postgres.Migrate. Use dedicated migration authority in an isolated Dream
database/cluster. Runtime writer credentials cannot create schema/roles or update
the version ledger. Existing unsafe group roles are rejected. Empty databases install v1 then v2 atomically; existing v1 databases upgrade forward to v2. Rerunning v2 is idempotent; unknown or noncontiguous ledger versions fail. The upgrade test preserves an existing journal event. No historical production schema or
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
