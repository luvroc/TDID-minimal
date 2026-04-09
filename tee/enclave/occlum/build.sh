#!/bin/bash
set -euo pipefail

THIS_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
ROOT_DIR="$( cd "${THIS_DIR}/../.." >/dev/null 2>&1 && pwd )"

OCCLUM_BIN="${OCCLUM_BIN:-$(command -v occlum || true)}"
if [[ -z "${OCCLUM_BIN}" ]]; then
  if [[ -x /opt/occlum/build/bin/occlum ]]; then
    OCCLUM_BIN="/opt/occlum/build/bin/occlum"
  else
    echo "occlum binary not found"
    exit 1
  fi
fi

OCCLUM_GO="${OCCLUM_GO:-/opt/occlum/toolchains/golang/bin/occlum-go}"
if [[ ! -x "${OCCLUM_GO}" ]]; then
  echo "occlum-go not found at ${OCCLUM_GO}"
  exit 1
fi

INSTANCE_DIR="${1:-${THIS_DIR}/instance}"
BIN_DIR="${THIS_DIR}/bin"
BIN_PATH="${BIN_DIR}/tee_enclave_service"

mkdir -p "${BIN_DIR}"

pushd "${ROOT_DIR}" >/dev/null
"${OCCLUM_GO}" build -o "${BIN_PATH}" ./enclave/cmd/tee_enclave_service
popd >/dev/null

rm -rf "${INSTANCE_DIR}"
mkdir -p "${INSTANCE_DIR}"
pushd "${INSTANCE_DIR}" >/dev/null

"${OCCLUM_BIN}" init
cp -f "${THIS_DIR}/Occlum.json" ./Occlum.json

mkdir -p image/bin image/etc image/var/lib/tee-a image/var/lib/tee-b
cp -f "${BIN_PATH}" image/bin/tee_enclave_service

if [[ -f /etc/hosts ]]; then
  cp -f /etc/hosts image/etc/hosts
fi
if [[ -f /etc/resolv.conf ]]; then
  cp -f /etc/resolv.conf image/etc/resolv.conf
fi

"${OCCLUM_BIN}" build

echo "[stage7] Occlum instance ready: ${INSTANCE_DIR}"
echo "[stage7] Run with: cd ${INSTANCE_DIR} && ${OCCLUM_BIN} run /bin/tee_enclave_service"

popd >/dev/null
