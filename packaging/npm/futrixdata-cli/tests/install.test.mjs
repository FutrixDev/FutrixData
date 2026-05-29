import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, chmod, writeFile, readFile } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { execFile as execFileCallback } from 'node:child_process';
import { promisify } from 'node:util';
import http from 'node:http';

import { installCLI } from '../lib/install.mjs';

const execFile = promisify(execFileCallback);

async function createFakeTarball() {
  const root = await mkdtemp(path.join(tmpdir(), 'futrixdata-cli-npm-fixture-'));
  const payloadDir = path.join(root, 'payload');
  const archivePath = path.join(root, 'futrixdata-cli_darwin_arm64.tar.gz');
  await execFile('mkdir', ['-p', payloadDir]);
  const binaryPath = path.join(payloadDir, 'futrixdata-cli');
  await writeFile(binaryPath, '#!/bin/sh\necho npm-install-ok\n', 'utf8');
  await chmod(binaryPath, 0o755);
  await execFile('tar', ['-czf', archivePath, '-C', payloadDir, 'futrixdata-cli']);
  return { archivePath };
}

async function serveFile(filePath) {
  const payload = await readFile(filePath);
  const server = http.createServer((req, res) => {
    if (req.url === '/releases/download/v9.9.9/futrixdata-cli_darwin_arm64.tar.gz') {
      res.writeHead(200, { 'Content-Type': 'application/gzip' });
      res.end(payload);
      return;
    }
    res.writeHead(404);
    res.end('not found');
  });
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const address = server.address();
  return {
    baseURL: `http://127.0.0.1:${address.port}/releases`,
    close: async () => new Promise((resolve, reject) => server.close((err) => (err ? reject(err) : resolve()))),
  };
}

test('installCLI downloads a release archive and installs futrixdata-cli', async () => {
  const { archivePath } = await createFakeTarball();
  const server = await serveFile(archivePath);
  const targetDir = await mkdtemp(path.join(tmpdir(), 'futrixdata-cli-npm-target-'));

  try {
    const binaryPath = await installCLI({
      version: 'v9.9.9',
      platform: 'darwin',
      arch: 'arm64',
      releaseBaseURL: server.baseURL,
      targetDir,
    });

    assert.ok(existsSync(binaryPath));
    const result = await execFile(binaryPath, []);
    assert.equal(result.stdout.trim(), 'npm-install-ok');
  } finally {
    await server.close();
  }
});
