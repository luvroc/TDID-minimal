#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-/home/ecs-user}"
TEE_DIR="${TEE_DIR:-${ROOT_DIR}/TDID-Final}"

log() {
  echo "[$(date +'%F %T')] $*"
}

cd "${TEE_DIR}"

log "P1 closeout(tee): run A4/Stage9/B3 relay tests"
CC=/usr/bin/gcc /opt/occlum/toolchains/golang/bin/occlum-go test ./host/relay -run 'A4|Stage9|SourceWorker' -v

log "P1 closeout(tee): run VerifyPeerCrossMessage core tests"
CC=/usr/bin/gcc /opt/occlum/toolchains/golang/bin/occlum-go test ./enclave/core -run VerifyPeerCrossMessage -v

log "P1 closeout(tee): run host package regression"
CC=/usr/bin/gcc /opt/occlum/toolchains/golang/bin/occlum-go test ./host/... -count=1

log "P1 closeout(tee): PASS"
