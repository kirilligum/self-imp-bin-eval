#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${ROOT_DIR}/scripts/lib/local_env.sh"
bin_eval_require_tools curl docker gh jq sg sha256sum systemctl tar
bin_eval_load_local_env "$ROOT_DIR"

REPOSITORY="${BIN_EVAL_GITHUB_REPOSITORY:-kirilligum/self-imp-bin-eval}"
RUNNER_NAME="${BIN_EVAL_RUNNER_NAME:-shaman-bin-eval-live}"
RUNNER_LABEL="bin-eval-live"
RUNNER_ROOT="${BIN_EVAL_RUNNER_ROOT:-${HOME}/.local/share/github-actions/self-imp-bin-eval}"
UNIT_NAME="github-actions-self-imp-bin-eval.service"
UNIT_PATH="${HOME}/.config/systemd/user/${UNIT_NAME}"
LITELLM_CONTAINER="${BIN_EVAL_LLM_CONTAINER:-shaman-api-litellm-1}"

if [[ -z "${BIN_EVAL_LLM_API_KEY:-}" || "${BIN_EVAL_LLM_API_KEY}" == "replace-with-local-llm-key" ]]; then
  echo "local LiteLLM API key is not configured" >&2
  exit 1
fi
docker inspect "$LITELLM_CONTAINER" >/dev/null

mkdir -p "$RUNNER_ROOT"
if [[ ! -f "${RUNNER_ROOT}/.runner" ]]; then
  if [[ -n "$(find "$RUNNER_ROOT" -mindepth 1 -maxdepth 1 -print -quit)" ]]; then
    echo "runner directory is non-empty but not configured: ${RUNNER_ROOT}" >&2
    exit 1
  fi

  release="$(gh api repos/actions/runner/releases/latest)"
  version="$(jq -r '.tag_name | sub("^v"; "")' <<<"$release")"
  asset_name="actions-runner-linux-x64-${version}.tar.gz"
  asset_url="$(jq -r --arg name "$asset_name" '.assets[] | select(.name == $name) | .url' <<<"$release")"
  asset_digest="$(jq -r --arg name "$asset_name" '.assets[] | select(.name == $name) | .digest // empty' <<<"$release")"
  if [[ -z "$asset_url" || -z "$asset_digest" ]]; then
    echo "runner release does not provide the expected verified linux-x64 asset" >&2
    exit 1
  fi

  archive="$(mktemp)"
  trap 'rm -f "${archive:-}"' EXIT
  gh api -H 'Accept: application/octet-stream' "$asset_url" >"$archive"
  printf '%s  %s\n' "${asset_digest#sha256:}" "$archive" | sha256sum --check --status
  tar -xzf "$archive" -C "$RUNNER_ROOT"

  registration_token="$(gh api --method POST "repos/${REPOSITORY}/actions/runners/registration-token" --jq .token)"
  (
    cd "$RUNNER_ROOT"
    ./config.sh --unattended \
      --url "https://github.com/${REPOSITORY}" \
      --token "$registration_token" \
      --name "$RUNNER_NAME" \
      --labels "$RUNNER_LABEL" \
      --work _work \
      --replace
  )
fi

mkdir -p "$(dirname "$UNIT_PATH")"
printf '%s\n' \
  '[Unit]' \
  'Description=GitHub Actions runner for self-imp-bin-eval live validation' \
  'After=network-online.target' \
  'Wants=network-online.target' \
  '' \
  '[Service]' \
  'Type=simple' \
  "WorkingDirectory=${RUNNER_ROOT}" \
  "ExecStart=/usr/bin/sg docker -c ${RUNNER_ROOT}/run.sh" \
  'Restart=always' \
  'RestartSec=5' \
  'KillMode=process' \
  '' \
  '[Install]' \
  'WantedBy=default.target' >"$UNIT_PATH"

printf '%s' 'http://bin-eval-litellm:4000' | gh secret set BIN_EVAL_LLM_BASE_URL --repo "$REPOSITORY"
printf '%s' "$BIN_EVAL_LLM_API_KEY" | gh secret set BIN_EVAL_LLM_API_KEY --repo "$REPOSITORY"
gh variable set BIN_EVAL_LLM_CONTAINER --body "$LITELLM_CONTAINER" --repo "$REPOSITORY"

systemctl --user daemon-reload
systemctl --user enable "$UNIT_NAME"
systemctl --user restart "$UNIT_NAME"
echo "live CI runner installed name=${RUNNER_NAME} label=${RUNNER_LABEL} service=${UNIT_NAME} secrets=redacted"
