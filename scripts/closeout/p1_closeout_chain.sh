#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$HOME}"
TDID_DIR="${TDID_DIR:-${ROOT_DIR}/TDID}"
CHAIN_DOT_HOME="${CHAIN_DOT_HOME:-${ROOT_DIR}/chain-DOT}"

log() {
  echo "[$(date +'%F %T')] $*"
}

export PATH="/usr/local/go/bin:${CHAIN_DOT_HOME}/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

log "P1 closeout(chain): ensure gatewaycc_target deployed"
cd "${TDID_DIR}"
CC_GATEWAY="gatewaycc_target" /bin/bash fabric/scripts/deploy-chaincodes.sh gateway

log "P1 closeout(chain): run A4+ baseline and A2/A3 probes"
log "P1 closeout(chain): proof-service is NOT a required dependency for this closeout path"
bash "${ROOT_DIR}/a4_deploy_and_test_agent123_plus.sh"

log "P1 closeout(chain): run P1-5 availability/call probes"
bash "${ROOT_DIR}/p15_fabric_withproof_probe.sh"
bash "${ROOT_DIR}/p15_fabric_withproof_call_probe.sh"

log "P1 closeout(chain): run P1-5 positive dual-cc proof path"
bash "${ROOT_DIR}/p15_fabric_withproof_positive_dualcc.sh"

log "P1 closeout(chain): PASS"
