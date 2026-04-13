#!/usr/bin/env bash
set -euo pipefail
cd ~/fisco/console
run(){ bash console.sh "$@"; }
addr(){ echo "$1" | sed -n 's/.*contract address: \(0x[0-9a-fA-F]\+\).*/\1/p' | tail -n1; }
mk_b32(){ python3 - <<'PY' "$1"
from eth_hash.auto import keccak
import re,sys
v=sys.argv[1].strip()
if v.startswith(("0x","0X")) and re.fullmatch(r"[0-9a-fA-F]+",v[2:]):
 h=v[2:].lower(); print("0x"+h[-64:].rjust(64,'0'))
else:
 print("0x"+keccak(v.encode()).hex())
PY
}
cp ~/TDID/fisco/contracts/TargetGateway.sol ~/fisco/console/contracts/solidity/FiscoGateway.sol
out_m=$(run deploy MockSigVerifier); m=$(addr "$out_m")
out_g=$(run deploy FiscoGateway "$m" fisco); g=$(addr "$out_g")
echo "gateway=$g"
key=0x1111111111111111111111111111111111111111111111111111111111111111
trace=$(mk_b32 fisco-commit-binding-trace)
sig=0x2222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222222
exp=9999999999999
o1=$(run call FiscoGateway "$g" lock "$trace" "$key" 3001 "$exp" "$sig")
echo "[fisco-commit-binding:lock] $o1"
o2_bad=$(run call FiscoGateway "$g" commitV2 "$trace" "$key" "" "" "" "" 2>&1 || true)
echo "[fisco-commit-binding:commit-empty] $o2_bad"
o2=$(run call FiscoGateway "$g" commitV2 "$trace" "$key" "0xtargettx" "0xtargetreceipt" "fabric" "0xtargetchainhash")
echo "[fisco-commit-binding:commit] $o2"
o3=$(run call FiscoGateway "$g" getTrace "$trace")
echo "[fisco-commit-binding:trace] $o3"
tid=$(run call FiscoGateway "$g" getTransferIdByTrace "$trace" | sed -n 's/.*(0x\([0-9a-fA-F]\{64\}\)).*/0x\1/p' | tail -n1)
if [[ -n "${tid:-}" ]]; then
  o3b=$(run call FiscoGateway "$g" getCommitTargetBindingHash "$tid")
  echo "[fisco-commit-binding:binding-hash] $o3b"
fi
o4=$(run call FiscoGateway "$g" refundV2 "$trace" "$key" || true)
echo "[fisco-commit-binding:refund-after-commit] $o4"
