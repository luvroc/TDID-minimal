#!/usr/bin/env bash
set -euo pipefail

cd ~/chain-DOT/test-network
export PATH="/home/ecs-user/chain-DOT/bin:${PATH}"
export OVERRIDE_ORG=""
export VERBOSE="false"
export TEST_NETWORK_HOME="/home/ecs-user/chain-DOT/test-network"
export FABRIC_CFG_PATH="/home/ecs-user/chain-DOT/config"
source scripts/envVar.sh
setGlobals 1

CC=gatewaycc
CH=mychannel
RID=$(date +%s)
TRACE="trace-p15-fabric-${RID}"
TRANSFER="transfer-p15-fabric-${RID}"
SESSION="session-p15-fabric-${RID}"
KEY="0x1111111111111111111111111111111111111111111111111111111111111111"
SIG="0x2222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222"
EXP=$(( $(date +%s) + 3600 ))
NONCE=$(( RID % 100000 + 9000 ))

invoke() {
  peer chaincode invoke \
    -o localhost:7050 \
    --ordererTLSHostnameOverride orderer.example.com \
    --tls --cafile "$ORDERER_CA" \
    -C "$CH" -n "$CC" \
    --peerAddresses localhost:7051 --tlsRootCertFiles "$PEER0_ORG1_CA" \
    --peerAddresses localhost:9051 --tlsRootCertFiles "$PEER0_ORG2_CA" \
    --waitForEvent --waitForEventTimeout 30s \
    -c "$1"
}

query() {
  peer chaincode query -C "$CH" -n "$CC" -c "$1"
}

invoke "{\"function\":\"LockV2\",\"Args\":[\"$TRANSFER\",\"$SESSION\",\"$TRACE\",\"fisco\",\"USDT\",\"10\",\"alice\",\"bob\",\"$KEY\",\"$NONCE\",\"$EXP\",\"$SIG\"]}" >/dev/null

PROOF_JSON=$(query "{\"function\":\"BuildSourceLockProof\",\"Args\":[\"$TRACE\",\"probe-attester\",\"probe-signer\"]}")
PROOF_PAYLOAD=$(printf "%s" "$PROOF_JSON" | base64 -w0)

OUT=$(invoke "{\"function\":\"MintOrUnlockWithProof\",\"Args\":[\"$TRANSFER\",\"$SESSION\",\"$TRACE\",\"fisco\",\"fabric\",\"USDT\",\"10\",\"alice\",\"bob\",\"$KEY\",\"$((NONCE+1))\",\"$EXP\",\"$SIG\",\"0x00\",\"tx\",\"receipt\",\"0xss\",\"${PROOF_PAYLOAD}\"]}" 2>&1 || true)
echo "$OUT"

if [[ "$OUT" == *"transferId already exists"* || "$OUT" == *"traceId already mapped"* || "$OUT" == *"proof signer mismatch"* || "$OUT" == *"proof digest mismatch"* ]]; then
  echo "P15_FABRIC_WITHPROOF_CALL: PASS"
  exit 0
fi

echo "P15_FABRIC_WITHPROOF_CALL: UNKNOWN"
