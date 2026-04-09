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
TRACE="trace-a3-fabric-${RID}"
KEY="0x1111111111111111111111111111111111111111111111111111111111111111"
SIG="0x2222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222"
EXP=$(( $(date +%s) + 3600 ))
TRANSFER="transfer-a3-fabric-${RID}"
SESSION="session-a3-fabric-${RID}"
NONCE=$(( RID % 100000 + 5000 ))

peer chaincode invoke -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile "$ORDERER_CA" -C "$CH" -n "$CC" --peerAddresses localhost:7051 --tlsRootCertFiles "$PEER0_ORG1_CA" --peerAddresses localhost:9051 --tlsRootCertFiles "$PEER0_ORG2_CA" --waitForEvent --waitForEventTimeout 30s -c "{\"function\":\"LockV2\",\"Args\":[\"$TRANSFER\",\"$SESSION\",\"$TRACE\",\"fisco\",\"USDT\",\"10\",\"alice\",\"bob\",\"$KEY\",\"$NONCE\",\"$EXP\",\"$SIG\"]}"

peer chaincode invoke -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile "$ORDERER_CA" -C "$CH" -n "$CC" --peerAddresses localhost:7051 --tlsRootCertFiles "$PEER0_ORG1_CA" --peerAddresses localhost:9051 --tlsRootCertFiles "$PEER0_ORG2_CA" --waitForEvent --waitForEventTimeout 30s -c "{\"function\":\"CommitV2\",\"Args\":[\"$TRACE\",\"$KEY\",\"tx-a3\",\"receipt-a3\",\"fisco\",\"hash-a3\"]}"

Q=$(peer chaincode query -C "$CH" -n "$CC" -c "{\"function\":\"GetTrace\",\"Args\":[\"$TRACE\"]}")
echo "[A3-FABRIC-TRACE] $Q"
R=$(peer chaincode invoke -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile "$ORDERER_CA" -C "$CH" -n "$CC" --peerAddresses localhost:7051 --tlsRootCertFiles "$PEER0_ORG1_CA" --peerAddresses localhost:9051 --tlsRootCertFiles "$PEER0_ORG2_CA" --waitForEvent --waitForEventTimeout 30s -c "{\"function\":\"RefundV2\",\"Args\":[\"$TRACE\",\"$KEY\"]}" 2>&1 || true)
echo "[A3-FABRIC-REFUND-AFTER-COMMIT] $R"
