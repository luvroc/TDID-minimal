#!/usr/bin/env bash
set -euo pipefail

# Managed release pipeline for tdid-remote-minimal.
# Goal:
# 1) template build params
# 2) template loader/lib dependencies
# 3) install start-time guard to block manual binary replacement

REMOTE_USER="${REMOTE_USER:-ecs-user}"
HOSTS_CSV="${HOSTS_CSV:-tee-a.example.internal,tee-b.example.internal}"
REMOTE_REPO_DIR="${REMOTE_REPO_DIR:-/opt/tdid/tdid-open-minimal/tee}"
REMOTE_MINIMAL_DIR="${REMOTE_MINIMAL_DIR:-/opt/tdid/tdid-remote-minimal}"
OCCLUM_BIN="${OCCLUM_BIN:-/opt/occlum/build/bin/occlum}"
OCCLUM_GO="${OCCLUM_GO:-/opt/occlum/toolchains/golang/bin/occlum-go}"

# MODE:
# - adopt-current: adopt existing minimal binary as managed baseline (safe default)
# - build-from-source: build from TDID-Final source before packaging
MODE="${MODE:-adopt-current}"
RELEASE_ID="${RELEASE_ID:-$(date +%Y%m%d%H%M%S)}"
REBUILD_INSTANCE="${REBUILD_INSTANCE:-1}"
RESTART_SERVICES="${RESTART_SERVICES:-1}"
# Safety gate:
# build-from-source is known to produce kernel-incompatible binaries on current minimal runtime
# unless toolchain/profile is explicitly pinned.
ALLOW_UNSTABLE_BUILD_FROM_SOURCE="${ALLOW_UNSTABLE_BUILD_FROM_SOURCE:-0}"

# Build params template (used when MODE=build-from-source)
BUILD_TARGET="${BUILD_TARGET:-./enclave/cmd/tee_enclave_service}"
BUILD_OUTPUT_BASENAME="${BUILD_OUTPUT_BASENAME:-tee_enclave_service}"
BUILD_ENV="${BUILD_ENV:-CC=/opt/occlum/toolchains/gcc/bin/occlum-gcc}"
BUILD_CMD="${BUILD_CMD:-${OCCLUM_GO} build}"
EXPECTED_INTERPRETER="${EXPECTED_INTERPRETER:-/lib/ld-musl-x86_64.so.1}"

SSH_OPTS=(
  -o BatchMode=yes
  -o ConnectTimeout=10
  -o StrictHostKeyChecking=accept-new
  -o ServerAliveInterval=15
  -o ServerAliveCountMax=6
  -o TCPKeepAlive=yes
)

