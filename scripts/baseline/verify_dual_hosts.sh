#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./common.sh
source "${SCRIPT_DIR}/common.sh"

JUMP_HOST="${JUMP_HOST:-${1:-tee}}"
SECONDARY_HOST="${SECONDARY_HOST:-${2:-tee-b.example.internal}}"
REMOTE_MINIMAL_DIR="${REMOTE_MINIMAL_DIR:-${3:-/opt/tdid/tdid-remote-minimal}}"

baseline_require_commands "${SSH_BIN}"

run_remote_health_check() {
  local label="$1"
  local output="$2"
  local status_text

  status_text="$(printf '%s' "${output}" | tr -d '\r' | tr -d '\n')"
  if [[ "${status_text}" != "200" ]]; then
    echo "${label} expected HTTP 200, got '${status_text}'" >&2
    exit 1
  fi
  echo "HTTP 200"
}

run_secondary_script() {
  local label="$1"
  local script="$2"
  local output

  if ! output="$(printf '%s\n' "${script}" | "${SSH_BIN}" "${JUMP_HOST}" "ssh \"${SECONDARY_HOST}\" 'bash -se'")"; then
    echo "${label} failed" >&2
    exit 1
  fi
  printf '%s\n' "${output}"
}

echo "== primary manifest =="
baseline_run_remote_command "${JUMP_HOST}" "primary manifest" "cat ${REMOTE_MINIMAL_DIR}/managed/current/manifest.env"

echo "== primary health:18080 =="
primary_18080="$(baseline_run_remote_command "${JUMP_HOST}" "primary health:18080" "curl -sS -m 5 -o /dev/null -w '%{http_code}' http://127.0.0.1:18080/health")"
run_remote_health_check "primary health:18080" "${primary_18080}"

echo "== primary health:19080 =="
primary_19080="$(baseline_run_remote_command "${JUMP_HOST}" "primary health:19080" "curl -sS -m 5 -o /dev/null -w '%{http_code}' http://127.0.0.1:19080/health")"
run_remote_health_check "primary health:19080" "${primary_19080}"

echo "== secondary manifest =="
run_secondary_script "secondary manifest" "cat '${REMOTE_MINIMAL_DIR}/managed/current/manifest.env'"

echo "== secondary health:18080 =="
secondary_18080="$(run_secondary_script "secondary health:18080" "curl -sS -m 5 -o /dev/null -w '%{http_code}' http://127.0.0.1:18080/health")"
run_remote_health_check "secondary health:18080" "${secondary_18080}"

echo "== secondary health:19080 =="
secondary_19080="$(run_secondary_script "secondary health:19080" "curl -sS -m 5 -o /dev/null -w '%{http_code}' http://127.0.0.1:19080/health")"
run_remote_health_check "secondary health:19080" "${secondary_19080}"

echo "== verify done =="
