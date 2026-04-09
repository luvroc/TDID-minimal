#!/usr/bin/env bash
set -euo pipefail

# A3 managed rollback for tdid-remote-minimal.
# Roll back managed/current and instance runtime to a specified release id.

REMOTE_USER="${REMOTE_USER:-ecs-user}"
HOSTS_CSV="${HOSTS_CSV:-tee-a.example.internal,tee-b.example.internal}"
REMOTE_MINIMAL_DIR="${REMOTE_MINIMAL_DIR:-/opt/tdid/tdid-remote-minimal}"
OCCLUM_BIN="${OCCLUM_BIN:-/opt/occlum/build/bin/occlum}"
TARGET_RELEASE_ID="${TARGET_RELEASE_ID:-}"
REBUILD_INSTANCE="${REBUILD_INSTANCE:-1}"
RESTART_SERVICES="${RESTART_SERVICES:-1}"
HEALTH_RETRIES="${HEALTH_RETRIES:-20}"
HEALTH_SLEEP_SECONDS="${HEALTH_SLEEP_SECONDS:-1}"

SSH_OPTS=(
  -o BatchMode=yes
  -o ConnectTimeout=10
  -o StrictHostKeyChecking=accept-new
  -o ServerAliveInterval=15
  -o ServerAliveCountMax=6
  -o TCPKeepAlive=yes
)

