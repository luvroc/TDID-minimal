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

OUT=$(peer chaincode invoke \
  -o localhost:7050 \
  --ordererTLSHostnameOverride orderer.example.com \
  --tls --cafile "$ORDERER_CA" \
  -C "$CH" -n "$CC" \
  --peerAddresses localhost:7051 --tlsRootCertFiles "$PEER0_ORG1_CA" \
  --peerAddresses localhost:9051 --tlsRootCertFiles "$PEER0_ORG2_CA" \
  --waitForEvent --waitForEventTimeout 20s \
  -c '{"function":"MintOrUnlockWithProof","Args":[]}' 2>&1 || true)

echo "$OUT"
if [[ "$OUT" == *"MintOrUnlockWithProof"* && "$OUT" == *"not found"* ]]; then
  echo "P15_FABRIC_WITHPROOF: MISSING"
  exit 0
fi

if [[ "$OUT" == *"Incorrect number of arguments"* || "$OUT" == *"provided 0, expected"* || "$OUT" == *"incorrect number of params"* ]]; then
  echo "P15_FABRIC_WITHPROOF: EXISTS_BUT_ARG_MISMATCH"
  exit 0
fi

echo "P15_FABRIC_WITHPROOF: UNKNOWN"
