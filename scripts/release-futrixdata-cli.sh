#!/bin/sh

set -eu

VERSION=""
OUTPUT_DIR=""
TARGETS="darwin/amd64,darwin/arm64,linux/amd64,linux/arm64,windows/amd64,windows/arm64"

usage() {
  echo "Usage: scripts/release-futrixdata-cli.sh --version vX.Y.Z --output DIR [--targets os/arch,...]" >&2
  exit 1
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      VERSION="${2:-}"
      shift 2
      ;;
    --output)
      OUTPUT_DIR="${2:-}"
      shift 2
      ;;
    --targets)
      TARGETS="${2:-}"
      shift 2
      ;;
    *)
      usage
      ;;
  esac
done

[ -n "${VERSION}" ] || usage
[ -n "${OUTPUT_DIR}" ] || usage

VERSION_NO_PREFIX="$(printf '%s' "${VERSION}" | sed 's/^v//')"
TMP_DIR="$(mktemp -d)"
CHECKSUMS_PATH="${OUTPUT_DIR}/futrixdata-cli_checksums.txt"
DARWIN_ARM64_SHA=""
DARWIN_AMD64_SHA=""

cleanup() {
  rm -rf "${TMP_DIR}"
}

trap cleanup EXIT INT TERM

mkdir -p "${OUTPUT_DIR}"
: > "${CHECKSUMS_PATH}"

build_asset() {
  platform="$1"
  arch="$2"
  work_dir="${TMP_DIR}/${platform}_${arch}"
  mkdir -p "${work_dir}"

  binary_name="futrixdata-cli"
  archive_ext="tar.gz"
  if [ "${platform}" = "windows" ]; then
    binary_name="futrixdata-cli.exe"
    archive_ext="zip"
  fi

  asset_name="futrixdata-cli_${platform}_${arch}.${archive_ext}"
  asset_path="${OUTPUT_DIR}/${asset_name}"

  echo "Building ${platform}/${arch} -> ${asset_name}" >&2
  GOOS="${platform}" GOARCH="${arch}" go build -trimpath -o "${work_dir}/${binary_name}" ./cmd/futrixdata-cli

  if [ "${platform}" = "windows" ]; then
    (
      cd "${work_dir}"
      zip -q "${asset_path}" "${binary_name}"
    )
  else
    tar -czf "${asset_path}" -C "${work_dir}" "${binary_name}"
  fi

  sha="$(shasum -a 256 "${asset_path}" | awk '{print $1}')"
  printf '%s  %s\n' "${sha}" "${asset_name}" >> "${CHECKSUMS_PATH}"

  if [ "${platform}" = "darwin" ] && [ "${arch}" = "arm64" ]; then
    DARWIN_ARM64_SHA="${sha}"
  fi
  if [ "${platform}" = "darwin" ] && [ "${arch}" = "amd64" ]; then
    DARWIN_AMD64_SHA="${sha}"
  fi
}

OLD_IFS="${IFS}"
IFS=','
for target in ${TARGETS}; do
  platform="${target%/*}"
  arch="${target#*/}"
  build_asset "${platform}" "${arch}"
done
IFS="${OLD_IFS}"

if [ -n "${DARWIN_ARM64_SHA}" ] && [ -n "${DARWIN_AMD64_SHA}" ]; then
  node packaging/cli/render-homebrew-formula.mjs \
    --version "${VERSION_NO_PREFIX}" \
    --darwin-arm64-sha256 "${DARWIN_ARM64_SHA}" \
    --darwin-amd64-sha256 "${DARWIN_AMD64_SHA}" \
    --output "${OUTPUT_DIR}/homebrew/futrixdata-cli.rb"
fi

NPM_TMP="${TMP_DIR}/npm-package"
mkdir -p "${NPM_TMP}"
cp -R packaging/npm/futrixdata-cli/. "${NPM_TMP}/"
node -e '
const fs = require("node:fs");
const pkgPath = process.argv[1];
const version = process.argv[2];
const pkg = JSON.parse(fs.readFileSync(pkgPath, "utf8"));
pkg.version = version;
pkg.private = false;
fs.writeFileSync(pkgPath, JSON.stringify(pkg, null, 2) + "\n");
' "${NPM_TMP}/package.json" "${VERSION_NO_PREFIX}"
mkdir -p "${OUTPUT_DIR}/npm"
(
  cd "${NPM_TMP}"
  npm pack --pack-destination "${OUTPUT_DIR}/npm" >/dev/null
)

echo "Release artifacts written to ${OUTPUT_DIR}" >&2
