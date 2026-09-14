# ADR-0014: bounded operations, audit and restore admission

Status: proposed for #15; integration requires independent exact-SHA review.

Operational recovery extends the existing journal, model reservations, policy
receipts and replay engine. It does not select a simulator policy, deploy a host,
configure a paid provider or establish scientific validity.

Schema 6 adds append-only model settlement facts linked to the attempt's versioned
event. Only SELECT/INSERT are granted to the writer. Settlement and the request
head change share one transaction: audit failure cannot leave a usable result.
Reserved token/spend ceilings never decrease. Actual successful input/output token
counts are recorded separately; failed/uncertain usage is unknown, not zero.
These are configured conservative microcurrency ceilings, not a claim about a
provider invoice. Pre-v6/fork-inherited reservations lack per-attempt settlement
facts; known totals are lower bounds, never evidence that the balance is free.

ReconcileModels processes at most 128 expired/stopped requests per scope/cycle.
It appends an operational event and advances fences atomically, retains all
reservations, and labels uncertainty distinctly. An explicit retry through the
ordinary gateway is required; reconciliation starts no provider. Current requests
are never stolen. Canonical operations, source revocation, cancellation and stale
lease checks remain authoritative. Project still commits derived effects,
outbox delivery and its checkpoint together. The worker uses one stable projector
name and bounded batches; it does not erase/recreate the checkpoint on restart.

The gateway has a process-wide ceiling of eight executions, shared across scopes
and gateway instances, in addition to durable per-run concurrency and budget
limits. Busy admission consumes no new reservation. A provider ignoring context
retains its slot until its call returns. BoundedProvider permits an explicitly
shared smaller route ceiling and redacted provider telemetry. Multiple processes
still need explicit host sizing; the eight-slot limit is per process, not a
cluster-wide promise. The standalone worker performs maintenance only.

Transport drain rejects new admissions and allows admitted handlers to finish with
normal final authorization checks. A caller deadline forces termination; there
is also a hard 30-second drain cap. /healthz and /readyz expose only empty status
responses, never diagnostics; readiness probes have a single concurrent slot and
2-second DB deadline. Production DB configuration rejects plaintext fallback and
requires TLS certificate verification (or a local Unix socket). TLS server
requirements from #14 remain. Database outage is not a behavioral WAIT.

Telemetry accepts only four operation kinds, seven outcomes and a duration capped
at one minute. Its 28 local counter cells are atomic; OTel counter/histogram/span
attributes are two closed strings. No error text, identifier, URL, prompt, latent
state, request baggage, inherited parent or arbitrary attribute can be supplied.
The API executable records local counters and emits the bounded matrix at shutdown.
An embedding host may explicitly supply the OTel recorder/providers; there is no
automatically discovered exporter or network telemetry destination. SDK export is
tested with in-memory providers. Counters reset on process restart; durable audit
and budget journals, not metrics, provide accounting and recovery authority.

## Audit window and reproducibility

ResearchView accepts an optional exact-scope audit snapshot/revision or a current
model-usage query. Audit export uses existing SubmitExport/DownloadExport with
kind=audit, explicit research Export rights, no time filtering, and the same
source/credential revalidation on every page. Actors and external clients cannot
obtain research audit content. A partial export is not completion evidence; a
revocation makes the remaining cursor unusable. Delivered bytes cannot be recalled.

An audit.v1 packet contains the bounded replay window, exact model intents/artifacts,
state/model hashes, engine/RNG versions and model-policy bindings. Recorded
cognitive state includes candidates/probabilities, selected output, appraisal
references, immediate outcomes and later outcomes within that window. It does not
invent outcomes preceding/following the requested window. Policy revalidation now
records the approved context revision, selected-context digest and logical clock
in its restricted audit payload. Successful audit verification requires matching
policy context evidence for every included model, not only a matching role name.
Earlier records without that linkage remain readable through their existing
interfaces, but cannot claim this stronger complete audit packet verification.
Hashes and linkage identifiers are retained metadata; they are not anonymization.

Limits: 128 replay frames, 256 model links, 1024 policy records, 32 MiB packet.
Exceeding them fails explicitly rather than truncating evidence. hws audit verify
requires an independently retained expected hash and recomputes recorded replay,
model/context linkage and content hashes. Fresh generation equivalence is false.
An administrator able to rewrite both logs and trusted checkpoints can forge a
consistent history. No append-only grants or hash chain defeat that administrator.
No independent timestamp/signing service or external immutable log is claimed.

## Recovery and retention

A restored old backup must first be isolated from all services and readers. An
operator separately retains the newest revocation journal and its expected digest;
neither freshness nor completeness can be inferred from the old backup. Explicit
quarantine records an event and closes adapter admission. Applying the exact
journal atomically retains every revoked reference, seeds tombstones for existing
records, traverses the existing cross-scope lineage, purges payloads/operations,
invalidates snapshots and records completion before reopening admission. A purge
failure leaves quarantine closed and rolls back the success record. References
absent in the backup are retained too, preventing later recreation of their IDs.

This is an application/adapter admission gate, not protection from direct SQL by a
privileged login. Drain/isolation remains mandatory before a restore. Migration
owners can rewrite the gate. Credential/view revocations live in trusted host
configuration and must independently be restored from the newest configuration;
never recover stale authorization files along with an old data backup.

Old backup files/WAL, filesystem snapshots, exported copies and administrator
copies may contain previously private bytes. This program neither deletes such
copies automatically nor promises absolute erasure. Keep them access-restricted,
set an owner-approved retention/rotation schedule, and reapply revocations before
serving recovered state. See the operational runbook for the concrete sequence.

## Requirement-to-test mapping

| Criterion | Evidence |
| --- | --- |
| Reservations + known usage across restarts/outages | UsageLedgerRetainsReservationsAcrossRestartAndFailure; previous admission and model-budget tests |
| Append-only audit and fail-closed settlement/recovery | real writer UPDATE/DELETE denial; injected usage/recovery audit failures roll back |
| Shared physical provider cap | ProcessProviderCeilingAcrossRunScopesAndCancellation; TestProviderSlotsSurviveIgnoredCancellation |
| Concurrent state/drain and DB readiness | TestConcurrentDrainCompletesAdmittedRequests, TestReadinessOutageAndBoundedProbe, TestDrainDeadlineForcesStuckRequest |
| Typed redacted metrics/OTel and WAIT separation | TestBoundedConcurrentMetricsAndRedactedSpans; CognitiveIntegration wait/outage cases |
| Old-backup restore + retained revocations | TestOldBackupRestoreRequiresNewRevocations: actual pre-revocation pg_dump/restore, quarantine, incomplete journal, purge failure, absent IDs, snapshot/projection negatives |
| Authorized audit, model/policy linkage, mutation, partial export | TestAuthenticatedServerPostgresOperationsAndExport; missing model/policy, changed context and broken chain fail even attempted resealing |
| Process loss, cancellation, stale writer/outbox | inherited KillRestartMatchesCleanTrajectory, lease expiry/fencing and rollback tests; compiled maintenance worker run against non-owner PG |
| Protected DB transport | TestProtectedDatabaseConnectionCannotFallBackToCleartext |
| Nonroot images | scripts/container-check.py builds pinned static binaries, checks user 65532, runs four networkless/read-only help checks |

All fixtures are synthetic, providers fake/recorded and PG clusters disposable.
Tests name their workload/resource caps; no live latency, throughput, long-horizon
capacity or 30-day operation claim is made. Full HWS registry/source gaps and
scientific gates are unchanged. The separate requested agent supervisor remains an
environment preflight issue, not something the product worker implements.
