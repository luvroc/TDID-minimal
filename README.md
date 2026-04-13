# TDID Open Minimal Repository

## Purpose

This repository is a trimmed open-source subset extracted from the current internal TDID prototype codebases.

The extraction target is:

- keep the source code and contracts that represent the current TDID paper prototype
- keep the minimum workflow needed to understand and replay the serial baseline
- keep chain-side `context-sharing` materials for follow-up development
- remove bulky deployment artifacts, local cluster state, packaged SDK bundles, built binaries, and environment-specific private material

## Included Layout

- `tee/`
  - minimal TEE-side source snapshot from the internal TEE-side prototype repo
  - includes `host/`, `shared/`, `enclave/api`, `enclave/core`, `enclave/cmd/tee_enclave_service`, and Occlum source config files
- `chain/`
  - minimal chain-side source snapshot from the internal chain-side prototype repo
  - includes Fabric chaincodes, Fabric scripts/config, FISCO contracts/scripts, and `context-sharing.md`
- `scripts/baseline/`
  - the three serial baseline entrypoints in Linux shell form
- `scripts/closeout/`
  - closeout scripts required by the serial baseline
- `scripts/deploy/`
  - minimal remote bundle / managed release / rollback helpers
- `scripts/probes/`
  - proof-path and mutex probes used by the closeout flow

## Serial Baseline

The serial baseline is the ordered execution of:

1. `scripts/baseline/verify_dual_hosts.sh`
2. `scripts/baseline/run_crosschain_closeout.sh`
3. `scripts/baseline/run_mutex_matrix_closeout.sh`

This repository keeps the code and scripts most directly related to that baseline.

The entrypoints are Linux shell scripts and assume:

- `bash`
- `ssh`
- `scp`
- reachable `chain` / `tee` hosts or equivalent SSH targets supplied through environment variables

## Deployment Prerequisites

The current code snapshot was extracted from a two-side setup:

- `tee/` side for relay + enclave logic
- `chain/` side for Fabric and FISCO-BCOS contracts / scripts

Observed environment on the remote servers at extraction time:

- TEE host
  - Go bootstrap: `go1.21.6`
  - Occlum installed under `/opt/occlum/build/bin/occlum`
  - Occlum Go toolchain expected under `/opt/occlum/toolchains/golang`
- chain host
  - Go: `go1.25.6`
  - Hyperledger Fabric peer: `v2.5.14`
  - Docker: `29.1.5`
  - Python: `3.10.12`
  - Java: `11.0.30`
  - FISCO-BCOS config in this repo is annotated as current `3.12` chain config

These versions are not a strict lockfile, but they are the closest reference for reproducing the extracted environment.

## TEE Toolchain And Required Config

The TEE side is not intended to be built only with plain `go build`.
It uses the Occlum-targeted Go toolchain prepared by `tee/go-for-occlum.sh`.

That script:

- bootstraps from the host `go`
- clones Occlum's Go fork
- defaults to branch `go1.18.4_for_occlum`
- installs to `/opt/occlum/toolchains/golang`
- creates `/opt/occlum/toolchains/golang/bin/occlum-go`

Minimal preparation on a TEE host:

```bash
cd tee
bash go-for-occlum.sh
cd enclave/occlum
bash build.sh
```

Required runtime variables are consistent with the remote `deploy/tee-a/config.env` and `deploy/tee-b/config.env` files:

```env
# tee-a / source
TEE_NODE_ID=tee-a
TEE_NODE_ROLE=source
TEE_PEER_ALLOWLIST=tee-b
TEE_ENCLAVE_ADDR=:18080
TEE_ENCLAVE_STATE_PATH=/var/lib/tee-a/state.sealed
TEE_ENCLAVE_SEAL_KEY=change-me-tee-a-seal-key
TEE_A_HOST_BIND=:28080
TEE_A_PEER_TARGET=127.0.0.1:29090
TEE_A_FISCO_BASE=http://127.0.0.1:8545
TEE_A_FABRIC_BASE=http://127.0.0.1:7050
TEE_A_AUDIT_BASE=http://127.0.0.1:9000
TEE_REFUND_METHOD=refundV2

# tee-b / target
TEE_NODE_ID=tee-b
TEE_NODE_ROLE=target
TEE_PEER_ALLOWLIST=tee-a,cap:proofdigest,cap:proofsig,vc:attester:expected-attester,vc:signer:proof-signer
TEE_ENCLAVE_ADDR=:19080
TEE_ENCLAVE_STATE_PATH=/var/lib/tee-b/state.sealed
TEE_ENCLAVE_SEAL_KEY=change-me-tee-b-seal-key
TEE_B_HOST_BIND=:29090
TEE_B_FISCO_BASE=http://127.0.0.1:8545
TEE_B_FABRIC_BASE=http://127.0.0.1:7050
TEE_B_AUDIT_BASE=http://127.0.0.1:9000
TEE_REFUND_METHOD=refundV2
```

If you keep TLS on the host side, also prepare:

- `deploy/certs/tee-a.crt`
- `deploy/certs/tee-a.key`
- `deploy/certs/tee-b.crt`
- `deploy/certs/tee-b.key`
- `deploy/certs/ca.crt`

The stage-style startup flow on the original TEE server used:

- `deploy/tee-a/config.env`
- `deploy/tee-b/config.env`
- `deploy/run-stage8-minimal.sh`

## Chain-Side Config

The chain-side scripts assume both Fabric and FISCO-BCOS are available.

Relevant configuration templates in this repository:

- FISCO SDK config template: `chain/fisco/config/gosdk_config.toml`
- FISCO RPC endpoints template: `chain/fisco/config/rpc_endpoints.env`
- deployed contracts snapshot: `chain/fisco/config/deployed_contracts.json`

Current FISCO endpoint template:

```env
RPC_20200=http://SERVER_IP:20200
RPC_20201=http://SERVER_IP:20201
RPC_20202=http://SERVER_IP:20202
RPC_20203=http://SERVER_IP:20203

WEB3_8545=http://SERVER_IP:8545
WEB3_8546=http://SERVER_IP:8546
WEB3_8547=http://SERVER_IP:8547
WEB3_8548=http://SERVER_IP:8548

GROUP_ID=group0
CHAIN_ID=chain0
WEB3_CHAIN_ID=20200
```

The current deployed contract naming is:

- `GovernanceRoot`
- `TDIDRegistry`
- `SessionKeyRegistry`
- `SigVerifier`
- `TargetGateway`

For Fabric, the extracted scripts expect:

- Fabric config under `chain/fabric/config`
- Fabric network management via `chain/fabric/scripts/network.sh`
- chaincode deployment via `chain/fabric/scripts/deploy-chaincodes.sh`

## Practical Deployment Notes

- Several helper scripts still expect you to provide runtime-specific directories and endpoints through env files.
- The public subset preserves the source and workflow logic, but not the full private deployment bundle.
- If you want to make this repository self-contained for public users, the next cleanup step should be adding public config templates for `deploy/tee-a/config.env`, `deploy/tee-b/config.env`, and chain-side endpoint env files.

## Deliberately Excluded

- `.git/`, `artifacts/`, deployment outputs, packaged config packs
- built Occlum instances and enclave binaries
- local-only infra state and private environment wiring
- large experimental byproducts not required to understand the prototype or replay the serial baseline

## Notes

- This is a source-oriented minimal open-source cut, not a turnkey private deployment mirror.
- Some helper scripts still need environment-specific directory values to be supplied explicitly.
- Before publishing, you should do one final pass for credentials, internal IPs, and organization-specific naming.

See `docs/BASELINE_SCOPE.md` for the extraction rationale and scope.
