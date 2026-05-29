import { mkdtemp, writeFile, chmod, mkdir, rename, rm } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { execFile as execFileCallback } from 'node:child_process';
import { promisify } from 'node:util';
import { fileURLToPath } from 'node:url';

import { buildReleaseAssetName, buildReleaseDownloadURL } from './distribution.mjs';

const execFile = promisify(execFileCallback);
const __dirname = path.dirname(fileURLToPath(import.meta.url));
const defaultPackageRoot = path.resolve(__dirname, '..');

function normalizePlatform(platform = process.platform) {
  switch (platform) {
    case 'darwin':
    case 'linux':
    case 'win32':
    case 'windows':
      return platform === 'win32' ? 'windows' : platform;
    default:
      throw new Error(`unsupported platform: ${platform}`);
  }
}

function normalizeArch(arch = process.arch) {
  switch (arch) {
    case 'x64':
      return 'amd64';
    case 'arm64':
    case 'amd64':
      return arch;
    default:
      throw new Error(`unsupported arch: ${arch}`);
  }
}

function binaryNameForPlatform(platform) {
  return platform === 'windows' ? 'futrixdata-cli.exe' : 'futrixdata-cli';
}

function buildURLFromBase({ releaseBaseURL, version, platform, arch }) {
  const base = String(releaseBaseURL || '').trim().replace(/\/+$/, '');
  const assetName = buildReleaseAssetName({ platform, arch });
  if (!base) {
    return buildReleaseDownloadURL({
      version,
      platform,
      arch,
    });
  }
  if (String(version || '').trim()) {
    return `${base}/download/${String(version).trim()}/${assetName}`;
  }
  return `${base}/latest/download/${assetName}`;
}

async function downloadFile(url, destination) {
  const response = await fetch(url, {
    headers: {
      'user-agent': 'futrixdata-cli-npm-installer',
      accept: 'application/octet-stream',
    },
  });
  if (!response.ok) {
    throw new Error(`download failed: ${response.status} ${response.statusText}`);
  }
  const buffer = Buffer.from(await response.arrayBuffer());
  await writeFile(destination, buffer);
}

async function extractArchive({ archivePath, outputDir, platform }) {
  await mkdir(outputDir, { recursive: true });
  if (platform === 'windows') {
    await execFile('unzip', ['-q', archivePath, '-d', outputDir]);
    return;
  }
  await execFile('tar', ['-xzf', archivePath, '-C', outputDir]);
}

export function installedBinaryPath({
  targetDir = path.join(process.env.FUTRIXDATA_CLI_NPM_PACKAGE_ROOT || defaultPackageRoot, 'vendor'),
  platform = process.platform,
} = {}) {
  const normalizedPlatform = normalizePlatform(platform);
  return path.join(targetDir, binaryNameForPlatform(normalizedPlatform));
}

export async function installCLI({
  version = '',
  platform = process.platform,
  arch = process.arch,
  releaseBaseURL = '',
  targetDir = path.join(process.env.FUTRIXDATA_CLI_NPM_PACKAGE_ROOT || defaultPackageRoot, 'vendor'),
} = {}) {
  const normalizedPlatform = normalizePlatform(platform);
  const normalizedArch = normalizeArch(arch);
  const url = buildURLFromBase({
    releaseBaseURL,
    version,
    platform: normalizedPlatform,
    arch: normalizedArch,
  });
  const tempRoot = await mkdtemp(path.join(tmpdir(), 'futrixdata-cli-npm-install-'));
  const archivePath = path.join(tempRoot, buildReleaseAssetName({ platform: normalizedPlatform, arch: normalizedArch }));
  const extractDir = path.join(tempRoot, 'extract');
  const binaryName = binaryNameForPlatform(normalizedPlatform);
  try {
    await downloadFile(url, archivePath);
    await extractArchive({ archivePath, outputDir: extractDir, platform: normalizedPlatform });
    const extractedBinary = path.join(extractDir, binaryName);
    if (!existsSync(extractedBinary)) {
      throw new Error(`archive did not contain ${binaryName}`);
    }
    await mkdir(targetDir, { recursive: true });
    const targetBinary = path.join(targetDir, binaryName);
    await rename(extractedBinary, targetBinary).catch(async () => {
      await rm(targetBinary, { force: true });
      await execFile('cp', [extractedBinary, targetBinary]);
    });
    if (normalizedPlatform !== 'windows') {
      await chmod(targetBinary, 0o755);
    }
    return targetBinary;
  } finally {
    await rm(tempRoot, { recursive: true, force: true });
  }
}

if (import.meta.url === `file://${process.argv[1]}`) {
  installCLI({
    version: process.env.FUTRIXDATA_CLI_VERSION || '',
    releaseBaseURL: process.env.FUTRIXDATA_CLI_RELEASE_BASE_URL || '',
  }).catch((error) => {
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(1);
  });
}
