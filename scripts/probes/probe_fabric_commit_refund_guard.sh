#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="${ROOT_DIR:-$HOME}"
CHAIN_DOT_HOME="${CHAIN_DOT_HOME:-${ROOT_DIR}/chain-DOT}"
cd "${CHAIN_DOT_HOME}/test-network"
export PATH="${CHAIN_DOT_HOME}/bin:${PATH}"
export OVERRIDE_ORG=""
export VERBOSE="false"
export TEST_NETWORK_HOME="${CHAIN_DOT_HOME}/test-network"
export FABRIC_CFG_PATH="${CHAIN_DOT_HOME}/config"
source scripts/envVar.sh
setGlobals 1
CC=gatewaycc
CH=mychannel
RID=$(date +%s)
TRACE="trace-fabric-commit-refund-${RID}"
KEY="0x1111111111111111111111111111111111111111111111111111111111111111"
SIG="0x2222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222"
EXP=$(( $(date +%s) + 3600 ))
TRANSFER="transfer-fabric-commit-refund-${RID}"
SESSION="session-fabric-commit-refund-${RID}"
NONCE=$(( RID % 100000 + 5000 ))

peer chaincode invoke -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile "$ORDERER_CA" -C "$CH" -n "$CC" --peerAddresses localhost:7051 --tlsRootCertFiles "$PEER0_ORG1_CA" --peerAddresses localhost:9051 --tlsRootCertFiles "$PEER0_ORG2_CA" --waitForEvent --waitForEventTimeout 30s -c "{\"function\":\"LockV2\",\"Args\":[\"$TRANSFER\",\"$SESSION\",\"$TRACE\",\"fisco\",\"USDT\",\"10\",\"alice\",\"bob\",\"$KEY\",\"$NONCE\",\"$EXP\",\"$SIG\"]}"

peer chaincode invoke -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile "$ORDERER_CA" -C "$CH" -n "$CC" --peerAddresses localhost:7051 --tlsRootCertFiles "$PEER0_ORG1_CA" --peerAddresses localhost:9051 --tlsRootCertFiles "$PEER0_ORG2_CA" --waitForEvent --waitForEventTimeout 30s -c "{\"function\":\"CommitV2\",\"Args\":[\"$TRACE\",\"$KEY\",\"tx-commit-refund\",\"receipt-commit-refund\",\"fisco\",\"hash-commit-refund\"]}"

Q=$(peer chaincode query -C "$CH" -n "$CC" -c "{\"function\":\"GetTrace\",\"Args\":[\"$TRACE\"]}")
echo "[fabric-commit-refund-guard:trace] $Q"
R=$(peer chaincode invoke -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile "$ORDERER_CA" -C "$CH" -n "$CC" --peerAddresses localhost:7051 --tlsRootCertFiles "$PEER0_ORG1_CA" --peerAddresses localhost:9051 --tlsRootCertFiles "$PEER0_ORG2_CA" --waitForEvent --waitForEventTimeout 30s -c "{\"function\":\"RefundV2\",\"Args\":[\"$TRACE\",\"$KEY\"]}" 2>&1 || true)
echo "[fabric-commit-refund-guard:refund-after-commit] $R"
