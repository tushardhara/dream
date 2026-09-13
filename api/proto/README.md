Versioned transport contracts live in dream/v1/service.proto. Generate with
`python3 scripts/generate-api.py --write`; `make verify` regenerates independently
and rejects drift. The script pins protoc36.1 by upstream SHA-256 and Go plugins
by version. Its compiler archive currently targets Linux x86_64. No remote plugin
service or credentials are used. See docs/adr/0013-authenticated-transport.md for
current implementation status and remaining #14 acceptance gates.
