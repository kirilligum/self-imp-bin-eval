#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

scripts/smoke_curl.sh
scripts/validate_smoke_invariants.sh
scripts/capture_artifacts.sh debug/smoke
