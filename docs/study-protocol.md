# Separately gated 30-real-day learning-study protocol

Status: executable recorded/fake protocol and synthetic-format checks; **no real
30-day study has been run**. A planned month of iteration is not a promised
completion date. The study's real UTC days are distinct from the demo's simulated
360-day horizon. Stage A/B mechanism and small-fixture checks precede later research
scale; 200 scenario families and 10,000+ cases are later targets, not delivered
study evidence.

The evaluator owns the study journal. `StudyPlan` freezes the scope/study ID,
explicit UTC start, 30-day deadline, baseline digest, candidate version/variant,
seed, two provider bindings with artifact digests, and daily/total prediction
quotas. The current controller supports recorded/fake ports only. The CLI binds
explicit networkless fake images; a separate configured host is needed for other
provider implementations. Merely naming two fake ports is NOT evidence of two
independent frontier models. Cross-model and human validity remain NOT TESTED.
There is no live-provider activation, deployment or scheduler in this command.

`examples/study/30-real-day.template.json` intentionally has unresolved owner
configuration. Copy it to an owner-only file, choose a real UTC start/stable scope,
pin a verified baseline artifact and local provider image digests, and select
quotas. Validation rejects unresolved placeholders, changed versions, duplicated
provider IDs and live-provider claims. The template cannot accidentally start a
study. No study credential or owner private key is included in the repository.

Daily procedure for a trusted operator/host:

1. Register the frozen plan once. Preserve its plan hash and explicit study scope.
2. Obtain the current application checkpoint and already learned, permitted daily
   observations. Existing state-update services own bounded actor state; this
   evaluator never updates model weights. For this program, imports are synthetic
   `evaluation-dataset.v1` only, with consent/provenance/time checks from #16.
3. Before generation, append one strict compare-and-swap daily reservation. Two
   provider ports receive separately cloned, approved requests with the frozen
   seed/variant. They receive no baseline labels, judge outputs or each other's
   predictions. The immutable generation image cannot persist online changes.
4. Append a versioned daily report with input/result hashes, reserved/finished
   prediction counts, operational success/uncertainty and separate behavioral,
   cross-model and human status. No unknown usage is refunded or scored as success.
5. Preserve the journal sequence/hash checkpoint. Reconstruct this exact study on
   restart. Re-registering the same ID with changed start/configuration fails;
   the fixed deadline never extends. There is no ambiguous last-session lookup.
6. Evaluate versioned candidates with #16's independent frozen holdout process.
   Promotion needs separately signed owner preregistration and exact-report
   approval. A daily report is not promotion authority and cannot change defaults.

One explicit `hws-eval` invocation registers, reads or advances at most one day:

```sh
bin/hws-eval --development --study-plan private-plan.json --study-action register
bin/hws-eval --development --study-plan private-plan.json --study-action status
bin/hws-eval --development --study-plan private-plan.json --study-action day \
  --dataset private-synthetic-observations.json
```

Use the existing non-owner runtime database role through `DREAM_DATABASE_URL`.
The development flag is only for an explicit local/disposable connection. Plan and
input files must be owned, owner-only regular files; symlinks and oversized JSON
are rejected. The registered plan is checked before any daily/abandon mutation.
Reports and plan metadata use the existing PostgreSQL versioned journal, with no
new schema or generation-facing label store. Revoking the baseline registration
purges its derivatives and prevents checkpoint resurrection. Dataset labels remain
in evaluator-only files and are never persisted in this protocol metadata journal.

A day may launch only in its own real 24-hour window, in order, with no duplicate
reservation. Missed windows fail; catch-up calls cannot fabricate elapsed observation
days. Provider callbacks run outside database transactions. Reservations count
against quotas before the first call and remain charged after errors/crashes.
A pending reservation blocks regeneration. After the bounded five-minute worker
window, an explicit `--study-action abandon-expired` records inconclusive evidence,
without retrying or refunding it. A stale completion loses the journal CAS.
Three consecutive inconclusive days stop further missions. There is no automatic
purchase, unlimited retry, configuration rewrite or capacity bypass.

The daily schema permits at most 32 input requests, two provider batches, 64
reserved predictions/day and 30 days. Per-study quotas are not a claim of VPC-wide
concurrency control. An operator scheduler must wait until the checkpoint's
`next_due`, honor explicit worker IDs/locks and stop new work at `fixed_deadline`.
No scheduler is installed or claimed by this ticket. A pending final report may be
checkpointed after the launch window; it does not authorize a new mission.

Before any live/paid/deployed study, the owner must separately approve and configure:

- Infrastructure, identity, persistent execution, stop controls and operational ownership.
- Provider identities/credentials and separately tracked per-provider/runtime budgets;
  coding subscriptions do not fund runtime APIs. The current fake CLI cannot activate live ports.
- Authorized source collection, consent, retention/revocation, validated daily observation
  pipelines and independent outcomes. Real human ingestion is unavailable in this program.
- Frozen baseline/holdouts, observation windows, missingness policy, adequate sample size,
  source-complete HWS falsifiers, candidate criteria and explicit promotion authority.

The fake-clock reducer test exercises 30 protocol days quickly; that is a unit
test, not 30 days of real observations. PostgreSQL tests exercise one actual fake
protocol day, concurrent reservation rejection, restart, revocation and early CLI
day rejection. All reports keep behavioral evidence inconclusive and cross-model/
human validity NOT TESTED. Unavailable live/scientific gates are not PASS.
