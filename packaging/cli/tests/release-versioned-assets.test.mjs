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

async function writeProjectVersion(projectDir, version) {
  await writeFile(path.join(projectDir, 'wails.json'), JSON.stringify({
    info: {
      productVersion: version,
    },
  }, null, 2));
}

test('package-macos writes versioned dmg filename to output', async () => {
  const tempRoot = await mkdtemp(path.join(tmpdir(), 'futrixdata-package-macos-versioned-'));
  const scriptsDir = path.join(tempRoot, 'skill-scripts');
  const projectDir = path.join(tempRoot, 'project');
  const outputDir = path.join(tempRoot, 'out');
  const binDir = path.join(tempRoot, 'bin');
  const projectScriptsDir = path.join(projectDir, 'scripts');

  await mkdir(scriptsDir, { recursive: true });
  await mkdir(projectDir, { recursive: true });
  await mkdir(outputDir, { recursive: true });
  await mkdir(binDir, { recursive: true });
  await mkdir(projectScriptsDir, { recursive: true });
  await mkdir(path.join(projectDir, 'build', 'bin'), { recursive: true });

  await copyReleaseScript('package-macos.sh', scriptsDir);
  await copyReleaseScript('release-common.sh', scriptsDir);
  await writeExecutable(path.join(projectScriptsDir, 'build.sh'), `#!/bin/sh
set -eu
project_dir="$(cd "$(dirname "$0")/.." && pwd)"
mkdir -p "$project_dir/build/bin"
`);
  await writeExecutable(path.join(scriptsDir, 'release-macos.sh'), `#!/bin/sh
set -eu
project_dir=""
platform=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --project-dir)
      project_dir="$2"
      shift 2
      ;;
    --platform)
      platform="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
mkdir -p "$project_dir/build/bin"
arch="\${platform#*/}"
printf 'dmg\\n' > "$project_dir/build/bin/FutrixData-v1.2.3-macos-\${arch}.dmg"
`);
  await writeProjectVersion(projectDir, '1.2.3');
  await writeExecutable(path.join(binDir, 'wails'), '#!/bin/sh\nexit 0\n');
  await writeExecutable(path.join(binDir, 'create-dmg'), '#!/bin/sh\nexit 0\n');

  await execFile('bash', [
    path.join(scriptsDir, 'package-macos.sh'),
    '--project-dir',
    projectDir,
    '--signed',
    '--platform',
    'darwin/arm64',
    '--output-dir',
    outputDir,
  ], {
    cwd: tempRoot,
    env: {
      ...process.env,
      PATH: `${binDir}:${process.env.PATH}`,
    },
  });

  const copied = await readFile(path.join(outputDir, 'FutrixData-v1.2.3-macos-arm64.dmg'), 'utf8');
  assert.equal(copied.trim(), 'dmg');
});

test('package-windows writes versioned installer filename to output', async () => {
  const tempRoot = await mkdtemp(path.join(tmpdir(), 'futrixdata-package-windows-versioned-'));
  const scriptsDir = path.join(tempRoot, 'skill-scripts');
  const projectDir = path.join(tempRoot, 'project');
  const outputDir = path.join(tempRoot, 'out');
  const binDir = path.join(tempRoot, 'bin');
  const projectScriptsDir = path.join(projectDir, 'scripts');

  await mkdir(scriptsDir, { recursive: true });
  await mkdir(projectDir, { recursive: true });
  await mkdir(outputDir, { recursive: true });
  await mkdir(binDir, { recursive: true });
  await mkdir(projectScriptsDir, { recursive: true });
  await mkdir(path.join(projectDir, 'build', 'bin'), { recursive: true });

  await copyReleaseScript('package-windows.sh', scriptsDir);
  await copyReleaseScript('release-common.sh', scriptsDir);
  await writeProjectVersion(projectDir, '1.2.3');
  await writeExecutable(path.join(projectScriptsDir, 'build.sh'), `#!/bin/sh
set -eu
project_dir="$(cd "$(dirname "$0")/.." && pwd)"
mkdir -p "$project_dir/build/bin"
printf 'installer\\n' > "$project_dir/build/bin/FutrixData-amd64-installer.exe"
`);
  await writeExecutable(path.join(binDir, 'wails'), '#!/bin/sh\nexit 0\n');
  await writeExecutable(path.join(binDir, 'makensis'), '#!/bin/sh\nexit 0\n');

  await execFile('bash', [
    path.join(scriptsDir, 'package-windows.sh'),
    '--project-dir',
    projectDir,
    '--output-dir',
    outputDir,
  ], {
    cwd: tempRoot,
    env: {
      ...process.env,
      PATH: `${binDir}:${process.env.PATH}`,
    },
  });

  const copied = await readFile(path.join(outputDir, 'FutrixData-v1.2.3-windows-amd64-installer.exe'), 'utf8');
  assert.equal(copied.trim(), 'installer');
});
