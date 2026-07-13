#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

COMPOSE=(docker compose --env-file deploy/compose/.env.example -f deploy/compose/docker-compose.yml)
"${COMPOSE[@]}" config >/dev/null
"${COMPOSE[@]}" up -d postgres temporal garage

exec go test -tags integration "$@" -count=1 -timeout 10m
