#!/usr/bin/env bash
set -euo pipefail

# A3 dual-host managed release pipeline.
# Purpose:
# 1) build or select managed release on primary host
# 2) promote artifact to secondary host(s)
# 3) adopt-current managed rollout on secondary host(s)
# 4) verify health and optionally run closeout

REMOTE_USER="${REMOTE_USER:-ecs-user}"
PRIMARY_HOST="${PRIMARY_HOST:-tee-a.example.internal}"
SECONDARY_HOSTS_CSV="${SECONDARY_HOSTS_CSV:-tee-b.example.internal}"
RELEASE_ID="${RELEASE_ID:-}"

REMOTE_MINIMAL_DIR="${REMOTE_MINIMAL_DIR:-/opt/tdid/tdid-remote-minimal}"
REMOTE_REPO_DIR="${REMOTE_REPO_DIR:-/opt/tdid/tdid-open-minimal/tee}"
OCCLUM_BIN="${OCCLUM_BIN:-/opt/occlum/build/bin/occlum}"
OCCLUM_GO="${OCCLUM_GO:-/opt/occlum/toolchains/golang/bin/occlum-go}"

PIPELINE_MODE="${PIPELINE_MODE:-build-and-promote}" # build-and-promote | promote-only
ALLOW_UNSTABLE_BUILD_FROM_SOURCE="${ALLOW_UNSTABLE_BUILD_FROM_SOURCE:-0}"
BUILD_ENV="${BUILD_ENV:-CC=/opt/occlum/toolchains/gcc/bin/occlum-gcc}"
BUILD_CMD="${BUILD_CMD:-/opt/occlum/toolchains/golang/bin/occlum-go build}"
BUILD_TARGET="${BUILD_TARGET:-./enclave/cmd/tee_enclave_service}"
BUILD_OUTPUT_BASENAME="${BUILD_OUTPUT_BASENAME:-tee_enclave_service}"
EXPECTED_INTERPRETER="${EXPECTED_INTERPRETER:-/lib/ld-musl-x86_64.so.1}"

REBUILD_INSTANCE="${REBUILD_INSTANCE:-1}"
RESTART_SERVICES="${RESTART_SERVICES:-1}"
RUN_CLOSEOUT_ON_PRIMARY="${RUN_CLOSEOUT_ON_PRIMARY:-1}"
DRY_RUN="${DRY_RUN:-0}"

SSH_OPTS=(
  -o BatchMode=yes
  -o ConnectTimeout=10
  -o StrictHostKeyChecking=accept-new
  -o ServerAliveInterval=15
  -o ServerAliveCountMax=6
  -o TCPKeepAlive=yes
)

log() {
  echo "[a3-dual-pipeline] $*"
}

require() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    echo "missing required env: ${name}" >&2
    exit 1
  fi
}

run_remote() {
  local host="$1"
  local cmd="$2"
  if [[ "${DRY_RUN}" == "1" ]]; then
    log "[dry-run][${host}] ${cmd}"
    return
  fi
  ssh "${SSH_OPTS[@]}" "${REMOTE_USER}@${host}" "bash -lc $(printf '%q' "${cmd}")"
}

primary_cmd() {
  local cmd="$1"
  run_remote "${PRIMARY_HOST}" "${cmd}"
}

copy_primary_to_secondary() {
  local secondary="$1"
  local src="$2"
  local dst="$3"
  local cmd="
set -euo pipefail
scp -o BatchMode=yes -o ConnectTimeout=10 '${src}' '${REMOTE_USER}@${secondary}:${dst}'
"
  primary_cmd "${cmd}"
}

prepare_script_on_host() {
  local host="$1"
  local script_path="/home/${REMOTE_USER}/a2_minimal_managed_release.sh"
  if [[ "${DRY_RUN}" == "1" ]]; then
    log "[dry-run] assume ${script_path} exists on ${host}"
    return
  fi
  scp "B:\\codex\\a2_minimal_managed_release.sh" "${REMOTE_USER}@${host}:${script_path}"
  ssh "${SSH_OPTS[@]}" "${REMOTE_USER}@${host}" "python3 - <<'PY'
from pathlib import Path
p=Path('${script_path}')
b=p.read_bytes()
if b.startswith(b'\xef\xbb\xbf'):
    b=b[3:]
p.write_bytes(b.replace(b'\r\n', b'\n'))
PY
chmod +x '${script_path}'"
}

build_on_primary() {
  local cmd="
set -euo pipefail
REMOTE_USER='${REMOTE_USER}' \
HOSTS_CSV='${PRIMARY_HOST}' \
MODE='build-from-source' \
ALLOW_UNSTABLE_BUILD_FROM_SOURCE='${ALLOW_UNSTABLE_BUILD_FROM_SOURCE}' \
RELEASE_ID='${RELEASE_ID}' \
REMOTE_REPO_DIR='${REMOTE_REPO_DIR}' \
REMOTE_MINIMAL_DIR='${REMOTE_MINIMAL_DIR}' \
OCCLUM_BIN='${OCCLUM_BIN}' \
OCCLUM_GO='${OCCLUM_GO}' \
REBUILD_INSTANCE='${REBUILD_INSTANCE}' \
RESTART_SERVICES='${RESTART_SERVICES}' \
BUILD_TARGET='${BUILD_TARGET}' \
BUILD_OUTPUT_BASENAME='${BUILD_OUTPUT_BASENAME}' \
BUILD_ENV='${BUILD_ENV}' \
BUILD_CMD='${BUILD_CMD}' \
EXPECTED_INTERPRETER='${EXPECTED_INTERPRETER}' \
/home/${REMOTE_USER}/a2_minimal_managed_release.sh
"
  primary_cmd "${cmd}"
}

