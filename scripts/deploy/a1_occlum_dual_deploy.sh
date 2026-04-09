#!/usr/bin/env bash
set -euo pipefail

# A1 unified Occlum deployment helper for two TEE hosts.
# Defaults align with deploy/remote-tee-dual-eval.sh conventions.

REMOTE_USER="${REMOTE_USER:-ecs-user}"
HOSTS_CSV="${HOSTS_CSV:-tee-a.example.internal,tee-b.example.internal}"
REMOTE_REPO_DIR="${REMOTE_REPO_DIR:-/opt/tdid/tdid-open-minimal/tee}"
REMOTE_MINIMAL_DIR="${REMOTE_MINIMAL_DIR:-/opt/tdid/tdid-remote-minimal}"
OCCLUM_BIN="${OCCLUM_BIN:-/opt/occlum/build/bin/occlum}"
OCCLUM_GO="${OCCLUM_GO:-/opt/occlum/toolchains/golang/bin/occlum-go}"
INSTANCE_DIR="${INSTANCE_DIR:-${REMOTE_REPO_DIR}/enclave/occlum/instance}"
FORCE_REBUILD="${FORCE_REBUILD:-1}"

SSH_OPTS=(
  -o BatchMode=yes
  -o ConnectTimeout=10
  -o StrictHostKeyChecking=accept-new
  -o ServerAliveInterval=15
  -o ServerAliveCountMax=6
  -o TCPKeepAlive=yes
)

log() {
  echo "[a1-occlum-dual] $*"
}

run_host() {
  local host="$1"
  local cmd="$2"
  if is_local_host "${host}"; then
    bash -lc "${cmd}"
    return
  fi
  ssh "${SSH_OPTS[@]}" "${REMOTE_USER}@${host}" "bash -lc $(printf '%q' "${cmd}")"
}

is_local_host() {
  local host="$1"
  [[ -n "${host}" ]] || return 1
  case "${host}" in
    localhost|127.0.0.1|::1) return 0 ;;
  esac
  local self_hostname self_fqdn
  self_hostname="$(hostname 2>/dev/null || true)"
  self_fqdn="$(hostname -f 2>/dev/null || true)"
  if [[ "${host}" == "${self_hostname}" || "${host}" == "${self_fqdn}" ]]; then
    return 0
  fi
  if hostname -I 2>/dev/null | tr ' ' '\n' | grep -Fxq "${host}"; then
    return 0
  fi
  if ip -o -4 addr show 2>/dev/null | awk '{print $4}' | cut -d/ -f1 | grep -Fxq "${host}"; then
    return 0
  fi
  return 1
}

deploy_one() {
  local host="$1"
  log "[${host}] detect deployment mode"
  local mode
  mode="$(run_host "${host}" "
    set -euo pipefail
    if [[ -d '${REMOTE_REPO_DIR}' && -x '${REMOTE_REPO_DIR}/enclave/occlum/build.sh' && -x '${OCCLUM_GO}' ]]; then
      echo repo
    elif [[ -d '${REMOTE_MINIMAL_DIR}' && -d '${REMOTE_MINIMAL_DIR}/tee-a-instance' && -d '${REMOTE_MINIMAL_DIR}/tee-b-instance' ]]; then
      echo minimal
    else
      echo unsupported
    fi
  ")"

  if [[ "${mode}" == "repo" ]]; then
    log "[${host}] mode=repo, precheck workspace and toolchain"
    run_host "${host}" "
      set -euo pipefail
      test -d '${REMOTE_REPO_DIR}'
      test -x '${OCCLUM_BIN}'
      test -x '${OCCLUM_GO}'
      test -x '${REMOTE_REPO_DIR}/enclave/occlum/build.sh'
    "

    log "[${host}] build occlum instance from source repo (FORCE_REBUILD_INSTANCE=${FORCE_REBUILD})"
    run_host "${host}" "
      set -euo pipefail
      cd '${REMOTE_REPO_DIR}'
      FORCE_REBUILD_INSTANCE='${FORCE_REBUILD}' OCCLUM_BIN='${OCCLUM_BIN}' OCCLUM_GO='${OCCLUM_GO}' ./enclave/occlum/build.sh '${INSTANCE_DIR}'
    "

    log "[${host}] verify source-repo instance artifacts"
    run_host "${host}" "
      set -euo pipefail
      test -f '${INSTANCE_DIR}/Occlum.json'
      test -f '${INSTANCE_DIR}/.__occlum_status'
      test -x '${REMOTE_REPO_DIR}/enclave/occlum/bin/tee_enclave_service'
      ls -la '${INSTANCE_DIR}' | sed -n '1,8p'
    "
    return
  fi

  if [[ "${mode}" == "minimal" ]]; then
    log "[${host}] mode=minimal, rebuild tee-a/tee-b instances"
    run_host "${host}" "
      set -euo pipefail
      test -x '${OCCLUM_BIN}'
      test -d '${REMOTE_MINIMAL_DIR}/tee-a-instance'
      test -d '${REMOTE_MINIMAL_DIR}/tee-b-instance'
      for inst in tee-a-instance tee-b-instance; do
        cd '${REMOTE_MINIMAL_DIR}'/\${inst}
        rm -rf run build
        '${OCCLUM_BIN}' build
      done
    "

    log "[${host}] verify minimal instances artifacts"
    run_host "${host}" "
      set -euo pipefail
      test -f '${REMOTE_MINIMAL_DIR}/tee-a-instance/Occlum.json'
      test -f '${REMOTE_MINIMAL_DIR}/tee-a-instance/.__occlum_status'
      test -f '${REMOTE_MINIMAL_DIR}/tee-b-instance/Occlum.json'
      test -f '${REMOTE_MINIMAL_DIR}/tee-b-instance/.__occlum_status'
      ls -la '${REMOTE_MINIMAL_DIR}'/tee-a-instance | sed -n '1,6p'
      ls -la '${REMOTE_MINIMAL_DIR}'/tee-b-instance | sed -n '1,6p'
    "
    return
  fi

  echo "[${host}] unsupported layout: need either ${REMOTE_REPO_DIR} repo mode or ${REMOTE_MINIMAL_DIR} minimal mode" >&2
  return 1
}

main() {
  IFS=',' read -r -a hosts <<< "${HOSTS_CSV}"
  if [[ "${#hosts[@]}" -lt 1 ]]; then
    echo "HOSTS_CSV must include at least one host, e.g. tee-a.example.internal,tee-b.example.internal" >&2
    exit 1
  fi

  for host in "${hosts[@]}"; do
    host="$(echo "${host}" | xargs)"
    [[ -n "${host}" ]] || continue
    deploy_one "${host}"
  done

  log "DONE: Occlum build artifacts are ready on ${HOSTS_CSV}"
}

main "$@"
