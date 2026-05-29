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

async function writeExecutable(destPath, content) {
  await writeFile(destPath, content, { mode: 0o755 });
}

test('release-macos script allows notarization without issuer for individual keys', async () => {
  const tempRoot = await mkdtemp(path.join(tmpdir(), 'futrixdata-release-macos-'));
  const scriptsDir = path.join(tempRoot, 'scripts');
  const binDir = path.join(tempRoot, 'bin');
  const buildDir = path.join(tempRoot, 'build', 'bin');
  const logPath = path.join(tempRoot, 'xcrun.log');
  const keyPath = path.join(tempRoot, 'AuthKey_TEST.p8');

  await mkdir(scriptsDir, { recursive: true });
  await mkdir(binDir, { recursive: true });
  await mkdir(buildDir, { recursive: true });

  await copyScript(path.join(releaseScriptsRoot, 'release-macos.sh'), path.join(scriptsDir, 'release-macos.sh'));
  await copyScript(path.join(releaseScriptsRoot, 'release-common.sh'), path.join(scriptsDir, 'release-common.sh'));
  await writeExecutable(path.join(scriptsDir, 'build.sh'), `#!/bin/sh
set -eu
project_dir="$(cd "$(dirname "$0")/.." && pwd)"
app_dir="$project_dir/build/bin/FutrixData.app/Contents/MacOS"
mkdir -p "$app_dir"
printf 'app\\n' > "$app_dir/FutrixData"
chmod +x "$app_dir/FutrixData"
`);
  await writeExecutable(path.join(scriptsDir, 'build-dmg.sh'), `#!/bin/sh
set -eu
project_dir="$(cd "$(dirname "$0")/.." && pwd)"
mkdir -p "$project_dir/build/bin"
printf 'dmg\\n' > "$project_dir/build/bin/FutrixData.dmg"
`);

  await writeExecutable(path.join(binDir, 'codesign'), '#!/bin/sh\nexit 0\n');
  await writeExecutable(path.join(binDir, 'security'), `#!/bin/sh
printf '  1) ABCDEF1234567890 "Developer ID Application: FutrixData, Inc. (TEAMID)"\\n'
`);
  await writeExecutable(path.join(binDir, 'xcrun'), `#!/bin/sh
set -eu
printf '%s\\n' "$*" >> "${logPath}"
exit 0
`);
  await writeExecutable(path.join(binDir, 'hdiutil'), `#!/bin/sh
set -eu
if [ "$1" = "attach" ]; then
  shift
  while [ "$#" -gt 0 ]; do
    case "$1" in
      -mountpoint)
        mountpoint="$2"
        shift 2
        ;;
      *)
        shift
        ;;
    esac
  done
  mkdir -p "$mountpoint/FutrixData.app"
fi
exit 0
`);
  await writeExecutable(path.join(binDir, 'spctl'), '#!/bin/sh\nexit 0\n');
  await writeExecutable(path.join(binDir, 'mount'), '#!/bin/sh\nexit 0\n');
  await writeFile(keyPath, 'test-key');
  await writeFile(path.join(tempRoot, 'wails.json'), JSON.stringify({
    info: {
      productVersion: '1.2.3',
    },
  }, null, 2));

  await execFile('bash', [
    path.join(scriptsDir, 'release-macos.sh'),
    '--project-dir',
    tempRoot,
    '--platform',
    'darwin/arm64',
  ], {
    cwd: tempRoot,
    env: {
      ...process.env,
      HOME: tempRoot,
      PATH: `${binDir}:${process.env.PATH}`,
      NOTARY_KEY: keyPath,
      NOTARY_KEY_ID: 'KEY123456',
      NOTARY_ISSUER_ID: '',
      MACOS_SIGN_IDENTITY: 'Developer ID Application: FutrixData, Inc. (TEAMID)',
    },
  });

  const log = await readFile(logPath, 'utf8');
  const submitLine = log
    .trim()
    .split('\n')
    .find((line) => line.startsWith('notarytool submit'));

  assert.ok(submitLine, `expected notarytool submit invocation in log, got: ${log}`);
  assert.match(submitLine, /--key-id KEY123456/);
  assert.match(submitLine, /--key /);
  assert.doesNotMatch(submitLine, /--issuer/);
  assert.doesNotMatch(log, /store-credentials/);
});