adopt_on_secondary() {
  local secondary="$1"
  local cmd="
set -euo pipefail
REMOTE_USER='${REMOTE_USER}' \
HOSTS_CSV='${secondary}' \
MODE='adopt-current' \
RELEASE_ID='${RELEASE_ID}' \
REMOTE_REPO_DIR='${REMOTE_REPO_DIR}' \
REMOTE_MINIMAL_DIR='${REMOTE_MINIMAL_DIR}' \
OCCLUM_BIN='${OCCLUM_BIN}' \
OCCLUM_GO='${OCCLUM_GO}' \
REBUILD_INSTANCE='${REBUILD_INSTANCE}' \
RESTART_SERVICES='${RESTART_SERVICES}' \
BUILD_TARGET='${BUILD_TARGET}' \
BUILD_OUTPUT_BASENAME='${BUILD_OUTPUT_BASENAME}' \
BUILD_ENV='${BUILD_ENV}' \
BUILD_CMD='${BUILD_CMD}' \
EXPECTED_INTERPRETER='${EXPECTED_INTERPRETER}' \
/home/${REMOTE_USER}/a2_minimal_managed_release.sh
"
  run_remote "${secondary}" "${cmd}"
}

promote_to_secondary() {
  local secondary="$1"
  local promoted_bin="/home/${REMOTE_USER}/tee_enclave_service_${RELEASE_ID}"
  copy_primary_to_secondary \
    "${secondary}" \
    "${REMOTE_MINIMAL_DIR}/managed/releases/${RELEASE_ID}/tee_enclave_service" \
    "${promoted_bin}"

  local cmd="
set -euo pipefail
cp '${promoted_bin}' '${REMOTE_MINIMAL_DIR}/tee-a-instance/image/bin/tee_enclave_service'
cp '${promoted_bin}' '${REMOTE_MINIMAL_DIR}/tee-b-instance/image/bin/tee_enclave_service'
"
  run_remote "${secondary}" "${cmd}"
  adopt_on_secondary "${secondary}"
}

health_probe() {
  local host="$1"
  local cmd="
set -euo pipefail
source '${REMOTE_MINIMAL_DIR}/remote.env'
curl -fsS \"http://127.0.0.1\${TEE_A_ADDR}/health\" >/dev/null
curl -fsS \"http://127.0.0.1\${TEE_B_ADDR}/health\" >/dev/null
echo '${host}: health ok'
"
  run_remote "${host}" "${cmd}"
}

closeout_primary() {
  local cmd="bash /home/${REMOTE_USER}/p1_closeout_tee.sh"
  primary_cmd "${cmd}"
}

main() {
  require RELEASE_ID
  case "${PIPELINE_MODE}" in
    build-and-promote|promote-only) ;;
    *)
      echo "PIPELINE_MODE must be build-and-promote or promote-only" >&2
      exit 1
      ;;
  esac

  prepare_script_on_host "${PRIMARY_HOST}"
  IFS=',' read -r -a secondaries <<< "${SECONDARY_HOSTS_CSV}"
  for s in "${secondaries[@]}"; do
    s="$(echo "${s}" | xargs)"
    [[ -n "${s}" ]] || continue
    prepare_script_on_host "${s}"
  done

  if [[ "${PIPELINE_MODE}" == "build-and-promote" ]]; then
    if [[ "${ALLOW_UNSTABLE_BUILD_FROM_SOURCE}" != "1" ]]; then
      echo "build-and-promote requires ALLOW_UNSTABLE_BUILD_FROM_SOURCE=1" >&2
      exit 1
    fi
    log "build release ${RELEASE_ID} on primary ${PRIMARY_HOST}"
    build_on_primary
  else
    log "skip primary build, promote-only mode"
  fi

  for s in "${secondaries[@]}"; do
    s="$(echo "${s}" | xargs)"
    [[ -n "${s}" ]] || continue
    log "promote release ${RELEASE_ID} to secondary ${s}"
    promote_to_secondary "${s}"
  done

  log "verify health on primary and secondaries"
  health_probe "${PRIMARY_HOST}"
  for s in "${secondaries[@]}"; do
    s="$(echo "${s}" | xargs)"
    [[ -n "${s}" ]] || continue
    health_probe "${s}"
  done

  if [[ "${RUN_CLOSEOUT_ON_PRIMARY}" == "1" ]]; then
    log "run closeout on primary"
    closeout_primary
  fi

  log "DONE: release ${RELEASE_ID} pipeline completed"
}

main "$@"
