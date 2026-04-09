#!/bin/bash
set -euo pipefail

ROOT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." >/dev/null 2>&1 && pwd )"
OCCLUM_BIN="${OCCLUM_BIN:-/opt/occlum/build/bin/occlum}"
ORIGIN_SERVER_IP="${1:-$(hostname -I | awk '{print $1}') }"
FORCE_REBUILD_SOURCE_INSTANCE="${FORCE_REBUILD_SOURCE_INSTANCE:-1}"
STAMP="$(date +%Y%m%d_%H%M%S)"

BUNDLE_DIR="/tmp/tdid-remote-minimal-${STAMP}"
PKG_DIR="${BUNDLE_DIR}/tdid-remote-minimal"
INSTANCE_SRC="${ROOT_DIR}/enclave/occlum/instance"

if [[ ! -x "${OCCLUM_BIN}" ]]; then
  echo "occlum not found at ${OCCLUM_BIN}"
  exit 1
fi

if [[ "${FORCE_REBUILD_SOURCE_INSTANCE}" == "1" ]]; then
  echo "rebuilding occlum instance from source config..."
  (cd "${ROOT_DIR}" && ./enclave/occlum/build.sh)
elif [[ ! -d "${INSTANCE_SRC}" ]]; then
  echo "occlum instance missing, building first..."
  (cd "${ROOT_DIR}" && ./enclave/occlum/build.sh)
fi

rm -rf "${BUNDLE_DIR}"
mkdir -p "${PKG_DIR}"

cp -a "${INSTANCE_SRC}" "${PKG_DIR}/tee-a-instance"
cp -a "${INSTANCE_SRC}" "${PKG_DIR}/tee-b-instance"

cat > "${PKG_DIR}/remote.env" <<EOF
# Remote deploy env
ORIGIN_SERVER_IP=${ORIGIN_SERVER_IP}
CHAIN_SERVER_IP=172.27.20.237
PEER_TEE_IP=172.27.20.236
TEE_A_ADDR=:18080
TEE_B_ADDR=:19080
TEE_A_STATE=state/tee-a/state.sealed
TEE_B_STATE=state/tee-b/state.sealed
TEE_A_SEAL_KEY=change-me-tee-a-seal-key
TEE_B_SEAL_KEY=change-me-tee-b-seal-key
TDID_T3_NO_SESSION_BASELINE=0

# chain endpoint examples (same subnet)
FISCO_RPC=http://172.27.20.237:8545
FABRIC_RPC=http://172.27.20.237:7050
AUDIT_RPC=http://172.27.20.237:9000
EOF

