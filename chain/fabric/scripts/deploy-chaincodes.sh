#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="${ROOT_DIR:-/home/ecs-user}"
TDID_DIR="${TDID_DIR:-${ROOT_DIR}/TDID}"
FABRIC_NET_DIR="${FABRIC_NET_DIR:-${ROOT_DIR}/chain-DOT/test-network}"
CHANNEL_NAME="${CHANNEL_NAME:-mychannel}"

CC_IDENTITY="${CC_IDENTITY:-tdid-identity-cc}"
CC_SESSION="${CC_SESSION:-tdid-session-cc}"
CC_GATEWAY="${CC_GATEWAY:-gatewaycc}"
CC_MOCK="${CC_MOCK:-mockverifiercc}"

CCP_IDENTITY="${TDID_DIR}/fabric/chaincode/tdid_identity"
CCP_SESSION="${TDID_DIR}/fabric/chaincode/tdid_session"
CCP_GATEWAY="${TDID_DIR}/fabric/chaincode/tdid_source_gateway"
CCP_MOCK="${TDID_DIR}/fabric/chaincode/common/mockverifier"

MODE="${1:-all}"

log() {
  echo "[$(date +'%F %T')] $*"
}

deploy_cc() {
  local cc_name="$1"
  local cc_path="$2"
  log "Deploy chaincode: ${cc_name} (${cc_path})"
  (
    cd "${FABRIC_NET_DIR}"
    ./network.sh deployCC -c "${CHANNEL_NAME}" -ccn "${cc_name}" -ccp "${cc_path}" -ccl go
  )
}

case "${MODE}" in
  all)
    deploy_cc "${CC_MOCK}" "${CCP_MOCK}"
    deploy_cc "${CC_IDENTITY}" "${CCP_IDENTITY}"
    deploy_cc "${CC_SESSION}" "${CCP_SESSION}"
    deploy_cc "${CC_GATEWAY}" "${CCP_GATEWAY}"
    ;;
  identity)
    deploy_cc "${CC_IDENTITY}" "${CCP_IDENTITY}"
    ;;
  session)
    deploy_cc "${CC_SESSION}" "${CCP_SESSION}"
    ;;
  gateway)
    deploy_cc "${CC_GATEWAY}" "${CCP_GATEWAY}"
    ;;
  mock)
    deploy_cc "${CC_MOCK}" "${CCP_MOCK}"
    ;;
  *)
    echo "Usage: $0 [all|identity|session|gateway|mock]"
    exit 1
    ;;
esac

log "Done: ${MODE}"
