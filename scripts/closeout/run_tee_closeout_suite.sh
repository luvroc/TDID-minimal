#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$HOME}"
TEE_DIR="${TEE_DIR:-${ROOT_DIR}/TDID-Final}"

log() {
  echo "[$(date +'%F %T')] $*"
}

cd "${TEE_DIR}"

log "tee closeout: run relay package regression and source-worker tests"
CC=/usr/bin/gcc /opt/occlum/toolchains/golang/bin/occlum-go test ./host/relay -count=1 -v

log "tee closeout: run VerifyPeerCrossMessage core tests"
CC=/usr/bin/gcc /opt/occlum/toolchains/golang/bin/occlum-go test ./enclave/core -run VerifyPeerCrossMessage -v

log "tee closeout: run host package regression"
CC=/usr/bin/gcc /opt/occlum/toolchains/golang/bin/occlum-go test ./host/... -count=1

log "tee closeout: PASS"