cat > "${PKG_DIR}/start-tee-b.sh" <<'EOF'
#!/bin/bash
set -euo pipefail
THIS_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
source "${THIS_DIR}/remote.env"
OCCLUM_BIN="${OCCLUM_BIN:-/opt/occlum/build/bin/occlum}"
STATE_PATH="${TEE_B_STATE}"
if [[ "${STATE_PATH}" != /* ]]; then
  STATE_PATH="${THIS_DIR}/${STATE_PATH}"
fi
mkdir -p "$(dirname "${STATE_PATH}")"
cd "${THIS_DIR}/tee-b-instance"
if [[ ! -f ".local_built" || "${FORCE_REBUILD_INSTANCE:-0}" == "1" ]]; then
  echo "[prep] rebuilding local occlum instance for tee-b"
  rm -rf run build
  "${OCCLUM_BIN}" build
  touch .local_built
fi
TEE_NODE_ID=tee-b \
TEE_NODE_ROLE=target \
TEE_PEER_ALLOWLIST=tee-a \
TEE_ENCLAVE_ADDR="${TEE_B_ADDR}" \
TEE_ENCLAVE_STATE_PATH="${STATE_PATH}" \
TEE_ENCLAVE_SEAL_KEY="${TEE_B_SEAL_KEY}" \
TDID_T3_NO_SESSION_BASELINE="${TDID_T3_NO_SESSION_BASELINE:-0}" \
/opt/occlum/build/bin/occlum run /bin/tee_enclave_service
EOF

cat > "${PKG_DIR}/start-tee-a.sh" <<'EOF'
#!/bin/bash
set -euo pipefail
THIS_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
source "${THIS_DIR}/remote.env"
OCCLUM_BIN="${OCCLUM_BIN:-/opt/occlum/build/bin/occlum}"
STATE_PATH="${TEE_A_STATE}"
if [[ "${STATE_PATH}" != /* ]]; then
  STATE_PATH="${THIS_DIR}/${STATE_PATH}"
fi
mkdir -p "$(dirname "${STATE_PATH}")"
cd "${THIS_DIR}/tee-a-instance"
if [[ ! -f ".local_built" || "${FORCE_REBUILD_INSTANCE:-0}" == "1" ]]; then
  echo "[prep] rebuilding local occlum instance for tee-a"
  rm -rf run build
  "${OCCLUM_BIN}" build
  touch .local_built
fi
TEE_NODE_ID=tee-a \
TEE_NODE_ROLE=source \
TEE_PEER_ALLOWLIST=tee-b \
TEE_ENCLAVE_ADDR="${TEE_A_ADDR}" \
TEE_ENCLAVE_STATE_PATH="${STATE_PATH}" \
TEE_ENCLAVE_SEAL_KEY="${TEE_A_SEAL_KEY}" \
TDID_T3_NO_SESSION_BASELINE="${TDID_T3_NO_SESSION_BASELINE:-0}" \
/opt/occlum/build/bin/occlum run /bin/tee_enclave_service
EOF

cat > "${PKG_DIR}/check-connectivity.sh" <<'EOF'
#!/bin/bash
set -euo pipefail
THIS_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
source "${THIS_DIR}/remote.env"

echo "[check] origin server: ${ORIGIN_SERVER_IP}"
echo "[check] chain server: ${CHAIN_SERVER_IP}"
echo "[check] peer tee server: ${PEER_TEE_IP}"
echo "[check] chain endpoints"
for url in "${FISCO_RPC}" "${FABRIC_RPC}" "${AUDIT_RPC}"; do
  timeout 2s bash -c "cat </dev/null >/dev/tcp/$(echo ${url#http://} | cut -d/ -f1 | cut -d: -f1)/$(echo ${url#http://} | cut -d/ -f1 | cut -d: -f2)" \
    && echo "  ok: ${url}" || echo "  fail: ${url}"
done

echo "[check] origin/peer tee sample ports"
for p in 18080 19080 28080 29090; do
  timeout 2s bash -c "cat </dev/null >/dev/tcp/${ORIGIN_SERVER_IP}/${p}" \
    && echo "  reachable: ${ORIGIN_SERVER_IP}:${p}" || echo "  closed/unreachable: ${ORIGIN_SERVER_IP}:${p}"
  if [[ "${PEER_TEE_IP}" != "${ORIGIN_SERVER_IP}" ]]; then
    timeout 2s bash -c "cat </dev/null >/dev/tcp/${PEER_TEE_IP}/${p}" \
      && echo "  reachable: ${PEER_TEE_IP}:${p}" || echo "  closed/unreachable: ${PEER_TEE_IP}:${p}"
  fi
done
EOF

cat > "${PKG_DIR}/run-dual.sh" <<'EOF'
#!/bin/bash
set -euo pipefail
THIS_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
source "${THIS_DIR}/remote.env"
SELF_IP="${SELF_IP:-$(hostname -I | awk '{print $1}') }"

addr_to_url() {
  local addr="$1"
  local host="$2"
  if [[ "$addr" == :* ]]; then
    echo "http://${host}${addr}"
  elif [[ "$addr" == http://* || "$addr" == https://* ]]; then
    echo "$addr"
  else
    echo "http://${host}:${addr}"
  fi
}

cleanup() {
  set +e
  [[ -n "${PID_B:-}" ]] && kill "${PID_B}" >/dev/null 2>&1 || true
  [[ -n "${PID_A:-}" ]] && kill "${PID_A}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "[run] start tee-b"
"${THIS_DIR}/start-tee-b.sh" >"${THIS_DIR}/tee-b.log" 2>&1 &
PID_B=$!
sleep 2

echo "[run] start tee-a"
"${THIS_DIR}/start-tee-a.sh" >"${THIS_DIR}/tee-a.log" 2>&1 &
PID_A=$!
sleep 3

echo "[run] local health check"
B_URL="$(addr_to_url "${TEE_B_ADDR}" "${SELF_IP}")"
A_URL="$(addr_to_url "${TEE_A_ADDR}" "${SELF_IP}")"
curl -fsS "${B_URL}/health" && echo
curl -fsS "${A_URL}/health" && echo
echo "[run] self ip used for health: ${SELF_IP}"

echo "[run] running, press Ctrl+C to stop"
wait
EOF

cat > "${PKG_DIR}/README.md" <<'EOF'
# TDID Remote Minimal Bundle

## Purpose
- Run both TEE-B and TEE-A on another TEE server (Occlum installed).
- Keep communication path to origin server and chain subnet endpoints.

## Files
- `tee-a-instance/`, `tee-b-instance/`: Occlum instances (first startup will rebuild locally)
- `remote.env`: runtime config
- `start-tee-b.sh`, `start-tee-a.sh`: single-node startup
- `run-dual.sh`: start B then A
- `check-connectivity.sh`: subnet connectivity checks

## On remote server
0. ask 236 operator to start probe services first (`./start-origin-probes.sh` in `~/TDID-Final/deploy`)
1. unpack tar.gz
2. edit `remote.env` (set chain endpoints and origin server ip if needed; `TDID_T3_NO_SESSION_BASELINE=1` 可打开 T3 no-session 运行态)
3. run: `chmod +x *.sh && ./check-connectivity.sh`
4. run: `./run-dual.sh`

## Notes
- refund semantic is kept as `refundV2` by deployment config.
- if SGX mode differs, set Occlum runtime env accordingly before run.
- if copied instance is incompatible across machines, local rebuild is automatic on first start.
EOF

chmod +x "${PKG_DIR}"/*.sh

TAR_PATH="/tmp/tdid-remote-minimal-${STAMP}.tar.gz"
tar -C "${BUNDLE_DIR}" -czf "${TAR_PATH}" "tdid-remote-minimal"

echo "bundle created: ${TAR_PATH}"
echo "bundle dir: ${PKG_DIR}"
