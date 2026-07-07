#!/usr/bin/env bash

bin_eval_root_dir() {
  cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd
}

bin_eval_env_file() {
  local root_dir="$1"
  printf '%s\n' "${BIN_EVAL_ENV_FILE:-${root_dir}/deploy/local/bin-eval.env}"
}

bin_eval_default_env_file_for_mode() {
  local root_dir="$1"
  local mode="$2"
  if [[ -n "${BIN_EVAL_ENV_FILE:-}" ]]; then
    printf '%s\n' "$BIN_EVAL_ENV_FILE"
  elif [[ "$mode" == "system" ]]; then
    printf '%s\n' "/etc/bin-eval/bin-eval.env"
  else
    printf '%s\n' "${root_dir}/deploy/local/bin-eval.env"
  fi
}

bin_eval_litellm_env_file() {
  printf '%s\n' "${LITELLM_ENV_FILE:-/home/kirill/p/litellm-chatgpt/.env}"
}

bin_eval_strip_quotes() {
  local value="$1"
  if [[ "$value" == \"*\" && "$value" == *\" ]]; then
    value="${value:1:${#value}-2}"
  elif [[ "$value" == \'*\' && "$value" == *\' ]]; then
    value="${value:1:${#value}-2}"
  fi
  printf '%s\n' "$value"
}

bin_eval_load_env_file() {
  local file="$1"
  local overwrite="${2:-false}"
  [[ -f "$file" ]] || return 0

  local line key value current
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%$'\r'}"
    [[ -z "$line" || "$line" == \#* ]] && continue
    if [[ "$line" == export\ * ]]; then
      line="${line#export }"
    fi
    [[ "$line" =~ ^[A-Za-z_][A-Za-z0-9_]*= ]] || continue
    key="${line%%=*}"
    value="${line#*=}"
    value="$(bin_eval_strip_quotes "$value")"
    current="${!key:-}"
    if [[ "$overwrite" == "true" || -z "$current" ]]; then
      export "$key=$value"
    fi
  done < "$file"
}

bin_eval_load_litellm_env_file() {
  local file="$1"
  [[ -f "$file" ]] || return 0

  local line key value current
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%$'\r'}"
    [[ -z "$line" || "$line" == \#* ]] && continue
    if [[ "$line" == export\ * ]]; then
      line="${line#export }"
    fi
    [[ "$line" =~ ^[A-Za-z_][A-Za-z0-9_]*= ]] || continue
    key="${line%%=*}"
    case "$key" in
      LITELLM_MASTER_KEY|LITELLM_PORT) ;;
      *) continue ;;
    esac
    value="${line#*=}"
    value="$(bin_eval_strip_quotes "$value")"
    current="${!key:-}"
    if [[ -z "$current" ]]; then
      export "$key=$value"
    fi
  done < "$file"
}

bin_eval_prepare_local_env_file() {
  local root_dir="$1"
  local env_file
  env_file="$(bin_eval_env_file "$root_dir")"
  if [[ ! -f "$env_file" ]]; then
    mkdir -p "$(dirname "$env_file")"
    cp "${root_dir}/deploy/local/bin-eval.env.example" "$env_file"
    chmod 0600 "$env_file"
  fi
}

bin_eval_load_local_env() {
  local root_dir="$1"
  local env_file litellm_env_file
  env_file="$(bin_eval_env_file "$root_dir")"
  litellm_env_file="$(bin_eval_litellm_env_file)"

  bin_eval_load_env_file "${root_dir}/deploy/compose/.env.example" false
  bin_eval_load_env_file "$env_file" true
  bin_eval_load_litellm_env_file "$litellm_env_file"

  if [[ -n "${LITELLM_PORT:-}" && "${BIN_EVAL_LLM_BASE_URL:-}" == "http://127.0.0.1:4000" ]]; then
    export BIN_EVAL_LLM_BASE_URL="http://127.0.0.1:${LITELLM_PORT}"
  fi
  if [[ -z "${BIN_EVAL_LLM_API_KEY:-}" || "${BIN_EVAL_LLM_API_KEY:-}" == "replace-with-local-llm-key" ]]; then
    if [[ -n "${LITELLM_MASTER_KEY:-}" ]]; then
      export BIN_EVAL_LLM_API_KEY="$LITELLM_MASTER_KEY"
    fi
  fi
}

bin_eval_systemd_mode() {
  if [[ -n "${BIN_EVAL_SYSTEMD_MODE:-}" ]]; then
    printf '%s\n' "$BIN_EVAL_SYSTEMD_MODE"
    return
  fi
  if sudo -n true >/dev/null 2>&1; then
    printf 'system\n'
  else
    printf 'user\n'
  fi
}

bin_eval_systemctl() {
  local mode="$1"
  shift
  if [[ "$mode" == "user" ]]; then
    systemctl --user "$@"
  elif [[ "$mode" == "system" ]]; then
    sudo -n systemctl "$@"
  else
    echo "unsupported systemd mode: $mode" >&2
    return 2
  fi
}

bin_eval_unit_dir() {
  local mode="$1"
  if [[ "$mode" == "user" ]]; then
    printf '%s\n' "${HOME}/.config/systemd/user"
  else
    printf '%s\n' "/etc/systemd/system"
  fi
}

bin_eval_require_tools() {
  local missing=()
  local tool
  for tool in "$@"; do
    if ! command -v "$tool" >/dev/null 2>&1; then
      missing+=("$tool")
    fi
  done
  if (( ${#missing[@]} > 0 )); then
    printf 'missing required tools: %s\n' "${missing[*]}" >&2
    return 1
  fi
}
