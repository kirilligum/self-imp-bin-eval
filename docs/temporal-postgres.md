# Temporal on existing PostgreSQL

The retained Temporal 1.28.4 image supports external PostgreSQL natively. No
custom persistence adapter is required. The production configuration uses
`TEMPORAL_POSTGRES_HOST=postgres-shared`, `TEMPORAL_POSTGRES_USER=temporal` and
a privately supplied password. Compose maps these to `POSTGRES_SEEDS`,
`POSTGRES_USER`, `POSTGRES_PWD`; the native `DB=postgres12` plugin name remains
valid on the verified PG17.9 server. It does not require PostgreSQL 12.

The PostgreSQL lifecycle owner is `prls-co/postgres-shared`, retained server
`prls-control-plane-postgres-1` on `compose_default`. It provisions dedicated
`temporal` and `temporal_visibility` databases owned by the ordinary `temporal`
login. The Temporal owner retains its schema/migration responsibility.
`SKIP_DB_CREATE=true` and `SKIP_SCHEMA_SETUP=true` prevent auto-setup from
reinitializing/upgrading the restored databases. A fresh deployment must first
apply the exact server release's standard Temporal SQL schemas deliberately.

Protected production credentials remain in ignored `deploy/local/bin-eval.env`.
Do not copy that file into this public repository. The example contains only a
password placeholder. The separate test example explicitly uses its isolated
local PostgreSQL and native initialization, never the production databases.

Production reconciliation includes the existing worker-network attachment:

```sh
docker compose --env-file deploy/local/bin-eval.env \
  -f deploy/compose/docker-compose.yml \
  -f deploy/compose/compose.temporal-shared.yml config --quiet
docker compose --env-file deploy/local/bin-eval.env \
  -f deploy/compose/docker-compose.yml \
  -f deploy/compose/compose.temporal-shared.yml up -d --no-deps temporal
```

Use named services. Preserve the existing `compose` project and both RPC routes:
host `127.0.0.1:7233` and `temporal:7233` on `prls-agent-platform-control`.
The base/test manifests do not join that production worker network. Do not use
an isolated test environment file to reconcile the retained production project.

For backup/move, inspect running activities, stop only Temporal, then export both
stores through the shared PostgreSQL owner with `scripts/manage.py backup
--database temporal --output /private/path/temporal.dump` and the equivalent
`temporal_visibility` command. Preserve role/credential recovery material.
Restart the same Temporal service with the production manifest above. Never
reintroduce the retired combined public storage-backup tooling.

The 2026-10-17 PG16-to-PG17 migration retained all 11 namespace IDs/configuration
and all previously open runs. A workflow completed one activity before the move,
resumed on the same run afterward and completed with the activity counter still
one. Full table/row/schema/sequence comparisons passed in both databases.
Old PG16 logical copies are fenced recovery snapshots. Once target writes resume,
rollback must reconcile those writes, not simply switch connection settings back.
This is bounded compatibility/continuation evidence, not a capacity or off-host
recovery certification. Exact receipts are owned by
[postgres-shared](https://github.com/prls-co/postgres-shared/blob/main/evidence/services-consolidation.json).

[Native auto-setup settings](https://github.com/temporalio/docker-builds/blob/main/docker-readme/auto-setup.md)
and [existing-server guide](https://github.com/prls-co/postgres-shared/blob/main/docs/use-existing-postgres.md).

## Shared application database, 2026-10-18

Bin Eval also uses native `BIN_EVAL_DATABASE_URL` on shared PG17. Its own
`bin_eval` database/login retains all application tables and rows; the login is
ordinary rather than the old PG16 bootstrap superuser. Host clients use
`127.0.0.1:55434`. The retained `55432` listener reaches the same server for
existing CI clients; it is not a second writable database authority.

The persistent base manifest and dependency systemd unit no longer own or start
PostgreSQL. Only postgres-shared owns its lifecycle. Explicit isolated tests
include deploy/test/compose.app.yml and receive a disposable pinned PG17 server
on tmpfs. Application and Temporal fixture state remains separate from shared
production databases. Preserve the existing production Compose project and
include compose.temporal-shared.yml when operating its named Temporal service.
