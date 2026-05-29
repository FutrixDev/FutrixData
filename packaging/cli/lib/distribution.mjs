const DEFAULT_REPO = 'FutrixDev/FutrixData';

const ARCHIVE_BY_PLATFORM = new Map([
  ['darwin', 'tar.gz'],
  ['linux', 'tar.gz'],
  ['windows', 'zip'],
]);

function normalizePlatform(platform) {
  const value = String(platform || '').trim();
  if (!ARCHIVE_BY_PLATFORM.has(value)) {
    throw new Error(`unsupported platform: ${value || '<empty>'}`);
  }
  return value;
}

function normalizeArch(arch) {
  const value = String(arch || '').trim();
  switch (value) {
    case 'x64':
      return 'amd64';
    case 'arm64':
    case 'amd64':
      return value;
    default:
      throw new Error(`unsupported arch: ${value || '<empty>'}`);
  }
}

export function buildReleaseAssetName({ platform, arch }) {
  const normalizedPlatform = normalizePlatform(platform);
  const normalizedArch = normalizeArch(arch);
  const ext = ARCHIVE_BY_PLATFORM.get(normalizedPlatform);
  return `futrixdata-cli_${normalizedPlatform}_${normalizedArch}.${ext}`;
}

export function buildReleaseDownloadURL({
  repo = DEFAULT_REPO,
  version = '',
  platform,
  arch,
}) {
  const assetName = buildReleaseAssetName({ platform, arch });
  const base = `https://github.com/${repo}/releases`;
  if (String(version || '').trim() === '') {
    return `${base}/latest/download/${assetName}`;
  }
  return `${base}/download/${String(version).trim()}/${assetName}`;
}

export function renderHomebrewFormula({
  version,
  repo = DEFAULT_REPO,
  darwinArm64SHA256,
  darwinAmd64SHA256,
}) {
  const normalizedVersion = String(version || '').trim();
  if (!normalizedVersion) {
    throw new Error('version is required');
  }
  if (!darwinArm64SHA256 || !darwinAmd64SHA256) {
    throw new Error('darwin arm64/amd64 sha256 values are required');
  }
  const armURL = buildReleaseDownloadURL({
    repo,
    version: `v${normalizedVersion.replace(/^v/, '')}`,
    platform: 'darwin',
    arch: 'arm64',
  });
  const amdURL = buildReleaseDownloadURL({
    repo,
    version: `v${normalizedVersion.replace(/^v/, '')}`,
    platform: 'darwin',
    arch: 'amd64',
  });
  return `class FutrixdataCli < Formula
  desc "FutrixData command line interface"
  homepage "https://github.com/${repo}"
  version "${normalizedVersion.replace(/^v/, '')}"

  on_macos do
    if Hardware::CPU.arm?
      url "${armURL}"
      sha256 "${darwinArm64SHA256}"
    else
      url "${amdURL}"
      sha256 "${darwinAmd64SHA256}"
    end
  end

  def install
    bin.install "futrixdata-cli"
  end

  test do
    system "#{bin}/futrixdata-cli", "--help"
  end
end
`;
}
