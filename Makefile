SHELL := /usr/bin/env bash

.PHONY: lint build test test-race test-integration test-e2e verify-plan verify-release install-local start-local stop-local status-local test-live-curl

lint:
	go run ./internal/cmd/verifyplan --manifest docs/test-matrix.yml --groups lint

build:
	go run ./internal/cmd/verifyplan --manifest docs/test-matrix.yml --groups build

test:
	go run ./internal/cmd/verifyplan --manifest docs/test-matrix.yml --groups unit

test-race:
	go run ./internal/cmd/verifyplan --manifest docs/test-matrix.yml --groups race

test-integration:
	go run ./internal/cmd/verifyplan --manifest docs/test-matrix.yml --groups integration

test-e2e:
	go run ./internal/cmd/verifyplan --manifest docs/test-matrix.yml --groups e2e

verify-plan:
	go run ./internal/cmd/verifyplan --manifest docs/test-matrix.yml $(if $(TEST),--test $(TEST),)

verify-release:
	go run ./internal/cmd/verifyplan --manifest docs/test-matrix.yml --groups lint,build,unit,race,integration,e2e,live,plan

install-local:
	scripts/install-local-systemd.sh

start-local:
	scripts/start-local.sh

stop-local:
	scripts/stop-local.sh

status-local:
	scripts/status-local.sh

test-live-curl:
	go run ./internal/cmd/verifyplan --manifest docs/test-matrix.yml --groups live
