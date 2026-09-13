# Contributing

Read AGENTS.md and the live epic/ticket. Install Git, Make, Python 3, Go 1.27.1
(a C compiler for the race detector, and Docker for mandatory Postgres checks). Run `make verify` from the repo root.
The Go command downloads the pinned toolchain if necessary. Default verification
uses disposable synthetic database credentials, no live provider or personal data.

`make fmt` formats; `make fmt-check` rejects formatting drift; `make lint` uses
Go's pinned vet analyzer; `make test` and `make test-race` run uncached tests;
`make build` builds all three commands; `make help-check` exercises each help path.
`make generate` reports N/A while no schema inputs exist, failing closed if inputs
arrive before real validation. `make migration-check` starts its own bounded,
disposable PostgreSQL 18.6 container and runs the real integration/race and
pg_dump/restore checks. It fails if Docker or Postgres is unavailable. It accepts
no existing database configuration and cleans up its labelled container.
See docs/toolchain.md and docs/adr/0003-event-storage.md.

Commands exit 0 for --help and 2 otherwise. They are scaffolds, not servers.
Local PostgreSQL is optional: `docker compose up -d --wait`; stop with
`docker compose down`. This is disposable test data in tmpfs, loopback-only port
54329, with a public local-only password. The optional compose instance is separate from the automatically provisioned verification instance.
`make live-provider` and `make soak` intentionally fail until a later implemented
runner and explicit provider/budget/infrastructure approval are available.

Create a dedicated branch from current origin/backend-integration. Use focused
commits (e.g. `build: bootstrap backend verification (#2)`). Include tests and
acceptance evidence, limitations and exact SHAs in the PR template. Claude's
independent review/integration gates and owner-only final main merge are defined
in docs/agent-workflow.md. Do not close tickets just because a non-main merge ran.
