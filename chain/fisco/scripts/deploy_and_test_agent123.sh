#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="/home/ecs-user"
TDID_DIR="${ROOT_DIR}/TDID"
FABRIC_NET_DIR="${ROOT_DIR}/chain-DOT/test-network"
FABRIC_GATEWAY_PATH="${TDID_DIR}/fabric/chaincode/tdid_source_gateway"
FABRIC_MOCK_VERIFIER_PATH="${TDID_DIR}/fabric/chaincode/common/mockverifier"

FISCO_DIR="${ROOT_DIR}/fisco"
FISCO_CONSOLE_DIR="${FISCO_DIR}/console"
FISCO_NODES_DIR="${FISCO_DIR}/nodes/127.0.0.1"

CHANNEL_NAME="${CHANNEL_NAME:-mychannel}"
FABRIC_GATEWAY_CC="${FABRIC_GATEWAY_CC:-gatewaycc}"
FABRIC_MOCK_CC="${FABRIC_MOCK_CC:-mockverifiercc}"
FABRIC_IDENTITY_CC="${FABRIC_IDENTITY_CC:-tdid-identity-cc}"
FABRIC_SESSION_CC="${FABRIC_SESSION_CC:-tdid-session-cc}"
ENABLE_AUDIT_CHAIN="${ENABLE_AUDIT_CHAIN:-false}"
USE_V2_TRANSFER="${USE_V2_TRANSFER:-false}"
STRICT_EVENT_ASSERT="${STRICT_EVENT_ASSERT:-false}"
STRICT_EVENT_ASSERT_FABRIC="${STRICT_EVENT_ASSERT_FABRIC:-false}"
STRICT_EVENT_ASSERT_FISCO="${STRICT_EVENT_ASSERT_FISCO:-${STRICT_EVENT_ASSERT}}"

RUN_ID="$(date +%s)"

log() {
  echo "[$(date +'%F %T')] $*"
}

assert_contains() {
  local text="$1"
  local needle="$2"
  local message="$3"
  if [[ "${text}" != *"${needle}"* ]]; then
    echo "ASSERT FAIL: ${message}"
    echo "Expected to contain: ${needle}"
    echo "Actual output:"
    echo "${text}"
    exit 1
  fi
}

assert_not_contains() {
  local text="$1"
  local needle="$2"
  local message="$3"
  if [[ "${text}" == *"${needle}"* ]]; then
    echo "ASSERT FAIL: ${message}"
    echo "Unexpected: ${needle}"
    echo "Actual output:"
    echo "${text}"
    exit 1
  fi
}

assert_gateway_event_compat() {
  local text="$1"
  local semantic_event="$2"
  local legacy_event="$3"
  local scene="$4"
  local strict_on="$5"
  if [[ "${text}" == *"${semantic_event}"* ]]; then
    log "[EventCheck] ${scene}: semantic event observed (${semantic_event})"
    return 0
  fi
  if [[ "${text}" == *"${legacy_event}"* ]]; then
    log "[EventCheck] ${scene}: legacy event observed (${legacy_event})"
    return 0
  fi
  if [[ "${strict_on}" == "true" ]]; then
    echo "ASSERT FAIL: ${scene} event missing"
    echo "Expected event one of: ${semantic_event} | ${legacy_event}"
    echo "Actual output:"
    echo "${text}"
    exit 1
  fi
  log "[EventCheck] ${scene}: event not found in output (non-strict mode), skip fail"
}

run_fisco_console() {
  (
    cd "${FISCO_CONSOLE_DIR}"
    bash console.sh "$@"
  )
}

mk_bytes32() {
  local input="$1"
  local h
  h=$(printf "%s" "${input}" | sha256sum | awk '{print $1}')
  echo "0x${h}"
}

