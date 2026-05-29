#!/bin/sh

set -eu

REPO="${FUTRIXDATA_CLI_REPO:-FutrixDev/FutrixData}"
VERSION="${FUTRIXDATA_CLI_VERSION:-}"
RELEASE_BASE_URL="${FUTRIXDATA_CLI_RELEASE_BASE_URL:-https://github.com/${REPO}/releases}"
INSTALL_DIR="${FUTRIXDATA_CLI_INSTALL_DIR:-${HOME}/.local/bin}"

detect_platform() {
  case "$(uname -s)" in
    Darwin) echo "darwin" ;;
    Linux) echo "linux" ;;
    *)
      echo "unsupported platform: $(uname -s)" >&2
      exit 1
      ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *)
      echo "unsupported architecture: $(uname -m)" >&2
      exit 1
      ;;
  esac
}

PLATFORM="$(detect_platform)"
ARCH="$(detect_arch)"
ASSET_NAME="futrixdata-cli_${PLATFORM}_${ARCH}.tar.gz"
TMP_DIR="$(mktemp -d)"
ARCHIVE_PATH="${TMP_DIR}/${ASSET_NAME}"
EXTRACT_DIR="${TMP_DIR}/extract"
TARGET_PATH="${INSTALL_DIR}/futrixdata-cli"

cleanup() {
  rm -rf "${TMP_DIR}"
}

trap cleanup EXIT INT TERM

mkdir -p "${INSTALL_DIR}" "${EXTRACT_DIR}"

if [ -n "${VERSION}" ]; then
  DOWNLOAD_URL="${RELEASE_BASE_URL%/}/download/${VERSION}/${ASSET_NAME}"
else
  DOWNLOAD_URL="${RELEASE_BASE_URL%/}/latest/download/${ASSET_NAME}"
fi

echo "Downloading ${DOWNLOAD_URL}" >&2
curl -fsSL "${DOWNLOAD_URL}" -o "${ARCHIVE_PATH}"
tar -xzf "${ARCHIVE_PATH}" -C "${EXTRACT_DIR}"

if [ ! -f "${EXTRACT_DIR}/futrixdata-cli" ]; then
  echo "archive did not contain futrixdata-cli" >&2
  exit 1
fi

install -m 755 "${EXTRACT_DIR}/futrixdata-cli" "${TARGET_PATH}"
echo "Installed futrixdata-cli to ${TARGET_PATH}" >&2
