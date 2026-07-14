SHELL := /usr/bin/env bash

.PHONY: lint build test test-race test-integration test-e2e verify-plan verify-release install-local install-live-ci-runner start-local stop-local status-local test-live-curl install-public start-public stop-public status-public backup-public test-public-gateway test-public-ingress test-public-curl

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

install-live-ci-runner:
	scripts/install-live-ci-runner.sh

start-local:
	scripts/start-local.sh

stop-local:
	scripts/stop-local.sh

status-local:
	scripts/status-local.sh

test-live-curl:
	BIN_EVAL_EXTERNAL_STACK=true BIN_EVAL_LOAD_LOCAL_ENV=true BIN_EVAL_DEBUG_DIR=debug/live-curl go run ./internal/cmd/verifyplan --manifest docs/test-matrix.yml --groups live

install-public:
	scripts/install-public.sh

start-public:
	scripts/public-gateway.sh start

stop-public:
	scripts/public-gateway.sh stop

status-public:
	scripts/status-public.sh

backup-public:
	scripts/backup-public.sh

test-public-gateway:
	go run ./internal/cmd/verifyplan --manifest docs/test-matrix.yml --test TEST-110

test-public-ingress:
	go run ./internal/cmd/verifyplan --manifest docs/test-matrix.yml --test TEST-111

test-public-curl:
	BIN_EVAL_EXTERNAL_STACK=true BIN_EVAL_LOAD_LOCAL_ENV=true BIN_EVAL_LOAD_PUBLIC_ENV=true BIN_EVAL_DEBUG_DIR=debug/public-curl BIN_EVAL_ENDPOINT_CLASS=public-live go run ./internal/cmd/verifyplan --manifest docs/test-matrix.yml --groups live
