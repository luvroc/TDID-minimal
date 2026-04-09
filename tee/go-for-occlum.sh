#!/bin/bash
set -euo pipefail

THIS_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" > /dev/null 2>&1 && pwd )"
BUILD_DIR=/tmp/occlum_golang_toolchain
INSTALL_DIR=/opt/occlum/toolchains/golang
GO_BRANCH=${1:-"go1.18.4_for_occlum"}

if ! command -v git > /dev/null 2>&1; then
  echo "git is required"
  exit 1
fi

if ! command -v go > /dev/null 2>&1; then
  echo "go is required for bootstrap"
  exit 1
fi

GOROOT_BOOTSTRAP_VAL="${GOROOT_BOOTSTRAP:-$(go env GOROOT)}"

echo "[go-for-occlum] using branch: ${GO_BRANCH}"
echo "[go-for-occlum] bootstrap: ${GOROOT_BOOTSTRAP_VAL}"

sudo rm -rf "${BUILD_DIR}" "${INSTALL_DIR}"
mkdir -p "${BUILD_DIR}"
cd "${BUILD_DIR}"

git clone -b "${GO_BRANCH}" https://github.com/occlum/go.git .

cd src
sudo GOROOT_BOOTSTRAP="${GOROOT_BOOTSTRAP_VAL}" ./make.bash
cd ..

sudo mv "${BUILD_DIR}" "${INSTALL_DIR}"

sudo tee "${INSTALL_DIR}/bin/occlum-go" > /dev/null <<'EOF'
#!/bin/bash
set -e
OCCLUM_GCC="${CC:-$(which occlum-gcc)}"
OCCLUM_GOFLAGS="-buildmode=pie ${GOFLAGS}"
CC="$OCCLUM_GCC" GOFLAGS="$OCCLUM_GOFLAGS" /opt/occlum/toolchains/golang/bin/go "$@"
EOF

sudo chmod +x "${INSTALL_DIR}/bin/occlum-go"

"${INSTALL_DIR}/bin/go" version
"${INSTALL_DIR}/bin/occlum-go" version

echo "[go-for-occlum] installed at ${INSTALL_DIR}"
