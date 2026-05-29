import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { execFile as execFileCallback } from 'node:child_process';
import { promisify } from 'node:util';

const execFile = promisify(execFileCallback);
const repoRoot = path.resolve(path.dirname(new URL(import.meta.url).pathname), '../../..');
const releaseScriptsRoot = path.join(repoRoot, 'scripts', 'release');

async function copyReleaseScript(name, destDir) {
  const sourcePath = path.join(releaseScriptsRoot, name);
  const content = await readFile(sourcePath, 'utf8');
  const destPath = path.join(destDir, name);
  await writeFile(destPath, content, { mode: 0o755 });
  return destPath;
}

async function writeExecutable(destPath, content) {
  await writeFile(destPath, content, { mode: 0o755 });
}

test('verify-live-download script downloads live assets and checks shipped version', async () => {
  const tempRoot = await mkdtemp(path.join(tmpdir(), 'futrixdata-verify-live-download-'));
  const scriptsDir = path.join(tempRoot, 'skill-scripts');
  const projectDir = path.join(tempRoot, 'project');
  const binDir = path.join(tempRoot, 'bin');
  const stateDir = path.join(tempRoot, 'state');
  const downloadDir = path.join(tempRoot, 'downloads');
  const curlLog = path.join(stateDir, 'curl.log');

  await mkdir(scriptsDir, { recursive: true });
  await mkdir(projectDir, { recursive: true });
  await mkdir(binDir, { recursive: true });
  await mkdir(stateDir, { recursive: true });
  await mkdir(downloadDir, { recursive: true });

  await copyReleaseScript('verify-live-download.sh', scriptsDir);
  await copyReleaseScript('release-common.sh', scriptsDir);

  await writeFile(path.join(projectDir, 'wails.json'), JSON.stringify({
    info: {
      productVersion: '1.2.3',
    },
  }, null, 2));

  await writeExecutable(path.join(binDir, 'curl'), `#!/bin/bash
set -euo pipefail
log_path="${curlLog}"
output=""
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o)
      output="$2"
      shift 2
      ;;
    -*)
      shift
      ;;
    *)
      url="$1"
      shift
      ;;
  esac
done

platform="\${url##*/}"
case "$platform" in
  macos-arm64)
    expected_file="FutrixData-v1.2.3-macos-arm64.dmg"
    ;;
  linux-amd64)
    expected_file="FutrixData-v1.2.3-linux-amd64.tar.gz"
    ;;
  *)
    echo "unexpected platform: $platform" >&2
    exit 1
    ;;
esac

printf '%s\\n' "$url" >> "$log_path"

if [ "$output" = "/dev/null" ]; then
  printf 'https://github.com/FutrixDev/FutrixData/releases/download/v1.2.3/%s?token=signed-link' "$expected_file"
  exit 0
fi

mkdir -p "$(dirname "$output")"
if [ "$platform" = "macos-arm64" ]; then
  printf 'dmg\\n' > "$output"
else
  temp_bundle_root="$(mktemp -d /tmp/futrix-verify-linux.XXXXXX)"
  bundle_dir="$temp_bundle_root/FutrixData-v1.2.3-linux-amd64"
  mkdir -p "$bundle_dir"
  printf 'linux\\n' > "$bundle_dir/FutrixData"
  tar -czf "$output" -C "$temp_bundle_root" "FutrixData-v1.2.3-linux-amd64"
  rm -rf "$temp_bundle_root"
fi
`);

  await writeExecutable(path.join(binDir, 'hdiutil'), `#!/bin/bash
set -euo pipefail
if [ "$1" = "attach" ]; then
  shift
  mountpoint=""
  while [ "$#" -gt 0 ]; do
    case "$1" in
      -mountpoint)
        mountpoint="$2"
        shift 2
        ;;
      -*)
        shift
        ;;
      *)
        shift
        ;;
    esac
  done
  mkdir -p "$mountpoint/FutrixData.app/Contents"
  cat > "$mountpoint/FutrixData.app/Contents/Info.plist" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleShortVersionString</key>
  <string>1.2.3</string>
</dict>
</plist>
EOF
  exit 0
fi
if [ "$1" = "detach" ]; then
  exit 0
fi
exit 0
`);
  await writeExecutable(path.join(binDir, 'PlistBuddy'), `#!/bin/sh
set -eu
if [ "$1" = "-c" ]; then
  shift 2
fi
printf '1.2.3\\n'
`);

  const { stdout } = await execFile('bash', [
    path.join(scriptsDir, 'verify-live-download.sh'),
    '--project-dir',
    projectDir,
    '--site-url',
    'https://futrixdata.com',
    '--platform',
    'macos-arm64',
    '--platform',
    'linux-amd64',
    '--download-dir',
    downloadDir,
    '--keep-downloads',
  ], {
    cwd: tempRoot,
    env: {
      ...process.env,
      PATH: `${binDir}:${process.env.PATH}`,
      PLIST_BUDDY_BIN: path.join(binDir, 'PlistBuddy'),
    },
  });

  assert.match(stdout, /DMG app version matches: 1\.2\.3/);
  assert.match(stdout, /Linux archive root matches: FutrixData-v1\.2\.3-linux-amd64/);

  const curlCalls = await readFile(curlLog, 'utf8');
  assert.match(curlCalls, /https:\/\/futrixdata\.com\/api\/download\/macos-arm64/);
  assert.match(curlCalls, /https:\/\/futrixdata\.com\/api\/download\/linux-amd64/);
});