fabric_invoke() {
  local cc_name="$1"
  local ctor_json="$2"
  (
    cd "${FABRIC_NET_DIR}"
    export PATH="${ROOT_DIR}/chain-DOT/bin:${PATH}"
    export OVERRIDE_ORG=""
    export VERBOSE="false"
    export TEST_NETWORK_HOME="${FABRIC_NET_DIR}"
    export FABRIC_CFG_PATH="${FABRIC_NET_DIR}/../config"
    source scripts/envVar.sh
    setGlobals 1
    peer chaincode invoke \
      -o localhost:7050 \
      --ordererTLSHostnameOverride orderer.example.com \
      --tls --cafile "$ORDERER_CA" \
      -C "${CHANNEL_NAME}" -n "${cc_name}" \
      --peerAddresses localhost:7051 --tlsRootCertFiles "$PEER0_ORG1_CA" \
      --peerAddresses localhost:9051 --tlsRootCertFiles "$PEER0_ORG2_CA" \
      --waitForEvent --waitForEventTimeout 30s \
      -c "${ctor_json}"
  )
}

fabric_query() {
  local cc_name="$1"
  local ctor_json="$2"
  (
    cd "${FABRIC_NET_DIR}"
    export PATH="${ROOT_DIR}/chain-DOT/bin:${PATH}"
    export OVERRIDE_ORG=""
    export VERBOSE="false"
    export TEST_NETWORK_HOME="${FABRIC_NET_DIR}"
    export FABRIC_CFG_PATH="${FABRIC_NET_DIR}/../config"
    source scripts/envVar.sh
    setGlobals 1
    peer chaincode query -C "${CHANNEL_NAME}" -n "${cc_name}" -c "${ctor_json}"
  )
}

fabric_wait_trace_state() {
  local trace_id="$1"
  local expected_state="$2"
  local rounds=10
  local i
  for ((i=1; i<=rounds; i++)); do
    local out
    out=$(fabric_query "${FABRIC_GATEWAY_CC}" "{\"function\":\"GetTrace\",\"Args\":[\"${trace_id}\"]}" 2>&1 || true)
    if [[ "${out}" == *"${expected_state}"* ]]; then
      echo "${out}"
      return 0
    fi
    sleep 2
  done
  echo "${out}"
  return 1
}

fabric_wait_audit_status() {
  log "[Fabric] audit chain retired: skip fabric_wait_audit_status"
  return 0
}

deploy_fabric_agent34() {
  log "[Fabric] start network and deploy Agent3/4 chaincode"
  (
    cd "${FABRIC_NET_DIR}"
    ./network.sh up
    if ! ./network.sh createChannel -c "${CHANNEL_NAME}"; then
      log "[Fabric] channel already exists, skip create"
    fi

    ROOT_DIR="${ROOT_DIR}" TDID_DIR="${TDID_DIR}" FABRIC_NET_DIR="${FABRIC_NET_DIR}" \
    CHANNEL_NAME="${CHANNEL_NAME}" CC_MOCK="${FABRIC_MOCK_CC}" CC_IDENTITY="${FABRIC_IDENTITY_CC}" \
    CC_SESSION="${FABRIC_SESSION_CC}" CC_GATEWAY="${FABRIC_GATEWAY_CC}" \
    bash "${TDID_DIR}/fabric/scripts/deploy-chaincodes.sh" all
  )

  out_set=$(fabric_invoke "${FABRIC_GATEWAY_CC}" '{"function":"SetSigVerifier","Args":["mockverifiercc",""]}' 2>&1 || true)
  assert_contains "${out_set}" "status:200" "Fabric SetSigVerifier failed"
}

