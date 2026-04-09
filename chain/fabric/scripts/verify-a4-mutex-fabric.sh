#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="${ROOT_DIR:-$HOME}"
CHAIN_DOT_HOME="${CHAIN_DOT_HOME:-${ROOT_DIR}/chain-DOT}"
cd "${CHAIN_DOT_HOME}/test-network"
export PATH="${CHAIN_DOT_HOME}/bin:/usr/local/go/bin:${PATH}"
export OVERRIDE_ORG=""
export VERBOSE="false"
export TEST_NETWORK_HOME="${CHAIN_DOT_HOME}/test-network"
export FABRIC_CFG_PATH="${CHAIN_DOT_HOME}/config"
source scripts/envVar.sh
setGlobals 1

CC=gatewaycc
CH=mychannel
RID=$(date +%s%N)
KEY="0x1111111111111111111111111111111111111111111111111111111111111111"
SIG="0x2222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222"

invoke(){
  peer chaincode invoke -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile "$ORDERER_CA" -C "$CH" -n "$CC" --peerAddresses localhost:7051 --tlsRootCertFiles "$PEER0_ORG1_CA" --peerAddresses localhost:9051 --tlsRootCertFiles "$PEER0_ORG2_CA" --waitForEvent --waitForEventTimeout 30s -c "$1"
}

must_contain() {
  local output="$1"
  local expected="$2"
  local hint="$3"
  if [[ "$output" != *"$expected"* ]]; then
    echo "ASSERT FAIL: $hint"
    echo "Expected: $expected"
    echo "Actual:"
    echo "$output"
    exit 1
  fi
}

must_not_contain() {
  local output="$1"
  local bad="$2"
  local hint="$3"
  if [[ "$output" == *"$bad"* ]]; then
    echo "ASSERT FAIL: $hint"
    echo "Unexpected: $bad"
    echo "Actual:"
    echo "$output"
    exit 1
  fi
}

# 1) mint_then_refund_should_fail => lock -> commit -> refund fail
trace1="trace-a4-fabric-1-${RID}"
transfer1="transfer-a4-fabric-1-${RID}"
session1="session-a4-fabric-1-${RID}"
nonce1=$((RID % 900000000 + 6001))
exp1=$(( $(date +%s) + 3600 ))
invoke "{\"function\":\"LockV2\",\"Args\":[\"$transfer1\",\"$session1\",\"$trace1\",\"fisco\",\"USDT\",\"10\",\"alice\",\"bob\",\"$KEY\",\"$nonce1\",\"$exp1\",\"$SIG\"]}" >/tmp/a4_fabric_lock1.out
invoke "{\"function\":\"CommitV2\",\"Args\":[\"$trace1\",\"$KEY\",\"tx-a4-1\",\"receipt-a4-1\",\"fisco\",\"hash-a4-1\"]}" >/tmp/a4_fabric_commit1.out
out_refund_after_commit=$(invoke "{\"function\":\"RefundV2\",\"Args\":[\"$trace1\",\"$KEY\"]}" 2>&1 || true)
echo "[mint_then_refund_should_fail] $out_refund_after_commit"
must_not_contain "$out_refund_after_commit" "status:200" "refund must fail after commit"

# 2) refund_then_mint_should_fail => lock(short ttl) -> refund -> commit fail
trace2="trace-a4-fabric-2-${RID}"
transfer2="transfer-a4-fabric-2-${RID}"
session2="session-a4-fabric-2-${RID}"
nonce2=$((RID % 900000000 + 6002))
exp2=$(( $(date +%s) + 2 ))
invoke "{\"function\":\"LockV2\",\"Args\":[\"$transfer2\",\"$session2\",\"$trace2\",\"fisco\",\"USDT\",\"10\",\"alice\",\"bob\",\"$KEY\",\"$nonce2\",\"$exp2\",\"$SIG\"]}" >/tmp/a4_fabric_lock2.out
sleep 3
invoke "{\"function\":\"RefundV2\",\"Args\":[\"$trace2\",\"$KEY\"]}" >/tmp/a4_fabric_refund2.out
out_commit_after_refund=$(invoke "{\"function\":\"CommitV2\",\"Args\":[\"$trace2\",\"$KEY\",\"tx-a4-2\",\"receipt-a4-2\",\"fisco\",\"hash-a4-2\"]}" 2>&1 || true)
echo "[refund_then_mint_should_fail] $out_commit_after_refund"
must_not_contain "$out_commit_after_refund" "status:200" "commit must fail after refund"

# 3) stale_locked_proof_after_refund_should_fail => BuildSourceLockProof should fail after refund
out_build_after_refund=$(peer chaincode query -C "$CH" -n "$CC" -c "{\"function\":\"BuildSourceLockProof\",\"Args\":[\"$trace2\",\"temp-attester\",\"temp-signer\"]}" 2>&1 || true)
echo "[stale_locked_proof_after_refund_should_fail] $out_build_after_refund"
must_contain "$out_build_after_refund" "not in LOCKED state" "proof build should fail after refund"

echo "A4_FABRIC_MUTEX: PASS"