test('verify-live-download script preserves caller download directories by default', async () => {
  const tempRoot = await mkdtemp(path.join(tmpdir(), 'futrixdata-verify-live-download-dir-'));
  const scriptsDir = path.join(tempRoot, 'skill-scripts');
  const projectDir = path.join(tempRoot, 'project');
  const binDir = path.join(tempRoot, 'bin');
  const downloadDir = path.join(tempRoot, 'downloads');
  const sentinelPath = path.join(downloadDir, 'keep-me.txt');

  await mkdir(scriptsDir, { recursive: true });
  await mkdir(projectDir, { recursive: true });
  await mkdir(binDir, { recursive: true });
  await mkdir(downloadDir, { recursive: true });

  await copyReleaseScript('verify-live-download.sh', scriptsDir);
  await copyReleaseScript('release-common.sh', scriptsDir);

  await writeFile(path.join(projectDir, 'wails.json'), JSON.stringify({
    info: {
      productVersion: '1.2.3',
    },
  }, null, 2));
  await writeFile(sentinelPath, 'keep');

  await writeExecutable(path.join(binDir, 'curl'), `#!/bin/bash
set -euo pipefail
output=""
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o)
      output="$2"
      shift 2
      ;;
    -*)
      shift
      ;;
    *)
      url="$1"
      shift
      ;;
  esac
done

if [ "$output" = "/dev/null" ]; then
  printf 'https://github.com/FutrixDev/FutrixData/releases/download/v1.2.3/FutrixData-v1.2.3-linux-amd64.tar.gz?token=signed-link'
  exit 0
fi

temp_bundle_root="$(mktemp -d /tmp/futrix-verify-linux.XXXXXX)"
bundle_dir="$temp_bundle_root/FutrixData-v1.2.3-linux-amd64"
mkdir -p "$bundle_dir"
printf 'linux\\n' > "$bundle_dir/FutrixData"
tar -czf "$output" -C "$temp_bundle_root" "FutrixData-v1.2.3-linux-amd64"
rm -rf "$temp_bundle_root"
`);

  await execFile('bash', [
    path.join(scriptsDir, 'verify-live-download.sh'),
    '--project-dir',
    projectDir,
    '--site-url',
    'https://futrixdata.com',
    '--platform',
    'linux-amd64',
    '--download-dir',
    downloadDir,
  ], {
    cwd: tempRoot,
    env: {
      ...process.env,
      PATH: `${binDir}:${process.env.PATH}`,
    },
  });

  const sentinel = await readFile(sentinelPath, 'utf8');
  const archive = await readFile(path.join(downloadDir, 'FutrixData-v1.2.3-linux-amd64.tar.gz'));
  assert.equal(sentinel, 'keep');
  assert.ok(archive.length > 0);
});
