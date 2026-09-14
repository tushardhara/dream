# Backend demo and measured engineering evidence

Run `make demo-check` from a fresh checkout with the pinned Go toolchain and
Docker available. The command creates one disposable PostgreSQL 18.6 cluster
(2 CPUs, 512 MiB, no persistent volume), uses non-owner runtime logins, and runs:

- Compiled `hws-demo` partial checkpoint/resume and fresh-run equivalence.
- 24 fictional adults, eight overlapping family/work/friends groups, 104 directional
  observer-owned relationship reports, two fixed seeds, and 12 simulated model months.
- Compiled recorded replay and own-actor export, including wrong-hash, unknown
  actor, foreign-note and hidden-research-label negatives.
- The independent core/graph-only second host: separate perspectives, correction,
  revocation and permitted export, without creating a world or run.
- Study journal strict reservation CAS, a blocked provider with no database lock,
  restart/quota retention, revocation and compiled study-host clock gates.

The printed `bin/demo-run-<id>/` directory retains canonical synthetic artifacts,
the demo binary and `acceptance.json`. It records source commit/dirty state,
measured elapsed time, configured database hardware, actual commands/tests and
skipped live checks. Runtime summaries record OS/architecture/GOMAXPROCS and
actual per-run elapsed time. Live API calls and incurred API cost are zero because
this command only uses deterministic reference/fake providers. Host infrastructure
and coding-subscription costs are not measured or represented as zero.

The 2/4-person configurations are smoke worlds; the five-person H/W/S/A/B configuration isolates spouse, sibling, friend and acquaintance perspectives. It has 12 directional reports, including W→A acquaintance and W↔S sibling-in-law/acquaintance; no W–B relationship is invented. The larger command is
a bounded operational smoke, not a stress test, production capacity result, or
30-day soak. The fixed seed pair is 11/23. Public scenarios contain routine,
joyful, supportive, scarce-resource and compound-event periods. Group observations
are visible only to members; typed deliveries reach only their recipient. Other
actors use their own routine observation. Raw private notes, future events and
research labels never enter another actor's choice. Shared help consumes finite
resources. The `backend-demo.v2` projection uses the shared 22-drive registry, 27-action
engine and permissioned relationship appraisal. Relationship type labels alone
do not set probabilities. Observed recipient replies can update later history;
shared time is finite and actual consumption is recorded. Legacy v1 decoding and
reproduction remain version-dispatched. The supplied HWS source registries are
mapped in ADR0017–0020; no full private-document audit is claimed.

A model month is explicitly 30 simulated days. There are two observation periods
per month, one day apart near month-end; 12 months configure a 360-day virtual
horizon. This sparse demonstration is not a daily-resolution or calendar-accurate
year. Individual choices retain the existing bounded action window. There is no
LLM call in the reference demo and no implication of human realism. The existing
four-actor model/cognitive pipeline remains separately tested by `make verify`.

The demo has a compact versioned checkpoint containing a period index and derived
world hash. Its bounded actor state is reconstructed from frozen scenario inputs
and counter-based recorded randomness. This avoids increasing the 4096-byte
runtime checkpoint limit, four-actor cognitive codec limit, 16-outcome limit or
32-receipt limit. At most 24 periods and 24 actors (576 decisions/outcomes) are supported in
the derived projection; no ledger is silently evicted or reset. Each new period's draws and resource consumption are recorded
through the existing runtime journal. Replaying the journal and recomputing the
projection must agree. Arbitrary injected events or unsupported forks fail closed;
this fixed demo is not a new general-purpose simulation policy.

For a separately configured local database, run `make build`, migrate explicitly
with the existing operator command, and supply a non-owner runtime login in
`DREAM_DATABASE_URL`. Example commands (use a new artifact path for each export):

```sh
bin/hws-demo --development --people 24 --months 12 --seed 11 \
  --namespace my-synthetic-demo --file year.json run
bin/hws-demo --file year.json --expected-sha256 <printed-artifact-hash> replay
bin/hws-demo --file year.json --expected-sha256 <printed-artifact-hash> \
  --actor person:01 export
```

The development flag permits an explicitly local/disposable cleartext database;
production connections otherwise require the existing protected connection policy.
The command never migrates, provisions credentials, opens a listener or deploys.
`--max-boundaries 1` checkpoints partial progress. Resume with the same namespace,
people/months/seed and a new output path; database operation IDs are stable.
The run's five-minute operational deadline is fixed on creation and is not reset
by resume. An expired run requires an explicit new run identity; this command does
not bypass it. A crash retains its lease's normal expiry; a clean partial exit
shortens only its own fenced lease through the existing audited renewal path.

Artifacts are canonical JSON, mode 0600. Do not pretty-print their embedded
canonical genesis bytes. Keep the printed digest independently; verification
rejects altered hashes, draws, completion flags and scientific claims. Exports
are offline projections over an already authorized synthetic artifact, not a new
network authentication mechanism. Previously delivered artifacts cannot be
recalled; current store revocation still blocks subsequent reads/exports.

Engineering acceptance, synthetic behavioral evidence and real-human validity are
separate. See `docs/study-protocol.md`, `docs/research-console-contract.md`, ADR0016
and the retained acceptance/falsifier report. The real 30-day study is NOT RUN,
cross-model/human validity NOT TESTED, and owner UI entry remains a separate decision.

For opt-in assertion sensitivity checks, run `python3 scripts/demo-mutations.py`
in an exclusive idle implementation worktree. It establishes green baselines,
removes one guard at a time (projection hash, unseen group, real-study claim,
retained quota, launch deadline), requires an actual test assertion failure and
restores the original source in `finally`. It makes no merge or provider calls;
do not run it alongside another check or writer in the same worktree.
