#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${ROOT_DIR}/scripts/lib/local_env.sh"
bin_eval_require_tools curl date docker git gzip id install jq sha256sum systemctl wc

MODE="${BIN_EVAL_SYSTEMD_MODE:-$(bin_eval_systemd_mode)}"
export BIN_EVAL_ENV_FILE
BIN_EVAL_ENV_FILE="$(bin_eval_default_env_file_for_mode "$ROOT_DIR" "$MODE")"
bin_eval_load_local_env "$ROOT_DIR"

backup_root="${BIN_EVAL_BACKUP_ROOT:-${ROOT_DIR}/backups}"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
backup_dir="${backup_root}/${timestamp}"
install -d -m 0700 "$backup_dir"

compose=(docker compose --env-file "$BIN_EVAL_ENV_FILE" -f "${ROOT_DIR}/deploy/compose/docker-compose.yml")
garage_container="$("${compose[@]}" ps -q garage)"
[[ -n "$garage_container" ]] || { echo "Garage container is not available" >&2; exit 1; }
compose_project="$(docker inspect "$garage_container" --format '{{index .Config.Labels "com.docker.compose.project"}}')"
[[ -n "$compose_project" ]] || { echo "Garage Compose project label is missing" >&2; exit 1; }
api_was_active="$(bin_eval_systemctl "$MODE" is-active bin-eval-api.service 2>/dev/null || true)"
worker_was_active="$(bin_eval_systemctl "$MODE" is-active bin-eval-worker.service 2>/dev/null || true)"
public_was_active=false
if [[ -f "$(bin_eval_public_env_file "$ROOT_DIR")" ]] && [[ "$(curl -sS -o /dev/null -w '%{http_code}' http://127.0.0.1:18081/healthz 2>/dev/null || true)" == "204" ]]; then
  public_was_active=true
fi

restore_services() {
  "${compose[@]}" up -d postgres temporal garage >/dev/null 2>&1 || true
  [[ "$worker_was_active" == "active" ]] && bin_eval_systemctl "$MODE" start bin-eval-worker.service >/dev/null 2>&1 || true
  [[ "$api_was_active" == "active" ]] && bin_eval_systemctl "$MODE" start bin-eval-api.service >/dev/null 2>&1 || true
  [[ "$public_was_active" == "true" ]] && "${ROOT_DIR}/scripts/public-gateway.sh" start >/dev/null 2>&1 || true
}
trap restore_services EXIT

[[ "$public_was_active" == "true" ]] && "${ROOT_DIR}/scripts/public-gateway.sh" stop >/dev/null
bin_eval_systemctl "$MODE" stop bin-eval-api.service bin-eval-worker.service >/dev/null
"${compose[@]}" stop temporal garage >/dev/null

"${compose[@]}" exec -T postgres pg_dumpall -U "${BIN_EVAL_POSTGRES_USER}" | gzip -9 >"${backup_dir}/postgres-all.sql.gz"

volume_for() {
  local logical_name="$1"
  local volume
  volume="$(docker volume ls -q \
    --filter "label=com.docker.compose.project=${compose_project}" \
    --filter "label=com.docker.compose.volume=${logical_name}")"
  [[ -n "$volume" && "$(wc -w <<<"$volume")" == "1" ]] || {
    echo "expected one Docker volume for ${logical_name}, got: ${volume:-none}" >&2
    exit 1
  }
  printf '%s\n' "$volume"
}

for logical_name in garage-meta garage-data; do
  volume="$(volume_for "$logical_name")"
  docker run --rm \
    -e "HOST_UID=$(id -u)" \
    -e "HOST_GID=$(id -g)" \
    -e "ARCHIVE_NAME=${logical_name}.tar.gz" \
    -v "${volume}:/source:ro" \
    -v "${backup_dir}:/backup" \
    nginx:1.28.2-alpine \
    sh -c 'tar -czf "/backup/${ARCHIVE_NAME}" -C /source . && chown "${HOST_UID}:${HOST_GID}" "/backup/${ARCHIVE_NAME}" && chmod 0600 "/backup/${ARCHIVE_NAME}"'
done

(
  cd "$backup_dir"
  sha256sum postgres-all.sql.gz garage-meta.tar.gz garage-data.tar.gz >SHA256SUMS
)
jq -n \
  --arg created_at "$timestamp" \
  --arg git_sha "$(git -C "$ROOT_DIR" rev-parse HEAD)" \
  --arg postgres "postgres-all.sql.gz" \
  --arg garage_meta "garage-meta.tar.gz" \
  --arg garage_data "garage-data.tar.gz" \
  '{created_at:$created_at, git_sha:$git_sha, files:[$postgres,$garage_meta,$garage_data], checksum_file:"SHA256SUMS", secrets:"redacted"}' \
  >"${backup_dir}/manifest.json"
chmod 0600 "${backup_dir}"/*

restore_services
trap - EXIT
echo "bin-eval backup complete path=${backup_dir} files=3 checksums=SHA256SUMS secrets=redacted"
