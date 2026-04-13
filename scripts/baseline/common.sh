#!/usr/bin/env bash

BASELINE_SCRIPTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BASELINE_REPO_ROOT="$(cd "${BASELINE_SCRIPTS_DIR}/../.." && pwd)"
SSH_BIN="${SSH_BIN:-ssh}"
SCP_BIN="${SCP_BIN:-scp}"

baseline_log() {
  echo "[$(date +'%F %T')] $*"
}

baseline_require_commands() {
  local cmd
  for cmd in "$@"; do
    command -v "$cmd" >/dev/null 2>&1 || {
      echo "missing required command: $cmd" >&2
      exit 1
    }
  done
}

baseline_sync_files() {
  local target="$1"
  local remote_root="$2"
  shift 2

  local relative_path src_path remote_path
  for relative_path in "$@"; do
    src_path="${BASELINE_REPO_ROOT}/${relative_path}"
    remote_path="${remote_root}/$(basename "${relative_path}")"
    baseline_log "sync ${relative_path} -> ${target}:${remote_path}"
    "${SCP_BIN}" "${src_path}" "${target}:${remote_path}"
  done
}

baseline_normalize_remote_files() {
  local target="$1"
  shift

  "${SSH_BIN}" "${target}" 'bash -se' -- "$@" <<'EOF'
for path in "$@"; do
  if [[ -f "${path}" ]]; then
    tmp="$(mktemp)"
    tr -d '\r' < "${path}" > "${tmp}"
    mv "${tmp}" "${path}"
    chmod +x "${path}"
  fi
done
EOF
}

baseline_run_remote_command() {
  local target="$1"
  local label="$2"
  local command="$3"
  local output

  if ! output="$("${SSH_BIN}" "${target}" "${command}")"; then
    echo "${label} failed" >&2
    return 1
  fi
  printf '%s\n' "${output}"
}

baseline_run_remote_script() {
  local target="$1"
  local label="$2"
  local script="$3"
  local output

  if ! output="$(printf '%s\n' "${script}" | "${SSH_BIN}" "${target}" 'bash -se')"; then
    echo "${label} failed" >&2
    return 1
  fi
  printf '%s\n' "${output}"
}