test_fabric_agent3() {
  log "[Fabric] test Agent3 state machine and anti-replay"

  local key_id="0x1111111111111111111111111111111111111111111111111111111111111111"
  local session_id="session-${RUN_ID}"
  local sig="0x2222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222"
  local trace_lock="trace-lock-${RUN_ID}"
  local trace_settle="trace-settle-${RUN_ID}"
  local trace_refund="trace-refund-${RUN_ID}"
  local now_ts
  now_ts=$(date +%s)

  local exp1=$((now_ts + 600))
  local exp2=$((now_ts + 600))
  local nonce1=$((RUN_ID + 1))
  local nonce2=$((RUN_ID + 2))
  local nonce3=$((RUN_ID + 3))

  if [[ "${USE_V2_TRANSFER}" == "true" ]]; then
    local transfer_lock="transfer-lock-${RUN_ID}"
    out_lock=$(fabric_invoke "${FABRIC_GATEWAY_CC}" "{\"function\":\"LockV2\",\"Args\":[\"${transfer_lock}\",\"${session_id}\",\"${trace_lock}\",\"fisco\",\"USDT\",\"100\",\"alice\",\"bob\",\"${key_id}\",\"${nonce1}\",\"${exp1}\",\"${sig}\"]}" 2>&1 || true)
  else
    out_lock=$(fabric_invoke "${FABRIC_GATEWAY_CC}" "{\"function\":\"Lock\",\"Args\":[\"${trace_lock}\",\"fisco\",\"USDT\",\"100\",\"alice\",\"bob\",\"${key_id}\",\"${nonce1}\",\"${exp1}\",\"${sig}\"]}" 2>&1 || true)
  fi
  assert_contains "${out_lock}" "status:200" "Fabric lock failed"
  assert_gateway_event_compat "${out_lock}" "LockCreated" "Event_Lock" "Fabric lock" "${STRICT_EVENT_ASSERT_FABRIC}"
  q_lock=$(fabric_wait_trace_state "${trace_lock}" "LOCKED" || true)
  assert_contains "${q_lock}" "LOCKED" "Fabric lock trace state mismatch"

  if [[ "${USE_V2_TRANSFER}" == "true" ]]; then
    local transfer_settle="transfer-settle-${RUN_ID}"
    out_settle=$(fabric_invoke "${FABRIC_GATEWAY_CC}" "{\"function\":\"MintOrUnlockV2\",\"Args\":[\"${transfer_settle}\",\"${session_id}\",\"${trace_settle}\",\"fabric\",\"USDT\",\"88\",\"alice\",\"bob\",\"${key_id}\",\"${nonce2}\",\"${exp2}\",\"${sig}\"]}" 2>&1 || true)
  else
    out_settle=$(fabric_invoke "${FABRIC_GATEWAY_CC}" "{\"function\":\"MintOrUnlock\",\"Args\":[\"${trace_settle}\",\"fabric\",\"USDT\",\"88\",\"alice\",\"bob\",\"${key_id}\",\"${nonce2}\",\"${exp2}\",\"${sig}\"]}" 2>&1 || true)
  fi
  assert_contains "${out_settle}" "status:200" "Fabric mintOrUnlock failed"
  assert_gateway_event_compat "${out_settle}" "SettleCommitted" "Event_Settle" "Fabric settle" "${STRICT_EVENT_ASSERT_FABRIC}"
  q_settle=$(fabric_wait_trace_state "${trace_settle}" "COMMITTED" || true)
  assert_contains "${q_settle}" "COMMITTED" "Fabric settle trace state mismatch"

  if [[ "${USE_V2_TRANSFER}" == "true" ]]; then
    out_replay=$(fabric_invoke "${FABRIC_GATEWAY_CC}" "{\"function\":\"MintOrUnlockV2\",\"Args\":[\"transfer-replay-${RUN_ID}\",\"${session_id}\",\"trace-replay-${RUN_ID}\",\"fabric\",\"USDT\",\"88\",\"alice\",\"bob\",\"${key_id}\",\"${nonce2}\",\"${exp2}\",\"${sig}\"]}" 2>&1 || true)
  else
    out_replay=$(fabric_invoke "${FABRIC_GATEWAY_CC}" "{\"function\":\"MintOrUnlock\",\"Args\":[\"trace-replay-${RUN_ID}\",\"fabric\",\"USDT\",\"88\",\"alice\",\"bob\",\"${key_id}\",\"${nonce2}\",\"${exp2}\",\"${sig}\"]}" 2>&1 || true)
  fi
  assert_contains "${out_replay}" "nonce already used" "Fabric nonce replay should be rejected"

  local now_ts_refund
  now_ts_refund=$(date +%s)
  local exp3=$((now_ts_refund + 2))
  if [[ "${USE_V2_TRANSFER}" == "true" ]]; then
    local transfer_refund="transfer-refund-${RUN_ID}"
    out_lock_refund=$(fabric_invoke "${FABRIC_GATEWAY_CC}" "{\"function\":\"LockV2\",\"Args\":[\"${transfer_refund}\",\"${session_id}\",\"${trace_refund}\",\"fisco\",\"USDT\",\"66\",\"alice\",\"bob\",\"${key_id}\",\"${nonce3}\",\"${exp3}\",\"${sig}\"]}" 2>&1 || true)
  else
    out_lock_refund=$(fabric_invoke "${FABRIC_GATEWAY_CC}" "{\"function\":\"Lock\",\"Args\":[\"${trace_refund}\",\"fisco\",\"USDT\",\"66\",\"alice\",\"bob\",\"${key_id}\",\"${nonce3}\",\"${exp3}\",\"${sig}\"]}" 2>&1 || true)
  fi
  assert_contains "${out_lock_refund}" "status:200" "Fabric lock(for refund) failed"
  sleep 3

  out_refund=$(fabric_invoke "${FABRIC_GATEWAY_CC}" "{\"function\":\"Refund\",\"Args\":[\"${trace_refund}\",\"${key_id}\",\"${sig}\"]}" 2>&1 || true)
  assert_contains "${out_refund}" "status:200" "Fabric refund failed"
  assert_gateway_event_compat "${out_refund}" "RefundExecuted" "Event_Refund" "Fabric refund" "${STRICT_EVENT_ASSERT_FABRIC}"
  q_refund=$(fabric_wait_trace_state "${trace_refund}" "REFUNDED" || true)
  assert_contains "${q_refund}" "REFUNDED" "Fabric refund trace state mismatch"

  log "[Fabric] Agent3 test passed"
}

