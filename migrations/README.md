# Database schema v1

001_initial.sql is embedded by this package and applied transactionally by
adapters/postgres.Migrate. Use dedicated migration authority in an isolated Dream
database/cluster. Runtime writer credentials cannot create schema/roles or update
the version ledger. Existing unsafe group roles are rejected. Rerunning version 1
is idempotent; unknown ledger versions fail. No historical production schema or
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