log() {
  echo "[managed-release] $*"
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

deploy_host() {
  local host="$1"
  log "[${host}] start mode=${MODE} release=${RELEASE_ID}"

  run_host "${host}" "
set -euo pipefail

REMOTE_REPO_DIR='${REMOTE_REPO_DIR}'
REMOTE_MINIMAL_DIR='${REMOTE_MINIMAL_DIR}'
OCCLUM_BIN='${OCCLUM_BIN}'
OCCLUM_GO='${OCCLUM_GO}'
MODE='${MODE}'
RELEASE_ID='${RELEASE_ID}'
REBUILD_INSTANCE='${REBUILD_INSTANCE}'
RESTART_SERVICES='${RESTART_SERVICES}'
BUILD_TARGET='${BUILD_TARGET}'
BUILD_OUTPUT_BASENAME='${BUILD_OUTPUT_BASENAME}'
BUILD_ENV='${BUILD_ENV}'
BUILD_CMD='${BUILD_CMD}'
EXPECTED_INTERPRETER='${EXPECTED_INTERPRETER}'

test -d \"\${REMOTE_MINIMAL_DIR}/tee-a-instance/image/bin\"
test -d \"\${REMOTE_MINIMAL_DIR}/tee-b-instance/image/bin\"
test -x \"\${OCCLUM_BIN}\"

MANAGED_DIR=\"\${REMOTE_MINIMAL_DIR}/managed\"
RELEASE_DIR=\"\${MANAGED_DIR}/releases/\${RELEASE_ID}\"
CURRENT_DIR=\"\${MANAGED_DIR}/current\"
mkdir -p \"\${RELEASE_DIR}\" \"\${CURRENT_DIR}\"

SRC_BIN=\"\"
if [[ \"\${MODE}\" == 'build-from-source' ]]; then
  test -d \"\${REMOTE_REPO_DIR}\"
  test -x \"\${OCCLUM_GO}\"
  OUT_BIN=\"\${RELEASE_DIR}/\${BUILD_OUTPUT_BASENAME}\"
  # shellcheck disable=SC2086
  cd \"\${REMOTE_REPO_DIR}\" && eval \${BUILD_ENV} \${BUILD_CMD} -o \"\${OUT_BIN}\" \"\${BUILD_TARGET}\"
  SRC_BIN=\"\${OUT_BIN}\"
elif [[ \"\${MODE}\" == 'adopt-current' ]]; then
  SRC_BIN=\"\${REMOTE_MINIMAL_DIR}/tee-a-instance/image/bin/tee_enclave_service\"
  test -f \"\${SRC_BIN}\"
  cp -f \"\${SRC_BIN}\" \"\${RELEASE_DIR}/tee_enclave_service\"
  SRC_BIN=\"\${RELEASE_DIR}/tee_enclave_service\"
else
  echo \"unsupported MODE=\${MODE}, expected adopt-current|build-from-source\" >&2
  exit 1
fi

TARGET_BIN=\"\${RELEASE_DIR}/tee_enclave_service\"
if [[ \"\${SRC_BIN}\" != \"\${TARGET_BIN}\" ]]; then
  cp -f \"\${SRC_BIN}\" \"\${TARGET_BIN}\"
fi
chmod +x \"\${TARGET_BIN}\"

BIN_SHA=\"\$(sha256sum \"\${TARGET_BIN}\" | awk '{print \$1}')\"

INTERP=\"\$(readelf -l \"\${TARGET_BIN}\" 2>/dev/null | awk '/Requesting program interpreter/ {gsub(/\\[|\\]/, \"\", \$NF); print \$NF; exit}' || true)\"
if [[ \"\${MODE}\" == 'build-from-source' && \"\${INTERP}\" != \"\${EXPECTED_INTERPRETER}\" ]]; then
  echo \"build-from-source interpreter mismatch: expected=\${EXPECTED_INTERPRETER}, actual=\${INTERP:-<none>}\" >&2
  echo \"refusing release to avoid runtime breakage on minimal\" >&2
  exit 3
fi
DEPS_FILE=\"\${RELEASE_DIR}/deps.list\"
: > \"\${DEPS_FILE}\"
if [[ -n \"\${INTERP}\" ]]; then
  echo \"\${INTERP}\" >> \"\${DEPS_FILE}\"
fi
ldd \"\${TARGET_BIN}\" 2>/dev/null | awk '
  /=> \\// {print \$3}
  /^\\// {print \$1}
' | sort -u >> \"\${DEPS_FILE}\" || true
sort -u \"\${DEPS_FILE}\" -o \"\${DEPS_FILE}\"

cat > \"\${RELEASE_DIR}/manifest.env\" <<EOF
TEE_MANAGED_RELEASE_ID=\${RELEASE_ID}
TEE_MANAGED_MODE=\${MODE}
TEE_MANAGED_BIN_SHA256=\${BIN_SHA}
TEE_MANAGED_BUILD_ENV='\${BUILD_ENV}'
TEE_MANAGED_BUILD_CMD='\${BUILD_CMD}'
TEE_MANAGED_BUILD_TARGET='\${BUILD_TARGET}'
TEE_MANAGED_BUILD_OUTPUT='\${BUILD_OUTPUT_BASENAME}'
EOF

cp -f \"\${RELEASE_DIR}/manifest.env\" \"\${CURRENT_DIR}/manifest.env\"
cp -f \"\${TARGET_BIN}\" \"\${CURRENT_DIR}/tee_enclave_service\"
cp -f \"\${DEPS_FILE}\" \"\${CURRENT_DIR}/deps.list\"

sync_instance() {
  local inst_dir=\"\$1\"
  install -m 0755 \"\${TARGET_BIN}\" \"\${inst_dir}/image/bin/tee_enclave_service\"
  while IFS= read -r dep; do
    [[ -n \"\${dep}\" ]] || continue
    [[ -f \"\${dep}\" ]] || continue
    local dst=\"\${inst_dir}/image\${dep}\"
    mkdir -p \"\$(dirname \"\${dst}\")\"
    cp -f \"\${dep}\" \"\${dst}\"
  done < \"\${DEPS_FILE}\"
  local sha_now
  sha_now=\"\$(sha256sum \"\${inst_dir}/image/bin/tee_enclave_service\" | awk '{print \$1}')\"
  if [[ \"\${sha_now}\" != \"\${BIN_SHA}\" ]]; then
    echo \"managed binary sha mismatch in \${inst_dir}\" >&2
    exit 1
  fi
}

sync_instance \"\${REMOTE_MINIMAL_DIR}/tee-a-instance\"
sync_instance \"\${REMOTE_MINIMAL_DIR}/tee-b-instance\"

cat > \"\${MANAGED_DIR}/enforce_managed_release.sh\" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
THIS_DIR=\"\${1:?missing minimal root dir}\"
INSTANCE=\"\${2:?missing instance dir name}\"
MANIFEST=\"\${THIS_DIR}/managed/current/manifest.env\"
BIN=\"\${THIS_DIR}/\${INSTANCE}/image/bin/tee_enclave_service\"

if [[ ! -f \"\${MANIFEST}\" ]]; then
  echo \"[managed-guard] missing manifest: \${MANIFEST}\" >&2
  exit 1
fi
source \"\${MANIFEST}\"
if [[ -z \"\${TEE_MANAGED_BIN_SHA256:-}\" ]]; then
  echo \"[managed-guard] missing TEE_MANAGED_BIN_SHA256 in manifest\" >&2
  exit 1
fi
if [[ ! -f \"\${BIN}\" ]]; then
  echo \"[managed-guard] missing binary: \${BIN}\" >&2
  exit 1
fi
SHA_NOW=\"\$(sha256sum \"\${BIN}\" | awk '{print \$1}')\"
if [[ \"\${SHA_NOW}\" != \"\${TEE_MANAGED_BIN_SHA256}\" ]]; then
  echo \"[managed-guard] binary checksum mismatch. manual replacement is forbidden.\" >&2
  echo \"[managed-guard] expected=\${TEE_MANAGED_BIN_SHA256} actual=\${SHA_NOW}\" >&2
  exit 1
fi
EOF
chmod +x \"\${MANAGED_DIR}/enforce_managed_release.sh\"

patch_start() {
  local role=\"\$1\"
  local start_file=\"\${REMOTE_MINIMAL_DIR}/start-tee-\${role}.sh\"
  local marker='managed/enforce_managed_release.sh'
  local inject=\"\\\"\\\${THIS_DIR}/managed/enforce_managed_release.sh\\\" \\\"\\\${THIS_DIR}\\\" \\\"tee-\${role}-instance\\\"\"
  if ! grep -q \"\${marker}\" \"\${start_file}\"; then
    local tmp_file
    tmp_file=\"\$(mktemp)\"
    awk -v inject=\"\${inject}\" '
      BEGIN { inserted = 0 }
      {
        print \$0
        if (!inserted && \$1 == \"source\" && \$2 ~ /remote.env/) {
          print inject
          inserted = 1
        }
      }
      END {
        if (!inserted) {
          print inject
        }
      }
    ' \"\${start_file}\" > \"\${tmp_file}\"
    mv \"\${tmp_file}\" \"\${start_file}\"
  fi
}

patch_start a
patch_start b

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
  sleep 5

  curl -fsS \"http://127.0.0.1\${TEE_A_ADDR}/health\" >/dev/null
  curl -fsS \"http://127.0.0.1\${TEE_B_ADDR}/health\" >/dev/null
fi

echo \"managed release applied: host='${host}' release=\${RELEASE_ID} mode=\${MODE}\"
"
}

main() {
  case "${MODE}" in
    adopt-current|build-from-source) ;;
    *)
      echo "MODE must be adopt-current or build-from-source, got: ${MODE}" >&2
      exit 1
      ;;
  esac

  if [[ "${MODE}" == "build-from-source" && "${ALLOW_UNSTABLE_BUILD_FROM_SOURCE}" != "1" ]]; then
    echo "build-from-source is blocked by default on minimal runtime (known compatibility risk: kernel too old)." >&2
    echo "Set ALLOW_UNSTABLE_BUILD_FROM_SOURCE=1 only for controlled canary after toolchain compatibility is pinned." >&2
    exit 2
  fi

  IFS=',' read -r -a hosts <<< "${HOSTS_CSV}"
  [[ "${#hosts[@]}" -ge 1 ]] || { echo "HOSTS_CSV is empty"; exit 1; }

  for host in "${hosts[@]}"; do
    host="$(echo "${host}" | xargs)"
    [[ -n "${host}" ]] || continue
    deploy_host "${host}"
  done

  log "DONE: managed minimal release applied on ${HOSTS_CSV} (release=${RELEASE_ID}, mode=${MODE})"
}

main "$@"
