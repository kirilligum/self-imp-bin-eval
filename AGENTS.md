# Repository Guidelines

## Project Structure & Module Organization

This repository is currently a minimal Git workspace with no committed source tree yet. Keep the top level focused on project metadata and entry points. When implementation code is added, prefer a small, predictable layout:

- `src/` for application or library code.
- `tests/` for integration tests and cross-module behavior.
- `src/**/__tests__/` or adjacent `*.test.*` files for unit tests that belong near a module.
- `docs/` for design notes, setup instructions, and evaluation reports.
- `assets/` or `fixtures/` for static inputs used by tests.

Avoid committing generated logs such as `firebase-debug.log`; add ignore rules when project tooling is introduced.

## Build, Test, and Development Commands

No build or test commands are configured yet. Add commands as soon as the first runtime or package manager is chosen, and document them here. Recommended conventions:

- `npm test`, `pnpm test`, or `make test`: run the full test suite.
- `npm run lint` or `make lint`: run static checks and formatting validation.
- `npm run build` or `make build`: produce distributable artifacts.
- `npm run dev` or `make dev`: start a local development server or watcher.

Prefer one canonical command per task so local development and CI stay aligned.

## Coding Style & Naming Conventions

Use the formatter and linter native to the chosen stack. Until tooling exists, keep files ASCII where practical, use clear module names, and avoid broad utility files. Prefer descriptive names such as `binaryEvaluator`, `runEvaluation`, or `evaluation-result.test.ts` over abbreviations. Keep configuration in explicit files at the repository root.

## Testing Guidelines

Add tests with the first behavior change. Name tests by behavior, not implementation detail, for example `evaluates-valid-binary.test.ts`. Keep fixtures small and checked in under `fixtures/` when they are needed for repeatable evaluations. Tests should be runnable from a single top-level command.

## Commit & Pull Request Guidelines

This repository has no commit history yet, so no existing convention is established. Use concise, imperative commit messages, optionally following Conventional Commits, such as `feat: add binary evaluation runner` or `test: cover invalid input handling`.

Pull requests should include a short summary, test results, linked issue or task when available, and screenshots or logs only when they clarify user-visible or CLI behavior.

## Agent-Specific Instructions

Before editing, inspect the current tree and preserve user changes. Keep generated artifacts out of commits unless they are intentional fixtures or documented outputs.