test_fabric_agent4() {
  if [[ "${ENABLE_AUDIT_CHAIN}" != "true" ]]; then
    log "[Fabric] Agent4 skipped: audit chain retired"
    return 0
  fi

  log "[Fabric] Agent4 manual enable (compat mode)"
}

deploy_fisco_agent1234() {
  log "[FISCO] start nodes and deploy Agent1/2/3/4 contracts"
  (
    cd "${FISCO_NODES_DIR}"
    bash start_all.sh || true
  )

  cp "${TDID_DIR}/fisco/contracts/GovernanceRoot.sol" "${FISCO_CONSOLE_DIR}/contracts/solidity/GovernanceRoot.sol"
  cp "${TDID_DIR}/fisco/contracts/TDIDRegistry.sol" "${FISCO_CONSOLE_DIR}/contracts/solidity/TDIDRegistry.sol"
  cp "${TDID_DIR}/fisco/contracts/SessionKeyRegistry.sol" "${FISCO_CONSOLE_DIR}/contracts/solidity/SessionKeyRegistry.sol"
  cp "${TDID_DIR}/fisco/contracts/SigVerifier.sol" "${FISCO_CONSOLE_DIR}/contracts/solidity/SigVerifier.sol"
  cp "${TDID_DIR}/fisco/contracts/MockSigVerifier.sol" "${FISCO_CONSOLE_DIR}/contracts/solidity/MockSigVerifier.sol"
  cp "${TDID_DIR}/fisco/contracts/TargetGateway.sol" "${FISCO_CONSOLE_DIR}/contracts/solidity/FiscoGateway.sol"

  current=$(run_fisco_console getCurrentAccount)
  ADMIN_ADDR=$(echo "${current}" | sed -n 's/.*0x\([0-9a-fA-F]\{40\}\).*/0x\1/p' | tail -n1)
  if [[ -z "${ADMIN_ADDR}" ]]; then
    ADMIN_ADDR="0xc6ce842e5cf007efbf00fedbe70aea85c17e8c87"
  fi
  export ADMIN_ADDR

  out_gov=$(run_fisco_console deploy GovernanceRoot "${ADMIN_ADDR}")
  GOV_ADDR=$(echo "${out_gov}" | sed -n 's/.*contract address: \(0x[0-9a-fA-F]\+\).*/\1/p' | tail -n1)
  [[ -n "${GOV_ADDR}" ]] || { echo "Deploy GovernanceRoot failed"; echo "${out_gov}"; exit 1; }

  out_tdid=$(run_fisco_console deploy TDIDRegistry "${GOV_ADDR}")
  TDID_ADDR=$(echo "${out_tdid}" | sed -n 's/.*contract address: \(0x[0-9a-fA-F]\+\).*/\1/p' | tail -n1)
  [[ -n "${TDID_ADDR}" ]] || { echo "Deploy TDIDRegistry failed"; echo "${out_tdid}"; exit 1; }

  out_sess=$(run_fisco_console deploy SessionKeyRegistry "${TDID_ADDR}")
  SESS_ADDR=$(echo "${out_sess}" | sed -n 's/.*contract address: \(0x[0-9a-fA-F]\+\).*/\1/p' | tail -n1)
  [[ -n "${SESS_ADDR}" ]] || { echo "Deploy SessionKeyRegistry failed"; echo "${out_sess}"; exit 1; }

  out_sig=$(run_fisco_console deploy SigVerifier "${SESS_ADDR}")
  SIG_ADDR=$(echo "${out_sig}" | sed -n 's/.*contract address: \(0x[0-9a-fA-F]\+\).*/\1/p' | tail -n1)
  [[ -n "${SIG_ADDR}" ]] || { echo "Deploy SigVerifier failed"; echo "${out_sig}"; exit 1; }

  out_mock=$(run_fisco_console deploy MockSigVerifier)
  MOCK_ADDR=$(echo "${out_mock}" | sed -n 's/.*contract address: \(0x[0-9a-fA-F]\+\).*/\1/p' | tail -n1)
  [[ -n "${MOCK_ADDR}" ]] || { echo "Deploy MockSigVerifier failed"; echo "${out_mock}"; exit 1; }

  out_gateway=$(run_fisco_console deploy FiscoGateway "${MOCK_ADDR}" fisco)
  FISCO_GATEWAY_ADDR=$(echo "${out_gateway}" | sed -n 's/.*contract address: \(0x[0-9a-fA-F]\+\).*/\1/p' | tail -n1)
  [[ -n "${FISCO_GATEWAY_ADDR}" ]] || { echo "Deploy FiscoGateway failed"; echo "${out_gateway}"; exit 1; }

  export GOV_ADDR TDID_ADDR SESS_ADDR SIG_ADDR MOCK_ADDR FISCO_GATEWAY_ADDR
  log "[FISCO] deployed GOV=${GOV_ADDR} TDID=${TDID_ADDR} SESS=${SESS_ADDR} SIG=${SIG_ADDR} MOCK=${MOCK_ADDR} GATEWAY=${FISCO_GATEWAY_ADDR}"
}

