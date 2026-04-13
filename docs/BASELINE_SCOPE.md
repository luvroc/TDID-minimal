# Serial Baseline Scope And Extraction Notes

## What The Serial Baseline Means Here

The public baseline documentation fixes the baseline as the serial pass of:

1. `verify_dual_hosts.sh`
2. `run_crosschain_closeout.sh`
3. `run_mutex_matrix_closeout.sh`

So this minimal repository is organized around one question:

> what is the smallest source set that still explains and supports the current serial baseline workflow?

## What Was Kept

### TEE side

- `go.mod`, `go.sum`
- `shared/`
- `host/`
- `enclave/api`
- `enclave/core`
- `enclave/cmd/tee_enclave_service`
- `enclave/occlum/build.sh`
- `enclave/occlum/copy_bom.yaml`
- `enclave/occlum/Occlum.json`
- `enclave/occlum/README.md`
- `enclave_occlum_reference.md`
- `go-for-occlum.sh`

Reason:

- these files cover the current prototype main path, enclave boundary, and Occlum packaging source configuration

### Chain side

- `context-sharing.md`
- `fabric/chaincode/tdid_identity/`
- `fabric/chaincode/tdid_session/`
- `fabric/chaincode/tdid_source_gateway/`
- `fabric/scripts/`
- `fabric/config/`
- `fisco/contracts/`
- `fisco/scripts/`

Reason:

- these files cover the on-chain contracts / chaincodes and the scripts most directly tied to proof-path deployment and mutex validation

### Workflow scripts

- `scripts/baseline/`
- `scripts/closeout/`
- `scripts/deploy/`
- `scripts/probes/`

Reason:

- these scripts preserve the current baseline execution logic without bringing the full local experiment workspace into the public snapshot

## What Was Removed

- private or environment-heavy bundles such as `fabric_sdk_bundle/`, `fisco-config-pack/`, `artifacts/`
- built outputs such as prebuilt enclave binaries and Occlum instances
- unrelated historical docs and experiment byproducts
- repo metadata and internal working state

## Residual Caveats

1. The current script set still reflects a managed remote workflow more than a polished public developer experience.
2. Some Fabric / FISCO scripts still require environment templates before public release.
3. The baseline entrypoints now run on Linux shell, but they still expect SSH-accessible remote hosts instead of a single-machine local sandbox.

## Recommended Next Step Before Publishing

1. Run a final grep pass for internal IPs and usernames across Git history.
2. Replace remote-host assumptions with a repo-local demo path where practical.
3. Add a small top-level `Makefile` or `justfile` for public-facing build and test entrypoints.
