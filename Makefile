SHELL := /bin/sh
export GOTOOLCHAIN := go1.27.1
.PHONY: generate generated-check migration-check fmt fmt-check lint test test-race build help-check verify live-provider soak

generate:
	@python3 scripts/artifact-check.py generated

generated-check: generate
migration-check:
	@python3 scripts/postgres-check.py
fmt:
	@"$$(go env GOROOT)/bin/gofmt" -w $$(git ls-files --cached --others --exclude-standard '*.go')
fmt-check:
	@files=$$("$$(go env GOROOT)/bin/gofmt" -l $$(git ls-files --cached --others --exclude-standard '*.go')); test -z "$$files" || { echo "Unformatted files: $$files"; exit 1; }
lint:
	go vet ./...
test:
	python3 -m unittest discover -s scripts -p 'test_*.py'
	go test -count=1 ./...
test-race:
	go test -race -count=1 ./...
build:
	@mkdir -p bin
	go build -trimpath -o bin/hws ./cmd/hws
	go build -trimpath -o bin/hws-api ./cmd/hws-api
	go build -trimpath -o bin/hws-worker ./cmd/hws-worker
help-check: build
	./bin/hws --help
	./bin/hws-api --help
	./bin/hws-worker --help
verify: fmt-check lint test test-race generated-check migration-check help-check
live-provider soak:
	@echo "BLOCKED: requires explicit owner authorization, provider/budget/infrastructure configuration and a later implemented runner."; exit 1
