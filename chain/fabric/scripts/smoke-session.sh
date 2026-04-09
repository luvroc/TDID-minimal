#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="${ROOT_DIR:-/home/ecs-user}"
FABRIC_NET_DIR="${FABRIC_NET_DIR:-${ROOT_DIR}/chain-DOT/test-network}"
CHANNEL_NAME="${CHANNEL_NAME:-mychannel}"
SESSION_CC="${SESSION_CC:-tdid-session-cc}"
DUMMY_KEY_ID="${DUMMY_KEY_ID:-0x1111111111111111111111111111111111111111111111111111111111111111}"

log() {
  echo "[$(date +'%F %T')] $*"
}

query_session_active() {
  (
    cd "${FABRIC_NET_DIR}"
    export PATH="${ROOT_DIR}/chain-DOT/bin:${PATH}"
    export OVERRIDE_ORG=""
    export VERBOSE="false"
    export TEST_NETWORK_HOME="${FABRIC_NET_DIR}"
    export FABRIC_CFG_PATH="${FABRIC_NET_DIR}/../config"
    source scripts/envVar.sh
    setGlobals 1
    peer chaincode query -C "${CHANNEL_NAME}" -n "${SESSION_CC}" -c "{\"function\":\"IsSessionActive\",\"Args\":[\"${DUMMY_KEY_ID}\"]}"
  )
}

log "Session smoke: query IsSessionActive"
out="$(query_session_active || true)"
echo "${out}"
if [[ "${out}" != *"false"* ]]; then
  echo "Session smoke failed: expected false"
  exit 1
fi
log "Session smoke passed"
