# Repository Guidelines

## Project Structure & Module Organization

This repository is a Go service workspace for `bin-eval`. Keep the top level focused on project metadata, canonical commands, and runtime entry points. The canonical layout is:

- `cmd/bin-eval-api/` for the HTTP API binary.
- `cmd/bin-eval-worker/` for the Temporal worker binary.
- `internal/` for application packages that are not public API.
- `migrations/` for Postgres schema migrations.
- `deploy/compose/` for the local Postgres, Temporal, and Garage dependency stack.
- `scripts/` for operator and smoke-test scripts.
- `fixtures/` for committed test and smoke inputs.
- `docs/` for design notes, setup instructions, and evaluation reports.
- `plans/` for implementation plans.

Avoid committing generated logs such as `firebase-debug.log`; add ignore rules when project tooling is introduced.

## Build, Test, and Development Commands

Use one canonical top-level command per task:

- `make lint`: run formatting validation and `go vet`.
- `make build`: compile all Go packages and binaries.
- `make test`: run unit tests.
- `make test-integration`: validate the Compose stack and run integration tests.
- `make test-e2e`: run the curl-based smoke path.

Prefer one canonical command per task so local development and CI stay aligned.

## Coding Style & Naming Conventions

Use `gofmt` and `go vet` through `make lint`. Keep files ASCII where practical, use clear package and file names, and avoid broad utility packages. Prefer descriptive names such as `ScoreChecklist`, `BuildActiveChecklist`, or `evaluation_result_test.go` over abbreviations. Keep configuration in explicit files at the repository root or under `deploy/compose/`.

## Testing Guidelines

Add tests with the first behavior change. Name tests by behavior, not implementation detail, for example `score_checklist_test.go` or `validates_binary_judgments_test.go`. Keep fixtures small and checked in under `fixtures/` when they are needed for repeatable evaluations. Unit tests should run with `make test`, integration tests with `make test-integration`, and the smoke path with `make test-e2e`.

## Commit & Pull Request Guidelines

This repository has no commit history yet, so no existing convention is established. Use concise, imperative commit messages, optionally following Conventional Commits, such as `feat: add binary evaluation runner` or `test: cover invalid input handling`.

Pull requests should include a short summary, test results, linked issue or task when available, and screenshots or logs only when they clarify user-visible or CLI behavior.

## Agent-Specific Instructions

Before editing, inspect the current tree and preserve user changes. Keep generated artifacts out of commits unless they are intentional fixtures or documented outputs.
