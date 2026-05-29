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

async function copyScript(sourcePath, destPath) {
  const content = await readFile(sourcePath, 'utf8');
  await writeFile(destPath, content, { mode: 0o755 });
}

async function writePackageStub(destPath, artifactName) {
  const script = `#!/bin/sh
set -eu
output_dir=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --output-dir)
      output_dir="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
if [ -z "$output_dir" ]; then
  echo "missing output dir" >&2
  exit 1
fi
mkdir -p "$output_dir"
printf '%s\\n' "${artifactName}" > "$output_dir/${artifactName}"
`;
  await writeFile(destPath, script, { mode: 0o755 });
}

test('package-all script rewrites checksums without hashing SHA256SUMS itself', async () => {
  const tempRoot = await mkdtemp(path.join(tmpdir(), 'futrixdata-package-all-'));
  const scriptsDir = path.join(tempRoot, 'scripts');
  const outputDir = path.join(tempRoot, 'out');
  await mkdir(scriptsDir, { recursive: true });
  await mkdir(outputDir, { recursive: true });

  await copyScript(path.join(releaseScriptsRoot, 'package-all.sh'), path.join(scriptsDir, 'package-all.sh'));
  await writePackageStub(path.join(scriptsDir, 'package-macos.sh'), 'macos-artifact.txt');
  await writePackageStub(path.join(scriptsDir, 'package-windows.sh'), 'windows-artifact.txt');
  await writePackageStub(path.join(scriptsDir, 'package-linux.sh'), 'linux-artifact.txt');
  await writeFile(path.join(outputDir, 'SHA256SUMS.txt'), 'stale\n');

  for (let i = 0; i < 2; i += 1) {
    await execFile('bash', [
      path.join(scriptsDir, 'package-all.sh'),
      '--project-dir',
      tempRoot,
      '--only',
      'macos',
      '--output-dir',
      outputDir,
    ], {
      cwd: tempRoot,
      env: process.env,
    });
  }

  const checksums = await readFile(path.join(outputDir, 'SHA256SUMS.txt'), 'utf8');
  assert.match(checksums, /macos-artifact\.txt$/m);
  assert.doesNotMatch(checksums, /SHA256SUMS\.txt/);
  assert.equal(checksums.trim().split('\n').length, 1);
});

test('package-all script preserves versioned artifact filenames in checksums', async () => {
  const tempRoot = await mkdtemp(path.join(tmpdir(), 'futrixdata-package-all-versioned-'));
  const scriptsDir = path.join(tempRoot, 'scripts');
  const outputDir = path.join(tempRoot, 'out');
  await mkdir(scriptsDir, { recursive: true });
  await mkdir(outputDir, { recursive: true });

  await copyScript(path.join(releaseScriptsRoot, 'package-all.sh'), path.join(scriptsDir, 'package-all.sh'));
  await writePackageStub(path.join(scriptsDir, 'package-macos.sh'), 'FutrixData-v9.9.9-macos-arm64.dmg');
  await writePackageStub(path.join(scriptsDir, 'package-windows.sh'), 'FutrixData-v9.9.9-windows-amd64-installer.exe');
  await writePackageStub(path.join(scriptsDir, 'package-linux.sh'), 'FutrixData-v9.9.9-linux-amd64.tar.gz');

  await execFile('bash', [
    path.join(scriptsDir, 'package-all.sh'),
    '--project-dir',
    tempRoot,
    '--output-dir',
    outputDir,
  ], {
    cwd: tempRoot,
    env: process.env,
  });

  const checksums = await readFile(path.join(outputDir, 'SHA256SUMS.txt'), 'utf8');
  assert.match(checksums, /FutrixData-v9\.9\.9-macos-arm64\.dmg$/m);
  assert.match(checksums, /FutrixData-v9\.9\.9-windows-amd64-installer\.exe$/m);
  assert.match(checksums, /FutrixData-v9\.9\.9-linux-amd64\.tar\.gz$/m);
});
