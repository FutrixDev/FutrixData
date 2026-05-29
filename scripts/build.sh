#!/bin/sh
#
# Build FutrixData desktop app with bundled CLI.
#
# Usage:
#   scripts/build.sh                  # full build (desktop + CLI)
#   scripts/build.sh --cli-only       # build CLI to ~/.local/bin (dev mode)
#   scripts/build.sh --platform darwin/arm64
#   scripts/build.sh --nsis           # Windows NSIS installer
#
# After a full build:
#   macOS:   build/bin/FutrixData.app  (includes futrixdata-cli in Contents/MacOS/)
#   Windows: build/bin/               (FutrixData.exe + futrixdata-cli.exe)
#   Linux:   build/bin/               (FutrixData + futrixdata-cli)

set -eu

CLI_ONLY=false
TARGET_PLATFORM=""
EXTRA_FLAGS=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --cli-only)
      CLI_ONLY=true
      shift
      ;;
    --platform)
      TARGET_PLATFORM="${2}"
      EXTRA_FLAGS="${EXTRA_FLAGS} -platform ${2}"
      shift 2
      ;;
    --nsis)
      EXTRA_FLAGS="${EXTRA_FLAGS} -nsis"
      shift
      ;;
    *)
      EXTRA_FLAGS="${EXTRA_FLAGS} ${1}"
      shift
      ;;
  esac
done

# Resolve target OS/ARCH. When --platform is given (e.g. "darwin/arm64"),
# use it for CLI cross-compilation; otherwise fall back to host values.
if [ -n "${TARGET_PLATFORM}" ]; then
  TARGET_OS="${TARGET_PLATFORM%/*}"
  TARGET_ARCH="${TARGET_PLATFORM#*/}"
else
  TARGET_OS="$(go env GOOS)"
  TARGET_ARCH="$(go env GOARCH)"
fi

CLI_NAME="futrixdata-cli"
if [ "${TARGET_OS}" = "windows" ]; then
  CLI_NAME="futrixdata-cli.exe"
fi

# Read productVersion from wails.json so the daemon and CLI share the same
# semver. internal/version.Version defaults to "dev" without this injection,
# which is the value MyView shows in the account menu.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WAILS_JSON="${SCRIPT_DIR}/../wails.json"
APP_VERSION="$(awk -F'"' '/"productVersion"[[:space:]]*:/{print $4; exit}' "${WAILS_JSON}")"
if [ -z "${APP_VERSION}" ]; then
  echo "==> WARN: could not read productVersion from ${WAILS_JSON}; defaulting to 'dev'" >&2
  APP_VERSION="dev"
fi
VERSION_LDFLAG="-X futrixdata/platform/internal/version.Version=${APP_VERSION}"

if [ "${CLI_ONLY}" = "true" ]; then
  # Dev mode: build CLI directly to ~/.local/bin (or %LOCALAPPDATA%\FutrixData\bin on Windows).
  # Skip the version ldflag here on purpose: the IPC handshake requires the CLI
  # and daemon versions to match exactly (internal/ipc/client.go:72), and
  # `wails dev` doesn't inject ldflags so its daemon stays at "dev". Pairing a
  # versioned CLI with a "dev" daemon would fail every handshake with
  # VERSION_MISMATCH. Release builds (full build path below) inject for both.
  HOST_OS="$(go env GOOS)"
  if [ "${HOST_OS}" = "windows" ]; then
    CLI_DEST="${LOCALAPPDATA:-${USERPROFILE}/AppData/Local}/FutrixData/bin/${CLI_NAME}"
  else
    CLI_DEST="${HOME}/.local/bin/${CLI_NAME}"
  fi
  mkdir -p "$(dirname "${CLI_DEST}")"
  echo "==> Building futrixdata-cli -> ${CLI_DEST} (version=dev, paired with wails dev daemon)" >&2
  go build -o "${CLI_DEST}" ./cmd/futrixdata-cli/
  echo "==> Done." >&2
  exit 0
fi

# Full build: desktop app + CLI.
echo "==> Building FutrixData desktop app (version=${APP_VERSION})..." >&2
# shellcheck disable=SC2086
wails build -ldflags "${VERSION_LDFLAG}" ${EXTRA_FLAGS}

echo "==> Building futrixdata-cli (${TARGET_OS}/${TARGET_ARCH})..." >&2
case "${TARGET_OS}" in
  darwin)
    CLI_DEST="build/bin/FutrixData.app/Contents/MacOS/${CLI_NAME}"
    ;;
  *)
    CLI_DEST="build/bin/${CLI_NAME}"
    ;;
esac

GOOS="${TARGET_OS}" GOARCH="${TARGET_ARCH}" go build -trimpath -ldflags "${VERSION_LDFLAG}" -o "${CLI_DEST}" ./cmd/futrixdata-cli/
echo "==> CLI bundled at ${CLI_DEST}" >&2
echo "==> Build complete." >&2
