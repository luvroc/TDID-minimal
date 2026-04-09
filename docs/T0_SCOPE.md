# T0 Scope And Extraction Notes

## What T0 Means Here

The local `T0` documentation fixes the experiment baseline as the serial pass of:

1. `a3_verify_dual_hosts.ps1`
2. `p1_closeout_run.ps1`
3. `a4_matrix_closeout_run.ps1`

So this minimal repository is organized around one question:

> what is the smallest source set that still explains and supports the current `T0` baseline workflow?

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

- `scripts/t0/`
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

1. The current script set still reflects the private remote layout more than a polished public DX.
2. Some Fabric / FISCO scripts may still require environment templates before public release.
3. `tee2` is configured in local SSH aliases, but the open-source cut itself does not depend on that host being reachable.

## Recommended Next Step Before Publishing

1. Run a grep pass for internal IPs and usernames.
2. Replace hard-coded remote paths with repo-relative paths where practical.
3. Add a small top-level `Makefile` or `justfile` for public-facing build and test entrypoints.
