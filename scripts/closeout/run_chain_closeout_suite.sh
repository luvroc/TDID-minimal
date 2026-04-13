#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$HOME}"
TDID_DIR="${TDID_DIR:-${ROOT_DIR}/TDID}"
CHAIN_DOT_HOME="${CHAIN_DOT_HOME:-${ROOT_DIR}/chain-DOT}"

log() {
  echo "[$(date +'%F %T')] $*"
}

export PATH="/usr/local/go/bin:${CHAIN_DOT_HOME}/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

log "chain closeout: ensure gateway target chaincode deployed"
cd "${TDID_DIR}"
CC_GATEWAY="gatewaycc_target" /bin/bash fabric/scripts/deploy-chaincodes.sh gateway

log "chain closeout: run gateway regression suite"
log "chain closeout: proof-service is NOT a required dependency for this closeout path"
bash "${ROOT_DIR}/run_gateway_regression_suite.sh"

log "chain closeout: run with-proof availability and call-path probes"
bash "${ROOT_DIR}/probe_fabric_withproof_availability.sh"
bash "${ROOT_DIR}/probe_fabric_withproof_call_path.sh"

log "chain closeout: run dual-gateway with-proof flow probe"
bash "${ROOT_DIR}/probe_fabric_withproof_dual_gateway_flow.sh"

log "chain closeout: PASS"
