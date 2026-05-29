import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, readdir, readFile } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { execFile as execFileCallback } from 'node:child_process';
import { promisify } from 'node:util';

const execFile = promisify(execFileCallback);

test('release script builds assets, checksums, homebrew formula, and npm tarball', async () => {
  const outputDir = await mkdtemp(path.join(tmpdir(), 'futrixdata-cli-release-'));

  await execFile('sh', [
    'scripts/release-futrixdata-cli.sh',
    '--version',
    'v9.9.9',
    '--targets',
    'darwin/amd64,darwin/arm64',
    '--output',
    outputDir,
  ], {
    cwd: process.cwd(),
    env: {
      ...process.env,
      CGO_ENABLED: '0',
    },
  });

  assert.ok(existsSync(path.join(outputDir, 'futrixdata-cli_darwin_amd64.tar.gz')));
  assert.ok(existsSync(path.join(outputDir, 'futrixdata-cli_darwin_arm64.tar.gz')));
  assert.ok(existsSync(path.join(outputDir, 'futrixdata-cli_checksums.txt')));
  assert.ok(existsSync(path.join(outputDir, 'homebrew', 'futrixdata-cli.rb')));

  const npmDir = path.join(outputDir, 'npm');
  const npmFiles = await readdir(npmDir);
  assert.ok(npmFiles.some((name) => name.startsWith('futrixdata-cli-') && name.endsWith('.tgz')));

  const formula = await readFile(path.join(outputDir, 'homebrew', 'futrixdata-cli.rb'), 'utf8');
  assert.match(formula, /version "9\.9\.9"/);
});
