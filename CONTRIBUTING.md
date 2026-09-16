# Contributing

Read AGENTS.md and the live epic/ticket. Install Git, Make, Python 3, Go 1.27.1
(a C compiler for the race detector, and Docker for mandatory Postgres checks). Run `make verify` from the repo root. On the shared host, use `scripts/disk-guard.py` with the complete retained-role account list as described in [test resource safety](docs/test-resource-safety.md).
The Go command downloads the pinned toolchain if necessary. Default verification
uses disposable synthetic database credentials, no live provider or personal data.

`make fmt` formats; `make fmt-check` rejects formatting drift; `make lint` uses
Go's pinned vet analyzer; `make test` and `make test-race` run uncached tests;
`make build` builds the fourteen commands listed below; `make help-check` exercises each help path.
`make generate` checks pinned protobuf, gRPC, HTTP gateway and OpenAPI regeneration. `make migration-check` starts its own bounded,
disposable PostgreSQL 18.6 container and runs the real integration/race and
pg_dump/restore checks. It fails if Docker or Postgres is unavailable. It accepts
no existing database configuration and cleans up its labelled container.
See docs/toolchain.md and docs/adr/0003-event-storage.md.

## Commands

`cmd/` holds fourteen programs. `make build` compiles all fourteen into `bin/`
and `make help-check` runs each `--help`; the last column names the other gates
that exercise the binary.

| Command | Purpose | Gates |
| --- | --- | --- |
| `hws` | offline scenario validation and audit-packet verification; opens no listener | help-check, container-check |
| `hws-api` | authenticated management/replay host; execution is injected by an embedding host | help-check, container-check |
| `hws-worker` | bounded single-host model/outbox reconciliation | help-check, container-check |
| `hws-admin` | explicit operator migrations and quarantined restore | help-check, container-check |
| `hws-eval` | independent offline evaluator; `-uplift` prints the matched-arm summary | help-check, evaluation-check, uplift-check |
| `hws-generate` | isolated label-blind stdin/stdout generator worker | help-check, evaluation-check, demo-check |
| `hws-demo` | recorded/fake 24-person synthetic backend demo: run, replay, export | help-check, demo-check |
| `hws-assistance` | bounded two-person helper arms (none, explicit preference, single and multi perspective) | help-check |
| `hws-listening` | bounded synthetic goal-aware listening fixture | help-check, listening-check |
| `hws-repair` | two authored multi-period repair sequences with later observations | help-check, repair-check |
| `hws-group` | five- or 24-person group history and unequal-burden fixture | help-check, group-check |
| `hws-ordinary` | ordinary-enjoyment arms (none, generic, permitted context) over four families | help-check, ordinary-check |
| `response-report` | regenerates `docs/evaluation/recipient-response-v1.json` | help-check, report-check |
| `temporal-report` | regenerates `docs/evaluation/temporal-v1.json` | help-check, report-check |

Use each `--help` path for supported operations. The API CLI composes authenticated
management operations; execution requires an embedding host with a configured
handler. The demo and evaluator use bounded synthetic inputs. See
[operations](docs/operations.md), [demo](docs/backend-demo.md) and
[evaluation](docs/evaluation.md) for executable paths and limits.
Local PostgreSQL is optional: `docker compose up -d --wait`; stop with
`docker compose down`. This is disposable test data in tmpfs, loopback-only port
54329, with a public local-only password. The optional compose instance is separate from the automatically provisioned verification instance.
`make live-provider` and `make soak` intentionally fail until a later implemented
runner and explicit provider/budget/infrastructure approval are available.

## Branches and PRs

Branch from current `origin/main` (or from the integration branch a live epic
names; epic #76 uses `post-merge-integration`). Open one PR per ticket into that
branch. Use focused commits (e.g. `build: bootstrap backend verification (#2)`).
Include tests and acceptance evidence, limitations and exact base/head SHAs in
the PR template. Review, integration and the owner-only merge into `main` are
defined in [docs/agent-workflow.md](docs/agent-workflow.md). Do not close tickets
just because a non-main merge ran.
