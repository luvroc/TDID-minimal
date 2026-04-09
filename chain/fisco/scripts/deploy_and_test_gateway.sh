#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="/home/ecs-user"
TDID_DIR="${ROOT_DIR}/TDID"
FABRIC_NET_DIR="${ROOT_DIR}/chain-DOT/test-network"
FABRIC_CC_PATH="${TDID_DIR}/fabric/chaincode/tdid_source_gateway"
FISCO_DIR="${ROOT_DIR}/fisco"
FISCO_CONSOLE_DIR="${FISCO_DIR}/console"
FISCO_NODES_DIR="${FISCO_DIR}/nodes/127.0.0.1"
SCRIPT_DIR="${TDID_DIR}/fisco/contracts"
FISCO_SOL_SRC="${SCRIPT_DIR}/TargetGateway.sol"
FISCO_SOL_DST="${FISCO_CONSOLE_DIR}/contracts/solidity/FiscoGateway.sol"

CHANNEL_NAME="mychannel"
CC_NAME="gatewaycc"
RUN_ID="$(date +%s)"

log() {
  echo "[$(date +'%F %T')] $*"
}

run_fisco_console() {
  (
    cd "${FISCO_CONSOLE_DIR}"
    bash console.sh "$@"
  )
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

fabric_invoke() {
  local ctor_json="$1"
  (
    cd "${FABRIC_NET_DIR}"
    export PATH="${ROOT_DIR}/chain-DOT/bin:${PATH}"
    export OVERRIDE_ORG=""
    export VERBOSE="false"
    export TEST_NETWORK_HOME="${FABRIC_NET_DIR}"
    export FABRIC_CFG_PATH="${ROOT_DIR}/chain-DOT/config"
    source scripts/envVar.sh
    setGlobals 1
    peer chaincode invoke \
      -o localhost:7050 \
      --ordererTLSHostnameOverride orderer.example.com \
      --tls --cafile "$ORDERER_CA" \
      -C "${CHANNEL_NAME}" -n "${CC_NAME}" \
      --peerAddresses localhost:7051 --tlsRootCertFiles "$PEER0_ORG1_CA" \
      --peerAddresses localhost:9051 --tlsRootCertFiles "$PEER0_ORG2_CA" \
      -c "${ctor_json}"
  )
}

fabric_query() {
  local ctor_json="$1"
  (
    cd "${FABRIC_NET_DIR}"
    export PATH="${ROOT_DIR}/chain-DOT/bin:${PATH}"
    export OVERRIDE_ORG=""
    export VERBOSE="false"
    export TEST_NETWORK_HOME="${FABRIC_NET_DIR}"
    export FABRIC_CFG_PATH="${ROOT_DIR}/chain-DOT/config"
    source scripts/envVar.sh
    setGlobals 1
    peer chaincode query -C "${CHANNEL_NAME}" -n "${CC_NAME}" -c "${ctor_json}"
  )
}

deploy_fabric() {
  log "[Fabric] start network"
  (
    cd "${FABRIC_NET_DIR}"
    ./network.sh up
    if ! ./network.sh createChannel -c "${CHANNEL_NAME}"; then
      log "[Fabric] channel ${CHANNEL_NAME} already exists, skip create"
    fi
    ./network.sh deployCC -c "${CHANNEL_NAME}" -ccn "${CC_NAME}" -ccp "${FABRIC_CC_PATH}" -ccl go
  )
}

test_fabric() {
  log "[Fabric] test SetBalance/Lock/Mint/Unlock/Burn"

  local lock_id="lock-${RUN_ID}"
  local mint_id="mint-${RUN_ID}"
  local burn_id="burn-${RUN_ID}"

  fabric_invoke '{"function":"SetBalance","Args":["alice","USDT","1000"]}' >/tmp/fabric_setbalance.log
  fabric_invoke '{"function":"SetBalance","Args":["bob","USDT","0"]}' >/tmp/fabric_setbalance_bob.log
  sleep 2

  out_lock=$(fabric_invoke '{"function":"Lock","Args":["'"${lock_id}"'","alice","bob","USDT","100","fisco"]}' 2>&1 || true)
  assert_contains "${out_lock}" "status:200" "Fabric Lock invoke failed"
  sleep 2

  out_bal_a1=$(fabric_query '{"function":"GetBalance","Args":["alice","USDT"]}')
  assert_contains "${out_bal_a1}" "900" "Fabric alice balance after lock should be 900"

  out_mint=$(fabric_invoke '{"function":"Mint","Args":["'"${mint_id}"'","bob","USDT","100","fisco","'"${lock_id}"'"]}' 2>&1 || true)
  assert_contains "${out_mint}" "status:200" "Fabric Mint invoke failed"
  sleep 2

  out_bal_b1=$(fabric_query '{"function":"GetBalance","Args":["bob","USDT"]}')
  assert_contains "${out_bal_b1}" "100" "Fabric bob balance after mint should be 100"

  out_unlock=$(fabric_invoke '{"function":"Unlock","Args":["'"${lock_id}"'","timeout","0xfiscoTx"]}' 2>&1 || true)
  assert_contains "${out_unlock}" "status:200" "Fabric Unlock invoke failed"
  sleep 2

  out_bal_a2=$(fabric_query '{"function":"GetBalance","Args":["alice","USDT"]}')
  assert_contains "${out_bal_a2}" "1000" "Fabric alice balance after unlock should be 1000"

  out_burn=$(fabric_invoke '{"function":"Burn","Args":["'"${burn_id}"'","bob","USDT","50","fisco","alice"]}' 2>&1 || true)
  assert_contains "${out_burn}" "status:200" "Fabric Burn invoke failed"
  sleep 2

  out_bal_b2=$(fabric_query '{"function":"GetBalance","Args":["bob","USDT"]}')
  assert_contains "${out_bal_b2}" "50" "Fabric bob balance after burn should be 50"

  log "[Fabric] test passed"
}

deploy_fisco() {
  log "[FISCO] start nodes"
  (
    cd "${FISCO_NODES_DIR}"
    bash start_all.sh || true
  )

  log "[FISCO] prepare and deploy FiscoGateway.sol"
  cp "${FISCO_SOL_SRC}" "${FISCO_SOL_DST}"
  deploy_out=$(run_fisco_console deploy FiscoGateway)
  echo "${deploy_out}" >/tmp/fisco_deploy_gateway.log

  FISCO_GATEWAY_ADDR=$(echo "${deploy_out}" | sed -n 's/.*contract address: \(0x[0-9a-fA-F]\+\).*/\1/p' | tail -n1)
  if [[ -z "${FISCO_GATEWAY_ADDR}" ]]; then
    echo "FISCO deploy failed: gateway address not found"
    echo "${deploy_out}"
    exit 1
  fi
  export FISCO_GATEWAY_ADDR
  log "[FISCO] contract address: ${FISCO_GATEWAY_ADDR}"
}

test_fisco() {
  log "[FISCO] test setBalance/lock/mint/unlock/burn"

  local lock_id="lock-${RUN_ID}"
  local mint_id="mint-${RUN_ID}"
  local burn_id="burn-${RUN_ID}"

  run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" setBalance alice USDT 1000 >/tmp/fisco_setbalance.log
  run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" setBalance bob USDT 0 >/tmp/fisco_setbalance_bob.log
  sleep 1

  out_lock=$(run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" lock "${lock_id}" alice bob USDT 100 fabric)
  assert_contains "${out_lock}" "transaction status: 0" "FISCO lock failed"

  out_bal_a1=$(run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" getBalance alice USDT)
  assert_contains "${out_bal_a1}" "(900)" "FISCO alice balance after lock should be 900"

  out_mint=$(run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" mint "${mint_id}" bob USDT 100 fabric "${lock_id}")
  assert_contains "${out_mint}" "transaction status: 0" "FISCO mint failed"

  out_bal_b1=$(run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" getBalance bob USDT)
  assert_contains "${out_bal_b1}" "(100)" "FISCO bob balance after mint should be 100"

  out_unlock=$(run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" unlock "${lock_id}" timeout 0xfabricTx)
  assert_contains "${out_unlock}" "transaction status: 0" "FISCO unlock failed"

  out_bal_a2=$(run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" getBalance alice USDT)
  assert_contains "${out_bal_a2}" "(1000)" "FISCO alice balance after unlock should be 1000"

  out_burn=$(run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" burn "${burn_id}" bob USDT 50 fabric alice)
  assert_contains "${out_burn}" "transaction status: 0" "FISCO burn failed"

  out_bal_b2=$(run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" getBalance bob USDT)
  assert_contains "${out_bal_b2}" "(50)" "FISCO bob balance after burn should be 50"

  out_tx_lock=$(run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" getTxMeta "${lock_id}")
  assert_contains "${out_tx_lock}" "(true, LOCK, REFUNDED" "FISCO lock tx status should be REFUNDED"

  out_tx_mint=$(run_fisco_console call FiscoGateway "${FISCO_GATEWAY_ADDR}" getTxMeta "${mint_id}")
  assert_contains "${out_tx_mint}" "(true, MINT, MINTED" "FISCO mint tx status should be MINTED"

  log "[FISCO] test passed"
}

main() {
  log "start deploying and testing gateway contract"
  deploy_fabric
  test_fabric
  deploy_fisco
  test_fisco
  log "all done: Fabric + FISCO gateway deployment and tests passed"
}

main "$@"