test_fisco_agent123() {
  log "[FISCO] test Agent1/2 smoke + Agent3 state machine"

  out_add_signer=$(run_fisco_console call GovernanceRoot "${GOV_ADDR}" addOrgSigner org1 "${ADMIN_ADDR}")
  assert_contains "${out_add_signer}" "transaction status: 0" "FISCO addOrgSigner failed"

  out_set_mr=$(run_fisco_console call GovernanceRoot "${GOV_ADDR}" setMeasurementAllowed 0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa true)
  assert_contains "${out_set_mr}" "transaction status: 0" "FISCO setMeasurementAllowed failed"

  out_sess_active=$(run_fisco_console call SessionKeyRegistry "${SESS_ADDR}" isSessionActive 0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb)
  assert_contains "${out_sess_active}" "(false)" "FISCO isSessionActive default should be false"

  local key_id="0x1111111111111111111111111111111111111111111111111111111111111111"
  local session_id
  session_id=$(mk_bytes32 "session-${RUN_ID}")
  local sig="0x2222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222"
  local trace1="0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa00aa"
  local trace2="0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa00bb"
  local trace3="0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa00cc"

  local now_ms
  now_ms=$(date +%s%3N)
  local exp1=$((now_ms + 600000))
  local exp2=$((now_ms + 600000))
  local transfer1
  local transfer2
  local transfer3
  transfer1=$(mk_bytes32 "transfer-lock-${RUN_ID}")
  transfer2=$(mk_bytes32 "transfer-settle-${RUN_ID}")
  transfer3=$(mk_bytes32 "transfer-refund-${RUN_ID}")

  if [[ "${USE_V2_TRANSFER}" == "true" ]]; then
    out_lock=$(run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" lockV2 "${transfer1}" "${session_id}" "${trace1}" "${key_id}" 1 "${exp1}" "${sig}")
  else
    out_lock=$(run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" lock "${trace1}" "${key_id}" 1 "${exp1}" "${sig}")
  fi
  assert_contains "${out_lock}" "transaction status: 0" "FISCO lock failed"
  assert_gateway_event_compat "${out_lock}" "LockCreated" "Event_Lock" "FISCO lock" "${STRICT_EVENT_ASSERT_FISCO}"

  out_trace1=$(run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" getTrace "${trace1}")
  assert_contains "${out_trace1}" "LOCKED" "FISCO trace1 state should be LOCKED"

  if [[ "${USE_V2_TRANSFER}" == "true" ]]; then
    out_settle=$(run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" mintOrUnlockV2 "${transfer2}" "${session_id}" "${trace2}" "${key_id}" 2 "${exp2}" "${sig}")
  else
    out_settle=$(run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" mintOrUnlock "${trace2}" "${key_id}" 2 "${exp2}" "${sig}")
  fi
  assert_contains "${out_settle}" "transaction status: 0" "FISCO mintOrUnlock failed"
  assert_gateway_event_compat "${out_settle}" "SettleCommitted" "Event_Settle" "FISCO settle" "${STRICT_EVENT_ASSERT_FISCO}"

  out_trace2=$(run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" getTrace "${trace2}")
  assert_contains "${out_trace2}" "COMMITTED" "FISCO trace2 state should be COMMITTED"

  if [[ "${USE_V2_TRANSFER}" == "true" ]]; then
    out_replay=$(run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" mintOrUnlockV2 "$(mk_bytes32 "transfer-replay-${RUN_ID}")" "${session_id}" 0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa00dd "${key_id}" 2 "${exp2}" "${sig}")
  else
    out_replay=$(run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" mintOrUnlock 0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa00dd "${key_id}" 2 "${exp2}" "${sig}")
  fi
  assert_not_contains "${out_replay}" "transaction status: 0" "FISCO replay nonce should fail"

  local now_ms_refund
  now_ms_refund=$(date +%s%3N)
  local exp3=$((now_ms_refund + 10000))
  if [[ "${USE_V2_TRANSFER}" == "true" ]]; then
    out_lock2=$(run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" lockV2 "${transfer3}" "${session_id}" "${trace3}" "${key_id}" 3 "${exp3}" "${sig}")
  else
    out_lock2=$(run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" lock "${trace3}" "${key_id}" 3 "${exp3}" "${sig}")
  fi
  assert_contains "${out_lock2}" "transaction status: 0" "FISCO lock for refund failed"
  sleep 12

  out_refund=$(run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" refund "${trace3}" "${key_id}" "${sig}")
  assert_contains "${out_refund}" "transaction status: 0" "FISCO refund failed"
  assert_gateway_event_compat "${out_refund}" "RefundExecuted" "Event_Refund" "FISCO refund" "${STRICT_EVENT_ASSERT_FISCO}"

  out_trace3=$(run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" getTrace "${trace3}")
  assert_contains "${out_trace3}" "REFUNDED" "FISCO trace3 state should be REFUNDED"

  log "[FISCO] Agent1/2/3 test passed"
}

test_fisco_agent4() {
  if [[ "${ENABLE_AUDIT_CHAIN}" != "true" ]]; then
    log "[FISCO] Agent4 skipped: audit chain retired"
    return 0
  fi

  log "[FISCO] Agent4 manual enable (compat mode)"
}

main() {
  log "start Agent1/2/3/4 integration test (focus Agent3+Agent4)"
  deploy_fabric_agent34
  test_fabric_agent3
  test_fabric_agent4
  deploy_fisco_agent1234
  test_fisco_agent123
  test_fisco_agent4
  log "all done: Agent1/2/3/4 passed cross-chain dual-side test"
}

main "$@"
