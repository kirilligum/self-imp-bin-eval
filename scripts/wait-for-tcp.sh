#!/usr/bin/env bash
set -euo pipefail

if [[ "$#" -lt 2 || "$#" -gt 3 ]]; then
  echo "usage: wait-for-tcp.sh HOST PORT [TIMEOUT_SECONDS]" >&2
  exit 2
fi

host="$1"
port="$2"
timeout="${3:-60}"

deadline=$((SECONDS + timeout))
while (( SECONDS < deadline )); do
  if (echo >"/dev/tcp/${host}/${port}") >/dev/null 2>&1; then
    exit 0
  fi
  sleep 1
done

echo "timed out waiting for ${host}:${port}" >&2
exit 1
