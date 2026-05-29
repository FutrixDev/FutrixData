import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, readFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { execFile as execFileCallback } from 'node:child_process';
import { promisify } from 'node:util';

const execFile = promisify(execFileCallback);

test('render-homebrew-formula script writes a formula file', async () => {
  const outputDir = await mkdtemp(path.join(tmpdir(), 'futrixdata-cli-formula-'));
  const outputPath = path.join(outputDir, 'futrixdata-cli.rb');
  await execFile('node', [
    'packaging/cli/render-homebrew-formula.mjs',
    '--version',
    '1.2.3',
    '--darwin-arm64-sha256',
    'armsha',
    '--darwin-amd64-sha256',
    'amdsha',
    '--output',
    outputPath,
  ], { cwd: process.cwd() });

  const formula = await readFile(outputPath, 'utf8');
  assert.match(formula, /version "1\.2\.3"/);
  assert.match(formula, /sha256 "armsha"/);
  assert.match(formula, /sha256 "amdsha"/);
});
