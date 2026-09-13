# Single-host operations runbook

This is a local deployment/recovery procedure for an explicitly configured host.
Writing or testing these instructions does not deploy the program. No cloud,
Kubernetes, registry publication, paid provider or real dataset is configured.
Use synthetic fixtures until the owner authorizes a real workload.

Build and verify with `env -u DREAM_TEST_DSN make verify`. The PG check creates a
uniquely named PostgreSQL 18.6 container capped at 2 CPUs/512 MiB/128 PIDs with a
tmpfs data directory. It removes only that container. Reusing a database or cluster
invalidates the test setup. Never clean up containers by listing/removing all host
containers. The container check uses 1 CPU/128 MiB/32 PIDs, no network, read-only
root and no capabilities; it builds four static linux/amd64 executables with Go
1.27.1. Other architectures have not been verified.

## Configuration and startup

Provide DREAM_DATABASE_URL through a protected service environment/secret file;
never put a credential in a command argument, image, repository or diagnostic log.
The API/worker require a non-owner login with the existing dream_writer role and
no membership in owner/superuser/bypass roles. The operator creates that login
separately. Do not use the migration login at runtime. TCP production DSNs use
sslmode=verify-full and a trusted sslrootcert; Unix sockets use filesystem access.
The scratch image has no default CA bundle: mount the approved CA file explicitly.
Development cleartext is explicit; API listeners must be numeric loopback.

Run `bin/hws-admin migrate` under dedicated migration authority only, with the
services stopped. Empty/v1–v5 databases upgrade transactionally to schema 6; rerun
is idempotent and unknown/noncontiguous versions deny. No destructive down migration
exists. Back up and test recovery before a real upgrade; migrations do not create
application credentials or choose their grants.

Supply the API's trusted version-1 JSON configuration described in ADR-0013:
management-only mode, explicit listener addresses, certificate/key paths, credential
digests and current view grants. Make it readable only by the intended runtime
identity. Then run `bin/hws-api --config /secure/dream-api.json`. Management mode
never selects a fake simulation handler. Execution hosts explicitly compose the
cognitive service/gateway and retain the same auth, audit, budget and fencing gates.

The maintenance worker takes this shape (scope must identify an existing run):

```json
{"version":1,"scopes":[{"actor":"operator","namespace":"synthetic","world":"world","branch":"branch","run":"run"}],"development":false,"interval_seconds":5,"max_cycles":120}
```

Run `bin/hws-worker --config /secure/dream-worker.json` using the runtime login.
It reconciles expired/stopped model attempts and projects at most 100 outbox events
per cycle. Each scope processes at most 128 attempts; more wait for later cycles.
It performs at most 3600 configured cycles, caps one cycle at 30 seconds and one
invocation at 24 hours. Three failed cycles stop the process; retries back off.
These are product-worker bounds, not an agent supervisor or an automatically
extended unattended engineering session. Resume explicitly with the same scopes
and projector after inspecting the blocker. Do not reset budget rows, event keys,
leases or checkpoints to get a fresh allowance.

`make container-build` creates the local scratch image dream-local:operations.
The image runs as 65532:65532 and has no shell, package manager, embedded credential
or writable service directory. Select /hws-api, /hws-worker or /hws-admin as the
entrypoint for the corresponding reviewed configuration. Mount required config,
TLS key/certificate and CA read-only with access restricted to that UID. Use a
read-only root, dropped capabilities and no-new-privileges. Explicitly configure
resource limits/network access for the approved host; no example starts a live
service automatically. The help-only default creates no listener.

## Health, drain and restart

GET /healthz returns 204 while the HTTP process is serving. GET /readyz returns 204
only when the actual DB role/schema/restore gate is ready; otherwise 503. Both
responses have empty bodies. One concurrent readiness probe is allowed. A DB
outage must not be interpreted as synthetic behavioral WAIT.

SIGINT/SIGTERM stops new API admission, drains existing requests, and preserves
ordinary authorization checks. The command allows 25 seconds; the library caps a
caller with no deadline at 30 seconds. Expiry forces connection termination and
reports non-success. A client must query/retry the SAME operation/export key after
an uncertain response; do not assume the transaction failed because the socket
closed. Wait for existing leases/attempts to expire, then reconcile. Providers
ignoring context retain physical concurrency slots. Unknown billing retains the
full conservative reservation. A stopped/cancelled run is never silently resumed.

The API logs a fixed operational counter matrix at shutdown. Order is operations
(api, provider, cognitive, recovery) × outcomes (success, behavioral_wait,
provider_outage, safety_denied, budget_denied, uncertain, failure). Counters are
per-process telemetry, not accounting. Embedders can inject the OTel recorder with
explicit local/approved providers; no exporter endpoint is auto-discovered.

## Backup and restore

1. Retain access-restricted backups plus the current authorization configuration.
   A backup predating revocation contains old bytes. Keep a separately retained,
   newest revocation journal and its digest outside that backup:
   `bin/hws-admin --file /secure/revocations-new.json export-revocations`.
   The output file is created exclusively with mode 0600; no overwrite occurs.
   The digest is printed, not the journal. Retained IDs are sensitive metadata.
2. Stop/drain all services/readers before dumping/restoring. Use PostgreSQL's
   pg_dump/psql with protected libpq environment/service configuration. Preserve
   the Dream schema, grants, roles and dependencies. Restore into an isolated
   database; do not attach the API/worker yet. Never restore over live data as an
   unattended test. Use a dedicated disposable database for rehearsals.
3. Migrate the isolated restored schema with dedicated authority if necessary.
   Fetch the newest journal digest and current credential/view grants from their
   independent retained source. An old backup cannot attest their freshness.
4. Run `bin/hws-admin --expected-sha256 <trusted-digest> quarantine-restore` against
   the isolated restored database. Adapter reads/writes and RuntimeReady now deny.
5. Run `bin/hws-admin --file /secure/revocations-new.json apply-revocations`.
   A wrong/incomplete journal or any purge failure leaves quarantine closed.
   Success purges the existing lineage closure and retains absent revoked IDs.
   It reopens the adapter gate, but does not start a service or restore credentials.
6. Verify known durable events, tombstones, forced RLS, intended runtime role and
   rejection of revoked snapshot/replay/export data. Install the NEWEST host
   credential/view configuration, then perform explicit maintenance reconciliation.
   Only after these checks should the operator reconnect approved service traffic.

Direct privileged SQL can bypass/rewrite this application gate. This procedure is
not protection from an administrator or a guarantee of backup/WAL erasure. Restrict
backup access and agree retention/rotation/deletion with the owner. Do not claim
that already delivered exports or retained administrator copies have disappeared.

## Audit and reproducibility

ResearchView's optional audit_source/audit_through_revision returns audit.v1 for an
exact-scope snapshot window. model_usage=true instead returns current reservation
and known/unknown usage counts. Existing research credentials and current view
permission are mandatory. For paged audit export submit kind=audit with the source
and ending revision, zero time filters and no source_ids; explicit Export rights
are required. Save every page, verify its checksum, then verify the whole manifest
hash/length. After revocation, discard an incomplete export; a cursor is not authority.

Use `bin/hws audit verify /secure/audit.json --sha256 <trusted-packet-hash>` offline.
The expected packet hash must be retained independently. The verifier checks the
recorded trajectory and linked typed model/policy context, not fresh stochastic
reproduction or real-human validity. It reports no private packet content. Source
snapshots and replay frames already carry the canonical engine/RNG/decision records;
missing or truncated evidence is a failure, never an inferred pass.