log() {
  echo "[a3-managed-rollback] $*"
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
  return 1
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

rollback_host() {
  local host="$1"
  log "[${host}] rollback to release=${TARGET_RELEASE_ID}"
  run_host "${host}" "
set -euo pipefail
REMOTE_MINIMAL_DIR='${REMOTE_MINIMAL_DIR}'
OCCLUM_BIN='${OCCLUM_BIN}'
TARGET_RELEASE_ID='${TARGET_RELEASE_ID}'
REBUILD_INSTANCE='${REBUILD_INSTANCE}'
RESTART_SERVICES='${RESTART_SERVICES}'
HEALTH_RETRIES='${HEALTH_RETRIES}'
HEALTH_SLEEP_SECONDS='${HEALTH_SLEEP_SECONDS}'

MANAGED_DIR=\"\${REMOTE_MINIMAL_DIR}/managed\"
REL_DIR=\"\${MANAGED_DIR}/releases/\${TARGET_RELEASE_ID}\"
CUR_DIR=\"\${MANAGED_DIR}/current\"
export CUR_DIR

test -d \"\${REL_DIR}\"
test -f \"\${REL_DIR}/manifest.env\"
test -f \"\${REL_DIR}/tee_enclave_service\"
test -f \"\${REL_DIR}/deps.list\"

cp -f \"\${REL_DIR}/manifest.env\" \"\${CUR_DIR}/manifest.env\"
cp -f \"\${REL_DIR}/tee_enclave_service\" \"\${CUR_DIR}/tee_enclave_service\"
cp -f \"\${REL_DIR}/deps.list\" \"\${CUR_DIR}/deps.list\"

# Normalize legacy manifest formatting to a source-safe canonical form.
normalize_manifest() {
  local mf=\"\${CUR_DIR}/manifest.env\"
  local tmp=\"\${mf}.tmp\"

  get_val() {
    local key=\"\$1\"
    awk -F= -v k=\"\${key}\" '\$1==k { print substr(\$0, index(\$0, \"=\")+1); exit }' \"\${mf}\"
  }

  strip_wrap() {
    local v=\"\$1\"
    if [[ \"\${v}\" =~ ^\\'(.*)\\'$ ]]; then
      printf '%s' \"\${BASH_REMATCH[1]}\"
      return
    fi
    if [[ \"\${v}\" =~ ^\\\"(.*)\\\"$ ]]; then
      printf '%s' \"\${BASH_REMATCH[1]}\"
      return
    fi
    printf '%s' \"\${v}\"
  }

  local release_id mode bin_sha build_env build_cmd build_target build_output
  release_id=\"\$(strip_wrap \"\$(get_val TEE_MANAGED_RELEASE_ID)\")\"
  mode=\"\$(strip_wrap \"\$(get_val TEE_MANAGED_MODE)\")\"
  bin_sha=\"\$(strip_wrap \"\$(get_val TEE_MANAGED_BIN_SHA256)\")\"
  build_env=\"\$(strip_wrap \"\$(get_val TEE_MANAGED_BUILD_ENV)\")\"
  build_cmd=\"\$(strip_wrap \"\$(get_val TEE_MANAGED_BUILD_CMD)\")\"
  build_target=\"\$(strip_wrap \"\$(get_val TEE_MANAGED_BUILD_TARGET)\")\"
  build_output=\"\$(strip_wrap \"\$(get_val TEE_MANAGED_BUILD_OUTPUT)\")\"

  {
    printf 'TEE_MANAGED_RELEASE_ID=%s\n' \"\${release_id}\"
    printf 'TEE_MANAGED_MODE=%s\n' \"\${mode}\"
    printf 'TEE_MANAGED_BIN_SHA256=%s\n' \"\${bin_sha}\"
    printf 'TEE_MANAGED_BUILD_ENV=%q\n' \"\${build_env}\"
    printf 'TEE_MANAGED_BUILD_CMD=%q\n' \"\${build_cmd}\"
    printf 'TEE_MANAGED_BUILD_TARGET=%q\n' \"\${build_target}\"
    printf 'TEE_MANAGED_BUILD_OUTPUT=%q\n' \"\${build_output}\"
  } > \"\${tmp}\"
  mv \"\${tmp}\" \"\${mf}\"
}

normalize_manifest

BIN_SHA=\"\$(awk -F= '/^TEE_MANAGED_BIN_SHA256=/{print \$2}' \"\${CUR_DIR}/manifest.env\")\"
if [[ -z \"\${BIN_SHA}\" ]]; then
  echo \"manifest missing TEE_MANAGED_BIN_SHA256\" >&2
  exit 1
fi

sync_instance() {
  local inst_dir=\"\$1\"
  install -m 0755 \"\${CUR_DIR}/tee_enclave_service\" \"\${inst_dir}/image/bin/tee_enclave_service\"
  while IFS= read -r dep; do
    [[ -n \"\${dep}\" ]] || continue
    [[ -f \"\${dep}\" ]] || continue
    local dst=\"\${inst_dir}/image\${dep}\"
    mkdir -p \"\$(dirname \"\${dst}\")\"
    cp -f \"\${dep}\" \"\${dst}\"
  done < \"\${CUR_DIR}/deps.list\"
  local sha_now
  sha_now=\"\$(sha256sum \"\${inst_dir}/image/bin/tee_enclave_service\" | awk '{print \$1}')\"
  if [[ \"\${sha_now}\" != \"\${BIN_SHA}\" ]]; then
    echo \"instance binary sha mismatch after rollback in \${inst_dir}\" >&2
    exit 1
  fi
}

sync_instance \"\${REMOTE_MINIMAL_DIR}/tee-a-instance\"
sync_instance \"\${REMOTE_MINIMAL_DIR}/tee-b-instance\"

if [[ \"\${REBUILD_INSTANCE}\" == '1' ]]; then
  for inst in tee-a-instance tee-b-instance; do
    cd \"\${REMOTE_MINIMAL_DIR}/\${inst}\"
    rm -rf run build
    \"\${OCCLUM_BIN}\" build
  done
fi

if [[ \"\${RESTART_SERVICES}\" == '1' ]]; then
  cd \"\${REMOTE_MINIMAL_DIR}\"
  source \"\${REMOTE_MINIMAL_DIR}/remote.env\"

  kill_by_addr() {
    local addr=\"\$1\"
    local port=\"\${addr#:}\"
    [[ -n \"\${port}\" ]] || return 0
    local pid
    pid=\"\$(ss -lntp 2>/dev/null | awk -v p=\":\${port} \" '
      index(\$0, p) {
        if (match(\$0, /pid=([0-9]+)/, m)) {
          print m[1]
          exit
        }
      }
    ' || true)\"
    if [[ -n \"\${pid}\" ]]; then
      kill \"\${pid}\" >/dev/null 2>&1 || true
    fi
  }

  kill_by_addr \"\${TEE_A_ADDR}\"
  kill_by_addr \"\${TEE_B_ADDR}\"

  setsid -f bash ./start-tee-a.sh > /tmp/tee_a_restart.log 2>&1
  setsid -f bash ./start-tee-b.sh > /tmp/tee_b_restart.log 2>&1

  wait_health() {
    local url=\"\$1\"
    local tries=\"\${HEALTH_RETRIES}\"
    while (( tries > 0 )); do
      if curl -fsS \"\${url}\" >/dev/null 2>&1; then
        return 0
      fi
      tries=\$((tries - 1))
      sleep \"\${HEALTH_SLEEP_SECONDS}\"
    done
    return 1
  }

  wait_health \"http://127.0.0.1\${TEE_A_ADDR}/health\"
  wait_health \"http://127.0.0.1\${TEE_B_ADDR}/health\"
fi

echo \"rollback applied: host='${host}' release=\${TARGET_RELEASE_ID}\"
"
}

main() {
  if [[ -z "${TARGET_RELEASE_ID}" ]]; then
    echo "TARGET_RELEASE_ID is required" >&2
    exit 1
  fi

  IFS=',' read -r -a hosts <<< "${HOSTS_CSV}"
  [[ "${#hosts[@]}" -ge 1 ]] || { echo "HOSTS_CSV is empty"; exit 1; }

  for host in "${hosts[@]}"; do
    host="$(echo "${host}" | xargs)"
    [[ -n "${host}" ]] || continue
    rollback_host "${host}"
  done

  log "DONE: rollback applied on ${HOSTS_CSV} -> ${TARGET_RELEASE_ID}"
}

main "$@"
