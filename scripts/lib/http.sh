#!/usr/bin/env bash

bin_eval_post_json() {
  local base_url="$1"
  local path="$2"
  local payload="$3"
  local output="$4"
  curl -fsS \
    -H 'Content-Type: application/json' \
    -X POST \
    --data "$payload" \
    "${base_url}${path}" \
    -o "$output"
}

bin_eval_wait_for_api() {
  local base_url="$1"
  local timeout_seconds="${BIN_EVAL_API_WAIT_TIMEOUT_SECONDS:-90}"
  local deadline=$((SECONDS + timeout_seconds))

  while ((SECONDS < deadline)); do
    local code
    code="$(curl -sS -o /dev/null -w '%{http_code}' "${base_url}/checklists/00000000-0000-0000-0000-000000000000" || true)"
    if [[ "$code" != "000" ]]; then
      return 0
    fi
    sleep 1
  done
  echo "timed out waiting for API at ${base_url} after ${timeout_seconds}s" >&2
  return 1
}

bin_eval_poll_entity() {
  local url="$1"
  local output="$2"
  local label="$3"
  local timeout_seconds="${BIN_EVAL_POLL_TIMEOUT_SECONDS:-1200}"
  local interval_seconds="${BIN_EVAL_POLL_INTERVAL_SECONDS:-2}"
  local deadline=$((SECONDS + timeout_seconds))

  while ((SECONDS < deadline)); do
    if ! curl -fsS "$url" -o "$output"; then
      sleep "$interval_seconds"
      continue
    fi

    local entity_state
    entity_state="$(jq -r '.status // empty' "$output")"
    case "$entity_state" in
      succeeded)
        return 0
        ;;
      failed)
        echo "${label} failed" >&2
        jq . "$output" >&2
        return 1
        ;;
    esac
    sleep "$interval_seconds"
  done

  echo "timed out polling ${label} after ${timeout_seconds}s: ${url}" >&2
  return 1
}
