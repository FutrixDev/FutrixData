import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, writeFile, chmod } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { execFile as execFileCallback } from 'node:child_process';
import { promisify } from 'node:util';

const execFile = promisify(execFileCallback);

test('npm launcher forwards argv to installed futrixdata-cli binary', async () => {
  const packageRoot = await mkdtemp(path.join(tmpdir(), 'futrixdata-cli-launcher-'));
  const vendorDir = path.join(packageRoot, 'vendor');
  await mkdir(vendorDir, { recursive: true });
  const binaryPath = path.join(vendorDir, 'futrixdata-cli');
  await writeFile(binaryPath, '#!/bin/sh\necho launcher:$1\n', 'utf8');
  await chmod(binaryPath, 0o755);

  const result = await execFile('node', [
    path.join(process.cwd(), 'packaging/npm/futrixdata-cli/bin/futrixdata-cli.mjs'),
    'version',
  ], {
    env: {
      ...process.env,
      FUTRIXDATA_CLI_NPM_PACKAGE_ROOT: packageRoot,
    },
  });

  assert.equal(result.stdout.trim(), 'launcher:version');
});
