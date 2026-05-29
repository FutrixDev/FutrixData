const ARCHIVE_BY_PLATFORM = new Map([
  ['darwin', 'tar.gz'],
  ['linux', 'tar.gz'],
  ['windows', 'zip'],
]);

function normalizePlatform(platform) {
  const value = String(platform || '').trim();
  if (value === 'win32') {
    return 'windows';
  }
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
  return `futrixdata-cli_${normalizedPlatform}_${normalizedArch}.${ARCHIVE_BY_PLATFORM.get(normalizedPlatform)}`;
}

export function buildReleaseDownloadURL({
  repo = 'wangqianqianjun/FutrixData',
  version = '',
  platform,
  arch,
}) {
  const assetName = buildReleaseAssetName({ platform, arch });
  const base = `https://github.com/${repo}/releases`;
  if (!String(version || '').trim()) {
    return `${base}/latest/download/${assetName}`;
  }
  return `${base}/download/${String(version).trim()}/${assetName}`;
}
