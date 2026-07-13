#!/usr/bin/env bash
set -euo pipefail


ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${ROOT_DIR}/scripts/lib/local_env.sh"

DRY_RUN=false
MODE=""

while [[ "$#" -gt 0 ]]; do
  case "$1" in
    --dry-run)
      DRY_RUN=true
      ;;
    --mode)
      MODE="${2:-}"
      shift
      ;;
    --mode=*)
      MODE="${1#--mode=}"
      ;;
    *)
      echo "unknown argument: $1" >&2
      exit 2
      ;;
  esac
  shift
done

if [[ -z "$MODE" ]]; then
  MODE="$(bin_eval_systemd_mode)"
fi
if [[ "$MODE" != "user" && "$MODE" != "system" ]]; then
  echo "mode must be user or system" >&2
  exit 2
fi

export BIN_EVAL_ENV_FILE
BIN_EVAL_ENV_FILE="$(bin_eval_default_env_file_for_mode "$ROOT_DIR" "$MODE")"
LITELLM_ENV_FILE="$(bin_eval_litellm_env_file)"
UNIT_DIR="$(bin_eval_unit_dir "$MODE")"
SERVICE_USER="${BIN_EVAL_SERVICE_USER:-$(id -un)}"
SERVICE_GROUP="${BIN_EVAL_SERVICE_GROUP:-docker}"
GIT_SHA="$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || printf 'unknown')"

if [[ "$DRY_RUN" == "false" ]]; then
  bin_eval_require_tools go docker systemctl jq curl
  if [[ "$MODE" == "system" ]] && ! sudo -n true >/dev/null 2>&1; then
    echo "system mode requires passwordless sudo for /etc/systemd/system and systemctl" >&2
    exit 1
  fi
  if [[ "$MODE" == "user" && "$(systemctl --user is-system-running 2>/dev/null || true)" == "offline" ]]; then
    echo "user systemd is not running" >&2
    exit 1
  fi
fi

if [[ "$DRY_RUN" == "true" ]]; then
  echo "mode=${MODE}"
  echo "root_dir=${ROOT_DIR}"
  echo "unit_dir=${UNIT_DIR}"
  echo "bin_eval_env_file=${BIN_EVAL_ENV_FILE}"
  echo "litellm_env_file=${LITELLM_ENV_FILE}"
  echo "build=go build -o bin/bin-eval-api ./cmd/bin-eval-api && go build -o bin/bin-eval-worker ./cmd/bin-eval-worker"
  echo "units=bin-eval-deps.service bin-eval-api.service bin-eval-worker.service"
  echo "secrets=redacted"
  exit 0
fi

if [[ "$MODE" == "system" ]]; then
  if [[ ! -f "$BIN_EVAL_ENV_FILE" ]]; then
    sudo -n install -d -m 0755 "$(dirname "$BIN_EVAL_ENV_FILE")"
    tmp_env="$(mktemp)"
    cp "${ROOT_DIR}/deploy/local/bin-eval.env.example" "$tmp_env"
    sudo -n install -m 0600 "$tmp_env" "$BIN_EVAL_ENV_FILE"
    rm -f "$tmp_env"
  fi
else
  bin_eval_prepare_local_env_file "$ROOT_DIR"
fi

go build -o "${ROOT_DIR}/bin/bin-eval-api" ./cmd/bin-eval-api
go build -o "${ROOT_DIR}/bin/bin-eval-worker" ./cmd/bin-eval-worker

bin_eval_load_local_env "$ROOT_DIR"
POSTGRES_PORT="${BIN_EVAL_POSTGRES_PORT:-55432}"

render_unit() {
  local template="$1"
  local out="$2"
  local install_target service_identity docker_unit_dependencies line

  if [[ "$MODE" == "system" ]]; then
    install_target="multi-user.target"
    service_identity=$'User='"${SERVICE_USER}"$'\nGroup='"${SERVICE_GROUP}"
    docker_unit_dependencies=$'Requires=docker.service\nAfter=network-online.target docker.service\nWants=network-online.target'
  else
    install_target="default.target"
    service_identity=""
    docker_unit_dependencies=""
  fi

  : > "$out"
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" == *"{{SERVICE_IDENTITY}}"* ]]; then
      [[ -n "$service_identity" ]] && printf '%s\n' "$service_identity" >> "$out"
      continue
    fi
    if [[ "$line" == *"{{DOCKER_UNIT_DEPENDENCIES}}"* ]]; then
      [[ -n "$docker_unit_dependencies" ]] && printf '%s\n' "$docker_unit_dependencies" >> "$out"
      continue
    fi
    line="${line//\{\{ROOT_DIR\}\}/$ROOT_DIR}"
    line="${line//\{\{BIN_EVAL_ENV_FILE\}\}/$BIN_EVAL_ENV_FILE}"
    line="${line//\{\{LITELLM_ENV_FILE\}\}/$LITELLM_ENV_FILE}"
    line="${line//\{\{GIT_SHA\}\}/$GIT_SHA}"
    line="${line//\{\{POSTGRES_PORT\}\}/$POSTGRES_PORT}"
    line="${line//\{\{INSTALL_TARGET\}\}/$install_target}"
    printf '%s\n' "$line" >> "$out"
  done < "$template"
}

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

for unit in bin-eval-deps.service bin-eval-api.service bin-eval-worker.service; do
  render_unit "${ROOT_DIR}/deploy/systemd/${unit}.in" "${tmp_dir}/${unit}"
done

if [[ "$MODE" == "system" ]]; then
  for unit in bin-eval-deps.service bin-eval-api.service bin-eval-worker.service; do
    sudo -n install -m 0644 "${tmp_dir}/${unit}" "${UNIT_DIR}/${unit}"
  done
else
  install -d -m 0755 "$UNIT_DIR"
  for unit in bin-eval-deps.service bin-eval-api.service bin-eval-worker.service; do
    install -m 0644 "${tmp_dir}/${unit}" "${UNIT_DIR}/${unit}"
  done
fi

bin_eval_systemctl "$MODE" daemon-reload
bin_eval_systemctl "$MODE" enable bin-eval-deps.service bin-eval-api.service bin-eval-worker.service >/dev/null

echo "installed bin-eval local systemd units mode=${MODE}"
echo "unit_dir=${UNIT_DIR}"
echo "bin_eval_env_file=${BIN_EVAL_ENV_FILE}"
echo "litellm_env_file=${LITELLM_ENV_FILE}"
echo "secrets=redacted"
