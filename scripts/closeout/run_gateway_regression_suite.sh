#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$HOME}"
TDID_DIR="${TDID_DIR:-${ROOT_DIR}/TDID}"
FISCO_SUITE_SCRIPT="${FISCO_SUITE_SCRIPT:-${TDID_DIR}/fisco/scripts/deploy_and_test_fisco_contract_suite.sh}"
PROOF_SIGNATURE_SCRIPT="${PROOF_SIGNATURE_SCRIPT:-${TDID_DIR}/fisco/scripts/verify-proof-signature.sh}"
COMMIT_BINDING_PROBE="${COMMIT_BINDING_PROBE:-${ROOT_DIR}/probe_fisco_commit_binding.sh}"

log() {
  echo "[$(date +'%F %T')] $*"
}

log "run gateway regression suite"
if [[ -x "${FISCO_SUITE_SCRIPT}" ]]; then
  bash "${FISCO_SUITE_SCRIPT}"
elif [[ -x "${ROOT_DIR}/deploy_and_test_fisco_contract_suite.sh" ]]; then
  bash "${ROOT_DIR}/deploy_and_test_fisco_contract_suite.sh"
else
  log "skip deploy_and_test_fisco_contract_suite.sh (not found)"
fi

log "run extra regression probes"
if [[ -x "${PROOF_SIGNATURE_SCRIPT}" ]]; then
  bash "${PROOF_SIGNATURE_SCRIPT}"
elif [[ -x "${ROOT_DIR}/verify-proof-signature.sh" ]]; then
  bash "${ROOT_DIR}/verify-proof-signature.sh"
else
  log "skip verify-proof-signature.sh (not found)"
fi

if [[ -x "${COMMIT_BINDING_PROBE}" ]]; then
  bash "${COMMIT_BINDING_PROBE}"
else
  log "skip probe_fisco_commit_binding.sh (not found)"
fi

log "gateway regression suite done"
