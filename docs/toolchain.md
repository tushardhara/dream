# Pinned tools

Checked official sources on 2026-09-12:

| Tool | Pin | Use and source |
| --- | --- | --- |
| Go | 1.27.1 | module minimum, Make GOTOOLCHAIN, CI; https://go.dev/doc/devel/release and https://go.dev/dl/?mode=json |
| gofmt, go vet, go generate | Go 1.27.1 distribution | formatting, lint, eventual Go generation; no third-party linter needed at bootstrap |
| PostgreSQL | 18.6 | disposable compose image; https://www.postgresql.org/docs/release/ |
| Buf | v1.73.0 | reserved protobuf generation/lint tool; https://github.com/bufbuild/buf/releases/tag/v1.73.0 |
| actions/checkout | 11d5960a326750d5838078e36cf38b85af677262 | resolved official v4 commit |
| actions/setup-go | 40f1582b2485089dde7abd97c1529aa768e1baff | resolved official v5 commit |

Go's policy supports a major release until two newer majors exist. 1.27.1 was
listed stable at implementation. Tool updates must update these pins and run
verification. `make fmt`/`fmt-check` select gofmt from the pinned Go toolchain.
Buf is recorded but not downloaded or executed because no schema exists. When
schemas arrive, add exact plugin versions, actual deterministic generation and
clean-diff validation; the current gate blocks added inputs until that exists.
No vacuous generation/migration success is presented as validation.

CI uses pull_request (not pull_request_target), read-only contents permission,
no persisted checkout credentials, no secrets or paid calls, 15-minute timeout.
The same make verify runs on PRs and pushes to backend-integration and main.
No test database, provider or soak is started by CI. Python 3 and Make are runner
utilities; not application runtime dependencies.
