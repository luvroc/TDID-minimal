#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="/home/ecs-user"
TDID_DIR="${ROOT_DIR}/TDID"

log() {
  echo "[$(date +'%F %T')] $*"
}

log "run baseline A4 suite"
bash "${ROOT_DIR}/a4_deploy_and_test_agent123.sh"

log "run extra regression probes"
if [[ -x "${TDID_DIR}/fisco/scripts/verify-proof-signature.sh" ]]; then
  bash "${TDID_DIR}/fisco/scripts/verify-proof-signature.sh"
elif [[ -x "${ROOT_DIR}/verify-proof-signature.sh" ]]; then
  bash "${ROOT_DIR}/verify-proof-signature.sh"
else
  log "skip verify-proof-signature.sh (not found)"
fi

if [[ -x "${ROOT_DIR}/p13_fisco_commit_binding_probe.sh" ]]; then
  bash "${ROOT_DIR}/p13_fisco_commit_binding_probe.sh"
else
  log "skip p13_fisco_commit_binding_probe.sh (not found)"
fi

log "A4+ regression suite done"
