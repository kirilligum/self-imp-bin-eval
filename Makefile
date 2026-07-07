SHELL := /usr/bin/env bash

.PHONY: lint build test test-integration test-e2e install-local start-local stop-local status-local test-live-curl

lint:
	@files=$$(find . -name '*.go' -not -path './.git/*'); \
	if [ -n "$$files" ]; then \
		unformatted=$$(gofmt -l $$files); \
		if [ -n "$$unformatted" ]; then echo "$$unformatted"; exit 1; fi; \
	fi
	go vet ./...

build:
	go build ./...

test:
	go test ./... -count=1

test-integration:
	docker compose --env-file deploy/compose/.env.example -f deploy/compose/docker-compose.yml config >/dev/null
	docker compose --env-file deploy/compose/.env.example -f deploy/compose/docker-compose.yml up -d postgres temporal garage
	go test -tags integration ./internal/db ./internal/artifacts -count=1 -timeout 10m

test-e2e:
	scripts/smoke_curl.sh

install-local:
	scripts/install-local-systemd.sh

start-local:
	scripts/start-local.sh

stop-local:
	scripts/stop-local.sh

status-local:
	scripts/status-local.sh

test-live-curl:
	scripts/live_curl_example.sh
